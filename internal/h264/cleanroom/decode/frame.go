package decode

import "image"

// Frame is a decoded frame in planar YCbCr (8 bits per sample). The planes are
// stored at the coded size (a multiple of the macroblock); the visible area is
// given by CropWidth/CropHeight (cropping from the SPS).
type Frame struct {
	MbWidth, MbHeight   int // size in macroblocks
	MbWidthC, MbHeightC int // chroma size of one MB in samples
	ChromaArrayType     uint32

	CodedWidth, CodedHeight int // full size of the Y plane, pixels
	CropWidth, CropHeight   int // visible size after cropping
	CropX, CropY            int // luma pixel origin of the visible region

	Y, Cb, Cr        []byte
	StrideY, StrideC int
}

// NewFrame allocates a frame buffer for the dimensions from the SPS.
func NewFrame(s *SPS) *Frame {
	mbW := int(s.WidthMbs)
	frameMbsOnly := 1
	if !s.FrameMbsOnly {
		frameMbsOnly = 0
	}
	mbH := int(s.HeightMapUnits) * (2 - frameMbsOnly) // FrameHeightInMbs

	f := &Frame{
		MbWidth:         mbW,
		MbHeight:        mbH,
		ChromaArrayType: s.ChromaArrayType(),
		CodedWidth:      mbW * 16,
		CodedHeight:     mbH * 16,
		CropWidth:       s.Width,
		CropHeight:      s.Height,
		CropX:           s.OffsetX,
		CropY:           s.OffsetY,
	}
	f.StrideY = f.CodedWidth
	f.Y = make([]byte, f.CodedWidth*f.CodedHeight)

	if f.ChromaArrayType != 0 {
		f.MbWidthC, f.MbHeightC = mbChromaDims(f.ChromaArrayType)
		cw := mbW * f.MbWidthC
		ch := mbH * f.MbHeightC
		f.StrideC = cw
		f.Cb = make([]byte, cw*ch)
		f.Cr = make([]byte, cw*ch)
	}
	return f
}

// mbChromaDims returns the chroma dimensions of one macroblock (MbWidthC, MbHeightC).
func mbChromaDims(chromaArrayType uint32) (w, h int) {
	switch chromaArrayType {
	case 1: // 4:2:0
		return 8, 8
	case 2: // 4:2:2
		return 8, 16
	default: // 3 = 4:4:4
		return 16, 16
	}
}

// chromaSubsample returns the horizontal/vertical chroma subsampling factors.
func (f *Frame) chromaSubsample() (subW, subH int) {
	switch f.ChromaArrayType {
	case 2:
		return 2, 1
	case 3:
		return 1, 1
	default: // 4:2:0
		return 2, 2
	}
}

// CroppedYUV returns the visible region as contiguous YUV420 planes (no stride
// padding), suitable for raw output or building an image. cr is nil for mono.
func (f *Frame) CroppedYUV() (y, cb, cr []byte) {
	y = make([]byte, f.CropWidth*f.CropHeight)
	for row := 0; row < f.CropHeight; row++ {
		src := (f.CropY+row)*f.StrideY + f.CropX
		copy(y[row*f.CropWidth:], f.Y[src:src+f.CropWidth])
	}
	if f.ChromaArrayType == 0 {
		return
	}
	subW, subH := f.chromaSubsample()
	cw, ch := f.CropWidth/subW, f.CropHeight/subH
	cx, cy := f.CropX/subW, f.CropY/subH
	cb = make([]byte, cw*ch)
	cr = make([]byte, cw*ch)
	for row := 0; row < ch; row++ {
		src := (cy+row)*f.StrideC + cx
		copy(cb[row*cw:], f.Cb[src:src+cw])
		copy(cr[row*cw:], f.Cr[src:src+cw])
	}
	return
}

// YCbCr returns the visible (cropped) area as an image.YCbCr for convenient
// conversion to RGB/thumbnail. The subsampling is derived from ChromaArrayType.
func (f *Frame) YCbCr() *image.YCbCr {
	ratio := image.YCbCrSubsampleRatio420
	switch f.ChromaArrayType {
	case 2:
		ratio = image.YCbCrSubsampleRatio422
	case 3:
		ratio = image.YCbCrSubsampleRatio444
	}
	subW, _ := f.chromaSubsample()
	y, cb, cr := f.CroppedYUV()
	return &image.YCbCr{
		Y:              y,
		Cb:             cb,
		Cr:             cr,
		YStride:        f.CropWidth,
		CStride:        f.CropWidth / subW,
		SubsampleRatio: ratio,
		Rect:           image.Rect(0, 0, f.CropWidth, f.CropHeight),
	}
}
