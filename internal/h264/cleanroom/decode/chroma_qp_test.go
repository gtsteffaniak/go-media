package decode

import (
	"bytes"
	"errors"
	"testing"
)

// chromaEdgeResult runs filterChromaEdge across one vertical chroma MB edge.
// Both planes start with the same samples, and only the QP offsets differ.
// It returns the Cb and Cr planes after filtering.
func chromaEdgeResult(t *testing.T, chroma, second int32) (cb, cr []byte) {
	t.Helper()
	sps, err := ParseSPS(encodeSPS(66, 30, 1, 0, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	pps := testPPS()
	pps.ChromaQPIndexOffset = chroma
	pps.SecondChromaQPIndexOffset = second
	frame, err := NewFrame(sps)
	if err != nil {
		t.Fatal(err)
	}
	// The edge is at chroma x=8. The step of 30 is at least alpha for QPc 28
	// (20) and below alpha for QPc 36 (50), so the outcome depends on the
	// QP that is used.
	for _, plane := range [][]byte{frame.Cb, frame.Cr} {
		for i := range plane {
			if i%frame.StrideC < 8 {
				plane[i] = 100
			} else {
				plane[i] = 130
			}
		}
	}
	sd := newSliceDecoder(bitsReader("1"), &SliceHeader{}, sps, pps, frame)
	sd.filterChromaEdge(1, 0, 0, 0, 1, 4, 40, 40, true)
	return frame.Cb, frame.Cr
}

// TestChromaEdgeUsesSecondOffsetForCr checks that the Cr plane is deblocked with
// second_chroma_qp_index_offset and Cb with chroma_qp_index_offset.
func TestChromaEdgeUsesSecondOffsetForCr(t *testing.T) {
	const second = -12
	cbBase, crBase := chromaEdgeResult(t, 0, 0)
	cbCrOnly, crCrOnly := chromaEdgeResult(t, 0, second)
	cbMatched, _ := chromaEdgeResult(t, second, second)

	if !bytes.Equal(cbBase, cbCrOnly) {
		t.Error("Cb changed when only the Cr offset changed")
	}
	if bytes.Equal(crBase, crCrOnly) {
		t.Fatal("Cr unchanged by second_chroma_qp_index_offset")
	}
	// Cr with the second offset must equal Cb filtered at that same offset.
	if !bytes.Equal(crCrOnly, cbMatched) {
		t.Error("Cr with second offset does not match Cb filtered at the same offset")
	}
}

// TestChromaQPForSelectsOffsetPerComponent checks the Cb and Cr QP selection
// used by reconstruction and deblocking.
func TestChromaQPForSelectsOffsetPerComponent(t *testing.T) {
	pps := testPPS()
	pps.ChromaQPIndexOffset = -4
	pps.SecondChromaQPIndexOffset = 6
	if got, want := chromaQPFor(pps, 0, 40), chromaQP(40, -4); got != want {
		t.Errorf("Cb QPc = %d, want %d", got, want)
	}
	if got, want := chromaQPFor(pps, 1, 40), chromaQP(40, 6); got != want {
		t.Errorf("Cr QPc = %d, want %d", got, want)
	}
	if chromaQPFor(pps, 0, 40) == chromaQPFor(pps, 1, 40) {
		t.Fatal("distinct offsets gave the same QPc; test values do not separate Cb and Cr")
	}
}

// ppsRBSPScaling builds a High-profile PPS with pic_scaling_matrix_present_flag
// set and no scaling lists.
func ppsRBSPScaling() []byte {
	w := &bitWriter{}
	w.ue(0)
	w.ue(0)
	w.flag(false)
	w.flag(false)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.flag(false)
	w.u(2, 0)
	w.se(0)
	w.se(0)
	w.se(0)
	w.flag(false)
	w.flag(false)
	w.flag(false)
	w.flag(false) // transform_8x8_mode_flag
	w.flag(true)  // pic_scaling_matrix_present_flag
	w.u(1, 1)     // rbsp_stop_one_bit
	return w.bytes()
}

// TestParsePPSRejectsScalingMatrix checks that a PPS with picture scaling
// matrices is rejected, because its Cr offset cannot be read.
func TestParsePPSRejectsScalingMatrix(t *testing.T) {
	if _, err := ParsePPS(ppsRBSPScaling()); !errors.Is(err, ErrPicScalingMatrix) {
		t.Fatalf("ParsePPS error = %v, want ErrPicScalingMatrix", err)
	}
}
