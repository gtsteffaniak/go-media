package ioat

import (
	"bytes"
	"io"
	"math/rand"
	"testing"
)

type countingReaderAt struct {
	r     io.ReaderAt
	calls int
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	return c.r.ReadAt(p, off)
}

func TestReaderMatchesBytesReader(t *testing.T) {
	data := make([]byte, 200<<10)
	rand.New(rand.NewSource(1)).Read(data)
	rng := rand.New(rand.NewSource(2))
	want := bytes.NewReader(data)
	got := New(bytes.NewReader(data), int64(len(data)))
	for i := 0; i < 5000; i++ {
		switch rng.Intn(3) {
		case 0:
			off := int64(rng.Intn(len(data) + 10))
			a, _ := want.Seek(off, io.SeekStart)
			b, _ := got.Seek(off, io.SeekStart)
			if a != b {
				t.Fatalf("seek %d: got %d want %d", off, b, a)
			}
		default:
			n := rng.Intn(1 << uint(rng.Intn(17)))
			pa := make([]byte, n)
			pb := make([]byte, n)
			na, ea := want.Read(pa)
			nb, eb := got.Read(pb)
			// A short read is legal; compare only what both returned.
			m := na
			if nb < m {
				m = nb
			}
			if !bytes.Equal(pa[:m], pb[:m]) {
				t.Fatalf("iter %d: data mismatch", i)
			}
			if nb < na {
				// Resync the reference so both readers stay at the same offset.
				cur, _ := got.Seek(0, io.SeekCurrent)
				want.Seek(cur, io.SeekStart)
			} else if nb > na {
				t.Fatalf("iter %d: read %d bytes, reference %d", i, nb, na)
			}
			_ = ea
			_ = eb
		}
	}
}

func TestReaderEOFAtEnd(t *testing.T) {
	data := make([]byte, 10)
	r := New(bytes.NewReader(data), int64(len(data)))
	r.Seek(0, io.SeekEnd)
	if n, err := r.Read(make([]byte, 4)); n != 0 || err != io.EOF {
		t.Fatalf("n=%d err=%v want 0, EOF", n, err)
	}
	r.Seek(8, io.SeekStart)
	n, err := r.Read(make([]byte, 4))
	if n != 2 || (err != nil && err != io.EOF) {
		t.Fatalf("n=%d err=%v want 2", n, err)
	}
	if n, err := r.Read(make([]byte, 4)); n != 0 || err != io.EOF {
		t.Fatalf("n=%d err=%v want 0, EOF", n, err)
	}
}

func TestReaderBatchesSmallReads(t *testing.T) {
	data := make([]byte, 100<<10)
	c := &countingReaderAt{r: bytes.NewReader(data)}
	r := New(c, int64(len(data)))
	var b [1]byte
	for i := 0; i < 20000; i++ {
		if _, err := r.Read(b[:]); err != nil {
			t.Fatal(err)
		}
	}
	if c.calls > 2 {
		t.Fatalf("20000 one-byte reads made %d underlying calls, want <=2", c.calls)
	}
}

func TestReaderLargeReadBypassesWindow(t *testing.T) {
	data := make([]byte, 100<<10)
	c := &countingReaderAt{r: bytes.NewReader(data)}
	r := New(c, int64(len(data)))
	p := make([]byte, 64<<10)
	if n, err := r.Read(p); err != nil || n != len(p) {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if c.calls != 1 {
		t.Fatalf("calls=%d want 1", c.calls)
	}
}
