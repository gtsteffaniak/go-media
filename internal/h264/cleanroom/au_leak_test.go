package cleanroom

import (
	"bytes"
	"errors"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
)

// twoMBStream builds a 32x16 (2 macroblock) IDR access unit with one I_PCM
// slice per macroblock. Slice 0 is filled with fill0; slice 1 with fill1, or is
// corrupt / omitted as requested.
func twoMBStream(t *testing.T, fill0, fill1 byte, corruptSecond, omitSecond bool) []byte {
	t.Helper()
	spsRBSP := decode.BaselineSPSRBSP(66, 30, 1, 0, 0, false)
	sps, err := decode.ParseSPS(spsRBSP)
	if err != nil {
		t.Fatal(err)
	}
	nalus := [][]byte{
		append([]byte{0x67}, spsRBSP...),
		append([]byte{0x68}, decode.BaselinePPSRBSP(0, 0, false)...),
		append([]byte{0x65}, decode.BuildIPCMSliceRBSPRange(sps, 0, 0, 1, fill0, false, 0)...),
	}
	if !omitSecond {
		nalus = append(nalus, append([]byte{0x65}, decode.BuildIPCMSliceRBSPRange(sps, 0, 1, 1, fill1, corruptSecond, 0)...))
	}
	return packAVCCSample(nalus...)
}

func TestMultiSliceFullCoverageDecodes(t *testing.T) {
	d := New()
	frames, err := d.DecodeIDRAVC(twoMBStream(t, 0x10, 0x20, false, false))
	if err != nil {
		t.Fatal(err)
	}
	f := frames[0]
	if f.Y[0] != 0x10 || f.Y[16] != 0x20 {
		t.Fatalf("unexpected samples: mb0=%#x mb1=%#x", f.Y[0], f.Y[16])
	}
}

func TestMissingMacroblocksIsError(t *testing.T) {
	d := New()
	_, err := d.DecodeIDRAVC(twoMBStream(t, 0x10, 0, false, true))
	if !errors.Is(err, ErrIncompletePicture) {
		t.Fatalf("err = %v, want ErrIncompletePicture", err)
	}
	if d.pic != nil {
		t.Fatal("picture buffer retained after incomplete decode")
	}
}

func TestDuplicateSliceDoesNotCountAsCoverage(t *testing.T) {
	spsRBSP := decode.BaselineSPSRBSP(66, 30, 1, 0, 0, false)
	sps, _ := decode.ParseSPS(spsRBSP)
	slice1 := decode.BuildIPCMSliceRBSPRange(sps, 0, 1, 1, 0x30, false, 0)
	data := packAVCCSample(
		append([]byte{0x67}, spsRBSP...),
		append([]byte{0x68}, decode.BaselinePPSRBSP(0, 0, false)...),
		append([]byte{0x65}, slice1...),
		append([]byte{0x65}, slice1...),
	)
	_, err := New().DecodeIDRAVC(data)
	if !errors.Is(err, ErrIncompletePicture) {
		t.Fatalf("err = %v, want ErrIncompletePicture", err)
	}
}

// TestFailedSliceDoesNotLeakPreviousPicture decodes stream A on a decoder
// (leaving its buffer pooled, as for a cropped picture), then a same-size
// stream B whose second slice fails. B must error and must not expose A's samples.
func TestFailedSliceDoesNotLeakPreviousPicture(t *testing.T) {
	const fillA, fillB = 0xAA, 0x11
	d := New()
	if _, err := d.DecodeIDRAVC(twoMBStream(t, fillA, fillA, false, false)); err != nil {
		t.Fatalf("decode A: %v", err)
	}
	if d.pic == nil {
		t.Fatal("test precondition: decoder should still hold A's buffer")
	}
	d.Reset()
	frames, err := d.DecodeIDRAVC(twoMBStream(t, fillB, 0, true, false))
	if err == nil {
		for _, f := range frames {
			if bytes.IndexByte(f.Y, fillA) >= 0 {
				t.Fatal("decode B returned samples from A")
			}
		}
		t.Fatal("decode B with failing second slice returned success")
	}
	if d.pic != nil {
		t.Fatal("picture buffer retained after failed decode")
	}
	// The next good decode must contain only its own samples.
	d.Reset()
	frames, err = d.DecodeIDRAVC(twoMBStream(t, 0x22, 0x33, false, false))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.IndexByte(frames[0].Y, fillA) >= 0 || bytes.IndexByte(frames[0].Y, fillB) >= 0 {
		t.Fatal("stale samples in subsequent decode")
	}
}
