package heifthumb_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
)

type sliceReaderAt struct {
	b []byte
}

func (s sliceReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(s.b)) {
		return 0, os.ErrInvalid
	}
	return copy(p, s.b[off:]), nil
}

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

func TestPrimaryDisplayRotationSyntheticNoIrot(t *testing.T) {
	data := heifthumb.SynthJPEGHEIF([]byte{0xff, 0xd8, 0xff, 0xd9}, 2, 1)
	rot, ok := heifthumb.PrimaryDisplayRotation(sliceReaderAt{b: data}, int64(len(data)))
	if ok {
		t.Fatalf("PrimaryDisplayRotation() = (%d, true), want not found without iprp/irot", rot)
	}
}

func TestPrimaryDisplayRotationSyntheticKnownIrot(t *testing.T) {
	jpeg := synthJPEG(320, 240)
	tests := []struct {
		name    string
		irot    uint8
		wantDeg int
	}{
		{"irot0", 0, 0},
		{"irot1", 1, 90},
		{"irot2", 2, 180},
		{"irot3", 3, 270},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := heifthumb.SynthJPEGHEIFWithPrimaryIrot(jpeg, 2, 1, tt.irot)
			rot, ok := heifthumb.PrimaryDisplayRotation(sliceReaderAt{b: data}, int64(len(data)))
			if !ok {
				t.Fatal("PrimaryDisplayRotation: want irot from synthetic ipma")
			}
			if rot != tt.wantDeg {
				t.Fatalf("PrimaryDisplayRotation() = %d°, want %d° for irot=%d", rot, tt.wantDeg, tt.irot)
			}
		})
	}
}

func TestPrimaryDisplayRotationUsesPrimaryNotThumbnail(t *testing.T) {
	jpeg := synthJPEG(100, 80)
	data := heifthumb.SynthJPEGHEIFWithItemIrot(jpeg, 2, 1, 0, 3)
	rot, ok := heifthumb.PrimaryDisplayRotation(sliceReaderAt{b: data}, int64(len(data)))
	if !ok || rot != 0 {
		t.Fatalf("PrimaryDisplayRotation() = (%d, %v), want (0, true) from primary irot=0", rot, ok)
	}
}

func TestPrimaryDisplayRotationRealHEICOptional(t *testing.T) {
	path := os.Getenv("GOMEDIA_TEST_HEIC")
	if path == "" {
		t.Skip("set GOMEDIA_TEST_HEIC to run against a local HEIC sample")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Skip("no HEIC sample:", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rot, ok := heifthumb.PrimaryDisplayRotation(f, st.Size())
	if !ok {
		t.Fatal("PrimaryDisplayRotation: want irot on phone HEIC sample")
	}
	if rot != 90 && rot != 270 {
		t.Fatalf("PrimaryDisplayRotation = %d°, want 90 or 270 for typical portrait phone still", rot)
	}
}
