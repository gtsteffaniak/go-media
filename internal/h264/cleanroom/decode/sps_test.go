package decode

import "testing"

func TestParseSPS(t *testing.T) {
	cases := []struct {
		name         string
		rbsp         []byte
		wantW, wantH int
		wantProfile  uint8
	}{
		{"QCIF baseline 176x144", encodeSPS(66, 11, 10, 8, 0, false), 176, 144, 66},
		{"720p high 1280x720", encodeSPS(100, 31, 79, 44, 0, true), 1280, 720, 100},
		{"1080p baseline 1920x1080", encodeSPS(66, 40, 119, 67, 4, false), 1920, 1080, 66},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sps, err := ParseSPS(c.rbsp)
			if err != nil {
				t.Fatalf("ParseSPS: %v", err)
			}
			if sps.Width != c.wantW || sps.Height != c.wantH {
				t.Errorf("size = %dx%d, want %dx%d", sps.Width, sps.Height, c.wantW, c.wantH)
			}
			if sps.ProfileIDC != c.wantProfile {
				t.Errorf("profile = %d, want %d", sps.ProfileIDC, c.wantProfile)
			}
			t.Logf("OK: %s", sps.Summary())
		})
	}
}

func TestParsePPS(t *testing.T) {
	pps, err := ParsePPS(BaselinePPSRBSP(0, 0, true))
	if err != nil {
		t.Fatalf("ParsePPS: %v", err)
	}
	if !pps.EntropyCodingMode || pps.EntropyName() != "CABAC" {
		t.Errorf("entropy = %s, want CABAC", pps.EntropyName())
	}
}
