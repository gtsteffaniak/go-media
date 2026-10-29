// Package cleanroom implements decode-only H.264 intra for thumbnails.
// Slice/MB decode is adapted from mgvs/go-openh264 (BSD-2); see NOTICE-mgvs and THIRD_PARTY.md.
package cleanroom

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

// Decoder holds parsed SPS/PPS and decodes IDR access units.
type Decoder struct {
	sps         map[uint32]*decode.SPS
	pps         map[uint32]*decode.PPS
	SkipDeblock bool
	pic         *decode.Frame // reused coded buffer when resolution unchanged
	rbspScratch []byte        // grown per NAL for zero-alloc EBSP→RBSP
	mbCovered   []bool        // per-MB coverage of the picture being decoded
	mbCount     int           // number of true entries in mbCovered
}

// New creates a decoder with empty parameter-set maps.
func New() *Decoder {
	return &Decoder{
		sps: map[uint32]*decode.SPS{},
		pps: map[uint32]*decode.PPS{},
	}
}

// ErrNoPicture means no decodable picture was found.
var ErrNoPicture = errors.New("h264 cleanroom: no decodable picture")

// ErrIncompletePicture means the slices of the IDR picture did not cover every
// macroblock, so the picture would contain undecoded (stale or blank) samples.
var ErrIncompletePicture = errors.New("h264 cleanroom: picture is missing macroblocks")

// DecodeIDRAVC decodes the first IDR picture from AVC length-prefixed NALs.
// A picture is returned only when every macroblock was decoded; on any error
// (including a recovered panic) the pooled picture buffer is dropped so stale
// samples can never surface in a later decode.
func (d *Decoder) DecodeIDRAVC(data []byte) (frames []*decode.Frame, err error) {
	defer func() {
		if p := recover(); p != nil {
			frames = nil
			err = fmt.Errorf("h264 cleanroom: internal error: %v", p)
		}
		if err != nil {
			d.pic = nil
		}
	}()
	nalus, err := extractAVCNalus(data)
	if err != nil {
		return nil, err
	}
	fr, err := d.decodeNALUs(nalus, true)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, ErrNoPicture
	}
	return []*decode.Frame{fr}, nil
}

func (d *Decoder) decodeNALUs(nalus [][]byte, idrOnly bool) (*decode.Frame, error) {
	var frame *decode.Frame
	var lastHdr *decode.SliceHeader
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		if len(d.rbspScratch) < len(nalu) {
			d.rbspScratch = make([]byte, len(nalu))
		}
		u, ok := nal.ParseInto(nalu, d.rbspScratch)
		if !ok {
			continue
		}
		switch u.Type {
		case nal.TypeSPS:
			if s, err := decode.ParseSPS(u.RBSP); err == nil {
				d.sps[s.ID] = s
			}
		case nal.TypePPS:
			if p, err := decode.ParsePPS(u.RBSP); err == nil {
				d.pps[p.ID] = p
			}
		case nal.TypeIDR:
			fr, hdr, err := d.decodeSliceUnit(u, frame, lastHdr)
			if err != nil {
				return nil, err
			}
			frame, lastHdr = fr, hdr
		case nal.TypeNonIDR:
			if idrOnly {
				continue
			}
			fr, hdr, err := d.decodeSliceUnit(u, frame, lastHdr)
			if err != nil {
				return nil, err
			}
			frame, lastHdr = fr, hdr
		}
	}
	if frame != nil && d.mbCount != frame.MbWidth*frame.MbHeight {
		return nil, fmt.Errorf("%w: decoded %d of %d", ErrIncompletePicture, d.mbCount, frame.MbWidth*frame.MbHeight)
	}
	return frame, nil
}

// markCovered records macroblocks [first, first+n) as decoded for the current picture.
func (d *Decoder) markCovered(first uint32, n int) {
	end := int(first) + n
	if end > len(d.mbCovered) {
		end = len(d.mbCovered)
	}
	for i := int(first); i < end; i++ {
		if !d.mbCovered[i] {
			d.mbCovered[i] = true
			d.mbCount++
		}
	}
}

