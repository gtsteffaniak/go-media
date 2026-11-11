package av1decode

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"sync"
	"time"

	"github.com/gtsteffaniak/go-media/internal/h264"
)

// maxPoolKeys bounds how many distinct av1C configurations keep decoders alive.
// Least-recently-used keys beyond this are evicted; eviction only ever closes
// idle decoders, a decoder checked out by a goroutine is closed by that
// goroutine when it returns it.
const maxPoolKeys = 8

// maxIdlePixels bounds the total coded frame area (max_frame_width ×
// max_frame_height from the sequence header) of idle pooled decoders across
// every configuration. A KeyframeDecoder's arenas scale with that area: about
// 265 MB for 3840×2160 and 1 GB for 7680×4320. Without a byte budget the pool
// retained up to maxPoolKeys × GOMAXPROCS decoders for the life of the
// process. The budget keeps one DCI 4K decoder (the largest MaxFrameLongSide ×
// MaxFrameShortSide allows) or up to four 1080p decoders warm.
var maxIdlePixels = int64(MaxFrameLongSide) * int64(MaxFrameShortSide)

// idleTTL closes pooled decoders that have not been used for this long, so a
// burst of AV1 thumbnails does not pin decoder memory forever.
var idleTTL = 30 * time.Second

// decoderPool is a bounded checkout/return pool of KeyframeDecoders per av1C
// configuration. Every decoder is owned by exactly one goroutine between
// checkout and checkin, so a decoder is never decoded on and closed
// concurrently, and replacing an undersized decoder only touches the
// caller's own instance.
type decoderPool struct {
	mu         sync.Mutex
	keys       map[string]*list.Element // value: *poolEntry
	lru        *list.List               // front = most recently used
	idlePixels int64                    // sum of Decoder.pixels over every idle decoder
	sweeping   bool                     // an idle sweep timer is pending
}

type poolEntry struct {
	key     string
	idle    []*Decoder // oldest first
	maxUnit []int      // parallel to idle: probe size each idle decoder was sized for
	since   []time.Time
	inUse   int
	dead    bool // evicted: returned decoders are closed instead of pooled
	cond    *sync.Cond
}

var av1Pool = &decoderPool{keys: map[string]*list.Element{}, lru: list.New()}

// perKeyLimit is the most decoders one configuration may hold (idle + in use):
// one per P, which is the most that can make progress at once.
func perKeyLimit() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		n = 1
	}
	return n
}

func configCacheKey(av1CExtradata []byte) string {
	if len(av1CExtradata) == 0 {
		return "empty"
	}
	sum := sha256.Sum256(av1CExtradata)
	return hex.EncodeToString(sum[:8])
}

// checkout returns an idle decoder for key (or nil with a reserved slot when the
// caller must create one). It blocks while the key already has perKeyLimit
// decoders checked out.
func (p *decoderPool) checkout(key string) (*poolEntry, *Decoder, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entryLocked(key)
	for {
		if n := len(e.idle); n > 0 {
			d, m := e.idle[n-1], e.maxUnit[n-1]
			e.idle, e.maxUnit, e.since = e.idle[:n-1], e.maxUnit[:n-1], e.since[:n-1]
			p.idlePixels -= d.pixels
			e.inUse++
			return e, d, m
		}
		if e.inUse < perKeyLimit() {
			e.inUse++
			return e, nil, 0
		}
		e.cond.Wait()
		if e.dead {
			// Evicted while waiting: move to the live entry for this key.
			e = p.entryLocked(key)
		}
	}
}

func (p *decoderPool) entryLocked(key string) *poolEntry {
	if el, ok := p.keys[key]; ok {
		p.lru.MoveToFront(el)
		return el.Value.(*poolEntry)
	}
	e := &poolEntry{key: key, cond: sync.NewCond(&p.mu)}
	p.keys[key] = p.lru.PushFront(e)
	for p.lru.Len() > maxPoolKeys {
		p.evictLocked(p.lru.Back())
	}
	return e
}

// evictLocked drops a key and closes only its idle decoders. Checked-out
// decoders are closed by their holder on checkin because e.dead is set.
func (p *decoderPool) evictLocked(el *list.Element) {
	e := el.Value.(*poolEntry)
	p.lru.Remove(el)
	delete(p.keys, e.key)
	e.dead = true
	for _, d := range e.idle {
		p.idlePixels -= d.pixels
		d.Close()
	}
	e.idle, e.maxUnit, e.since = nil, nil, nil
	e.cond.Broadcast()
}

