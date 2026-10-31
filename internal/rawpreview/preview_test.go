package rawpreview

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

func synthJPEG(w, h int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func TestJPEGPreviewTIFF(t *testing.T) {
	big := synthJPEG(640, 480, color.RGBA{10, 20, 30, 255})
	small := synthJPEG(160, 120, color.RGBA{200, 10, 10, 255})
	tiff := SynthTIFFJPEG(big)
	// Append a second smaller JPEG after the first (scan fallback would find both).
	tiff = append(tiff, small...)
	r := bytes.NewReader(tiff)
	id, err := sniff.Detect(r, int64(len(tiff)))
	if err != nil || id != format.DNG {
		t.Fatalf("sniff = %v %v", id, err)
	}
	out, err := JPEGPreview(bytes.NewReader(tiff), int64(len(tiff)), id, 320, 240)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 640 || cfg.Height != 480 {
		t.Fatalf("picked %dx%d, want 640x480 preview", cfg.Width, cfg.Height)
	}
}

func TestJPEGPreviewScanCR2Marker(t *testing.T) {
	prev := synthJPEG(320, 240, color.RGBA{1, 2, 3, 255})
	var cr2 []byte
	cr2 = append(cr2, 'I', 'I', 42, 0, 0, 0, 0, 0, 'C', 'R', 0, 0)
	cr2 = append(cr2, prev...)
	id, err := sniff.Detect(bytes.NewReader(cr2), int64(len(cr2)))
	if err != nil || id != format.CR2 {
		t.Fatalf("sniff = %v %v", id, err)
	}
	out, err := JPEGPreview(bytes.NewReader(cr2), int64(len(cr2)), id, 160, 120)
	if err != nil || len(out) == 0 {
		t.Fatalf("preview: %v len=%d", err, len(out))
	}
}
