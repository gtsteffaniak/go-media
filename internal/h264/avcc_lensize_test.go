package h264

import (
	"bytes"
	"testing"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
)

// lenPrefixed packs NAL units with lenSize-byte big-endian length prefixes.
func lenPrefixed(lenSize int, nalus ...[]byte) []byte {
	var out []byte
	for _, n := range nalus {
		out = appendNAL(out, lenSize, n)
	}
	return out
}

// TestDecodeAVCCFrameNALLengthSize checks that a keyframe whose avcC declares
// 2- or 3-byte NAL lengths decodes to the same picture as the 4-byte form.
// ffmpeg splits NALs with nal_length_size from avcC; a decoder that assumed 4
// bytes would accept the SPS and then fail on the sample.
func TestDecodeAVCCFrameNALLengthSize(t *testing.T) {
	spsRBSP := decode.BaselineSPSRBSP(66, 30, 1, 0, 0, false)
	sps, err := decode.ParseSPS(spsRBSP)
	if err != nil {
		t.Fatal(err)
	}
	spsNAL := append([]byte{0x67}, spsRBSP...)
	ppsNAL := append([]byte{0x68}, decode.BaselinePPSRBSP(0, 0, false)...)
	slices := [][]byte{
		append([]byte{0x65}, decode.BuildIPCMSliceRBSPRange(sps, 0, 0, 1, 0x40, false, 0)...),
		append([]byte{0x65}, decode.BuildIPCMSliceRBSPRange(sps, 0, 1, 1, 0x80, false, 0)...),
	}
	decodeWith := func(lenSize int) []byte {
		t.Helper()
		avcc := &mp4.AVCDecoderConfiguration{
			LengthSizeMinusOne:    uint8(lenSize - 1),
			SequenceParameterSets: []mp4.AVCParameterSet{{Length: uint16(len(spsNAL)), NALUnit: spsNAL}},
			PictureParameterSets:  []mp4.AVCParameterSet{{Length: uint16(len(ppsNAL)), NALUnit: ppsNAL}},
		}
		fr, err := DecodeAVCCFrame(avcc, lenPrefixed(lenSize, slices...), true)
		if err != nil {
			t.Fatalf("lenSize %d: %v", lenSize, err)
		}
		return append([]byte(nil), fr.Y...)
	}
	want := decodeWith(4)
	for _, n := range []int{2, 3} {
		if got := decodeWith(n); !bytes.Equal(got, want) {
			t.Fatalf("lenSize %d: luma differs from 4-byte decode", n)
		}
	}
}
