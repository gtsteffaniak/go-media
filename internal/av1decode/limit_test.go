package av1decode

import (
	"errors"
	"strings"
	"testing"

	"github.com/gen2brain/gav1d/av1"

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

// obuOfType returns the first OBU of type typ (raw bytes) in a low-overhead
// stream whose OBUs all carry a size field.
func obuOfType(t *testing.T, src []byte, typ byte) []byte {
	t.Helper()
	for len(src) > 0 {
		hdr := 1
		if src[0]&0x04 != 0 { // obu_extension_flag
			hdr = 2
		}
		if src[0]&0x02 == 0 || len(src) <= hdr { // obu_has_size_field
			t.Fatal("OBU without size field")
		}
		size, n := 0, 0
		for i := 0; i < 8; i++ {
			b := src[hdr+i]
			size |= int(b&0x7f) << (7 * i)
			n++
			if b&0x80 == 0 {
				break
			}
		}
		end := hdr + n + size
		if (src[0]>>3)&0x0f == typ {
			return append([]byte(nil), src[:end]...)
		}
		src = src[end:]
	}
	t.Fatalf("no OBU of type %d", typ)
	return nil
}

const (
	obuSequenceHeader = 1
	obuFrame          = 6
)

// TestFrameSizeEverySequenceHeader checks that a small sequence header ahead
// of a larger one in one temporal unit does not bypass the limit: gav1d
// adopts the later header, and FrameSizeLimit is checked against every frame
// header before picture memory is allocated.
func TestFrameSizeEverySequenceHeader(t *testing.T) {
	small := av1.Encode(av1.EncodeConfig{Width: 64, Height: 64, BitDepth: 8, QIndex: 128, Speed: 10, Threads: 1})
	big := av1.Encode(av1.EncodeConfig{Width: 64, Height: 128, BitDepth: 8, QIndex: 128, Speed: 10, Threads: 1})
	smallSeq := obuOfType(t, small, obuSequenceHeader)
	bigSeq := obuOfType(t, big, obuSequenceHeader)
	bigFrame := obuOfType(t, big, obuFrame)
	cat := func(parts ...[]byte) []byte {
		var b []byte
		for _, p := range parts {
			b = append(b, p...)
		}
		return b
	}
	// Small header, larger header, then the larger frame; and a whole small
	// unit followed by a whole larger one.
	mixed := cat(smallSeq, bigSeq, bigFrame)
	twoUnits := cat(small, big)

	if th, err := DecodeSampleCached(nil, mixed); err != nil || th.YCbCr.Bounds().Dy() != 128 {
		t.Fatalf("in-limit mixed unit: %v", err)
	}
	if _, err := DecodeSampleCached(nil, small); err != nil {
		t.Fatal(err)
	}

	setFrameLimit(t, 64, 64)
	for name, unit := range map[string][]byte{"mixed": mixed, "two units": twoUnits} {
		_, err := DecodeSampleCached(nil, unit)
		var pe *errs.ProfileError
		if !errors.As(err, &pe) || !strings.Contains(err.Error(), "frame header exceeds") {
			t.Fatalf("%s: want frame-size profile error, got %v", name, err)
		}
		// av1C config OBUs carry the small header; the sample the larger one.
		av1C := cat([]byte{0x81, 0, 0, 0}, smallSeq)
		if _, err := DecodeSampleCached(av1C, cat(bigSeq, bigFrame)); !errors.As(err, &pe) {
			t.Fatalf("%s via av1C: want profile error, got %v", name, err)
		}
	}
	if _, err := DecodeSampleCached(nil, small); err != nil {
		t.Fatalf("in-limit sample after rejection: %v", err)
	}
}

// setFrameLimit sets the frame-size limit for one test.
func setFrameLimit(t *testing.T, long, short int) {
	oldL, oldS := MaxFrameLongSide, MaxFrameShortSide
	MaxFrameLongSide, MaxFrameShortSide = long, short
	t.Cleanup(func() { MaxFrameLongSide, MaxFrameShortSide = oldL, oldS })
}