// dropOldestIdleLocked closes the oldest idle decoder of the least recently
// used configuration that has one. It reports false when nothing is idle.
func (p *decoderPool) dropOldestIdleLocked() bool {
	for el := p.lru.Back(); el != nil; el = el.Prev() {
		e := el.Value.(*poolEntry)
		if len(e.idle) == 0 {
			continue
		}
		d := e.idle[0]
		e.idle, e.maxUnit, e.since = e.idle[1:], e.maxUnit[1:], e.since[1:]
		p.idlePixels -= d.pixels
		d.Close()
		return true
	}
	return false
}

// keepIdleLocked reports whether d may be pooled, making room under
// maxIdlePixels by closing older idle decoders first.
func (p *decoderPool) keepIdleLocked(d *Decoder) bool {
	if d.pixels <= 0 || d.pixels > maxIdlePixels {
		return false
	}
	for p.idlePixels+d.pixels > maxIdlePixels {
		if !p.dropOldestIdleLocked() {
			return false
		}
	}
	return true
}

// scheduleSweepLocked arms one timer that closes decoders idle for idleTTL.
func (p *decoderPool) scheduleSweepLocked() {
	if p.sweeping || idleTTL <= 0 {
		return
	}
	p.sweeping = true
	time.AfterFunc(idleTTL, p.sweep)
}

func (p *decoderPool) sweep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweeping = false
	cutoff := time.Now().Add(-idleTTL)
	remaining := false
	for el := p.lru.Front(); el != nil; el = el.Next() {
		e := el.Value.(*poolEntry)
		n := 0
		for n < len(e.idle) && !e.since[n].After(cutoff) {
			p.idlePixels -= e.idle[n].pixels
			e.idle[n].Close()
			n++
		}
		e.idle, e.maxUnit, e.since = e.idle[n:], e.maxUnit[n:], e.since[n:]
		if len(e.idle) > 0 {
			remaining = true
		}
	}
	if remaining {
		p.scheduleSweepLocked()
	}
}

// checkin returns a held decoder (nil when creation failed) and frees its slot.
func (p *decoderPool) checkin(e *poolEntry, d *Decoder, maxUnit int) {
	p.mu.Lock()
	e.inUse--
	if d != nil {
		if e.dead || !p.keepIdleLocked(d) {
			d.Close()
		} else {
			e.idle = append(e.idle, d)
			e.maxUnit = append(e.maxUnit, maxUnit)
			e.since = append(e.since, time.Now())
			p.idlePixels += d.pixels
			p.scheduleSweepLocked()
		}
	}
	e.cond.Signal()
	p.mu.Unlock()
}

// DecodeSampleCached decodes one keyframe with a pooled KeyframeDecoder for the
// av1C configuration. Decoders are checked out exclusively (at most GOMAXPROCS
// per configuration, at most maxPoolKeys configurations), so concurrent
// thumbnails never share or close a decoder another goroutine is using. When a
// sample's temporal unit is larger than the decoder was sized for, the caller
// replaces its own decoder with one sized for that unit.
func DecodeSampleCached(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	e, dec, maxUnit := av1Pool.checkout(configCacheKey(av1CExtradata))
	ycc, err := decodeHeld(&dec, &maxUnit, av1CExtradata, sample)
	av1Pool.checkin(e, dec, maxUnit)
	return ycc, err
}

func decodeHeld(dec **Decoder, maxUnit *int, av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	if *dec != nil {
		ycc, err := (*dec).DecodeSample(av1CExtradata, sample)
		if err == nil {
			return ycc, nil
		}
	}
	unit, err := ProbeTemporalUnit(av1CExtradata, sample)
	if err != nil {
		return nil, err
	}
	if *dec != nil && len(unit) <= *maxUnit {
		// Decoder is already large enough; the failure was the sample itself.
		return (*dec).DecodeSample(av1CExtradata, sample)
	}
	nd, err := NewDecoderFromProbe(unit)
	if err != nil {
		return nil, err
	}
	if *dec != nil {
		(*dec).Close() // exclusively held: no other goroutine can be using it
	}
	*dec, *maxUnit = nd, len(unit)
	return nd.DecodeSample(av1CExtradata, sample)
}
