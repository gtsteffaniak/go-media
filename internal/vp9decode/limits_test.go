package vp9decode

import (
	"errors"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// bitWriter packs MSB-first bits for a hand-built VP9 uncompressed header.
type bitWriter struct {
	b []byte
	n int
}

func (w *bitWriter) put(v uint32, bits int) {
	for i := bits - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		if v>>uint(i)&1 == 1 {
			w.b[len(w.b)-1] |= 0x80 >> uint(w.n%8)
		}
		w.n++
	}
}

// keyframeHeader returns a profile 0 VP9 key frame whose uncompressed header
// declares w×h; the compressed data is junk, which is never reached when the
// size is rejected.
func keyframeHeader(w, h int) []byte {
	var bw bitWriter
	bw.put(2, 2)         // frame_marker
	bw.put(0, 2)         // profile 0
	bw.put(0, 1)         // show_existing_frame
	bw.put(0, 1)         // frame_type = KEY_FRAME
	bw.put(1, 1)         // show_frame
	bw.put(0, 1)         // error_resilient_mode
	bw.put(0x498342, 24) // sync code
	bw.put(2, 3)         // color_space BT.709
	bw.put(0, 1)         // color_range
	bw.put(uint32(w-1), 16)
	bw.put(uint32(h-1), 16)
	bw.put(0, 1) // render_and_frame_size_different
	bw.put(0, 1) // refresh_frame_context
	bw.put(0, 1) // frame_parallel_decoding_mode
	bw.put(0, 2) // frame_context_idx
	bw.put(10, 6)
	bw.put(0, 3)  // sharpness
	bw.put(0, 1)  // mode_ref_delta_enabled
	bw.put(60, 8) // base_q_idx
	bw.put(0, 3)  // no delta q
	bw.put(0, 1)  // segmentation_enabled
	bw.put(0, 8)  // tile cols increments (stops at first 0) and tile_rows
	bw.put(64, 16)
	return append(bw.b, make([]byte, 256)...)
}

// TestOversizedFrameRejected checks that a key frame header declaring more
// than MaxDimension is refused before any frame buffer is allocated.
func TestOversizedFrameRejected(t *testing.T) {
	d, err := NewFrameDecoder()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	_, err = d.DecodeFramePicture(keyframeHeader(32768, 32768))
	var pe *errs.ProfileError
	if !errors.As(err, &pe) {
		t.Fatalf("want profile error for 32768x32768 frame, got %v", err)
	}
}
