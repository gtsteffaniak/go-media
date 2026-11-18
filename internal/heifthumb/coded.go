package heifthumb

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"

	"github.com/gen2brain/h265/heic"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// CodedPrimaryYCbCr decodes the primary HEVC-coded image when no embedded JPEG
// thumbnail exists.
func CodedPrimaryYCbCr(r io.ReaderAt, size int64) (*h264.ThumbYCbCr, error) {
	if size <= 0 {
		return nil, errs.Corrupt
	}
	rc := io.NewSectionReader(r, 0, wellFormedBoxLength(r, size))
	img, ci, err := heic.DecodeColor(rc, heic.Options{ToYCbCr: true})
	if err != nil {
		return nil, errs.Unsup("thumbnail", "heif coded image: "+err.Error())
	}
	switch v := img.(type) {
	case *image.YCbCr:
		return &h264.ThumbYCbCr{YCbCr: v, CS: colorFromCI(ci), RNG: rangeFromCI(ci)}, nil
	case *image.Gray:
		ycc := image.NewYCbCr(v.Rect, image.YCbCrSubsampleRatio420)
		w, h := v.Rect.Dx(), v.Rect.Dy()
		for y := 0; y < h; y++ {
			copy(ycc.Y[y*ycc.YStride:y*ycc.YStride+w], v.Pix[y*v.Stride:y*v.Stride+w])
		}
		for i := range ycc.Cb {
			ycc.Cb[i] = 128
			ycc.Cr[i] = 128
		}
		return &h264.ThumbYCbCr{YCbCr: ycc, CS: colorFromCI(ci), RNG: rangeFromCI(ci)}, nil
	default:
		return nil, errs.Unsup("thumbnail", "heif: coded image type "+typeName(img))
	}
}

// wellFormedBoxLength returns the end of the leading run of well-formed top-level
// ISOBMFF boxes. Some writers append bytes that are not a box after the last
// one: Samsung adds a sefd box and then an XML-like trailer whose first four
// bytes read as a huge box size. The HEVC decoder rejects the whole file for
// that, so it is given only the well-formed part. If no box parses, the full
// size is returned and the decoder reports the error as before.
func wellFormedBoxLength(r io.ReaderAt, size int64) int64 {
	var end int64
	var hdr [16]byte
	for end+8 <= size {
		n := int64(len(hdr))
		if rem := size - end; rem < n {
			n = rem
		}
		if got, _ := r.ReadAt(hdr[:n], end); got < 8 {
			break
		}
		boxSize := int64(binary.BigEndian.Uint32(hdr[0:4]))
		headerLen := int64(8)
		switch boxSize {
		case 0: // box runs to the end of the file
			boxSize = size - end
		case 1: // 64-bit largesize
			if n < 16 {
				return boxLengthOrSize(end, size)
			}
			boxSize = int64(binary.BigEndian.Uint64(hdr[8:16]))
			headerLen = 16
		}
		// Unknown box types are skipped, as ISO/IEC 14496-12 requires. The
		// size checks alone reject trailers: their first bytes read as a
		// size far beyond the file.
		if boxSize < headerLen || boxSize > size-end {
			break
		}
		end += boxSize
	}
	return boxLengthOrSize(end, size)
}

// boxLengthOrSize returns end when any box parsed, otherwise the full size.
func boxLengthOrSize(end, size int64) int64 {
	if end == 0 {
		return size
	}
	return end
}

func colorFromCI(ci heic.ColorInfo) yuv.ColorSpace {
	if ci.Matrix != 0 && ci.Matrix != 2 {
		return yuv.ColorSpaceFromMatrixCoefficients(uint(ci.Matrix))
	}
	return yuv.BT601
}

func rangeFromCI(ci heic.ColorInfo) yuv.Range {
	if ci.FullRange {
		return yuv.FullRange
	}
	return yuv.LimitedRange
}

func typeName(img image.Image) string {
	return fmt.Sprintf("%T", img)
}
