package ts

import (
	"bytes"
	"fmt"
	"image"
	"io"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/mpeg2video"
	"github.com/gtsteffaniak/go-media/internal/mpeg2video/bitreader"
)

// ThumbnailYCbCr demuxes the primary video PID and decodes an I/IDR picture near seekPercent.
func ThumbnailYCbCr(r io.ReaderAt, size int64, meta *Meta, seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	if meta == nil || meta.VideoPID == 0 {
		return nil, errs.Unsup("thumbnail", "mpegts: no decodable video stream")
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	es, err := concatVideoES(r, size, meta)
	if err != nil {
		return nil, err
	}
	switch meta.VideoTyp {
	case 0x02:
		idx := int(meta.Duration * 25.0 * seekPercent / 100.0)
		if idx < 0 {
			idx = 0
		}
		pic, err := mpeg2video.DecodeIFrameIndex(es, idx)
		if err != nil {
			return nil, errs.Unsup("thumbnail", fmt.Sprintf("mpegts: %v (es %d bytes)", err, len(es)))
		}
		return mpeg2Thumb(pic), nil
	case 0x1B:
		fr, err := h264.DecodeAnnexBFrame(es, fast)
		if err != nil {
			return nil, err
		}
		return h264.NewThumbYCbCr(fr), nil
	default:
		return nil, errs.Unsup("thumbnail", "mpegts: video codec "+streamTypeName(meta.VideoTyp))
	}
}

func mpeg2Thumb(pic *mpeg2video.Picture) *h264.ThumbYCbCr {
	ycc := &image.YCbCr{
		Y: pic.Y, Cb: pic.Cb, Cr: pic.Cr,
		YStride: pic.YStride, CStride: pic.CStride,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, pic.Width, pic.Height),
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.LimitedRange}
}

func concatVideoES(r io.ReaderAt, size int64, meta *Meta) ([]byte, error) {
	lay := meta.Layout
	var out bytes.Buffer
	pktBuf := make([]byte, packetSize)
	p := &Packet{}
	var pes []byte
	flush := func() {
		if len(pes) == 0 {
			return
		}
		if body := pesPayload(pes); len(body) > 0 {
			out.Write(body)
		}
		pes = pes[:0]
	}
	for off := int64(0); off+int64(lay.Stride) <= size; off += int64(lay.Stride) {
		rec := make([]byte, lay.Stride)
		n, _ := ioat.ReadRange(r, size, off, rec)
		if n < lay.Stride {
			break
		}
		copy(pktBuf, rec[lay.Offset:lay.Offset+packetSize])
		if pktBuf[0] != SyncByte {
			continue
		}
		br := bitreader.NewReader(bytes.NewReader(pktBuf))
		if err := p.Next(br); err != nil {
			continue
		}
		if uint16(p.PID) != meta.VideoPID {
			continue
		}
		if p.PayloadUnitStartIndicator {
			flush()
			pes = append(pes, p.Payload...)
		} else if len(pes) > 0 {
			pes = append(pes, p.Payload...)
		}
	}
	flush()
	if out.Len() == 0 {
		return nil, errs.Unsup("thumbnail", "mpegts: no video payload")
	}
	return out.Bytes(), nil
}

func pesPayload(pes []byte) []byte {
	if len(pes) < 9 || pes[0] != 0 || pes[1] != 0 || pes[2] != 1 {
		return pes
	}
	hlen := 9 + int(pes[8])
	if hlen > len(pes) {
		return nil
	}
	return pes[hlen:]
}

// Streams maps probe metadata to detect.Stream entries.
func Streams(meta *Meta) []detect.Stream {
	if meta == nil {
		return nil
	}
	out := make([]detect.Stream, 0, len(meta.Streams))
	for _, s := range meta.Streams {
		st := detect.StreamAudio
		if s.Codec == "h264" || s.Codec == "mpeg2video" || s.Codec == "mpeg1video" || s.Codec == "mpeg4" {
			st = detect.StreamVideo
		}
		out = append(out, detect.Stream{
			Index: len(out),
			Type:  st,
			Codec: s.Codec,
		})
	}
	return out
}
