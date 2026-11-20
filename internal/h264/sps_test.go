package h264

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// spsWriter builds SPS RBSP bit strings for tests.
type spsWriter struct {
	bits []byte // one entry per bit
}

// u appends the n low bits of v, most significant first.
func (w *spsWriter) u(n int, v uint64) *spsWriter {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, byte(v>>uint(i))&1)
	}
	return w
}

// ue appends v as an unsigned Exp-Golomb code.
func (w *spsWriter) ue(v uint64) *spsWriter {
	v++
	n := 0
	for x := v; x > 1; x >>= 1 {
		n++
	}
	w.u(n, 0)
	return w.u(n+1, v)
}

// se appends v as a signed Exp-Golomb code.
func (w *spsWriter) se(v int64) *spsWriter {
	if v > 0 {
		return w.ue(uint64(2*v - 1))
	}
	return w.ue(uint64(-2 * v))
}

// nal returns the NAL unit: header 0x67, the bits padded with a stop bit, and
// emulation prevention bytes inserted.
func (w *spsWriter) nal() []byte {
	bits := append(append([]byte(nil), w.bits...), 1)
	for len(bits)%8 != 0 {
		bits = append(bits, 0)
	}
	rbsp := make([]byte, len(bits)/8)
	for i, b := range bits {
		rbsp[i/8] |= b << (7 - uint(i%8))
	}
	out := []byte{0x67}
	zeros := 0
	for _, b := range rbsp {
		if zeros == 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// spsOpts selects the SPS syntax elements spsNAL writes.
type spsOpts struct {
	profile        uint64
	level          uint64
	spsID          uint64
	chroma         uint64
	lumaMinus8     uint64
	chromaMinus8   uint64
	scaling        bool
	pocType        uint64
	pocCycle       uint64
	frameMbsOnly   bool
	mbaff          bool
	crop           bool
	vui            bool
	aspectIDC      uint64
	noAspect       bool
	truncateAtBits int
}

// spsNAL writes an SPS NAL unit from o.
func spsNAL(o spsOpts) []byte {
	w := &spsWriter{}
	w.u(8, o.profile).u(8, 0).u(8, o.level).ue(o.spsID)
	switch o.profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		w.ue(o.chroma)
		if o.chroma == 3 {
			w.u(1, 0)
		}
		w.ue(o.lumaMinus8).ue(o.chromaMinus8).u(1, 0)
		if o.scaling {
			w.u(1, 1)
			lists := 8
			if o.chroma == 3 {
				lists = 12
			}
			for i := 0; i < lists; i++ {
				w.u(1, uint64(i%2))
				if i%2 == 1 {
					size := 16
					if i >= 6 {
						size = 64
					}
					w.se(5)
					for j := 1; j < size; j++ {
						w.se(int64(j % 3))
					}
				}
			}
		} else {
			w.u(1, 0)
		}
	}
	w.ue(0).ue(o.pocType)
	switch o.pocType {
	case 0:
		w.ue(2)
	case 1:
		w.u(1, 0).se(1).se(-1).ue(o.pocCycle)
		for i := uint64(0); i < o.pocCycle && i < 4; i++ {
			w.se(int64(i))
		}
	}
	w.ue(1).u(1, 0).ue(3).ue(2)
	if o.frameMbsOnly {
		w.u(1, 1)
	} else {
		w.u(1, 0)
		if o.mbaff {
			w.u(1, 1)
		} else {
			w.u(1, 0)
		}
	}
	w.u(1, 1)
	if o.crop {
		w.u(1, 1).ue(0).ue(1).ue(0).ue(1)
	} else {
		w.u(1, 0)
	}
	if o.vui {
		w.u(1, 1)
		if o.noAspect {
			w.u(1, 0)
		} else {
			w.u(1, 1).u(8, o.aspectIDC)
			if o.aspectIDC == 255 {
				w.u(16, 4).u(16, 3)
			}
		}
		w.u(1, 0) // overscan_info_present_flag; not read by the parser
	} else {
		w.u(1, 0)
	}
	if o.truncateAtBits > 0 {
		w.bits = w.bits[:o.truncateAtBits]
	}
	return w.nal()
}

// spsCase is one validateSPSNAL input and the expected error text ("" for
// accepted).
type spsCase struct {
	name    string
	nal     []byte
	wantErr string
}

// spsCases covers every accept and reject path of validateSPSNAL. Each case was
// checked against mp4ff v0.50.0.
func spsCases() []spsCase {
	return []spsCase{
		{"baseline", spsNAL(spsOpts{profile: 66, frameMbsOnly: true}), ""},
		{"main with crop and VUI", spsNAL(spsOpts{profile: 77, frameMbsOnly: true, crop: true, vui: true, aspectIDC: 1}), ""},
		{"high with scaling lists", spsNAL(spsOpts{profile: 100, chroma: 1, scaling: true, frameMbsOnly: true}), ""},
		{"high 10", spsNAL(spsOpts{profile: 110, chroma: 1, lumaMinus8: 2, chromaMinus8: 2, frameMbsOnly: true}), ""},
		{"high 12-bit", spsNAL(spsOpts{profile: 244, chroma: 1, lumaMinus8: 4, chromaMinus8: 4, frameMbsOnly: true}),
			"media: unsupported profile h264: 12-bit luma / 12-bit chroma (profile_idc 244); decoder supports 8, 9 and 10 bits"},
		{"high 4:2:2", spsNAL(spsOpts{profile: 122, chroma: 2, frameMbsOnly: true}), "media: unsupported profile h264: chroma format 2 (need 4:2:0)"},
		{"high 4:4:4 with scaling lists", spsNAL(spsOpts{profile: 244, chroma: 3, scaling: true, frameMbsOnly: true}), "media: unsupported profile h264: chroma format 3 (need 4:2:0)"},
		{"monochrome profile 138", spsNAL(spsOpts{profile: 138, chroma: 0, frameMbsOnly: true}), "media: unsupported profile h264: chroma format 0 (need 4:2:0)"},
		{"interlaced", spsNAL(spsOpts{profile: 77}), "media: unsupported profile h264: interlaced frame encoding"},
		{"MBAFF", spsNAL(spsOpts{profile: 77, mbaff: true}), "media: unsupported profile h264: MBAFF"},
		{"POC type 1", spsNAL(spsOpts{profile: 66, pocType: 1, pocCycle: 3, frameMbsOnly: true}), ""},
		{"POC cycle longer than the data", spsNAL(spsOpts{profile: 66, pocType: 1, pocCycle: 1 << 40, frameMbsOnly: true}), "media: unsupported profile h264: sps: EOF"},
		{"chroma_format_idc 7 with crop", spsNAL(spsOpts{profile: 100, chroma: 7, frameMbsOnly: true, crop: true}), "media: unsupported profile h264: sps: invalid chroma_format_idc value: 7"},
		{"chroma_format_idc 7 without crop", spsNAL(spsOpts{profile: 100, chroma: 7, frameMbsOnly: true}), "media: unsupported profile h264: chroma format 7 (need 4:2:0)"},
		{"extended SAR", spsNAL(spsOpts{profile: 66, frameMbsOnly: true, vui: true, aspectIDC: 255}), ""},
		{"reserved aspect_ratio_idc", spsNAL(spsOpts{profile: 66, frameMbsOnly: true, vui: true, aspectIDC: 17}), "media: unsupported profile h264: sps: SAR bad index 17"},
		{"VUI without aspect ratio", spsNAL(spsOpts{profile: 66, frameMbsOnly: true, vui: true, noAspect: true}), ""},
		{"not an SPS", []byte{0x68, 0xCE, 0x38, 0x80}, "media: unsupported profile h264: sps: not an SPS NAL unit"},
		{"empty", nil, "media: unsupported profile h264: sps: not an SPS NAL unit"},
		{"header only", []byte{0x67}, "media: unsupported profile h264: sps: EOF"},
		{"truncated", spsNAL(spsOpts{profile: 100, chroma: 1, frameMbsOnly: true})[:4], "media: unsupported profile h264: sps: EOF"},
		{"emulation prevention", spsNAL(spsOpts{profile: 66, spsID: 63, frameMbsOnly: true}), ""},
	}
}

// TestValidateSPSNAL runs every spsCase.
func TestValidateSPSNAL(t *testing.T) {
	if epb := spsNAL(spsOpts{profile: 66, spsID: 63, frameMbsOnly: true}); epb[4] != 0x03 {
		t.Fatalf("emulation prevention case has no 0x03 at byte 4: % x", epb)
	}
	for _, tc := range spsCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSPSNAL(tc.nal)
			got := ""
			if err != nil {
				got = err.Error()
				if !errors.Is(err, errs.UnsupportedProfile) {
					t.Errorf("error %v does not unwrap to UnsupportedProfile", err)
				}
			}
			if got != tc.wantErr {
				t.Errorf("validateSPSNAL(% x)\n got %q\nwant %q", tc.nal, got, tc.wantErr)
			}
		})
	}
}

// TestParseSPSHugePOCCycle checks that a POC cycle count far beyond the data is
// rejected without walking or allocating the count.
func TestParseSPSHugePOCCycle(t *testing.T) {
	nal := spsNAL(spsOpts{profile: 66, pocType: 1, pocCycle: 1<<63 - 1, frameMbsOnly: true})
	start := time.Now()
	_, err := parseSPSNAL(nal)
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("err = %v, want EOF", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("parse took %v", d)
	}
}

// FuzzValidateSPS checks that validateSPSNAL never panics and that every error
// unwraps to UnsupportedProfile.
func FuzzValidateSPS(f *testing.F) {
	for _, tc := range spsCases() {
		f.Add(tc.nal)
	}
	f.Fuzz(func(t *testing.T, nal []byte) {
		if err := validateSPSNAL(nal); err != nil && !errors.Is(err, errs.UnsupportedProfile) {
			t.Fatalf("error %v does not unwrap to UnsupportedProfile", err)
		}
	})
}
