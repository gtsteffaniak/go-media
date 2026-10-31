package avi

import (
	"encoding/binary"
	"io"
)

// chunkHeader is an 8-byte RIFF chunk header.
type chunkHeader struct {
	id  [4]byte
	sz  uint32
	off int64 // absolute offset of id in the file
}

func readChunkHeader(r io.ReaderAt, size, off int64) (chunkHeader, error) {
	if off+8 > size {
		return chunkHeader{}, io.EOF
	}
	var hdr [8]byte
	if _, err := r.ReadAt(hdr[:], off); err != nil {
		return chunkHeader{}, err
	}
	return chunkHeader{
		id:  [4]byte{hdr[0], hdr[1], hdr[2], hdr[3]},
		sz:  binary.LittleEndian.Uint32(hdr[4:8]),
		off: off,
	}, nil
}

// chunkDataEnd returns the byte after chunk payload (including pad).
func chunkDataEnd(h chunkHeader) int64 {
	end := h.off + 8 + int64(h.sz)
	if h.sz%2 == 1 {
		end++
	}
	return end
}

func chunkIDString(id [4]byte) string {
	for _, c := range id {
		if c < 0x20 || c > 0x7e {
			return string(id[:])
		}
	}
	return string(id[:])
}

func listType(r io.ReaderAt, h chunkHeader) ([4]byte, error) {
	var t [4]byte
	if _, err := r.ReadAt(t[:], h.off+8); err != nil {
		return t, err
	}
	return t, nil
}

func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	b := make([]byte, n)
	if _, err := r.ReadAt(b, off); err != nil {
		return nil, err
	}
	return b, nil
}
