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

// ParseAVCConfigRecord parses a raw AVCDecoderConfigurationRecord (ISO/IEC
// 14496-15 §5.3.3.1), the payload of an MP4 avcC box, Matroska
// V_MPEG4/ISO/AVC CodecPrivate or AVI strf extradata. It returns nil when b
// is not a version 1 record or a parameter set runs past the end. Trailing
// High-profile fields (chroma format, bit depths, SPS extensions) are ignored;
// the decoder reads them from the SPS itself.
func ParseAVCConfigRecord(b []byte) *mp4.AVCDecoderConfiguration {
	if len(b) < 6 || b[0] != 1 {
		return nil
	}
	cfg := &mp4.AVCDecoderConfiguration{
		ConfigurationVersion: b[0],
		Profile:              b[1],
		ProfileCompatibility: b[2],
		Level:                b[3],
		LengthSizeMinusOne:   b[4] & 3,
	}
	p := 5
	readSets := func(n int) ([]mp4.AVCParameterSet, bool) {
		sets := make([]mp4.AVCParameterSet, 0, n)
		for range n {
			if p+2 > len(b) {
				return nil, false
			}
			l := int(binary.BigEndian.Uint16(b[p:]))
			p += 2
			if p+l > len(b) {
				return nil, false
			}
			sets = append(sets, mp4.AVCParameterSet{Length: uint16(l), NALUnit: b[p : p+l : p+l]})
			p += l
		}
		return sets, true
	}
	nSPS := int(b[p] & 0x1f)
	p++
	sps, ok := readSets(nSPS)
	if !ok {
		return nil
	}
	if p >= len(b) {
		return nil
	}
	nPPS := int(b[p])
	p++
	pps, ok := readSets(nPPS)
	if !ok {
		return nil
	}
	cfg.NumOfSequenceParameterSets = uint8(nSPS)
	cfg.SequenceParameterSets = sps
	cfg.NumOfPictureParameterSets = uint8(nPPS)
	cfg.PictureParameterSets = pps
	return cfg
}
