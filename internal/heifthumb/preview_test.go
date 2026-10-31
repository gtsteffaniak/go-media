package heifthumb

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func synthJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 50, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func TestJPEGPreviewSyntheticHEIF(t *testing.T) {
	jpeg := synthJPEG(256, 192)
	data := SynthJPEGHEIF(jpeg, 2, 1)
	out, err := JPEGPreview(bytes.NewReader(data), int64(len(data)), 128, 128)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, jpeg) {
		t.Fatalf("got %d bytes, want embedded jpeg %d bytes", len(out), len(jpeg))
	}
}

func TestJPEGPreviewCodedThumbUnsupported(t *testing.T) {
	hvc := synthInfe(2, "hvc1")
	infe1 := synthInfe(1, "jpeg")
	iinfBody := append([]byte{0, 0, 0, 0}, u16(2)...)
	iinfBody = append(iinfBody, infe1...)
	iinfBody = append(iinfBody, hvc...)
	iinf := synthBox("iinf", iinfBody)
	iref := synthThmbRef(1, 2)
	iloc := synthIloc(2, 100, 4)
	ftyp := synthBox("ftyp", []byte("heic"), u32(0), []byte("heic"))
	meta := synthBox("meta", append([]byte{0, 0, 0, 0}, append(iinf, append(iref, iloc...)...)...))
	file := append(ftyp, meta...)
	_, err := JPEGPreview(bytes.NewReader(file), int64(len(file)), 128, 128)
	if err == nil {
		t.Fatal("expected unsupported for HEVC thumb")
	}
}
