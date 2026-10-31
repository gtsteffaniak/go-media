// Package rawpreview extracts embedded JPEG previews from camera RAW files.
package rawpreview

import (
	"bytes"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/heifthumb"
)

const (
	maxScanBytes = 32 << 20
	maxJPEGBytes = 24 << 20
)

// JPEGPreview returns the largest embedded JPEG suitable for downscaling to maxW×maxH.
func JPEGPreview(r io.ReaderAt, size int64, id format.ID, maxW, maxH int) ([]byte, error) {
	if size <= 0 {
		return nil, errs.Corrupt
	}
	if maxW <= 0 {
		maxW = 320
	}
	if maxH <= 0 {
		maxH = maxW
	}
	if id == format.CR3 {
		if b, err := heifthumb.JPEGPreview(r, size, maxW, maxH); err == nil {
			return b, nil
		}
	}
	cands, err := collectCandidates(r, size, id)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, errs.Unsup("thumbnail", string(id)+": no embedded JPEG preview")
	}
	best := pickBest(cands, maxW, maxH)
	return best.data, nil
}

type jpegCand struct {
	data   []byte
	width  int
	height int
	score  int
}

func collectCandidates(r io.ReaderAt, size int64, id format.ID) ([]jpegCand, error) {
	var out []jpegCand
	if id != format.RAF {
		if cs, err := tiffJPEGCandidates(r, size); err == nil {
			out = append(out, cs...)
		}
	}
	if len(out) == 0 || id == format.RAF || id == format.CR3 {
		scanned, err := scanJPEGCandidates(r, size)
		if err != nil {
			return out, err
		}
		out = append(out, scanned...)
	}
	return dedupeCandidates(out), nil
}

func dedupeCandidates(in []jpegCand) []jpegCand {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]jpegCand, 0, len(in))
	for _, c := range in {
		key := string(c.data[:min(32, len(c.data))]) + "|" + itoa(len(c.data))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}

func pickBest(cands []jpegCand, maxW, maxH int) jpegCand {
	best := cands[0]
	bestScore := fitScore(best.width, best.height, maxW, maxH)
	for _, c := range cands[1:] {
		s := fitScore(c.width, c.height, maxW, maxH)
		if s > bestScore || (s == bestScore && c.width*c.height > best.width*best.height) {
			best = c
			bestScore = s
		}
	}
	return best
}

func fitScore(w, h, maxW, maxH int) int {
	if w <= 0 || h <= 0 {
		return 0
	}
	fw, fh := fitDim(w, h, maxW, maxH)
	return fw * fh
}

func fitDim(w, h, maxW, maxH int) (int, int) {
	if w <= maxW && h <= maxH {
		return w, h
	}
	sx := float64(maxW) / float64(w)
	sy := float64(maxH) / float64(h)
	s := sx
	if sy < s {
		s = sy
	}
	return int(float64(w)*s + 0.5), int(float64(h)*s + 0.5)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ValidateJPEGPrefix reports whether data looks like a baseline/progressive JPEG.
func ValidateJPEGPrefix(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xff && data[1] == 0xd8
}

func readAt(r io.ReaderAt, off, n int64) ([]byte, error) {
	if off < 0 || n <= 0 || off+n > int64(^uint(0)>>1) {
		return nil, errs.Corrupt
	}
	b := make([]byte, n)
	if _, err := r.ReadAt(b, off); err != nil {
		return nil, err
	}
	return b, nil
}

func candidateFromBytes(data []byte) (jpegCand, bool) {
	if !ValidateJPEGPrefix(data) || len(data) > maxJPEGBytes {
		return jpegCand{}, false
	}
	w, h, err := jpegDimensions(data)
	if err != nil || w <= 0 || h <= 0 {
		return jpegCand{}, false
	}
	return jpegCand{data: bytes.Clone(data), width: w, height: h}, true
}
