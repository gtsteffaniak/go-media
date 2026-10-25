package main

import (
	"io"
	"os"
)

// fileAt holds a media file in memory so go-media can outlive a single os.File handle
// (Matroska readers may continue async reads after the primary call returns).
type fileAt struct {
	data []byte
}

// openFileAt reads path into memory for ReaderAt access.
func openFileAt(path string) (*fileAt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &fileAt{data: data}, nil
}

// ReadAt implements io.ReaderAt.
func (f *fileAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(f.data)) {
		return 0, os.ErrInvalid
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Size returns the byte length.
func (f *fileAt) Size() int64 {
	return int64(len(f.data))
}
