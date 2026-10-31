package hevcdecode

import (
	"image"

	"github.com/gen2brain/h265/hevc"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// ThumbColorFromPicture maps HEVC VUI / nclx to ffmpeg swscale matrix and range.
func ThumbColorFromPicture(p *hevc.Picture) (yuv.ColorSpace, yuv.Range) {
	cs := yuv.BT601
	rng := yuv.LimitedRange
	if p.ColorMatrix != 0 && p.ColorMatrix != 2 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(uint(p.ColorMatrix))
	}
	if p.FullRange {
		rng = yuv.FullRange
	}
	return cs, rng
}

// PictureThumbYCbCr copies a decoded HEVC picture into ThumbYCbCr (cropped size).
func PictureThumbYCbCr(p *hevc.Picture) *h264.ThumbYCbCr {
	if p == nil {
		return nil
	}
	w, h := p.CropW, p.CropH
	if w <= 0 || h <= 0 {
		w, h = p.Width, p.Height
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	cx, cy := p.CropX, p.CropY
	ratio := image.YCbCrSubsampleRatio420
	mono := p.ChromaFormat == 0
	switch p.ChromaFormat {
	case 2:
		ratio = image.YCbCrSubsampleRatio422
	case 3:
		ratio = image.YCbCrSubsampleRatio444
	}
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), ratio)
	if p.BitDepth > 8 || p.BitDepthC > 8 {
		copyPlane16To8(ycc.Y, ycc.YStride, p.Y16, p.StrideY, cx, cy, w, h, p.BitDepth)
		if mono {
			fillNeutralChroma(ycc)
		} else {
			copyChroma16To8(ycc, p, cx, cy, w, h, ratio, p.BitDepthC)
		}
	} else {
		copyPlane8(ycc.Y, ycc.YStride, p.Y, p.StrideY, cx, cy, w, h)
		if mono {
			fillNeutralChroma(ycc)
		} else {
			copyChroma8(ycc, p, cx, cy, w, h, ratio)
		}
	}
	cs, rng := ThumbColorFromPicture(p)
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: cs, RNG: rng}
}

func copyPlane8(dst []byte, dstStride int, src []byte, srcStride, cx, cy, w, h int) {
	for y := 0; y < h; y++ {
		copy(dst[y*dstStride:y*dstStride+w], src[(cy+y)*srcStride+cx:(cy+y)*srcStride+cx+w])
	}
}

func copyPlane16To8(dst []byte, dstStride int, src []uint16, srcStride, cx, cy, w, h, bitDepth int) {
	for y := 0; y < h; y++ {
		row := src[(cy+y)*srcStride+cx : (cy+y)*srcStride+cx+w]
		out := dst[y*dstStride : y*dstStride+w]
		for x := range out {
			out[x] = sample16To8(row[x], bitDepth)
		}
	}
}

func sample16To8(v uint16, bitDepth int) byte {
	shift := bitDepth - 8
	if shift <= 0 {
		return byte(v)
	}
	return byte(v >> shift)
}

func copyChroma8(ycc *image.YCbCr, p *hevc.Picture, cx, cy, w, h int, ratio image.YCbCrSubsampleRatio) {
	hc, wc := chromaSize(w, h, ratio)
	cxC, cyC := chromaOrigin(cx, cy, ratio)
	for y := 0; y < hc; y++ {
		copy(ycc.Cb[y*ycc.CStride:y*ycc.CStride+wc], p.Cb[(cyC+y)*p.StrideC+cxC:(cyC+y)*p.StrideC+cxC+wc])
		copy(ycc.Cr[y*ycc.CStride:y*ycc.CStride+wc], p.Cr[(cyC+y)*p.StrideC+cxC:(cyC+y)*p.StrideC+cxC+wc])
	}
}

func copyChroma16To8(ycc *image.YCbCr, p *hevc.Picture, cx, cy, w, h int, ratio image.YCbCrSubsampleRatio, bitDepthC int) {
	hc, wc := chromaSize(w, h, ratio)
	cxC, cyC := chromaOrigin(cx, cy, ratio)
	for y := 0; y < hc; y++ {
		cbRow := p.Cb16[(cyC+y)*p.StrideC+cxC : (cyC+y)*p.StrideC+cxC+wc]
		crRow := p.Cr16[(cyC+y)*p.StrideC+cxC : (cyC+y)*p.StrideC+cxC+wc]
		outCb := ycc.Cb[y*ycc.CStride : y*ycc.CStride+wc]
		outCr := ycc.Cr[y*ycc.CStride : y*ycc.CStride+wc]
		for x := range outCb {
			outCb[x] = sample16To8(cbRow[x], bitDepthC)
			outCr[x] = sample16To8(crRow[x], bitDepthC)
		}
	}
}

func fillNeutralChroma(ycc *image.YCbCr) {
	for i := range ycc.Cb {
		ycc.Cb[i] = 128
		ycc.Cr[i] = 128
	}
}

func chromaOrigin(cx, cy int, ratio image.YCbCrSubsampleRatio) (cxC, cyC int) {
	switch ratio {
	case image.YCbCrSubsampleRatio444:
		return cx, cy
	case image.YCbCrSubsampleRatio422:
		return cx / 2, cy
	default:
		return cx / 2, cy / 2
	}
}

func chromaSize(w, h int, ratio image.YCbCrSubsampleRatio) (hc, wc int) {
	switch ratio {
	case image.YCbCrSubsampleRatio444:
		return h, w
	case image.YCbCrSubsampleRatio422:
		return h, (w + 1) / 2
	default:
		return (h + 1) / 2, (w + 1) / 2
	}
}
