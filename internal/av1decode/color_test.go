package av1decode

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// TestSequenceColorBT709Full decodes a BT.709 full-range keyframe and checks the
// thumbnail is tagged from color_config and converts to within a few code
// values of ffmpeg's RGB (sidecar made with ffmpeg 9.0.2 default swscale flags, -pix_fmt rgb24).
func TestSequenceColorBT709Full(t *testing.T) {
	sample := readAV1(t, "tu_bt709_full.obu")
	th, err := DecodeSampleCached(nil, sample)
	if err != nil {
		t.Fatal(err)
	}
	if th.CS != yuv.BT709 || th.RNG != yuv.FullRange {
		t.Fatalf("got cs=%v rng=%v, want BT.709 full", th.CS, th.RNG)
	}
	f, err := os.Open(filepath.Join("..", "..", "test", "data", "av1", "tu_bt709_full_ffmpeg.png"))
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	mean := func(cs yuv.ColorSpace, rng yuv.Range) float64 {
		var sum, n float64
		b := th.YCbCr.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := th.YCbCr.YCbCrAt(x, y)
				r, g, bl := yuv.YCbCrToRGBCS(int(c.Y), int(c.Cb), int(c.Cr), cs, rng)
				rr, rg, rb, _ := ref.At(x, y).RGBA()
				sum += absf(float64(r)-float64(rr>>8)) + absf(float64(g)-float64(rg>>8)) + absf(float64(bl)-float64(rb>>8))
				n += 3
			}
		}
		return sum / n
	}
	got := mean(th.CS, th.RNG)
	old := mean(yuv.BT601, yuv.LimitedRange)
	t.Logf("mean abs diff vs ffmpeg: tagged=%.2f old BT.601 limited=%.2f", got, old)
	if got > 3 || got >= old {
		t.Fatalf("tagged conversion too far from ffmpeg: %.2f (old %.2f)", got, old)
	}
}

func TestSequenceColorDefault(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	th, err := DecodeSampleCached(nil, sample)
	if err != nil {
		t.Fatal(err)
	}
	if th.CS != yuv.BT601 || th.RNG != yuv.LimitedRange {
		t.Fatalf("untagged stream: %v %v", th.CS, th.RNG)
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
