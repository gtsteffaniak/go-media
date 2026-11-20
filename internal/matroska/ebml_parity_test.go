package matroska

// Parity tests between decodeEBML and the ebml-go Unmarshal it replaces. They
// exist only to show the two decoders agree and are removed together with the
// ebml-go dependency.

import (
	"bytes"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/at-wat/ebml-go"
)

// legacyDecode is the decode step readHead used before decodeEBML.
func legacyDecode(buf []byte, v any) error {
	if err := ebml.Unmarshal(bytes.NewReader(buf), v); err != nil && err != ebml.ErrReadStopped {
		return err
	}
	return nil
}

// legacyReadHead is readHead with the ebml-go decode step.
func legacyReadHead(r io.ReaderAt, size int64) (*segmentHead, error) {
	var head segmentHead
	segStart, segEnd, found, err := findSegment(r, size)
	if err != nil {
		return nil, err
	}
	if !found {
		return &head, nil
	}
	var gotInfo, gotTracks bool
	off := segStart
	for off < segEnd {
		id, hdrLen, dataSize, err := readElementHeader(r, off, segEnd)
		if err != nil || dataSize < 0 {
			break
		}
		elemEnd := min(off+hdrLen+dataSize, segEnd)
		switch id {
		case idInfo:
			if !gotInfo {
				var el infoElement
				if err := legacyDecodeAt(r, off, elemEnd, &el); err != nil {
					return nil, err
				}
				head.Segment.Info = el.Info
				gotInfo = true
			}
		case idTracks:
			if !gotTracks {
				var el tracksElement
				if err := legacyDecodeAt(r, off, elemEnd, &el); err != nil {
					return nil, err
				}
				head.Segment.Tracks = el.Tracks
				gotTracks = true
			}
		}
		if gotInfo && gotTracks {
			break
		}
		off = elemEnd
	}
	return &head, nil
}

// legacyDecodeAt is decodeElement with the ebml-go decode step.
func legacyDecodeAt(r io.ReaderAt, off, end int64, v any) error {
	n := end - off
	if n <= 0 || n > maxHeaderElement {
		return errCorruptForTest
	}
	buf := make([]byte, n)
	if m, err := r.ReadAt(buf, off); int64(m) != n {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	return legacyDecode(buf, v)
}

// errCorruptForTest stands in for the size-cap error in legacyDecodeAt.
var errCorruptForTest = io.ErrShortBuffer

// checkBufferParity decodes buf as an Info element and as a Tracks element with
// both decoders and fails when they disagree.
func checkBufferParity(t testing.TB, name string, buf []byte) {
	t.Helper()
	var oldInfo, newInfo infoElement
	oe, ne := legacyDecode(buf, &oldInfo), decodeEBML(buf, &newInfo)
	if (oe == nil) != (ne == nil) {
		t.Fatalf("%s: info error mismatch: ebml-go %v, new %v (% x)", name, oe, ne, head64(buf))
	}
	if oe == nil && !bitEqual(oldInfo, newInfo) {
		t.Fatalf("%s: info mismatch:\nebml-go %+v\nnew     %+v", name, oldInfo, newInfo)
	}
	var oldTracks, newTracks tracksElement
	oe, ne = legacyDecode(buf, &oldTracks), decodeEBML(buf, &newTracks)
	if (oe == nil) != (ne == nil) {
		t.Fatalf("%s: tracks error mismatch: ebml-go %v, new %v (% x)", name, oe, ne, head64(buf))
	}
	if oe == nil && !bitEqual(oldTracks, newTracks) {
		t.Fatalf("%s: tracks mismatch:\nebml-go %+v\nnew     %+v", name, oldTracks, newTracks)
	}
}

// bitEqual is reflect.DeepEqual except that floats compare by their bits, so
// two NaN payloads read from the same bytes are equal.
func bitEqual(a, b any) bool {
	return bitEqualValue(reflect.ValueOf(a), reflect.ValueOf(b))
}

// bitEqualValue compares two values of the same type field by field.
func bitEqualValue(a, b reflect.Value) bool {
	if a.Kind() != b.Kind() || a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Float32, reflect.Float64:
		return math.Float64bits(a.Float()) == math.Float64bits(b.Float())
	case reflect.Ptr:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return bitEqualValue(a.Elem(), b.Elem())
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !bitEqualValue(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !bitEqualValue(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a.Interface(), b.Interface())
	}
}

// head64 returns at most the first 64 bytes of b for failure messages.
func head64(b []byte) []byte { return b[:min(len(b), 64)] }

// parityFiles returns the Matroska files in the repository, its fuzz corpora,
// and any directories listed in GOMEDIA_PARITY_DIRS (colon separated).
func parityFiles(t testing.TB) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	roots := []string{"../.."}
	if extra := os.Getenv("GOMEDIA_PARITY_DIRS"); extra != "" {
		roots = append(roots, strings.Split(extra, ":")...)
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() > 512<<20 {
				return nil
			}
			if strings.Contains(p, string(filepath.Separator)+"fuzz"+string(filepath.Separator)) {
				for i, c := range fuzzCorpusBytes(p) {
					out[p+"#"+strconv.Itoa(i)] = c
				}
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return nil
			}
			var magic [4]byte
			_, rerr := io.ReadFull(f, magic[:])
			f.Close()
			if rerr != nil || magic != [4]byte{0x1A, 0x45, 0xDF, 0xA3} {
				return nil
			}
			if b, err := os.ReadFile(p); err == nil {
				out[p] = b
			}
			return nil
		})
	}
	return out
}

