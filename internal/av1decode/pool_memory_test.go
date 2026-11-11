package av1decode

import (
	"container/list"
	"errors"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-av1/pkg/av1"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// TestCheckFrameSize checks the DCI 4K limit in both orientations.
func TestCheckFrameSize(t *testing.T) {
	for _, c := range []struct {
		w, h int
		ok   bool
	}{
		{1920, 1080, true},
		{3840, 2160, true},
		{2160, 3840, true},
		{4096, 2160, true},
		{2160, 4096, true},
		{4097, 2160, false},
		{4096, 2161, false},
		{2161, 4096, false},
		{2160, 4097, false},
		{7680, 4320, false},
		{16384, 8192, false},
	} {
		err := checkFrameSize(c.w, c.h)
		var pe *errs.ProfileError
		if c.ok != (err == nil) || (err != nil && !errors.As(err, &pe)) {
			t.Errorf("%dx%d: err=%v, want ok=%v", c.w, c.h, err, c.ok)
		}
	}
}

// seqHeaderOBU returns the first sequence header OBU (raw bytes) in src.
func seqHeaderOBU(t *testing.T, src []byte) []byte {
	t.Helper()
	it := av1.NewLowOverheadIterator(src)
	for {
		u, more, err := it.Next()
		if err != nil || !more {
			t.Fatal("no sequence header OBU")
		}
		if u.Header.Type == av1.OBUSequenceHeader {
			return append([]byte(nil), u.Raw...)
		}
	}
}

// tallerSeqHeaderOBU returns a copy of the sequence header OBU seq with one
// payload bit flipped so it parses with the same max_frame_width and a larger
// max_frame_height (the fixtures are all 128x72; a 7-bit height field can
// reach 128). It fails the test if no such bit exists.
func tallerSeqHeaderOBU(t *testing.T, seq []byte) []byte {
	t.Helper()
	it := av1.NewLowOverheadIterator(seq)
	u, _, err := it.Next()
	if err != nil {
		t.Fatal(err)
	}
	orig, err := av1.ParseSequenceHeader(u.Payload)
	if err != nil {
		t.Fatal(err)
	}
	off := int(u.HeaderLen) + int(u.SizeLen)
	for bit := 0; bit < len(u.Payload)*8; bit++ {
		c := append([]byte(nil), seq...)
		c[off+bit/8] ^= 0x80 >> (bit % 8)
		sh, err := av1.ParseSequenceHeader(c[off:])
		if err == nil && sh.MaxFrameWidth == orig.MaxFrameWidth && sh.MaxFrameHeight > orig.MaxFrameHeight {
			return c
		}
	}
	t.Fatal("no bit flip enlarges max_frame_height")
	return nil
}

// sampleFrameSize returns the sequence frame size of a test sample.
func sampleFrameSize(t *testing.T, src []byte) (w, h int) {
	t.Helper()
	w, h, ok, err := sequenceFrameSize(src)
	if err != nil || !ok || w <= 0 || h <= 0 {
		t.Fatalf("sequence header: %dx%d ok=%v err=%v", w, h, ok, err)
	}
	return w, h
}

// setFrameLimit sets the frame-size limit for one test.
func setFrameLimit(t *testing.T, long, short int) {
	oldL, oldS := MaxFrameLongSide, MaxFrameShortSide
	MaxFrameLongSide, MaxFrameShortSide = long, short
	t.Cleanup(func() { MaxFrameLongSide, MaxFrameShortSide = oldL, oldS })
}

// TestSequenceFrameSizeCap checks that a sequence header above the limit is
// rejected before go-av1 sizes its arenas.
func TestSequenceFrameSizeCap(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	w, h := sampleFrameSize(t, sample)
	setFrameLimit(t, max(w, h)-1, min(w, h))
	_, err := NewDecoderFromProbe(sample)
	var pe *errs.ProfileError
	if !errors.As(err, &pe) {
		t.Fatalf("want profile error for oversized frame, got %v", err)
	}
}

// TestSequenceFrameSizeEveryHeader checks that a small sequence header placed
// ahead of a larger one does not bypass the limit, either when a decoder is
// built or when a pooled decoder decodes a later sample, and that the largest
// in-limit header sets the decoder's pool area.
func TestSequenceFrameSizeEveryHeader(t *testing.T) {
	small := readAV1(t, "tu_small.obu")
	sw, sh := sampleFrameSize(t, small)
	smallSeq := seqHeaderOBU(t, small)
	bigSeq := tallerSeqHeaderOBU(t, smallSeq)
	bw, bh := sampleFrameSize(t, bigSeq)
	// Small header first, larger header second, then the original unit.
	mixed := append(append(append([]byte(nil), smallSeq...), bigSeq...), small...)

	// Both in range: the larger header sets the size.
	w, h, ok, err := sequenceFrameSize(smallSeq, bigSeq)
	if err != nil || !ok || w != bw || h != bh {
		t.Fatalf("got %dx%d ok=%v err=%v, want %dx%d", w, h, ok, err, bw, bh)
	}
	d, err := NewDecoderFromProbe(small)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if want := int64(sw) * int64(sh); d.pixels != want {
		t.Fatalf("pixels %d, want %d", d.pixels, want)
	}
	_, _ = d.DecodeSample(nil, mixed) // the frame may not match; only accounting matters here
	if want := int64(bw) * int64(bh); d.pixels != want {
		t.Fatalf("pixels after larger header %d, want %d", d.pixels, want)
	}

	// Limit at the small size: the second header must be rejected.
	setFrameLimit(t, max(sw, sh), min(sw, sh))
	var pe *errs.ProfileError
	if _, _, _, err := sequenceFrameSize(smallSeq, bigSeq); !errors.As(err, &pe) {
		t.Fatalf("sequenceFrameSize: want profile error, got %v", err)
	}
	if _, err := NewDecoderFromProbe(mixed); !errors.As(err, &pe) {
		t.Fatalf("NewDecoderFromProbe: want profile error, got %v", err)
	}
	d2, err := NewDecoderFromProbe(small)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if _, err := d2.DecodeSample(nil, mixed); !errors.As(err, &pe) {
		t.Fatalf("DecodeSample: want profile error, got %v", err)
	}
	if _, err := d2.DecodeSample(nil, small); err != nil {
		t.Fatalf("in-limit sample after rejection: %v", err)
	}
}

// setPoolKnobs sets the idle budget and TTL under the pool lock, since a
// pending sweep timer reads them.
func setPoolKnobs(t *testing.T, budget int64, ttl time.Duration) {
	av1Pool.mu.Lock()
	maxIdlePixels, idleTTL = budget, ttl
	av1Pool.mu.Unlock()
}

// openCount counts decoders not yet closed, under the pool lock so the read
// cannot overlap a sweep's Close.
func openCount(ds []*Decoder) int {
	av1Pool.mu.Lock()
	defer av1Pool.mu.Unlock()
	n := 0
	for _, d := range ds {
		if d.kd != nil {
			n++
		}
	}
	return n
}

// TestPoolIdleBudgetAndTTL checks that idle decoders stay within
// maxIdlePixels and are closed after idleTTL, so decoder memory is not pinned
// for the life of the process. The budget phase runs with expiry disabled so
// a slow test cannot see a sweep; expiry is then checked on its own.
func TestPoolIdleBudgetAndTTL(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	resetPoolForTest()
	defer resetPoolForTest()
	w, h := sampleFrameSize(t, sample)
	px := int64(w) * int64(h)

	oldBudget, oldTTL := maxIdlePixels, idleTTL
	defer setPoolKnobs(t, oldBudget, oldTTL)
	setPoolKnobs(t, 2*px, 0)

	// Check out four decoders at once under distinct keys, then return them.
	type held struct {
		e   *poolEntry
		d   *Decoder
		max int
	}
	var hs []held
	var ds []*Decoder
	for i := 0; i < 4; i++ {
		e, d, m := av1Pool.checkout(string(rune('k' + i)))
		if _, err := decodeHeld(&d, &m, nil, sample); err != nil {
			t.Fatal(err)
		}
		hs = append(hs, held{e, d, m})
		ds = append(ds, d)
	}
	for _, h := range hs {
		av1Pool.checkin(h.e, h.d, h.max)
	}
	av1Pool.mu.Lock()
	idle := av1Pool.idlePixels
	av1Pool.mu.Unlock()
	if idle != maxIdlePixels {
		t.Fatalf("idle pixels %d, want budget %d", idle, maxIdlePixels)
	}
	if n := openCount(ds); n != 2 {
		t.Fatalf("%d decoders open, want 2 within budget", n)
	}

	// Expiry phase: arm the sweep with a short TTL and wait for it.
	setPoolKnobs(t, 2*px, 50*time.Millisecond)
	av1Pool.mu.Lock()
	av1Pool.scheduleSweepLocked()
	av1Pool.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for openCount(ds) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("idle decoders not swept after idle TTL")
		}
		time.Sleep(10 * time.Millisecond)
	}
	av1Pool.mu.Lock()
	idle = av1Pool.idlePixels
	av1Pool.mu.Unlock()
	if idle != 0 {
		t.Fatalf("idle pixels %d after sweep, want 0", idle)
	}
}

