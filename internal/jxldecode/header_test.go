package jxldecode_test

import (
	"bytes"
	"encoding/binary"
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

// capOpt is an example caller policy (what FileBrowser might choose).
var capOpt = jxldecode.Options{MaxPixels: 4096 * 4096}

func TestDecodeRejectsOversizedFromPrefix(t *testing.T) {
	const size = 200 << 20 // 200 MiB claimed file
	boxes := [][2]int{{0, 0}, {256, 256}, {8192, 8192}, {16384, 16384}}
	cases := []struct {
		opt    jxldecode.Options
		dims   [][2]int
		source string
	}{
		// With a caller cap, every box rejects the same images.
		{capOpt, [][2]int{{6000, 4000}, {4097, 4096}, {12000, 9000}, {20000, 15000}}, "caller MaxPixels cap of 16777216 px"},
		// Without one, only the decoder's own 16384x16384 ceiling applies.
		{jxldecode.Options{}, [][2]int{{20000, 15000}, {16385, 16384}, {262144, 1100}}, "decoder limit of 268435456 px"},
	}
	for _, c := range cases {
		for _, d := range c.dims {
			want := fmt.Sprintf("%dx%d", d[0], d[1])
			for _, box := range boxes {
				pr := &paddedReader{head: syntheticHeader(d[0], d[1]), size: size}
				_, err := jxldecode.DecodeWithOptions(pr, size, box[0], box[1], c.opt)
				if !errors.Is(err, errs.Unsupported) {
					t.Fatalf("%s box %v: want errs.Unsupported, got %v", want, box, err)
				}
				if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), c.source) {
					t.Fatalf("%s box %v: reason lacks dimensions or limit: %v", want, box, err)
				}
				if got := pr.max.Load(); got > 4096 {
					t.Fatalf("%s box %v: read %d bytes, want <= 4096 (prefix only)", want, box, got)
				}
			}
		}
	}
}

func TestDecodeSameLimitForEveryBox(t *testing.T) {
	// Images at or under the effective limit must pass the probe for small
	// and large boxes alike. The probe stage is asserted directly on the same
	// prefix DecodeWithOptions uses. The decode stage is then asserted by
	// checking that the reader was consumed to the end, which only happens
	// after the probe accepted the header. The decode stage fails because the
	// synthetic stream has no frame data.
	cases := []struct {
		opt  jxldecode.Options
		dims [][2]int
	}{
		{capOpt, [][2]int{{4096, 4096}, {4000, 3000}, {8192, 2048}}},
		{jxldecode.Options{}, [][2]int{{6000, 4000}, {12000, 9000}, {16384, 16384}}},
		// A cap above the decoder ceiling is clamped, not an error.
		{jxldecode.Options{MaxPixels: 1 << 40}, [][2]int{{16384, 16384}}},
	}
	const size = 2 << 20
	for _, c := range cases {
		for _, d := range c.dims {
			for _, box := range [][2]int{{0, 0}, {256, 256}, {8192, 8192}, {16384, 16384}} {
				head := syntheticHeader(d[0], d[1])

				// Probe stage: DecodeConfig must accept the header from a
				// prefix of the same size the decoder probes.
				prefix := make([]byte, jxldecode.MaxProbeBytes)
				copy(prefix, head)
				cfg, err := jxl.DecodeConfig(bytes.NewReader(prefix))
				if err != nil || cfg.Width != d[0] || cfg.Height != d[1] {
					t.Fatalf("%+v %v: probe stage DecodeConfig = %+v, %v; want %dx%d", c.opt, d, cfg, err, d[0], d[1])
				}

				// Decode stage: the probe accepted it, so the file is read
				// to the end and jxl.Decode fails on the missing frame data.
				pr := &paddedReader{head: head, size: size}
				_, err = jxldecode.DecodeWithOptions(pr, pr.size, box[0], box[1], c.opt)
				if err == nil || errors.Is(err, errs.Unsupported) {
					t.Fatalf("%+v %v box %v: want non-Unsupported error past the probe, got %v", c.opt, d, box, err)
				}
				if got := pr.max.Load(); got != size {
					t.Fatalf("%+v %v box %v: decode stage not reached: read %d of %d bytes (%v)", c.opt, d, box, got, size, err)
				}
			}
		}
	}
}

// box encodes a 32-bit-size ISO BMFF box.
func box(typ string, payload []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(8+len(payload)))
	b = append(b, typ...)
	return append(b, payload...)
}

func TestDecodeRejectsOversizedBehindLargeBox(t *testing.T) {
	// A multi-MiB leading box (ICC, Exif, XMP) must not force a full read.
	// The codestream is located from box headers alone, and the header is
	// rejected from its first 4 KiB.
	cs := syntheticHeader(20000, 15000)
	const freeLen = 2 << 20
	var c []byte
	c = append(c, box("JXL ", []byte{0x0D, 0x0A, 0x87, 0x0A})...)
	c = append(c, box("ftyp", []byte("jxl \x00\x00\x00\x00jxl "))...)
	c = append(c, box("free", make([]byte, freeLen))...)
	jxlcOff := int64(len(c))
	c = append(c, box("jxlc", cs)...)
	const size = 64 << 20
	pr := &paddedReader{head: c, size: size}
	_, err := jxldecode.DecodeWithOptions(pr, size, 256, 256, capOpt)
	if !errors.Is(err, errs.Unsupported) || !strings.Contains(err.Error(), "20000x15000") {
		t.Fatalf("want errs.Unsupported naming 20000x15000, got %v", err)
	}
	// Box headers, then the codestream probe prefix (jxlc header is 8 bytes).
	if got, want := pr.max.Load(), jxlcOff+8+4096; got > want {
		t.Fatalf("read up to byte %d, want <= %d (box headers and probe prefix only)", got, want)
	}
}

func TestDecodeHugeLargeSizeBoxTerminates(t *testing.T) {
	// A largesize near 2^64 must not wrap the box walk to an earlier offset
	// (which previously looped forever). The decode must return an error.
	var c []byte
	c = append(c, box("JXL ", []byte{0x0D, 0x0A, 0x87, 0x0A})...)
	c = binary.BigEndian.AppendUint32(c, 1)
	c = append(c, "free"...)
	c = binary.BigEndian.AppendUint64(c, 0xFFFFFFFFFFFFFFF0)
	c = append(c, make([]byte, 64)...)
	const size = 1 << 20
	pr := &paddedReader{head: c, size: size}
	if _, err := jxldecode.DecodeWithOptions(pr, size, 256, 256, capOpt); err == nil {
		t.Fatal("want error for a file with no decodable codestream")
	}
}

func TestDecodeUnsplashCallerCap(t *testing.T) {
	f, err := os.Open(testDataPath(referenceJXL))
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	if _, err := jxldecode.DecodeWithOptions(f, st.Size(), 256, 256, capOpt); err != nil {
		t.Fatalf("within cap: %v", err)
	}
	_, err = jxldecode.DecodeWithOptions(f, st.Size(), 256, 256, jxldecode.Options{MaxPixels: 1000})
	if !errors.Is(err, errs.Unsupported) || !strings.Contains(err.Error(), "caller MaxPixels cap of 1000 px") {
		t.Fatalf("tiny cap: want Unsupported naming the cap, got %v", err)
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
	_, err := jxldecode.DecodeWithOptions(pr, size, 256, 256, capOpt)
	if !errors.Is(err, errs.Unsupported) {
		t.Fatalf("want errs.Unsupported, got %v", err)
	}
	if !strings.Contains(err.Error(), "20000x15000") {
		t.Fatalf("reason lacks dimensions: %v", err)
	}
}
