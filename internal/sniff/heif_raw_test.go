package sniff

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/format"
)

func TestDetectHEICBrand(t *testing.T) {
	ftyp := synthFTYP("heic", "heic", "mif1")
	if got := detect(t, ftyp); got != format.HEIC {
		t.Fatalf("got %v want HEIC", got)
	}
}

func TestDetectAVIFBrand(t *testing.T) {
	ftyp := synthFTYP("avif", "avif")
	if got := detect(t, ftyp); got != format.AVIF {
		t.Fatalf("got %v want AVIF", got)
	}
}

func synthFTYP(major string, compat ...string) []byte {
	body := append([]byte(major), 0, 0, 0)
	for _, c := range compat {
		body = append(body, []byte(c)...)
	}
	out := make([]byte, 8+len(body))
	copy(out[4:8], "ftyp")
	copy(out[8:], body)
	n := len(out)
	out[0] = byte(n >> 24)
	out[1] = byte(n >> 16)
	out[2] = byte(n >> 8)
	out[3] = byte(n)
	return out
}

func TestDetectCR2(t *testing.T) {
	var cr2 []byte
	cr2 = append(cr2, 'I', 'I', 0x2a, 0, 0, 0, 0, 0, 'C', 'R')
	cr2 = append(cr2, 0xff, 0xd8, 0xff, 0xd9)
	if got := detect(t, cr2); got != format.CR2 {
		t.Fatalf("got %v want CR2", got)
	}
}

func TestDetectTIFFDNGFallback(t *testing.T) {
	tiff := []byte{'I', 'I', 0x2a, 0, 8, 0, 0, 0, 0, 0, 0, 0}
	if got := detect(t, tiff); got != format.DNG {
		t.Fatalf("got %v want DNG", got)
	}
}
