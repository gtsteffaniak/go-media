package av1decode

import (
	"encoding/binary"
	"errors"
	"image"

	"github.com/gen2brain/gav1d/av1"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// MaxFrameLongSide and MaxFrameShortSide are the default per-side caps for
// video keyframes when no caller override is passed (see FrameLimits). They
// default to DCI 4K (4096×2160 in either orientation). Setting both to zero
// disables the cap (gav1d's own DefaultFrameSizeLimit then applies).
var (
	MaxFrameLongSide  = DefaultFrameLongSide
	MaxFrameShortSide = DefaultFrameShortSide
)

// Decoder decodes AV1 keyframes with gen2brain/gav1d. A Decoder holds no
// frame memory between calls: DecodeSample flushes the decoder (dropping its
// reference frames and picture pool) before returning, and the returned
// picture is an independent copy.
type Decoder struct {
	d      *av1.Decoder
	limits FrameLimits
}

// NewDecoder returns a keyframe decoder with film grain synthesis skipped,
// one frame in flight and the default frame-size limit.
func NewDecoder() *Decoder {
	return NewDecoderWithLimits(FrameLimits{})
}

// NewDecoderWithLimits returns a decoder using lim for per-frame dimension
// checks and gav1d's FrameSizeLimit. Zero limits select MaxFrameLongSide and
// MaxFrameShortSide.
func NewDecoderWithLimits(lim FrameLimits) *Decoder {
	return &Decoder{
		d: &av1.Decoder{
			FrameThreads:   1,
			FrameSizeLimit: lim.frameSizeLimit(),
			SkipFilmGrain:  true,
		},
		limits: lim,
	}
}

// Close drops the underlying decoder. It is safe to call more than once.
func (d *Decoder) Close() {
	if d == nil || d.d == nil {
		return
	}
	d.d.Flush()
	d.d = nil
}

// DecodeSample decodes one keyframe using optional av1C extradata and sample
// bytes. The returned picture is a copy tagged with the sequence header's
// matrix_coefficients and color_range, mapped the way ffmpeg's swscale does
// (see colorFromConfig).
func (d *Decoder) DecodeSample(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	if d == nil || d.d == nil {
		return nil, errs.UnsupProfile("av1 keyframe", "decoder closed")
	}
	if len(sample) == 0 {
		return nil, errs.UnsupProfile("av1 keyframe", "empty sample")
	}
	d.d.FrameSizeLimit = d.limits.frameSizeLimit()
	defer d.d.Flush()
	if cfg := ConfigOBUs(av1CExtradata); len(cfg) > 0 {
		// Sequence header (and metadata) OBUs from av1C / CodecPrivate. A
		// repeated in-band sequence header in the sample is accepted.
		if err := d.d.Send(cfg); err != nil {
			return nil, d.mapErr(err)
		}
	}
	pic, err := d.decodeSampleLastPicture(sample)
	if err != nil {
		return nil, err
	}
	defer pic.Release()
	if pic.MatrixCoeffs == 0 && pic.Layout != av1.LayoutI444 && pic.Layout != av1.LayoutI400 {
		return nil, errs.UnsupProfile("av1 keyframe", "identity matrix requires 4:4:4")
	}
	return PictureThumbYCbCr(pic), nil
}

// decodeSampleLastPicture decodes sample OBUs with Send/Receive so decoded
// pictures are not all queued before the first Receive. It returns the last
// shown picture; the caller must Release it.
func (d *Decoder) decodeSampleLastPicture(sample []byte) (*av1.Picture, error) {
	var (
		last      *av1.Picture
		frames    int
		frameOBUs int
		pixels    int64
		budget    = decodePixelBudget(d.limits)
	)
	for len(sample) > 0 {
		obu, rest, ok := nextOBU(sample)
		if !ok {
			return nil, errs.UnsupProfile("av1 keyframe", "invalid OBU")
		}
		sample = rest
		if obuTriggersFrameDecode(obuType(obu)) {
			frameOBUs++
			if frameOBUs > maxFrameOBUsPerSample {
				if last != nil {
					last.Release()
				}
				return nil, errs.UnsupProfile("av1 keyframe", "too many frame OBUs in sample")
			}
		}
		if err := d.d.Send(obu); err != nil {
			if last != nil {
				last.Release()
			}
			return nil, d.mapErr(err)
		}
		for {
			pic, err := d.d.Receive()
			if err != nil {
				if last != nil {
					last.Release()
				}
				return nil, d.mapErr(err)
			}
			if pic == nil {
				break
			}
			frames++
			if frames > maxShownFramesPerSample {
				pic.Release()
				if last != nil {
					last.Release()
				}
				return nil, errs.UnsupProfile("av1 keyframe", "too many frames in sample")
			}
			px := int64(pic.Width) * int64(pic.Height)
			pixels += px
			if pixels > budget {
				pic.Release()
				if last != nil {
					last.Release()
				}
				return nil, d.limits.frameTooLarge("aggregate decoded pixels")
			}
			if err := d.limits.checkFrameSize(pic.Width, pic.Height); err != nil {
				pic.Release()
				if last != nil {
					last.Release()
				}
				return nil, err
			}
			if last != nil {
				last.Release()
			}
			last = pic
		}
	}
	if last == nil {
		return nil, errs.UnsupProfile("av1 keyframe", "sample shows no frame")
	}
	return last, nil
}

// DecodeSampleOnce decodes one keyframe with a fresh decoder and default limits.
func DecodeSampleOnce(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	return DecodeSampleCached(av1CExtradata, sample, FrameLimits{})
}

// DecodeSampleOnceWithLimits is DecodeSampleOnce with an explicit frame cap.
func DecodeSampleOnceWithLimits(av1CExtradata, sample []byte, lim FrameLimits) (*h264.ThumbYCbCr, error) {
	return DecodeSampleCached(av1CExtradata, sample, lim)
}

// ConfigOBUs returns the configOBUs of an av1C box or Matroska CodecPrivate
// payload (AV1CodecConfigurationRecord: 4 header bytes, then OBUs). It returns
// nil when the record is missing or malformed.
func ConfigOBUs(av1CExtradata []byte) []byte {
	if len(av1CExtradata) < 4 {
		return nil
	}
	// marker (1 bit) must be 1 and version (7 bits) must be 1.
	if av1CExtradata[0] != 0x81 {
		return nil
	}
	if len(av1CExtradata) == 4 {
		return nil
	}
	return av1CExtradata[4:]
}

// PictureThumbYCbCr copies a decoded gav1d picture into an 8-bit 4:2:0
// ThumbYCbCr. Samples above 8 bits are rounded to 8 bits; 4:2:2 and 4:4:4
// chroma is averaged down to 4:2:0; monochrome gets neutral chroma. The color
// tag comes from the sequence header (see colorFromConfig).
func PictureThumbYCbCr(p *av1.Picture) *h264.ThumbYCbCr {
	if p == nil || p.Width <= 0 || p.Height <= 0 {
		return nil
	}
	if p.MatrixCoeffs == 0 && p.Layout == av1.LayoutI444 {
		return pictureThumbYCbCrIdentity444(p)
	}
	w, h := p.Width, p.Height
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	shift := max(p.BitDepth-8, 0)
	bps := 1
	if p.BitDepth > 8 {
		bps = 2
	}
	copyPlane(ycc.Y, ycc.YStride, p.Data[0], p.Stride[0], w, h, bps, shift)
	cw, ch := (w+1)/2, (h+1)/2
	switch {
	case p.Layout == av1.LayoutI400 || len(p.Data[1]) == 0 || len(p.Data[2]) == 0:
		for i := range ycc.Cb {
			ycc.Cb[i] = 128
			ycc.Cr[i] = 128
		}
	case p.Layout == av1.LayoutI420:
		copyPlane(ycc.Cb, ycc.CStride, p.Data[1], p.Stride[1], cw, ch, bps, shift)
		copyPlane(ycc.Cr, ycc.CStride, p.Data[2], p.Stride[2], cw, ch, bps, shift)
	default:
		sx := 1 // 4:4:4: average 2×2; 4:2:2: average 1×2 (vertical only)
		if p.Layout == av1.LayoutI422 {
			sx = 0
		}
		srcW := w // 4:4:4 chroma is full width
		if sx == 0 {
			srcW = cw // 4:2:2 chroma is already half width
		}
		downsample(ycc.Cb, ycc.CStride, p.Data[1], p.Stride[1], cw, ch, srcW, h, sx, bps, shift)
		downsample(ycc.Cr, ycc.CStride, p.Data[2], p.Stride[2], cw, ch, srcW, h, sx, bps, shift)
	}
	cs, rng := colorFromConfig(uint8(p.MatrixCoeffs), p.FullRange)
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: cs, RNG: rng}
}

