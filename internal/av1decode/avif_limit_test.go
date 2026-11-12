package av1decode

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func TestAVIFDecodeLimit(t *testing.T) {
	t.Parallel()
	const callerCap int64 = 1000
	if got := avifDecodeLimit(0); got != MaxAVIFPixels {
		t.Fatalf("zero cap: got %d want %d", got, MaxAVIFPixels)
	}
	if got := avifDecodeLimit(callerCap); got != callerCap {
		t.Fatalf("caller cap: got %d want %d", got, callerCap)
	}
	if got := avifDecodeLimit(MaxAVIFPixels + 1); got != MaxAVIFPixels {
		t.Fatalf("clamp: got %d want %d", got, MaxAVIFPixels)
	}
}

func TestAVIFMaxSourcePixelsRejectsLargeStill(t *testing.T) {
	path := testAVIFFixture(t)
	data := readFile(t, path)
	_, err := DecodeAVIFFileWithLimit(data, 1000)
	var ue *errs.UnsupportedError
	if !errors.As(err, &ue) || !strings.Contains(err.Error(), "1000") {
		t.Fatalf("want unsupported naming 1000 px cap, got %v", err)
	}
}

func TestAVIFUnsetMaxSourceUsesCeiling(t *testing.T) {
	path := testAVIFFixture(t)
	data := readFile(t, path)
	ycc, err := DecodeAVIFFileWithLimit(data, 0)
	if err != nil || ycc == nil || ycc.Bounds().Dx() < 100 {
		t.Fatalf("1080p AVIF with no caller cap: %v ycc=%v", err, ycc)
	}
}

func testAVIFFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "test", "data", "av1", "i8_1080.avif")
	if _, err := os.Stat(path); err != nil {
		t.Skip(path, err)
	}
	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