func (d *Decoder) decodeSliceUnit(u nal.Unit, frame *decode.Frame, lastHdr *decode.SliceHeader) (*decode.Frame, *decode.SliceHeader, error) {
	pps := d.pps[slicePPSID(u.RBSP)]
	if pps == nil {
		return frame, lastHdr, nil
	}
	sps := d.sps[pps.SPSID]
	if sps == nil {
		return frame, lastHdr, nil
	}
	if frame != nil && sliceFirstMB(u.RBSP) == 0 {
		return frame, lastHdr, nil
	}
	if frame == nil {
		var allocErr error
		frame, allocErr = d.pictureForSPS(sps)
		if allocErr != nil {
			return nil, lastHdr, allocErr
		}
		total := frame.MbWidth * frame.MbHeight
		if cap(d.mbCovered) < total {
			d.mbCovered = make([]bool, total)
		} else {
			d.mbCovered = d.mbCovered[:total]
			clear(d.mbCovered)
		}
		d.mbCount = 0
	} else if !pictureDimsMatch(frame, sps) {
		return nil, lastHdr, errors.New("h264 cleanroom: slice parameter sets change picture geometry mid-picture")
	}
	h, err := decode.DecodeSlice(u.RBSP, u.Type, u.RefIDC, sps, pps, frame, d.SkipDeblock)
	if err != nil {
		// Any failing slice invalidates the whole picture: returning a partial
		// frame would expose undecoded (stale) samples as a success.
		return nil, h, err
	}
	d.markCovered(h.FirstMB, h.DecodedMBs)
	return frame, h, nil
}

func slicePPSID(rbsp []byte) uint32 {
	r := bitstream.NewReader(rbsp)
	r.UE()
	r.UE()
	return r.UE()
}

func sliceFirstMB(rbsp []byte) uint32 {
	return bitstream.NewReader(rbsp).UE()
}

func (d *Decoder) pictureForSPS(sps *decode.SPS) (*decode.Frame, error) {
	if d.pic != nil && pictureDimsMatch(d.pic, sps) {
		return d.pic, nil
	}
	f, err := decode.NewFrame(sps)
	if err != nil {
		return nil, err
	}
	d.pic = f
	return d.pic, nil
}

// Reset clears per-stream parameter sets so a pooled decoder cannot reuse SPS/PPS
// from a previous access unit. The coded picture buffer is kept for allocation
// reuse; this is safe because DecodeIDRAVC only returns a picture after every
// macroblock has been rewritten and drops the buffer on any error.
func (d *Decoder) Reset() {
	clear(d.sps)
	clear(d.pps)
	d.mbCount = 0
}

// DropPicture discards the pooled coded picture buffer so its samples cannot
// be reused by a later decode.
func (d *Decoder) DropPicture() {
	d.pic = nil
}

func pictureDimsMatch(f *decode.Frame, s *decode.SPS) bool {
	frameMbsOnly := 1
	if !s.FrameMbsOnly {
		frameMbsOnly = 0
	}
	mbH := int(s.HeightMapUnits) * (2 - frameMbsOnly)
	return f.MbWidth == int(s.WidthMbs) &&
		f.MbHeight == mbH &&
		f.CropWidth == s.Width &&
		f.CropHeight == s.Height &&
		f.ChromaArrayType == s.ChromaArrayType()
}

func extractAVCNalus(data []byte) ([][]byte, error) {
	var nalus [][]byte
	for len(data) >= 4 {
		length := int(binary.BigEndian.Uint32(data[:4]))
		data = data[4:]
		if length < 0 || length > len(data) {
			return nil, errors.New("avc: invalid NALU length")
		}
		nalus = append(nalus, data[:length])
		data = data[length:]
	}
	return nalus, nil
}
