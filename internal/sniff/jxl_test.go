package sniff_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

func TestDetectJXLContainer(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "zoltan_unsplash.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	id, err := sniff.Detect(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if id != format.JXL {
		t.Fatalf("Detect() = %q, want jxl", id)
	}
}

func TestDetectJXLContainerPartialSignatureNotJXL(t *testing.T) {
	// "JXL " at offset 4 without the full 12-byte signature must not match.
	buf := append([]byte{0, 0, 0, 0}, []byte("JXL ")...)
	buf = append(buf, 0, 0, 0, 0)
	r := bytes.NewReader(buf)
	id, err := sniff.Detect(r, int64(len(buf)))
	if err == nil && id == format.JXL {
		t.Fatalf("Detect() = %q, want not jxl for partial signature", id)
	}
}

func TestDetectJXLCodestream(t *testing.T) {
	// A bare JXL codestream starts with the 0xFF 0x0A signature (no container box).
	buf := []byte{0xFF, 0x0A, 0x00, 0x00, 0x00, 0x00}
	id, err := sniff.Detect(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		t.Fatalf("Detect() error = %v, want jxl", err)
	}
	if id != format.JXL {
		t.Fatalf("Detect() = %q, want jxl", id)
	}
}
