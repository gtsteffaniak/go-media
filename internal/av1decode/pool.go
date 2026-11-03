package av1decode

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"sync"

	"github.com/gtsteffaniak/go-media/internal/h264"
)

// maxPoolKeys bounds how many distinct av1C configurations keep decoders alive.
// Least-recently-used keys beyond this are evicted; eviction only ever closes
// idle decoders, a decoder checked out by a goroutine is closed by that
// goroutine when it returns it.
const maxPoolKeys = 8

// decoderPool is a bounded checkout/return pool of KeyframeDecoders per av1C
// configuration. Every decoder is owned by exactly one goroutine between
// checkout and checkin, so a decoder is never decoded on and closed
// concurrently, and replacing an undersized decoder only touches the
// caller's own instance.
type decoderPool struct {
	mu   sync.Mutex
	keys map[string]*list.Element // value: *poolEntry
	lru  *list.List               // front = most recently used
}

type poolEntry struct {
	key     string
	idle    []*Decoder
	maxUnit []int // parallel to idle: probe size each idle decoder was sized for
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
			e.idle, e.maxUnit = e.idle[:n-1], e.maxUnit[:n-1]
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
		d.Close()
	}
	e.idle, e.maxUnit = nil, nil
	e.cond.Broadcast()
}

// checkin returns a held decoder (nil when creation failed) and frees its slot.
func (p *decoderPool) checkin(e *poolEntry, d *Decoder, maxUnit int) {
	p.mu.Lock()
	e.inUse--
	if d != nil {
		if e.dead {
			d.Close()
		} else {
			e.idle = append(e.idle, d)
			e.maxUnit = append(e.maxUnit, maxUnit)
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
