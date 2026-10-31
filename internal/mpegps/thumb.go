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

// extractAllVideoES walks pack headers and PES packets in order and returns
// the concatenated video elementary stream. Both MPEG-1 system streams (as
// written by `ffmpeg -f mpeg`) and MPEG-2 program streams are handled, and
// packets that straddle read chunks are carried over to the next chunk.
func extractAllVideoES(r io.ReaderAt, size int64) ([]byte, error) {
	const chunk = 256 * 1024
	var out bytes.Buffer
	var carry []byte
	for off := int64(0); off < size; {
		buf := make([]byte, chunk)
		n, _ := ioat.ReadRange(r, size, off, buf)
		if n == 0 {
			break
		}
		off += int64(n)
		data := append(carry, buf[:n]...)
		consumed := walkPS(data, &out, off >= size)
		carry = append([]byte(nil), data[consumed:]...)
	}
	if out.Len() == 0 {
		return nil, errs.Unsup("thumbnail", "mpegps: no video payload")
	}
	return out.Bytes(), nil
}

// walkPS appends video payloads found in data to out and returns how many
// bytes were fully consumed. Unless final is set, an incomplete trailing
// pack or packet is left unconsumed for the next call.
func walkPS(data []byte, out *bytes.Buffer, final bool) int {
	i := 0
	for i+4 <= len(data) {
		if data[i] != 0 || data[i+1] != 0 || data[i+2] != 1 {
			i++
			continue
		}
		code := data[i+3]
		switch {
		case code == 0xB9: // program end
			i += 4
		case code == 0xBA: // pack header
			if i+5 > len(data) {
				return partial(i, len(data), final)
			}
			hl := 12                    // MPEG-1
			if data[i+4]&0xC0 == 0x40 { // MPEG-2
				if i+14 > len(data) {
					return partial(i, len(data), final)
				}
				hl = 14 + int(data[i+13]&7)
			}
			if i+hl > len(data) {
				return partial(i, len(data), final)
			}
			i += hl
		case code >= 0xBB: // system header or PES packet
			if i+6 > len(data) {
				return partial(i, len(data), final)
			}
			end := i + 6 + (int(data[i+4])<<8 | int(data[i+5]))
			if end > len(data) {
				return partial(i, len(data), final)
			}
			if code >= 0xE0 && code <= 0xEF {
				if body := pesBody(data[i+6 : end]); len(body) > 0 {
					out.Write(body)
				}
			}
			i = end
		default:
			i++
		}
	}
	return partial(i, len(data), final)
}

func partial(i, n int, final bool) int {
	if final {
		return n
	}
	return i
}

// pesBody strips an MPEG-2 or MPEG-1 PES header from the bytes after
// packet_length.
func pesBody(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	if b[0]&0xC0 == 0x80 { // MPEG-2 PES header
		if len(b) < 3 || 3+int(b[2]) > len(b) {
			return nil
		}
		return b[3+int(b[2]):]
	}
	i := 0
	for i < len(b) && i < 16 && b[i] == 0xFF { // stuffing
		i++
	}
	if i < len(b) && b[i]&0xC0 == 0x40 { // STD buffer
		i += 2
	}
	if i >= len(b) {
		return nil
	}
	switch b[i] & 0xF0 {
	case 0x20:
		i += 5 // PTS
	case 0x30:
		i += 10 // PTS + DTS
	default:
		if b[i] == 0x0F {
			i++
		}
	}
	if i > len(b) {
		return nil
	}
	return b[i:]
}
