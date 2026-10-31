package heifthumb

import (
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
	rc := io.NewSectionReader(r, 0, size)
	img, ci, err := heic.DecodeColor(rc, heic.Options{ToYCbCr: true})
	if err != nil {
		return nil, errs.Unsup("thumbnail", "heif coded image: "+err.Error())
	}
	switch v := img.(type) {
	case *image.YCbCr:
		return &h264.ThumbYCbCr{YCbCr: v, CS: colorFromCI(ci), RNG: rangeFromCI(ci)}, nil
	case *image.Gray:
		ycc := image.NewYCbCr(v.Rect, image.YCbCrSubsampleRatio420)
		for i := range ycc.Y {
			ycc.Y[i] = v.Pix[i]
			ycc.Cb[i/4] = 128
			ycc.Cr[i/4] = 128
		}
		return &h264.ThumbYCbCr{YCbCr: ycc, CS: colorFromCI(ci), RNG: rangeFromCI(ci)}, nil
	default:
		return nil, errs.Unsup("thumbnail", "heif: coded image type "+typeName(img))
	}
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
