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
