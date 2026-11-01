package thumb

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/internal/rawpreview"
)

func synthJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 200), uint8(y % 200), 90, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82})
	return buf.Bytes()
}

func TestGenerateHEIFEmbedded(t *testing.T) {
	jpeg := synthJPEG(240, 180)
	data := heifthumb.SynthJPEGHEIF(jpeg, 2, 1)
	res, err := Generate(bytes.NewReader(data), int64(len(data)), Options{MaxWidth: 96, MaxHeight: 96, SeekPercent: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "embedded" || res.Width <= 0 || res.Height <= 0 {
		t.Fatalf("result: %+v", res)
	}
}

func TestGenerateHEIFEmbeddedPrimaryIrotSwapsDimensions(t *testing.T) {
	jpeg := synthJPEG(200, 100)
	data := heifthumb.SynthJPEGHEIFWithPrimaryIrot(jpeg, 2, 1, 1) // 90° CCW display
	res, err := Generate(bytes.NewReader(data), int64(len(data)), Options{
		MaxWidth: 512, MaxHeight: 512, SeekPercent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "embedded" {
		t.Fatalf("source=%q want embedded", res.Source)
	}
	if res.Width != 100 || res.Height != 200 {
		t.Fatalf("rotated size = %dx%d, want 100x200 (coded 200x100, irot=1)", res.Width, res.Height)
	}

	noRot := heifthumb.SynthJPEGHEIFWithPrimaryIrot(jpeg, 2, 1, 0)
	res0, err := Generate(bytes.NewReader(noRot), int64(len(noRot)), Options{
		MaxWidth: 512, MaxHeight: 512, SeekPercent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res0.Width != 200 || res0.Height != 100 {
		t.Fatalf("unrotated size = %dx%d, want 200x100", res0.Width, res0.Height)
	}
}

func TestGenerateRAWEmbedded(t *testing.T) {
	jpeg := synthJPEG(400, 300)
	tiff := rawpreview.SynthTIFFJPEG(jpeg)
	res, err := Generate(bytes.NewReader(tiff), int64(len(tiff)), Options{MaxWidth: 128, MaxHeight: 128, SeekPercent: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "embedded" {
		t.Fatalf("source=%q", res.Source)
	}
}
