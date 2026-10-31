package sniff

import (
	"bytes"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// ftypBrands reads major and compatible brands from an ISO BMFF ftyp at offset 0.
func ftypBrands(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (major string, compat []string, ok bool) {
	if size < 16 {
		return "", nil, false
	}
	buf := make([]byte, 256)
	n, _ := ioat.ReadRange(r, size, 0, buf)
	buf = buf[:n]
	if len(buf) < 16 || !bytes.Equal(buf[4:8], []byte("ftyp")) {
		return "", nil, false
	}
	boxSize := int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
	if boxSize < 16 || boxSize > len(buf) {
		if boxSize > int(size) {
			boxSize = int(size)
		}
		if boxSize > len(buf) {
			extra := make([]byte, boxSize)
			m, _ := ioat.ReadRange(r, size, 0, extra)
			buf = extra[:m]
		}
	}
	if len(buf) < 16 {
		return "", nil, false
	}
	major = string(buf[8:12])
	for off := 16; off+4 <= len(buf); off += 4 {
		compat = append(compat, string(buf[off:off+4]))
	}
	return major, compat, true
}

func hasBrand(major string, compat []string, want ...string) bool {
	for _, w := range want {
		if major == w {
			return true
		}
		for _, c := range compat {
			if c == w {
				return true
			}
		}
	}
	return false
}

// sniffISOBrand maps ftyp brands to a format ID. ok is false when the file should
// stay on the generic MP4 path (video/quicktime style brands).
func sniffISOBrand(major string, compat []string) (format.ID, bool) {
	if hasBrand(major, compat, "avif", "avis") {
		return format.AVIF, true
	}
	if hasBrand(major, compat, "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1") {
		if major == "mif1" || major == "msf1" || hasBrand(major, compat, "mif1", "msf1") {
			return format.HEIF, true
		}
		return format.HEIC, true
	}
	if hasBrand(major, compat, "crx ") {
		return format.CR3, true
	}
	return "", false
}
