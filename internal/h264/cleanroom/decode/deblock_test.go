package decode

import "testing"

func TestFilterLumaNormal(t *testing.T) {
	// Line p3..p0|q0..q3 = 100,100,100,100 | 110,110,110,110. bS=3, tc0=4.
	// Expect: p0=104, q0=106, p1=102, q1=107.
	plane := []byte{100, 100, 100, 100, 110, 110, 110, 110}
	filterLuma(plane, 4, 1, 3, 255, 255, 4)
	if plane[3] != 104 || plane[4] != 106 {
		t.Errorf("p0/q0 = %d/%d, want 104/106", plane[3], plane[4])
	}
	if plane[2] != 102 || plane[5] != 107 {
		t.Errorf("p1/q1 = %d/%d, want 102/107", plane[2], plane[5])
	}
}

func TestFilterLumaNoFilterWhenFlat(t *testing.T) {
	// If |p0-q0| >= alpha, the filter is not applied.
	plane := []byte{100, 100, 100, 100, 110, 110, 110, 110}
	orig := append([]byte(nil), plane...)
	filterLuma(plane, 4, 1, 3, 4, 255, 4) // alpha=4 < |p0-q0|=10
	for i := range plane {
		if plane[i] != orig[i] {
			t.Fatalf("filter applied when alpha<|p0-q0|: %v", plane)
		}
	}
}

func TestFilterChroma(t *testing.T) {
	// p1,p0,q0,q1 = 100,100,110,110. bS=3, tc0=4 → p0=104, q0=106.
	plane := []byte{100, 100, 110, 110}
	filterChroma(plane, 2, 1, 3, 255, 255, 4)
	if plane[1] != 104 || plane[2] != 106 {
		t.Errorf("chroma p0/q0 = %d/%d, want 104/106", plane[1], plane[2])
	}
	// bS=4 strong filter: p0=(2*100+100+110+2)>>2=103, q0=(2*110+110+100+2)>>2=108.
	plane = []byte{100, 100, 110, 110}
	filterChroma(plane, 2, 1, 4, 255, 255, 0)
	if plane[1] != 103 || plane[2] != 108 {
		t.Errorf("chroma bS4 p0/q0 = %d/%d, want 103/108", plane[1], plane[2])
	}
}

// TestDeblockFlatFrameUnchanged: on a flat frame (32x16, 2 MB, all samples 128)
// deblocking changes nothing — there are no jumps at the edges.
func TestDeblockFlatFrameUnchanged(t *testing.T) {
	sps, _ := ParseSPS(encodeSPS(66, 30, 1, 0, 0, false)) // 2 MB per row
	pps := testPPS()
	frame, err := NewFrame(sps)
	if err != nil {
		t.Fatal(err)
	}
	h := &SliceHeader{FirstMB: 0, Type: SliceI, SliceQP: 26}
	// Two I_NxN DC macroblocks with no residual → the whole plane is 128.
	mb := "1" + "1111111111111111" + "1" + "00100"
	sd := newSliceDecoder(bitsReader(mb+mb+"1"), h, sps, pps, frame)
	if err := sd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	sd.deblock()
	for i := range frame.Y {
		if frame.Y[i] != 128 {
			t.Fatalf("after deblocking Y[%d]=%d, want 128 (flat frame)", i, frame.Y[i])
		}
	}
	for i := range frame.Cb {
		if frame.Cb[i] != 128 || frame.Cr[i] != 128 {
			t.Fatalf("after deblocking chroma[%d]=%d/%d, want 128", i, frame.Cb[i], frame.Cr[i])
		}
	}
}
