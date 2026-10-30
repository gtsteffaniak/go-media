package compare_test

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// TestOpenGOPThumbnailMatchesFFmpeg seeks onto non-IDR I keyframes of an x264
// open-GOP fixture and checks the picture against ffmpeg's decode of the same
// frame. ffmpeg fast seek starts decoding at such a keyframe with an empty
// DPB; go-media now does the same instead of rejecting the sample.
func TestOpenGOPThumbnailMatchesFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join("..", "..", "testdata", "opengop_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("fixtures missing, run scripts/generate-fixtures.sh")
	}
	defer f.Close()
	st, _ := f.Stat()

	const w, h = 160, 96
	// Every frame, deblocking off to match the fast intra decode.
	raw, err := exec.Command("ffmpeg", "-v", "error", "-skip_loop_filter", "all", "-i", path,
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
	if err != nil {
		t.Fatalf("ffmpeg: %v", err)
	}
	n := len(raw) / (w * h * 3)
	// Keyframes at 2s and 4s (frames 20 and 40) are non-IDR I pictures.
	nonIDRKey := map[int]bool{20: true, 40: true}

	hit := 0
	for _, p := range []float64{40, 60, 75, 90} {
		opt := media.OptionsForPreset(media.ThumbnailPresetLarge, p)
		opt.Preset = ""
		opt.MaxWidth, opt.MaxHeight = w, h
		data, err := media.ThumbnailRasterPNG(f, st.Size(), opt)
		if err != nil {
			t.Fatalf("seek %v%%: %v", p, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		rgba := image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
		if rgba.Bounds().Dx() != w || rgba.Bounds().Dy() != h {
			t.Fatalf("seek %v%%: size %v", p, rgba.Bounds())
		}
		best, bestIdx := math.Inf(-1), -1
		for i := 0; i < n; i++ {
			fr := raw[i*w*h*3 : (i+1)*w*h*3]
			var se float64
			for px := 0; px < w*h; px++ {
				for c := 0; c < 3; c++ {
					d := float64(rgba.Pix[px*4+c]) - float64(fr[px*3+c])
					se += d * d
				}
			}
			mse := se / float64(w*h*3)
			psnr := 99.0
			if mse > 0 {
				psnr = 10 * math.Log10(255*255/mse)
			}
			if psnr > best {
				best, bestIdx = psnr, i
			}
		}
		t.Logf("seek %v%%: best ffmpeg frame %d PSNR %.1f dB", p, bestIdx, best)
		if best < 40 {
			t.Errorf("seek %v%%: PSNR %.1f dB vs ffmpeg frame %d, want >= 40", p, best, bestIdx)
		}
		if nonIDRKey[bestIdx] {
			hit++
		}
	}
	if hit == 0 {
		t.Fatal("no seek landed on a non-IDR keyframe; fixture no longer exercises open GOP")
	}
}
