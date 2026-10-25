package h264

import (
	"encoding/binary"

	"github.com/abema/go-mp4"
)

// prependParameterSets builds an AVC sample stream: SPS/PPS length-prefixed NALs + sample.
func prependParameterSets(avcc *mp4.AVCDecoderConfiguration, sample []byte) []byte {
	nalLen := int(avcc.LengthSizeMinusOne) + 1
	if nalLen < 1 || nalLen > 4 {
		nalLen = 4
	}
	out := make([]byte, 0, len(sample)+256)
	for _, sps := range avcc.SequenceParameterSets {
		out = appendNAL(out, nalLen, sps.NALUnit)
	}
	for _, pps := range avcc.PictureParameterSets {
		out = appendNAL(out, nalLen, pps.NALUnit)
	}
	out = append(out, sample...)
	return out
}

func appendNAL(dst []byte, nalLen int, nal []byte) []byte {
	if len(nal) == 0 {
		return dst
	}
	var hdr [4]byte
	switch nalLen {
	case 1:
		dst = append(dst, byte(len(nal)))
	case 2:
		binary.BigEndian.PutUint16(hdr[:2], uint16(len(nal)))
		dst = append(dst, hdr[:2]...)
	case 3:
		hdr[0] = byte(len(nal) >> 16)
		binary.BigEndian.PutUint16(hdr[1:3], uint16(len(nal)))
		dst = append(dst, hdr[:3]...)
	default:
		binary.BigEndian.PutUint32(hdr[:], uint32(len(nal)))
		dst = append(dst, hdr[:4]...)
	}
	return append(dst, nal...)
}