// TestSweepSchedulesEarliestExpiry checks that after a sweep the next timer
// targets the oldest remaining idle decoder's since+idleTTL rather than a
// full idleTTL later, so a reused decoder is not kept for up to 2×idleTTL.
func TestSweepSchedulesEarliestExpiry(t *testing.T) {
	ttl := time.Hour // long enough that the timer never fires during the test
	p := &decoderPool{keys: map[string]*list.Element{}, lru: list.New()}
	oldBudget, oldTTL := maxIdlePixels, idleTTL
	setPoolKnobs(t, oldBudget, ttl)
	defer setPoolKnobs(t, oldBudget, oldTTL)
	defer func() {
		p.mu.Lock()
		if p.sweepTimer != nil {
			p.sweepTimer.Stop()
		}
		p.mu.Unlock()
	}()

	now := time.Now()
	expired := &Decoder{pixels: 1}
	kept := &Decoder{pixels: 1}
	recent := now.Add(-ttl + time.Minute) // expires in one minute
	p.mu.Lock()
	a := p.entryLocked("a")
	a.idle, a.maxUnit, a.since = []*Decoder{expired}, []int{1}, []time.Time{now.Add(-2 * ttl)}
	b := p.entryLocked("b")
	b.idle, b.maxUnit, b.since = []*Decoder{kept}, []int{1}, []time.Time{recent}
	p.idlePixels = 2
	p.mu.Unlock()

	p.sweep()

	want := recent.Add(ttl)
	p.mu.Lock()
	if len(a.idle) != 0 || len(b.idle) != 1 || p.idlePixels != 1 {
		t.Errorf("after sweep: a=%d b=%d idlePixels=%d, want 0 1 1", len(a.idle), len(b.idle), p.idlePixels)
	}
	if !p.sweepAt.Equal(want) {
		t.Errorf("next sweep at %v, want %v (since+idleTTL)", p.sweepAt, want)
	}
	b.inUse++
	p.mu.Unlock()

	// A later checkin must not push the armed deadline back.
	p.checkin(b, &Decoder{pixels: 1}, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.sweepAt.Equal(want) {
		t.Errorf("checkin moved next sweep to %v, want %v", p.sweepAt, want)
	}
}
