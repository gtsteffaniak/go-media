package sniff

import (
	"bytes"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func sniffCameraRaw(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (format.ID, bool) {
	if size < 16 {
		return "", false
	}
	head := make([]byte, 512)
	n, _ := ioat.ReadRange(r, size, 0, head)
	head = head[:n]

	if bytes.HasPrefix(head, []byte("FUJIFILMCCD-RAW")) {
		return format.RAF, true
	}
	if len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) {
		major, compat, ok := ftypBrands(r, size)
		if ok {
			if id, yes := sniffISOBrand(major, compat); yes && id == format.CR3 {
				return format.CR3, true
			}
		}
	}
	if len(head) < 8 {
		return "", false
	}
	le := bytes.Equal(head[:2], []byte("II"))
	be := bytes.Equal(head[:2], []byte("MM"))
	if !le && !be {
		return "", false
	}
	if len(head) >= 10 && bytes.Equal(head[8:10], []byte("CR")) {
		return format.CR2, true
	}
	if bytes.Contains(head, []byte("ORF")) || bytes.HasPrefix(head, []byte("IIRO")) || bytes.HasPrefix(head, []byte("MMOR")) {
		return format.ORF, true
	}
	if bytes.HasPrefix(head, []byte("PEF")) || bytes.Contains(head, []byte("PENTAX")) {
		return format.PEF, true
	}
	// Panasonic RW2 / RWl: "II" + bytes 8-10 == "RW" or signature at 8
	if len(head) >= 11 && le && head[8] == 'R' && head[9] == 'W' {
		return format.RW2, true
	}
	if bytes.Contains(head, []byte("NIKON")) {
		return format.NEF, true
	}
	if bytes.Contains(head, []byte("SONY")) || bytes.Contains(head, []byte("ARW")) {
		return format.ARW, true
	}
	if bytes.Contains(head, []byte("DNG")) {
		return format.DNG, true
	}
	// Generic little/big TIFF without another marker: treat as DNG-style TIFF RAW.
	return format.DNG, true
}
