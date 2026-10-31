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
