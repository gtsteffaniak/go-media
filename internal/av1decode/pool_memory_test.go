package av1decode

import (
	"errors"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// TestCheckFrameSize checks the 4K limit in both orientations.
func TestCheckFrameSize(t *testing.T) {
	for _, c := range []struct {
		w, h int
		ok   bool
	}{
		{1920, 1080, true},
		{3840, 2160, true},
		{2160, 3840, true},
		{3841, 2160, false},
		{3840, 2161, false},
		{2161, 3840, false},
		{4096, 2160, false},
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

// TestSequenceFrameSizeCap checks that a sequence header above the limit is
// rejected before go-av1 sizes its arenas.
func TestSequenceFrameSizeCap(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	w, h, ok := sequenceFrameSize(sample)
	if !ok || w <= 0 || h <= 0 {
		t.Fatalf("sequence header not found: %dx%d ok=%v", w, h, ok)
	}
	oldL, oldS := MaxFrameLongSide, MaxFrameShortSide
	MaxFrameLongSide, MaxFrameShortSide = max(w, h)-1, min(w, h)
	defer func() { MaxFrameLongSide, MaxFrameShortSide = oldL, oldS }()
	_, err := NewDecoderFromProbe(sample)
	var pe *errs.ProfileError
	if !errors.As(err, &pe) {
		t.Fatalf("want profile error for oversized frame, got %v", err)
	}
}

// TestPoolIdleBudgetAndTTL checks that idle decoders stay within
// maxIdlePixels and are closed after idleTTL, so decoder memory is not pinned
// for the life of the process.
func TestPoolIdleBudgetAndTTL(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	resetPoolForTest()
	defer resetPoolForTest()
	w, h, _ := sequenceFrameSize(sample)
	px := int64(w) * int64(h)

	oldBudget, oldTTL := maxIdlePixels, idleTTL
	maxIdlePixels, idleTTL = 2*px, 50*time.Millisecond
	defer func() { maxIdlePixels, idleTTL = oldBudget, oldTTL }()

	// Check out four decoders at once under distinct keys, then return them.
	type held struct {
		e   *poolEntry
		d   *Decoder
		max int
	}
	var hs []held
	for i := 0; i < 4; i++ {
		e, d, m := av1Pool.checkout(string(rune('k' + i)))
		if _, err := decodeHeld(&d, &m, nil, sample); err != nil {
			t.Fatal(err)
		}
		hs = append(hs, held{e, d, m})
	}
	for _, h := range hs {
		av1Pool.checkin(h.e, h.d, h.max)
	}
	av1Pool.mu.Lock()
	idle := av1Pool.idlePixels
	av1Pool.mu.Unlock()
	if idle > maxIdlePixels {
		t.Fatalf("idle pixels %d exceed budget %d", idle, maxIdlePixels)
	}
	closed := 0
	for _, h := range hs {
		if h.d.kd == nil {
			closed++
		}
	}
	if closed != 2 {
		t.Fatalf("closed %d decoders over budget, want 2", closed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		av1Pool.mu.Lock()
		idle = av1Pool.idlePixels
		av1Pool.mu.Unlock()
		if idle == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle decoders not swept: idlePixels=%d", idle)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, h := range hs {
		if h.d.kd != nil {
			t.Fatal("decoder still open after idle TTL")
		}
	}
}
