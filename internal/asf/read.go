package asf

import (
	"encoding/binary"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

const (
	maxHeaderScan = 4 << 20
	maxPacketSize = 1 << 20
)

type reader struct {
	r    io.ReaderAt
	size int64
}

func newReader(r io.ReaderAt, size int64) reader {
	return reader{r: r, size: size}
}

func (rd reader) readAt(off int64, n int) ([]byte, error) {
	if off < 0 || off >= rd.size {
		return nil, errs.Corrupt
	}
	if int64(n) > rd.size-off {
		n = int(rd.size - off)
	}
	buf := make([]byte, n)
	got, err := ioat.ReadRange(rd.r, rd.size, off, buf)
	if err != nil {
		return nil, err
	}
	return buf[:got], nil
}

func leU64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

func leU32(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b)
}

func leU16(b []byte) uint16 {
	return binary.LittleEndian.Uint16(b)
}

// readUTF16LE reads a UTF-16LE string with 16-bit length prefix (in bytes).
func readUTF16LE(b []byte) (string, int, error) {
	if len(b) < 2 {
		return "", 0, errs.Corrupt
	}
	n := int(leU16(b))
	if n < 2 || n&1 != 0 || len(b) < 2+n {
		return "", 0, errs.Corrupt
	}
	u := b[2 : 2+n]
	// Trim UTF-16 null terminator.
	if len(u) >= 2 && u[len(u)-2] == 0 && u[len(u)-1] == 0 {
		u = u[:len(u)-2]
	}
	runes := make([]rune, 0, len(u)/2)
	for i := 0; i+1 < len(u); i += 2 {
		runes = append(runes, rune(leU16(u[i:])))
	}
	return string(runes), 2 + n, nil
}
