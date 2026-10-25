// Package ioat supports go-media parsers.

package ioat

import "io"

// Reader wraps io.ReaderAt with a known size and implements io.ReadSeeker for parsers that need it.
type Reader struct {
	R    io.ReaderAt
	Size int64
	off  int64
}

// Read implements io.Reader.
func (r *Reader) Read(p []byte) (int, error) {
	if r.off >= r.Size {
		return 0, io.EOF
	}
	n, err := r.R.ReadAt(p, r.off)
	r.off += int64(n)
	if r.off >= r.Size && err == nil {
		err = io.EOF
	}
	return n, err
}

// Seek implements io.Seeker.
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.off + offset
	case io.SeekEnd:
		abs = r.Size + offset
	default:
		return 0, io.ErrUnexpectedEOF
	}
	if abs < 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if abs > r.Size {
		abs = r.Size
	}
	r.off = abs
	return r.off, nil
}

// ReadAt implements io.ReaderAt.
func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= r.Size {
		return 0, io.EOF
	}
	n, err := r.R.ReadAt(p, off)
	if int64(n)+off >= r.Size {
		if err == nil {
			err = io.EOF
		}
	}
	return n, err
}

// New supports media inspection.
func New(r io.ReaderAt, size int64) *Reader {
	return &Reader{R: r, Size: size}
}

// ReadRange reads up to len(p) bytes starting at off.
func ReadRange(r io.ReaderAt, size int64, off int64, p []byte) (int, error) {
	if off < 0 || off >= size {
		return 0, io.EOF
	}
	max := int64(len(p))
	if off+max > size {
		max = size - off
	}
	return r.ReadAt(p[:max], off)
}

// ReadAll reads the entire range [off, off+n) capped by size.
func ReadAll(r io.ReaderAt, size int64, off int64, n int64) ([]byte, error) {
	if off < 0 || off >= size {
		return nil, io.EOF
	}
	if n <= 0 || off+n > size {
		n = size - off
	}
	buf := make([]byte, n)
	got, err := r.ReadAt(buf, off)
	buf = buf[:got]
	if got < int(n) && err == nil {
		err = io.EOF
	}
	return buf, err
}

// SectionReaderAt exposes a sub-range as ReaderAt.
type SectionReaderAt struct {
	r    io.ReaderAt
	base int64
	size int64
}

// NewSection supports media inspection.
func NewSection(r io.ReaderAt, base, size int64) *SectionReaderAt {
	return &SectionReaderAt{r: r, base: base, size: size}
}

// ReadAt implements io.ReaderAt.
func (s *SectionReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= s.size {
		return 0, io.EOF
	}
	max := int64(len(p))
	if off+max > s.size {
		max = s.size - off
	}
	return s.r.ReadAt(p[:max], s.base+off)
}
