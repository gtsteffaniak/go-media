package decode

import (
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

// ppsRBSPWithQP builds a CAVLC PPS with the given QP fields. When second is
// non-nil the High-profile extension is written with second_chroma_qp_index_offset.
func ppsRBSPWithQP(picInitQPMinus26, chromaOff int32, second *int32) []byte {
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
	w.se(picInitQPMinus26)
	w.se(0)
	w.se(chromaOff)
	w.flag(false)
	w.flag(false)
	w.flag(false)
	if second != nil {
		w.flag(false) // transform_8x8_mode_flag
		w.flag(false) // pic_scaling_matrix_present_flag
		w.se(*second)
	}
	w.u(1, 1) // rbsp_stop_one_bit
	return w.bytes()
}

func i32(v int32) *int32 { return &v }

func TestParsePPSQPRanges(t *testing.T) {
	cases := []struct {
		name          string
		init, chroma  int32
		second        *int32
		wantErrSubstr string
	}{
		{"min-init", -26, 0, nil, ""},
		{"max-init", 25, 0, nil, ""},
		{"init-too-low", -27, 0, nil, "pic_init_qp_minus26"},
		{"init-too-high", 26, 0, nil, "pic_init_qp_minus26"},
		{"chroma-min", 0, -12, nil, ""},
		{"chroma-max", 0, 12, nil, ""},
		{"chroma-too-low", 0, -13, nil, "chroma_qp_index_offset"},
		{"chroma-too-high", 0, 13, nil, "chroma_qp_index_offset"},
		{"second-ok", 0, 0, i32(-12), ""},
		{"second-too-high", 0, 0, i32(13), "second_chroma_qp_index_offset"},
		{"second-too-low", 0, 0, i32(-13), "second_chroma_qp_index_offset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParsePPS(ppsRBSPWithQP(tc.init, tc.chroma, tc.second))
			if tc.wantErrSubstr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if p.PicInitQPMinus26 != tc.init || p.ChromaQPIndexOffset != tc.chroma {
					t.Fatalf("got init=%d chroma=%d", p.PicInitQPMinus26, p.ChromaQPIndexOffset)
				}
				want := tc.chroma
				if tc.second != nil {
					want = *tc.second
				}
				if p.SecondChromaQPIndexOffset != want {
					t.Fatalf("second offset = %d, want %d", p.SecondChromaQPIndexOffset, want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErrSubstr)
			}
		})
	}
}

func TestSliceQPRange(t *testing.T) {
	sps, err := ParseSPS(BaselineSPSRBSP(66, 30, 0, 0, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		init, delta int32
		ok          bool
	}{
		{0, 0, true},
		{-26, 0, true},   // QP 0
		{25, 0, true},    // QP 51
		{-26, -1, false}, // QP -1: would index level scale tables negatively
		{25, 1, false},   // QP 52
		{0, 1000, false},
		{0, -1000, false},
	}
	for _, tc := range cases {
		pps, err := ParsePPS(ppsRBSPWithQP(tc.init, 0, nil))
		if err != nil {
			t.Fatal(err)
		}
		frame, err := NewFrame(sps)
		if err != nil {
			t.Fatal(err)
		}
		rbsp := BuildIPCMSliceRBSPRange(sps, 0, 0, 1, 0x40, false, tc.delta)
		_, err = DecodeSlice(rbsp, nal.TypeIDR, 3, sps, pps, frame, false)
		if tc.ok && err != nil {
			t.Fatalf("init=%d delta=%d: unexpected error %v", tc.init, tc.delta, err)
		}
		if !tc.ok && (err == nil || !strings.Contains(err.Error(), "SliceQPY")) {
			t.Fatalf("init=%d delta=%d: err = %v, want SliceQPY range error", tc.init, tc.delta, err)
		}
	}
}

// FuzzParsePPS exercises PPS RBSP parsing, seeded with QP edge values.
func FuzzParsePPS(f *testing.F) {
	f.Add(BaselinePPSRBSP(0, 0, false))
	for _, s := range [][]byte{
		ppsRBSPWithQP(-26, -12, nil),
		ppsRBSPWithQP(25, 12, nil),
		ppsRBSPWithQP(-27, 0, nil),
		ppsRBSPWithQP(26, 0, nil),
		ppsRBSPWithQP(0, 13, nil),
		ppsRBSPWithQP(0, 0, i32(-13)),
		ppsRBSPWithQP(-1<<20, 1<<20, i32(1<<20)),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParsePPS(data)
		if err != nil {
			return
		}
		if p.PicInitQPMinus26 < -26 || p.PicInitQPMinus26 > 25 ||
			p.ChromaQPIndexOffset < -12 || p.ChromaQPIndexOffset > 12 ||
			p.SecondChromaQPIndexOffset < -12 || p.SecondChromaQPIndexOffset > 12 {
			t.Fatalf("accepted out-of-range PPS QP fields: %+v", p)
		}
	})
}
