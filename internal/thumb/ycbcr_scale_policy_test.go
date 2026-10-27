package thumb

import "testing"

func TestThumbDownscaleUsesRGBArea(t *testing.T) {
	cases := []struct {
		srcW, srcH, dstW, dstH int
		wantRGB                bool
	}{
		{1920, 1080, 256, 256, true},
		{1920, 1080, 1280, 720, false},
		{1920, 1080, 640, 360, true},
		{1280, 720, 256, 256, true},
	}
	for _, c := range cases {
		got := thumbDownscaleUsesRGBArea(c.srcW, c.srcH, c.dstW, c.dstH)
		if got != c.wantRGB {
			t.Fatalf("%dx%d -> %dx%d: rgbArea=%v want %v", c.srcW, c.srcH, c.dstW, c.dstH, got, c.wantRGB)
		}
	}
}
