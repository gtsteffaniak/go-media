package av1decode

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/gen2brain/gav1d/av1"
	"github.com/gen2brain/gav1d/avif"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// DecodeAVIFFile decodes the primary AVIF item (still or grid) with no caller
// pixel cap beyond MaxAVIFPixels.
func DecodeAVIFFile(data []byte) (*h264.ThumbYCbCr, error) {
	return DecodeAVIFFileWithLimit(data, 0)
}

// DecodeAVIFFileWithLimit decodes the primary AVIF item (still or grid) from a
// full file buffer with gav1d, which handles grids itself. Film grain is
// applied (avif.Options has no option to skip it).
//
// The clap, irot and imir properties are applied (AutoRotate), matching
// ffmpeg, whose AVIF demuxer exports them as crop and display-matrix side data
// that the ffmpeg CLI applies by default (-autorotate). maxSourcePixels is the
// caller area cap (media.ThumbnailOptions.MaxSourcePixels): positive values are
// honored as min(maxSourcePixels, MaxAVIFPixels); zero keeps MaxAVIFPixels.
//
// gav1d decodes a frame's tiles over up to GOMAXPROCS goroutines (fewer for
// fewer tiles). avif.Options has no thread setting, and capping concurrent
// decodes to GOMAXPROCS measured as no win (−4% on 16-tile 1080p, +5% on
// single-tile, 32 concurrent callers), so no global cap is applied.
func DecodeAVIFFileWithLimit(data []byte, maxSourcePixels int64) (*h264.ThumbYCbCr, error) {
	limit := avifDecodeLimit(maxSourcePixels)
	img, ci, err := avif.DecodeColor(bytes.NewReader(data), avif.Options{
		ToYCbCr:        true,
		AutoRotate:     true,
		FrameSizeLimit: int(limit),
	})
	if err != nil {
		return nil, avifErr(err, limit)
	}
	return avifToThumb(img, ci), nil
}

// MaxAVIFPixels bounds the decoded area (width × height) of an AVIF primary
// item or assembled grid, and of each grid tile. 2^27 pixels (about 134 MP)
// covers current phone and camera stills while keeping a hostile header far
// below gav1d's default of 16384×16384. Larger images return an unsupported
// error naming the limit.
const MaxAVIFPixels = 1 << 27

// avifColor maps the AVIF nclx matrix and range to the swscale matrix and
// range, the same way the HEVC path does (hevcdecode.ThumbColorFromPicture):
// unspecified (2) and 0 fall back to BT.601 like ffmpeg's swscale default,
// 1 is BT.709 and 9 is BT.2020.
func avifColor(ci avif.ColorInfo) (yuv.ColorSpace, yuv.Range) {
	cs := yuv.BT601
	if ci.Matrix != 0 && ci.Matrix != 2 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(uint(ci.Matrix))
	}
	rng := yuv.LimitedRange
	if ci.FullRange {
		rng = yuv.FullRange
	}
	return cs, rng
}

// avifToThumb converts a decoded AVIF image into a 4:2:0 ThumbYCbCr. Native
// YCbCr planes keep the file's matrix and range: 4:2:0 is used as-is, and
// 4:2:2/4:4:4 chroma is subsampled to 4:2:0 without a trip through RGB, so
// the declared matrix and range still apply. Monochrome keeps its luma range
// with neutral chroma. Anything else (RGB output from AutoRotate or high bit
// depth, where gav1d already applied the file's matrix) goes through
// rgbToThumb, which yields full-range BT.601 samples.
func avifToThumb(img image.Image, ci avif.ColorInfo) *h264.ThumbYCbCr {
	cs, rng := avifColor(ci)
	switch t := img.(type) {
	case *image.YCbCr:
		return yccToThumb(t, cs, rng)
	case *image.NYCbCrA:
		// Alpha is dropped; thumbnails are opaque.
		return yccToThumb(&t.YCbCr, cs, rng)
	case *image.Gray:
		return grayToThumb(t, rng)
	}
	return rgbToThumb(img)
}

// yccToThumb returns 4:2:0 planes for src in the given matrix and range. A
// 4:2:0 image at the origin is returned as-is; other layouts are copied with
// the chroma sample nearest each 2×2 block's top-left corner.
func yccToThumb(src *image.YCbCr, cs yuv.ColorSpace, rng yuv.Range) *h264.ThumbYCbCr {
	if src.SubsampleRatio == image.YCbCrSubsampleRatio420 && src.Rect.Min == (image.Point{}) {
		return &h264.ThumbYCbCr{YCbCr: src, CS: cs, RNG: rng}
	}
	b := src.Rect
	w, h := b.Dx(), b.Dy()
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		so := src.YOffset(b.Min.X, b.Min.Y+y)
		copy(dst.Y[y*dst.YStride:y*dst.YStride+w], src.Y[so:so+w])
	}
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x += 2 {
			si := src.COffset(b.Min.X+x, b.Min.Y+y)
			di := (y/2)*dst.CStride + x/2
			dst.Cb[di] = src.Cb[si]
			dst.Cr[di] = src.Cr[si]
		}
	}
	return &h264.ThumbYCbCr{YCbCr: dst, CS: cs, RNG: rng}
}

