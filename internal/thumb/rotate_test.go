package thumb

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// meanAbsDiff returns the mean absolute RGB difference of two same-size images.
func meanAbsDiff(a, b image.Image) float64 {
	ab, bb := a.Bounds(), b.Bounds()
	var sum, n float64
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			r1, g1, b1, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			for _, d := range []int{int(r1>>8) - int(r2>>8), int(g1>>8) - int(g2>>8), int(b1>>8) - int(b2>>8)} {
				if d < 0 {
					d = -d
				}
				sum += float64(d)
				n++
			}
		}
	}
	return sum / n
}

// TestRotatedMP4MatchesFFmpegAutorotate checks that rotated MP4 thumbnails come
// out in the same orientation and size as ffmpeg's autorotated frame.
func TestRotatedMP4MatchesFFmpegAutorotate(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	base := filepath.Join("..", "..", "testdata", "testsrc_96x64.mp4")
	for _, rot := range []int{0, 90, -90, 180} {
		t.Run(strconv.Itoa(rot), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "rot.mp4")
			if out, err := exec.Command(ffmpeg, "-v", "error", "-display_rotation", strconv.Itoa(rot), "-i", base, "-c", "copy", src).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg remux: %v %s", err, out)
			}
			ffPNG := filepath.Join(dir, "ff.png")
			if out, err := exec.Command(ffmpeg, "-v", "error", "-i", src, "-frames:v", "1", ffPNG).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg frame: %v %s", err, out)
			}
			ffData, err := os.ReadFile(ffPNG)
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(bytes.NewReader(ffData))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			res, err := Generate(bytes.NewReader(data), int64(len(data)), Options{MaxWidth: 96, MaxHeight: 96, SeekPercent: 1, Format: "png"})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			got, err := png.Decode(bytes.NewReader(res.Data))
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds().Size() != want.Bounds().Size() {
				t.Fatalf("size %v, ffmpeg %v", got.Bounds().Size(), want.Bounds().Size())
			}
			d := meanAbsDiff(got, want)
			flipped := meanAbsDiff(rotateImage(got, 180), want)
			t.Logf("rotation %d: mean abs diff %.2f (180-flipped %.2f)", rot, d, flipped)
			if d > 12 || d*2 > flipped {
				t.Fatalf("orientation mismatch: diff %.2f, flipped %.2f", d, flipped)
			}
		})
	}
}

// TestRotateImageYCbCrMatchesRGBA checks the plane rotation against the RGBA path.
func TestRotateImageYCbCrMatchesRGBA(t *testing.T) {
	cases := []struct {
		ratio image.YCbCrSubsampleRatio
		w, h  int
	}{
		{image.YCbCrSubsampleRatio420, 8, 6},
		{image.YCbCrSubsampleRatio420, 7, 5}, // odd: RGBA fallback
		{image.YCbCrSubsampleRatio444, 7, 5},
	}
	for _, c := range cases {
		ratio := c.ratio
		src := image.NewYCbCr(image.Rect(0, 0, c.w, c.h), ratio)
		for i := range src.Y {
			src.Y[i] = byte(i * 7)
		}
		for i := range src.Cb {
			src.Cb[i] = byte(i * 13)
			src.Cr[i] = byte(255 - i*11)
		}
		for _, r := range []int{90, -90, 180} {
			got := rotateImage(src, r)
			rgba := image.NewRGBA(src.Rect)
			for y := 0; y < c.h; y++ {
				for x := 0; x < c.w; x++ {
					rgba.Set(x, y, src.At(x, y))
				}
			}
			want := rotateImage(rgba, r)
			if got.Bounds() != want.Bounds() {
				t.Fatalf("ratio %v rot %d: bounds %v want %v", ratio, r, got.Bounds(), want.Bounds())
			}
			if d := meanAbsDiff(got, want); d != 0 {
				t.Fatalf("ratio %v rot %d: diff %.2f", ratio, r, d)
			}
		}
	}
}
