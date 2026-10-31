// Package thumb renders scaled keyframe thumbnails (JPEG, PNG, WebP).
//
// Colour matches ffmpeg's go-ffmpeg capture (scale then mjpeg/libwebp/png):
//   - H.264 honors the SPS VUI range and matrix; untagged video is BT.601
//     limited range at every resolution (swscale's default).
//   - JPEG and WebP output use the BT.601 matrix for every source, because
//     ffmpeg converts YUV→YUV and those formats are displayed as BT.601; PNG
//     goes through RGB and keeps the tagged matrix
//     (h264.ThumbYCbCr.MatchFFmpegYUVOutput).
//   - VP8 is BT.601 limited range on every path (RFC 6386 §9.2), which
//     is what ffmpeg's vp8 decoder reports.
//
// Residual differences: full-range and BT.709 sources go through an RGB
// intermediate that clips out-of-gamut values (ffmpeg stays in YUV), roughly
// 5 dB PSNR lower than untagged sources, and image/jpeg quantizes differently
// from ffmpeg's mjpeg encoder.
package thumb

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"slices"
	"strings"

	"github.com/gen2brain/vpx/webp"
	"github.com/gtsteffaniak/go-media/internal/asf"
	"github.com/gtsteffaniak/go-media/internal/avi"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	ps "github.com/gtsteffaniak/go-media/internal/mpegps"
	ts "github.com/gtsteffaniak/go-media/internal/mpegts"
	"github.com/gtsteffaniak/go-media/internal/rawpreview"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/tagread"
	"github.com/gtsteffaniak/go-media/internal/vp8decode"
	"github.com/gtsteffaniak/go-media/internal/vp9decode"
)

// Options configures thumbnail generation.
type Options struct {
	MaxWidth       int
	MaxHeight      int
	SeekPercent    float64
	Format         string // jpeg, png, webp-lossless
	JPEGQuality    int    // 1–100; used when Quality is zero
	Quality        int    // 1–100 preferred JPEG quality
	Filter         Filter
	Scaler         ScalerMode
	Fast           bool
	PreferEmbedded bool
	// Exact decodes forward to the frame at the seek time where the codec
	// supports it (VP8, VP9). Default: keyframe at or before the seek time.
	Exact bool
}

// Result is a data type used by this package.
type Result struct {
	MIME   string
	Data   []byte
	Source string
	Width  int
	Height int
}

// Generate supports media inspection.
func Generate(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, opt Options) (Result, error) {
	normalizeOptions(&opt)
	img, src, rot, err := sourceImage(r, size, opt)
	if err != nil {
		return Result{}, err
	}
	return encodeImageRaster(img, fitOptions(opt, rot), src, rot)
}

// fitOptions returns opt with MaxWidth and MaxHeight swapped when rot turns
// the frame on its side, so the coded frame is fitted to the display box.
func fitOptions(opt Options, rot int) Options {
	if swapsAxes(rot) {
		opt.MaxWidth, opt.MaxHeight = opt.MaxHeight, opt.MaxWidth
	}
	return opt
}

// sourceImage decodes embedded art or a video frame without encoding. rot is
// the display rotation still to apply to the scaled image (MP4 tkhd matrix).
func sourceImage(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, opt Options) (image.Image, string, int, error) {
	var rot int
	img, src, err := decodeSource(r, size, opt, &rot)
	if err != nil {
		return nil, "", 0, err
	}
	return img, src, rot, nil
}

