package heifthumb

import (
	"encoding/binary"
	"io"
	"testing"
)

func testBox(typ string, payload []byte) []byte {
	b := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(b[0:4], uint32(len(b)))
	copy(b[4:8], typ)
	copy(b[8:], payload)
	return b
}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestWellFormedBoxLength(t *testing.T) {
	ftyp := testBox("ftyp", []byte("heicmif1"))
	meta := testBox("meta", make([]byte, 40))
	mdat := testBox("mdat", make([]byte, 100))
	sefd := testBox("sefd", make([]byte, 20))
	valid := append(append(append(append([]byte{}, ftyp...), mdat...), meta...), sefd...)

	tests := []struct {
		name string
		data []byte
		want int64
	}{
		{"clean file", valid, int64(len(valid))},
		{"samsung trailer after last box", append(append([]byte{}, valid...), []byte("/>\n<xmp>junk")...), int64(len(valid))},
		{"oversized box stops the walk", append(append(append([]byte{}, ftyp...), mdat...), []byte{0x3c, 0x00, 0x00, 0x00, 'x', 'y', 'z', 'w', 1, 2, 3}...), int64(len(ftyp) + len(mdat))},
		{"no parseable box keeps full size", []byte("not a box at all, just bytes"), 28},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wellFormedBoxLength(bytesReaderAt(tt.data), int64(len(tt.data)))
			if got != tt.want {
				t.Fatalf("wellFormedBoxLength() = %d, want %d", got, tt.want)
			}
		})
	}
}
