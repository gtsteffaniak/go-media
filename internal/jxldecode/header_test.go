package jxldecode_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gen2brain/jxl"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/jxldecode"
)

// bitWriter writes LSB-first bits as JPEG XL codestreams do.
type bitWriter struct {
	buf  []byte
	nbit uint
}

func (w *bitWriter) put(v uint64, n uint) {
	for i := uint(0); i < n; i++ {
		if w.nbit%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if v>>i&1 == 1 {
			w.buf[len(w.buf)-1] |= 1 << (w.nbit % 8)
		}
		w.nbit++
	}
}

// syntheticHeader builds a bare codestream: signature, SizeHeader with
// explicit 30-bit dimensions, and all_default ImageMetadata. It carries no
// frame data, so only header-level parsing can succeed.
func syntheticHeader(w, h int) []byte {
	bw := &bitWriter{buf: []byte{0xFF, 0x0A}, nbit: 16}
	bw.put(0, 1)            // small = false
	bw.put(3, 2)            // height selector: 1 + u(30)
	bw.put(uint64(h-1), 30) // height
	bw.put(0, 3)            // ratio = 0: explicit width
	bw.put(3, 2)            // width selector: 1 + u(30)
	bw.put(uint64(w-1), 30) // width
	bw.put(1, 1)            // ImageMetadata all_default
	// Zero padding stands in for the (absent) frame header so the bit
	// reader has bytes past the image header.
	return append(bw.buf, make([]byte, 64)...)
}

// paddedReader serves header bytes followed by zeros up to size and records
// the furthest byte read.
type paddedReader struct {
	head []byte
	size int64
	max  atomic.Int64
}

func (p *paddedReader) ReadAt(b []byte, off int64) (int, error) {
	if off >= p.size {
		return 0, io.EOF
	}
	n := len(b)
	if rem := p.size - off; int64(n) > rem {
		n = int(rem)
	}
	for i := 0; i < n; i++ {
		o := off + int64(i)
		if o < int64(len(p.head)) {
			b[i] = p.head[o]
		} else {
			b[i] = 0
		}
	}
	if end := off + int64(n); end > p.max.Load() {
		p.max.Store(end)
	}
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func TestSyntheticHeaderParses(t *testing.T) {
	cfg, err := jxl.DecodeConfig(strings.NewReader(string(syntheticHeader(12000, 9000))))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 12000 || cfg.Height != 9000 {
		t.Fatalf("got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestDecodeRejectsOversizedFromPrefix(t *testing.T) {
	const size = 200 << 20 // 200 MiB claimed file
	// Every case exceeds 4096x4096 by area. The same budget applies to
	// every box, so thumbnails and full raster are rejected identically.
	dims := [][2]int{{6000, 4000}, {4097, 4096}, {12000, 9000}, {20000, 15000}, {262144, 1100}}
	boxes := [][2]int{{0, 0}, {256, 256}, {8192, 8192}, {16384, 16384}}
	for _, d := range dims {
		want := fmt.Sprintf("%dx%d", d[0], d[1])
		for _, box := range boxes {
			pr := &paddedReader{head: syntheticHeader(d[0], d[1]), size: size}
			_, err := jxldecode.Decode(pr, size, box[0], box[1])
			if !errors.Is(err, errs.Unsupported) {
				t.Fatalf("%s box %v: want errs.Unsupported, got %v", want, box, err)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s box %v: reason lacks dimensions: %v", want, box, err)
			}
			if got := pr.max.Load(); got > 4096 {
				t.Fatalf("%s box %v: read %d bytes, want <= 4096 (prefix only)", want, box, got)
			}
		}
	}
}

func TestDecodeSameBudgetForEveryBox(t *testing.T) {
	// Exactly 4096x4096 and smaller images are under the constant budget,
	// so they pass the probe for small and large boxes alike and fail only
	// later because the synthetic stream has no frame data.
	for _, d := range [][2]int{{4096, 4096}, {4000, 3000}, {8192, 2048}} {
		for _, box := range [][2]int{{0, 0}, {256, 256}, {8192, 8192}, {16384, 16384}} {
			head := syntheticHeader(d[0], d[1])
			pr := &paddedReader{head: head, size: int64(len(head))}
			_, err := jxldecode.Decode(pr, pr.size, box[0], box[1])
			if err == nil || errors.Is(err, errs.Unsupported) {
				t.Fatalf("%v box %v: want non-Unsupported error past the probe, got %v", d, box, err)
			}
		}
	}
}

func TestDecodeUnsplashLargeBox(t *testing.T) {
	f, err := os.Open(testDataPath(referenceJXL))
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	img, err := jxldecode.Decode(f, st.Size(), 16384, 16384)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Empty() {
		t.Fatal("empty image")
	}
}

func TestDecodeRejectsOversizedContainer(t *testing.T) {
	cs := syntheticHeader(20000, 15000)
	var c []byte
	c = append(c, 0, 0, 0, 12, 'J', 'X', 'L', ' ', 0x0D, 0x0A, 0x87, 0x0A)
	c = append(c, 0, 0, 0, 20, 'f', 't', 'y', 'p', 'j', 'x', 'l', ' ', 0, 0, 0, 0, 'j', 'x', 'l', ' ')
	n := 8 + len(cs)
	c = append(c, byte(n>>24), byte(n>>16), byte(n>>8), byte(n), 'j', 'x', 'l', 'c')
	c = append(c, cs...)
	const size = 64 << 20
	pr := &paddedReader{head: c, size: size}
	_, err := jxldecode.Decode(pr, size, 256, 256)
	if !errors.Is(err, errs.Unsupported) {
		t.Fatalf("want errs.Unsupported, got %v", err)
	}
	if !strings.Contains(err.Error(), "20000x15000") {
		t.Fatalf("reason lacks dimensions: %v", err)
	}
}