// decodeSource does the work of sourceImage, storing the MP4 rotation in rot.
func decodeSource(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, opt Options, rot *int) (image.Image, string, error) {
	if opt.PreferEmbedded {
		if data, mime, err := tagread.Artwork(r, size); err == nil {
			img, err := decodeBytes(data, mime, opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err == nil {
				return img, "embedded", nil
			}
		}
	}

	fmtID, err := sniff.Detect(r, size)
	if err != nil {
		return nil, "", err
	}

	switch fmtID {
	case format.MP4:
		// One bounded moov header walk serves the rotation check and the
		// MJPEG/H.264 seek. Rotation is applied by Generate after scaling.
		src, err := isobmff.OpenThumbSourceErr(r, size)
		if err != nil {
			return nil, "", err
		}
		*rot = src.Rotation()
		opt = fitOptions(opt, *rot)
		if b, ok := src.MJPEGFrame(opt.SeekPercent); ok {
			img, err := decodeBytes(b, "image/jpeg", opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err != nil {
				return nil, "", err
			}
			return img, "decode", nil
		}
		// Intra-only IDR decode (fast preview / skip deblock). YCbCr → matrix-aware area downscale → JPEG.
		if useH264RGBPipeline(opt) {
			if rgba, err := src.H264ThumbnailRGBA(opt.SeekPercent, true); err != nil {
				return nil, "", err
			} else if rgba != nil {
				return rgba, "decode", nil
			}
		} else if yimg, err := src.H264ThumbnailYCbCr(opt.SeekPercent, true); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
		if yimg, err := src.MPEG2ThumbnailYCbCr(opt.SeekPercent); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
		// MPEG-4 Part 2 (mp4v) I-VOP.
		if yimg, err := src.MPEG4ThumbnailYCbCr(opt.SeekPercent); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
		// No MJPEG, H.264 or MPEG-4 track: name the codec from the index (no I/O).
		return nil, "", src.UnsupportedReason()
	case format.AVI:
		src, err := avi.OpenThumbSourceErr(r, size)
		if err != nil {
			return nil, "", err
		}
		opt = fitOptions(opt, 0)
		if b, ok := src.MJPEGFrame(opt.SeekPercent); ok {
			img, err := decodeBytes(b, "image/jpeg", opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err != nil {
				return nil, "", err
			}
			return img, "decode", nil
		}
		if useH264RGBPipeline(opt) {
			if rgba, err := src.H264ThumbnailRGBA(opt.SeekPercent, true); err != nil {
				return nil, "", err
			} else if rgba != nil {
				return rgba, "decode", nil
			}
		} else if yimg, err := src.H264ThumbnailYCbCr(opt.SeekPercent, true); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
		if yimg, err := src.MPEG4ThumbnailYCbCr(opt.SeekPercent); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
		return nil, "", src.UnsupportedReason()
	case format.ASF, format.WMV:
		src, err := asf.OpenThumbSourceErr(r, size)
		if err != nil {
			return nil, "", err
		}
		opt = fitOptions(opt, 0)
		if yimg, err := src.WMV3ThumbnailYCbCr(opt.SeekPercent); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
		return nil, "", src.UnsupportedReason()
	case format.MKV, format.WebM:
		// Probe the bounded header once and decide from it before any demux pass.
		// Demuxing a multi-GB file only to find no usable track costs several
		// full-file reads. The header is passed down so the decoders do not re-read it.
		hdr, err := matroska.ProbeHeader(r, size)
		if err != nil {
			return nil, "", errs.Unsup("thumbnail", "matroska header unreadable")
		}
		if len(hdr.VideoCodecIDs) == 0 {
			return nil, "", errs.Unsup("thumbnail", "matroska: no video track in bounded header (unknown-size cluster, truncated, or no Segment)")
		}
		if slices.ContainsFunc(hdr.VideoCodecIDs, matroska.IsMPEG4CodecID) && !hasThumbCodec(hdr.VideoCodecIDs) {
			pic, err := matroska.MPEG4PictureAtSeek(r, size, hdr, opt.SeekPercent)
			if err != nil {
				return nil, "", err
			}
			if pic != nil {
				return isobmff.PictureYCbCr(pic), "decode", nil
			}
		}
		if !hasThumbCodec(hdr.VideoCodecIDs) {
			if slices.Contains(hdr.VideoCodecIDs, "V_MPEG4/ISO/AVC") {
				return nil, "", errs.UnsupProfile("thumbnail", "h264 in matroska")
			}
			return nil, "", errs.Unsup("thumbnail", "matroska video codec "+strings.Join(hdr.VideoCodecIDs, ","))
		}
		// VP8 and MJPEG seeking both map a percentage to time, so they need a Duration.
		if !hdr.HasDuration() {
			return nil, "", errs.Unsup("thumbnail", "matroska: no duration, seek impossible")
		}
		jpegOut := opt.Format == "" || opt.Format == "jpeg"
		if jpegOut {
			if pic, ok := matroska.VP8PictureAtSeek(r, size, hdr, opt.SeekPercent, opt.Exact); ok {
				if ycc := vp8decode.PictureYCbCr(pic); ycc != nil {
					// VP8 samples are BT.601 limited range (RFC 6386 §9.2,
					// color_space 0); ffmpeg's vp8 decoder reports the same and
					// swscale expands the range for MJPEG. Wrapping them as
					// video YCbCr applies that expansion instead of treating
					// them as full-range JFIF samples.
					return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.LimitedRange}, "decode", nil
				}
			}
			if slices.Contains(hdr.VideoCodecIDs, "V_VP9") {
				// Hand VP9 frames over as video YCbCr too, so JPEG output uses the
				// shared YUV area downscaler instead of a full-frame RGB resize.
				if pic, err := matroska.VP9PictureAtSeek(r, size, hdr, opt.SeekPercent, opt.Exact); err == nil && pic != nil {
					if ycc := vp9decode.PictureThumbYCbCr(pic); ycc != nil {
						return ycc, "decode", nil
					}
				}
			}
		}
		tp := matroska.ThumbnailParams{
			SeekPercent: opt.SeekPercent,
			MaxWidth:    opt.MaxWidth,
			MaxHeight:   opt.MaxHeight,
			Scaler:      int(scalerFromOptions(opt)),
			Exact:       opt.Exact,
		}
		if img, ok := matroska.ThumbnailRGBA(r, size, hdr, tp); ok {
			return img, "decode", nil
		}
		// H.264 is decided from the IDs already parsed; no extra header read.
		if slices.Contains(hdr.VideoCodecIDs, "V_MPEG4/ISO/AVC") {
			return nil, "", errs.UnsupProfile("thumbnail", "h264 in matroska")
		}
		return nil, "", errs.Unsup("thumbnail", "matroska: no decodable "+strings.Join(hdr.VideoCodecIDs, ",")+" frame near seek target (no cues/keyframe or frame decode failed)")
	case format.MP3, format.FLAC, format.OGG:
		data, mime, err := tagread.Artwork(r, size)
		if err != nil {
			return nil, "", errs.Unsup("thumbnail", string(fmtID)+": no embedded cover art")
		}
		img, err := decodeBytes(data, mime, opt.MaxWidth, opt.MaxHeight, opt.Fast)
		if err != nil {
			return nil, "", errs.Unsup("thumbnail", string(fmtID)+": embedded cover art ("+mime+") not decodable")
		}
		return img, "embedded", nil
	case format.MPEGTS, format.M2TS:
		meta, err := ts.Probe(r, size)
		if err != nil {
			return nil, "", err
		}
		yimg, err := ts.ThumbnailYCbCr(r, size, meta, opt.SeekPercent, opt.Fast)
		if err != nil {
			return nil, "", err
		}
		return yimg, "decode", nil
	case format.MPEGPS:
		meta, err := ps.Probe(r, size)
		if err != nil {
			return nil, "", err
		}
		yimg, err := ps.ThumbnailYCbCr(r, size, meta, opt.SeekPercent, opt.Fast)
		if err != nil {
			return nil, "", err
		}
		return yimg, "decode", nil
	case format.HEIC, format.HEIF, format.AVIF:
		data, err := heifthumb.JPEGPreview(r, size, opt.MaxWidth, opt.MaxHeight)
		if err != nil {
			return nil, "", err
		}
		img, err := decodeBytes(data, "image/jpeg", opt.MaxWidth, opt.MaxHeight, opt.Fast)
		if err != nil {
			return nil, "", err
		}
		return img, "embedded", nil
	default:
		if format.IsCameraRAW(fmtID) {
			data, err := rawpreview.JPEGPreview(r, size, fmtID, opt.MaxWidth, opt.MaxHeight)
			if err != nil {
				return nil, "", err
			}
			img, err := decodeBytes(data, "image/jpeg", opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err != nil {
				return nil, "", err
			}
			return img, "embedded", nil
		}
	}
	return nil, "", errs.Unsup("thumbnail", string(fmtID)+": container has no thumbnail decoder in go-media")
}

// hasThumbCodec reports whether any Matroska video codec is one the thumbnail
// path decodes (VP8, VP9, or MJPEG).
func hasThumbCodec(ids []string) bool {
	for _, id := range ids {
		if id == "V_VP8" || id == "V_VP9" || id == "V_MJPEG" {
			return true
		}
	}
	return false
}

func decodeBytes(data []byte, mime string, maxW, maxH int, fast bool) (image.Image, error) {
	img, _, err := jpegDecode(data, maxW, maxH, fast)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return nil, errs.Unsup("thumbnail", "unsupported cover image format")
		}
		return nil, err
	}
	return img, nil
}

// encodeImageRaster scales img to fit opt, turns it by rot, and encodes it.
func encodeImageRaster(img image.Image, opt Options, src string, rot int) (Result, error) {
	mode := scalerFromOptions(opt)
	jpegOut := opt.Format == "" || opt.Format == "jpeg"
	if t, ok := img.(*h264.ThumbYCbCr); ok && opt.Format != "png" {
		// ffmpeg's JPEG/WebP capture keeps YUV end to end, so the viewer applies
		// BT.601 whatever the source tag says. PNG goes through RGB and keeps
		// the tagged matrix. See ThumbYCbCr.MatchFFmpegYUVOutput.
		t.MatchFFmpegYUVOutput()
	}
	if jpegOut {
		if _, ok := img.(*h264.ThumbYCbCr); ok {
			if scaled, ok := tryScaleYCbCrSource(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
				img = scaled
			} else {
				img = rgbaFromImage(img)
			}
		} else if scaled, ok := tryScaleYCbCrDown(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
			img = scaled
		}
	}
	if _, yccOK := img.(*image.YCbCr); !yccOK || !jpegOut {
		if scaled, ok := tryScaleYCbCrSource(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
			img = scaled
		} else if needsScale(img, opt.MaxWidth, opt.MaxHeight) {
			img = scaleImage(img, opt.MaxWidth, opt.MaxHeight, mode)
		}
	}
	img = rotateImage(img, rot)
	var buf bytes.Buffer
	if jpegOut {
		b := img.Bounds()
		buf.Grow(b.Dx() * b.Dy() / 4)
	}
	switch opt.Format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return Result{}, err
		}
		return Result{MIME: "image/png", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	case "webp-lossless", "webp":
		if err := webp.Encode(&buf, img, webp.EncodeOptions{Lossless: true, Quality: 100}); err != nil {
			return Result{}, err
		}
		return Result{MIME: "image/webp", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	}
	q := jpegQuality(opt)
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		return Result{}, err
	}
	return Result{MIME: "image/jpeg", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
}

func jpegQuality(opt Options) int {
	if opt.Quality > 0 {
		return clampQuality(opt.Quality)
	}
	return clampQuality(opt.JPEGQuality)
}

func clampQuality(q int) int {
	if q < 1 {
		return 1
	}
	if q > 100 {
		return 100
	}
	return q
}

func needsScale(img image.Image, maxW, maxH int) bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	return w > maxW || h > maxH
}

func normalizeOptions(opt *Options) {
	if opt.MaxWidth <= 0 {
		opt.MaxWidth = 320
	}
	if opt.SeekPercent <= 0 {
		opt.SeekPercent = 10
	}
	if opt.Quality > 0 {
		opt.JPEGQuality = opt.Quality
	}
	if opt.JPEGQuality <= 0 {
		opt.JPEGQuality = 85
	}
	if opt.Format == "" {
		opt.Format = "jpeg"
	}
}
