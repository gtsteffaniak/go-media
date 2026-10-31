package rawpreview

import (
	"encoding/binary"
)

// SynthTIFFJPEG builds a little-endian TIFF with one embedded JPEG IFD entry.
func SynthTIFFJPEG(jpeg []byte) []byte {
	// Minimal little-endian TIFF: IFD0 with JPEGInterchangeFormat tags.
	jpegOff := int64(8 + 2 + 12*2 + 4)
	var out []byte
	out = append(out, 'I', 'I', 42, 0)
	out = append(out, u32LE(8)...)
	out = append(out, u16LE(2)...)
	// Tag JPEGInterchangeFormat
	out = append(out, u16LE(tagJPEGInterchangeFormat)...)
	out = append(out, u16LE(4)...) // LONG
	out = append(out, u32LE(1)...)
	out = append(out, u32LE(uint32(jpegOff))...)
	// Tag JPEGInterchangeFormatLength
	out = append(out, u16LE(tagJPEGInterchangeFormatLen)...)
	out = append(out, u16LE(4)...)
	out = append(out, u32LE(1)...)
	out = append(out, u32LE(uint32(len(jpeg)))...)
	out = append(out, u32LE(0)...) // next IFD
	out = append(out, jpeg...)
	return out
}

func u16LE(v uint16) []byte {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	return b[:]
}

func u32LE(v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return b[:]
}