func sample(src []byte, off, bps int) int {
	if bps == 1 {
		return int(src[off])
	}
	return int(binary.NativeEndian.Uint16(src[off : off+2]))
}

// pictureThumbYCbCrIdentity444 converts identity-matrix 4:4:4 planes (G, B, R)
// to 4:2:0 YCbCr before subsampling, matching ffmpeg's GBR handling.
func pictureThumbYCbCrIdentity444(p *av1.Picture) *h264.ThumbYCbCr {
	w, h := p.Width, p.Height
	cs, rng := colorFromConfig(0, p.FullRange)
	shift := max(p.BitDepth-8, 0)
	bps := 1
	if p.BitDepth > 8 {
		bps = 2
	}
	yFull := make([]byte, w*h)
	cbFull := make([]byte, w*h)
	crFull := make([]byte, w*h)
	for y := 0; y < h; y++ {
		gRow := y * p.Stride[0]
		bRow := y * p.Stride[1]
		rRow := y * p.Stride[2]
		for x := 0; x < w; x++ {
			g := to8(sample(p.Data[0], gRow+x*bps, bps), shift)
			b := to8(sample(p.Data[1], bRow+x*bps, bps), shift)
			r := to8(sample(p.Data[2], rRow+x*bps, bps), shift)
			c := yuv.RGBToYCbCrCS(r, g, b, cs, rng)
			i := y*w + x
			yFull[i], cbFull[i], crFull[i] = c.Y, c.Cb, c.Cr
		}
	}
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	copyPlane(ycc.Y, ycc.YStride, yFull, w, w, h, 1, 0)
	cw, ch := (w+1)/2, (h+1)/2
	downsample(ycc.Cb, ycc.CStride, cbFull, w, cw, ch, w, h, 1, 1, 0)
	downsample(ycc.Cr, ycc.CStride, crFull, w, cw, ch, w, h, 1, 1, 0)
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: cs, RNG: rng}
}

