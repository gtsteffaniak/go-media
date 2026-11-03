package heifthumb

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// countingReaderAt records how many bytes callers read from the file.
type countingReaderAt struct {
	r     *bytes.Reader
	bytes int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.bytes += int64(len(p))
	return c.r.ReadAt(p, off)
}

func readAVIFFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "data", "avif", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPrimaryTransformIrotImir(t *testing.T) {
	b := readAVIFFixture(t, "irot1_imir0.avif")
	r := &countingReaderAt{r: bytes.NewReader(b)}
	m, err := ReadMeta(r, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if m.box == nil {
		t.Fatal("small meta box should be buffered")
	}
	got, ok := m.PrimaryTransform(r, int64(len(b)))
	want := Transform{Rotation: 90, Mirror: true, MirrorAxis: 0}
	if !ok || got != want {
		t.Fatalf("PrimaryTransform = %+v, %v; want %+v", got, ok, want)
	}
	// Reads through the shared view must not hit the file inside meta.
	before := r.bytes
	ra := m.ReaderAt(r)
	buf := make([]byte, m.end-m.start)
	if _, err := ra.ReadAt(buf, m.start); err != nil {
		t.Fatal(err)
	}
	if r.bytes != before {
		t.Fatalf("meta reads went to the file: %d bytes", r.bytes-before)
	}
	if !bytes.Equal(buf, b[m.start:m.end]) {
		t.Fatal("shared meta bytes differ from file")
	}
	// Oversized-meta fallback (box not buffered) must parse the same transform.
	big := *m
	big.box = nil
	got, ok = big.PrimaryTransform(r, int64(len(b)))
	if !ok || got != want {
		t.Fatalf("unbuffered PrimaryTransform = %+v, %v; want %+v", got, ok, want)
	}
}
