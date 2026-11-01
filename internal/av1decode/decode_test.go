package av1decode_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/av1decode"
	"github.com/gtsteffaniak/go-media/internal/thumb"
)

func TestAV1MP4Fixture(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "s8_1080.mp4")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip(path, err)
	}
	f := bytesReaderAt{data}
	res, err := thumb.Generate(f, int64(len(data)), thumb.Options{
		MaxWidth: 320, MaxHeight: 320, SeekPercent: 10, Format: "jpeg", JPEGQuality: 76,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) < 100 || res.MIME != "image/jpeg" {
		t.Fatalf("unexpected thumb: mime=%q len=%d", res.MIME, len(res.Data))
	}
}

func TestAVIFPrimaryFixture(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "i8_1080.avif")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip(path, err)
	}
	ycc, err := av1decode.DecodeAVIFFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if ycc == nil || ycc.Bounds().Dx() < 100 {
		t.Fatalf("bad picture %v", ycc)
	}
}

type bytesReaderAt struct{ b []byte }

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.b)) {
		return 0, os.ErrInvalid
	}
	n := copy(p, b.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
