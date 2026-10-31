package ps

import (
	"bytes"
	"image"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/mpeg2video"
)

// ThumbnailYCbCr finds a video PES near seekPercent and decodes an MPEG-2 I-picture.
func ThumbnailYCbCr(r io.ReaderAt, size int64, meta *Meta, seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	if meta == nil {
		return nil, errs.Unsup("thumbnail", "mpegps: missing metadata")
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	target := int(meta.Duration * 25.0 * seekPercent / 100.0)
	if target < 0 {
		target = 0
	}
	es, err := extractAllVideoES(r, size)
	if err != nil {
		return nil, err
	}
	pic, err := mpeg2video.DecodeIFrameIndex(es, target)
	if err != nil {
		return nil, errs.Unsup("thumbnail", "mpegps: "+err.Error())
	}
	ycc := &image.YCbCr{
		Y: pic.Y, Cb: pic.Cb, Cr: pic.Cr,
		YStride: pic.YStride, CStride: pic.CStride,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, pic.Width, pic.Height),
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.LimitedRange}, nil
}

func extractAllVideoES(r io.ReaderAt, size int64) ([]byte, error) {
	const chunk = 256 * 1024
	var out bytes.Buffer
	for off := int64(0); off < size; off += chunk {
		buf := make([]byte, chunk)
		n, _ := ioat.ReadRange(r, size, off, buf)
		if n == 0 {
			break
		}
		data := buf[:n]
		for i := 0; i+6 <= len(data); i++ {
			if data[i] != 0 || data[i+1] != 0 || data[i+2] != 1 {
				continue
			}
			sid := data[i+3]
			if sid < 0xE0 || sid > 0xEF {
				continue
			}
			pesLen := int(data[i+4])<<8 | int(data[i+5])
			end := i + 6 + pesLen
			if pesLen == 0 || end > len(data) {
				continue
			}
			pes := data[i:end]
			if body := pesBody(pes); len(body) > 0 {
				out.Write(body)
			}
		}
	}
	if out.Len() == 0 {
		return nil, errs.Unsup("thumbnail", "mpegps: no video payload")
	}
	return out.Bytes(), nil
}

func pesBody(pes []byte) []byte {
	if len(pes) < 9 {
		return pes
	}
	h := 9 + int(pes[8])
	if h > len(pes) {
		return nil
	}
	return pes[h:]
}