// fuzzCorpusBytes returns the []byte values in a "go test fuzz v1" corpus file.
func fuzzCorpusBytes(p string) [][]byte {
	b, err := os.ReadFile(p)
	if err != nil || !bytes.HasPrefix(b, []byte("go test fuzz v1\n")) {
		return nil
	}
	var out [][]byte
	for _, line := range strings.Split(string(b), "\n")[1:] {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[]byte(") || !strings.HasSuffix(line, ")") {
			continue
		}
		s, err := strconv.Unquote(line[len("[]byte(") : len(line)-1])
		if err == nil {
			out = append(out, []byte(s))
		}
	}
	return out
}

// headerBuffers returns the raw Info and Tracks elements of a Matroska file, as
// readHead hands them to the decoder.
func headerBuffers(data []byte) [][]byte {
	r := bytes.NewReader(data)
	segStart, segEnd, found, err := findSegment(r, int64(len(data)))
	if err != nil || !found {
		return nil
	}
	var out [][]byte
	off := segStart
	for off < segEnd {
		id, hdrLen, dataSize, err := readElementHeader(r, off, segEnd)
		if err != nil || dataSize < 0 {
			break
		}
		end := min(off+hdrLen+dataSize, segEnd)
		if (id == idInfo || id == idTracks) && end-off <= maxHeaderElement {
			out = append(out, data[off:end])
		}
		off = end
	}
	return out
}

// TestEBMLParityFiles runs readHead with both decoders over every Matroska file
// and fuzz corpus entry and requires identical heads, then mutates each Info and
// Tracks element byte by byte and requires the decoders to agree on every
// mutant.
func TestEBMLParityFiles(t *testing.T) {
	files := parityFiles(t)
	if len(files) == 0 {
		t.Fatal("no Matroska files found")
	}
	var nFiles, nOK, nBufs, nMutants int
	for name, data := range files {
		nFiles++
		r := bytes.NewReader(data)
		oldHead, oe := legacyReadHead(r, int64(len(data)))
		newHead, ne := readHead(r, int64(len(data)))
		if (oe == nil) != (ne == nil) {
			t.Fatalf("%s: readHead error mismatch: ebml-go %v, new %v", name, oe, ne)
		}
		if oe == nil {
			nOK++
			if !bitEqual(oldHead, newHead) {
				t.Fatalf("%s: readHead mismatch:\nebml-go %+v\nnew     %+v", name, oldHead, newHead)
			}
		}
		for _, buf := range headerBuffers(data) {
			nBufs++
			checkBufferParity(t, name, buf)
			if testing.Short() {
				continue
			}
			mut := make([]byte, len(buf))
			for i := range buf {
				for _, v := range []byte{0x00, 0xFF, buf[i] ^ 0x80, buf[i] + 1, buf[i] - 1, 0x01, 0x81} {
					copy(mut, buf)
					mut[i] = v
					checkBufferParity(t, name, mut)
					nMutants++
				}
				nMutants++
				checkBufferParity(t, name, buf[:i])
			}
		}
	}
	t.Logf("parity: %d files (%d decoded without error), %d Info/Tracks elements, %d mutants", nFiles, nOK, nBufs, nMutants)
}

// TestEBMLParityCases checks every handmade ebmlCase against ebml-go: the error
// result must match the case's expectation and the decoded values must match.
func TestEBMLParityCases(t *testing.T) {
	for _, tc := range ebmlCases() {
		var info infoElement
		if err := legacyDecode(tc.buf, &info); (err != nil) != tc.wantErr {
			t.Errorf("%s: ebml-go error %v, case expects error %v", tc.name, err, tc.wantErr)
		}
		checkBufferParity(t, tc.name, tc.buf)
	}
}

// FuzzEBMLParity feeds arbitrary bytes to both decoders, as an Info element and
// as a Tracks element, and requires the same error result and the same values.
func FuzzEBMLParity(f *testing.F) {
	for _, data := range parityFiles(f) {
		for _, buf := range headerBuffers(data) {
			f.Add(buf)
		}
	}
	for _, tc := range ebmlCases() {
		f.Add(tc.buf)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		checkBufferParity(t, "fuzz", data)
	})
}

// BenchmarkReadHead compares readHead with the ebml-go decoder and with
// decodeEBML on every Matroska fixture in the repository.
func BenchmarkReadHead(b *testing.B) {
	files := parityFiles(b)
	var all [][]byte
	for name, data := range files {
		if !strings.Contains(name, "#") {
			all = append(all, data)
		}
	}
	b.Run("ebml-go", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, d := range all {
				_, _ = legacyReadHead(bytes.NewReader(d), int64(len(d)))
			}
		}
	})
	b.Run("decodeEBML", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, d := range all {
				_, _ = readHead(bytes.NewReader(d), int64(len(d)))
			}
		}
	})
}