// grayToThumb wraps a monochrome image as 4:2:0 with neutral (128) chroma,
// keeping the luma samples and their declared range, as ffmpeg does when it
// converts gray to yuv420p.
func grayToThumb(src *image.Gray, rng yuv.Range) *h264.ThumbYCbCr {
	b := src.Rect
	w, h := b.Dx(), b.Dy()
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		so := src.PixOffset(b.Min.X, b.Min.Y+y)
		copy(dst.Y[y*dst.YStride:y*dst.YStride+w], src.Pix[so:so+w])
	}
	for i := range dst.Cb {
		dst.Cb[i] = 128
		dst.Cr[i] = 128
	}
	return &h264.ThumbYCbCr{YCbCr: dst, CS: yuv.BT601, RNG: rng}
}

// rgbToThumb converts gav1d's RGB output (NRGBA when AutoRotate transforms an
// 8-bit image, NRGBA64 above 8 bits) to 4:2:0 full-range BT.601 YCbCr, the
// samples Go's image/color and the JPEG encoder expect. NRGBA and NRGBA64 are
// read row by row directly, with no intermediate image copy; other image
// types go through image.Image.At. 16-bit samples are truncated to 8 bits
// (as image/draw did before), which measured closer to ffmpeg than rounding.
//
// Chroma is the BT.601 transform of the rounded mean RGB of each 2×2 block
// (fewer pixels on odd edges). ffmpeg never takes this route, since it
// decodes AVIF to YUV natively; measured against ffmpeg 9.0.2 RGB output on
// 1080p stills, the box average lowers the mean absolute RGB error versus
// top-left sampling (10-bit: 0.505 → 0.465; rotated: 0.541 → 0.500).
func rgbToThumb(img image.Image) *h264.ThumbYCbCr {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	row := rgbRowReader(img)
	r0 := make([]uint8, 3*w)
	r1 := make([]uint8, 3*w)
	for y := 0; y < h; y += 2 {
		row(r0, b.Min.Y+y)
		lumaRow(ycc.Y[y*ycc.YStride:y*ycc.YStride+w], r0)
		p1 := r0
		if y+1 < h {
			row(r1, b.Min.Y+y+1)
			lumaRow(ycc.Y[(y+1)*ycc.YStride:(y+1)*ycc.YStride+w], r1)
			p1 = r1
		}
		cb := ycc.Cb[(y/2)*ycc.CStride:]
		cr := ycc.Cr[(y/2)*ycc.CStride:]
		for x := 0; x < w; x += 2 {
			o, o1 := 3*x, 3*x
			if x+1 < w {
				o1 = 3 * (x + 1)
			}
			// Duplicating the edge column/row keeps the divisor at 4.
			sr := int32(r0[o]) + int32(r0[o1]) + int32(p1[o]) + int32(p1[o1])
			sg := int32(r0[o+1]) + int32(r0[o1+1]) + int32(p1[o+1]) + int32(p1[o1+1])
			sb := int32(r0[o+2]) + int32(r0[o1+2]) + int32(p1[o+2]) + int32(p1[o1+2])
			cb[x/2], cr[x/2] = chroma((sr+2)>>2, (sg+2)>>2, (sb+2)>>2)
		}
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.FullRange}
}

// rgbRowReader returns a function filling dst with the 8-bit RGB of row y.
func rgbRowReader(img image.Image) func(dst []uint8, y int) {
	b := img.Bounds()
	w := b.Dx()
	switch m := img.(type) {
	case *image.NRGBA:
		return func(dst []uint8, y int) {
			s := m.Pix[m.PixOffset(b.Min.X, y):]
			for x := 0; x < w; x++ {
				dst[3*x], dst[3*x+1], dst[3*x+2] = s[4*x], s[4*x+1], s[4*x+2]
			}
		}
	case *image.NRGBA64:
		return func(dst []uint8, y int) {
			s := m.Pix[m.PixOffset(b.Min.X, y):]
			for x := 0; x < w; x++ {
				o := 8 * x
				// Big-endian: the high byte is the sample truncated to 8 bits.
				dst[3*x], dst[3*x+1], dst[3*x+2] = s[o], s[o+2], s[o+4]
			}
		}
	}
	return func(dst []uint8, y int) {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, y)).(color.NRGBA)
			dst[3*x], dst[3*x+1], dst[3*x+2] = c.R, c.G, c.B
		}
	}
}

// lumaRow writes full-range BT.601 luma for one packed RGB row, with the
// fixed-point weights of image/color.RGBToYCbCr.
func lumaRow(dst, rgb []uint8) {
	for x := range dst {
		r, g, b := int32(rgb[3*x]), int32(rgb[3*x+1]), int32(rgb[3*x+2])
		dst[x] = uint8((19595*r + 38470*g + 7471*b + 1<<15) >> 16)
	}
}

// chroma returns full-range BT.601 Cb and Cr with the fixed-point weights and
// rounding of image/color.RGBToYCbCr.
func chroma(r, g, b int32) (uint8, uint8) {
	cb := -11056*r - 21712*g + 32768*b + 257<<15
	cr := 32768*r - 27440*g - 5328*b + 257<<15
	return clamp8(cb >> 16), clamp8(cr >> 16)
}

// clamp8 clamps v to 0..255.
func clamp8(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func avifErr(err error, limit int64) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, av1.ErrFrameTooLarge) {
		return errs.Unsup("thumbnail", fmt.Sprintf("avif: decoded image exceeds the %d-pixel decode limit", limit))
	}
	if errors.Is(err, avif.ErrUnsupported) {
		return errs.Unsup("thumbnail", "avif: "+err.Error())
	}
	return errs.UnsupProfile("avif", err.Error())
}
