package h264

import (
	"image"

	"github.com/Eyevinn/hi264/pkg/frame"
	"github.com/abema/go-mp4"
)

// YUV420 planes and strides from a decoded frame (4:2:0, 8-bit).
type YUV420 struct {
	Width   int
	Height  int
	Y, Cb, Cr []byte
	StrideY, StrideC int
}

// DecodeAVCCYUV420 decodes a sync sample to planar YUV (matches ffmpeg yuv420p on golden fixtures).
func DecodeAVCCYUV420(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (YUV420, error) {
	fr, err := decodeFrame(avcc, sample, fast)
	if err != nil {
		return YUV420{}, err
	}
	return frameYUV(fr), nil
}

// FrameToYCbCr builds a standard library YCbCr image (limited range) for fast downscale paths.
func FrameToYCbCr(f *frame.Frame) *image.YCbCr {
	rect := image.Rect(0, 0, f.Width, f.Height)
	yimg := &image.YCbCr{
		Rect: rect, SubsampleRatio: image.YCbCrSubsampleRatio420,
		YStride: f.StrideY, CStride: f.StrideC,
		Y: f.Y, Cb: f.Cb, Cr: f.Cr,
	}
	return yimg
}

func frameYUV(f *frame.Frame) YUV420 {
	return YUV420{
		Width: f.Width, Height: f.Height,
		Y: f.Y, Cb: f.Cb, Cr: f.Cr,
		StrideY: f.StrideY, StrideC: f.StrideC,
	}
}

// WriteI420 appends Y, U, V in ffmpeg yuv420p/I420 order (no padding between planes).
func (p YUV420) WriteI420(dst []byte) []byte {
	w, h := p.Width, p.Height
	for y := 0; y < h; y++ {
		dst = append(dst, p.Y[y*p.StrideY:y*p.StrideY+w]...)
	}
	cw, ch := w/2, h/2
	for y := 0; y < ch; y++ {
		dst = append(dst, p.Cb[y*p.StrideC:y*p.StrideC+cw]...)
	}
	for y := 0; y < ch; y++ {
		dst = append(dst, p.Cr[y*p.StrideC:y*p.StrideC+cw]...)
	}
	return dst
}