func to8(v, shift int) byte {
	if shift > 0 {
		v = (v + 1<<(shift-1)) >> shift
	}
	if v > 255 {
		v = 255
	}
	return byte(v)
}

func copyPlane(dst []byte, dstStride int, src []byte, srcStride, w, h, bps, shift int) {
	if bps == 1 {
		for y := 0; y < h; y++ {
			if y*srcStride+w > len(src) || y*dstStride+w > len(dst) {
				return
			}
			copy(dst[y*dstStride:y*dstStride+w], src[y*srcStride:y*srcStride+w])
		}
		return
	}
	for y := 0; y < h; y++ {
		so := y * srcStride
		if so+w*2 > len(src) || y*dstStride+w > len(dst) {
			return
		}
		row := dst[y*dstStride : y*dstStride+w]
		for x := range row {
			row[x] = to8(sample(src, so+2*x, 2), shift)
		}
	}
}

// downsample averages chroma to 4:2:0: two rows always, and two columns when
// sx is 1 (4:4:4). Edge samples are clamped for odd sizes.
func downsample(dst []byte, dstStride int, src []byte, srcStride, cw, ch, srcW, srcH, sx, bps, shift int) {
	for y := 0; y < ch; y++ {
		y0, y1 := 2*y, min(2*y+1, srcH-1)
		row := dst[y*dstStride : y*dstStride+cw]
		for x := range row {
			x0 := x << sx
			x1 := min(x0+sx, srcW-1)
			s := sample(src, y0*srcStride+x0*bps, bps) + sample(src, y1*srcStride+x0*bps, bps)
			n := 2
			if sx == 1 {
				s += sample(src, y0*srcStride+x1*bps, bps) + sample(src, y1*srcStride+x1*bps, bps)
				n = 4
			}
			row[x] = to8((s+n/2)/n, shift)
		}
	}
}

func (d *Decoder) mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, av1.ErrFrameTooLarge) {
		return d.limits.frameTooLarge("frame header")
	}
	var ue *errs.UnsupportedError
	if errors.As(err, &ue) {
		return ue
	}
	var pe *errs.ProfileError
	if errors.As(err, &pe) {
		return pe
	}
	msg := err.Error()
	if msg == "" {
		msg = "decode failed"
	}
	return errs.UnsupProfile("av1 keyframe", msg)
}
