// Package cleanroom implements decode-only H.264 intra for thumbnails.
// Slice/MB decode is adapted from mgvs/go-openh264 (BSD-2); see NOTICE-mgvs and THIRD_PARTY.md.
package cleanroom

import (
	"encoding/binary"
	"errors"

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

// DecodeIDRAVC decodes the first IDR picture from AVC length-prefixed NALs.
func (d *Decoder) DecodeIDRAVC(data []byte) ([]*decode.Frame, error) {
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
		u, ok := nal.Parse(nalu)
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
				return frame, err
			}
			frame, lastHdr = fr, hdr
		}
	}
	return frame, nil
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
		frame = d.pictureForSPS(sps)
	}
	h, err := decode.DecodeSlice(u.RBSP, u.Type, u.RefIDC, sps, pps, frame, d.SkipDeblock)
	if err != nil {
		if lastHdr == nil {
			return nil, h, err
		}
		return frame, lastHdr, nil
	}
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

func (d *Decoder) pictureForSPS(sps *decode.SPS) *decode.Frame {
	if d.pic != nil && pictureDimsMatch(d.pic, sps) {
		return d.pic
	}
	d.pic = decode.NewFrame(sps)
	return d.pic
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
