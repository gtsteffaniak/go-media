package h264

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
)

func TestBorrowDecoderClearsParameterSets(t *testing.T) {
	stream1 := minimalIDRAccessUnit(0, 0)
	dec := borrowDecoder(true)
	frames, err := dec.DecodeIDRAVC(stream1)
	releaseDecoder(dec)
	if err != nil || len(frames) == 0 {
		t.Fatalf("first decode: frames=%d err=%v", len(frames), err)
	}

	// Second stream: IDR references PPS 1 but defines no PPS NAL.
	stream2 := idrOnlyAccessUnit(1)
	dec2 := borrowDecoder(true)
	_, err2 := dec2.DecodeIDRAVC(stream2)
	releaseDecoder(dec2)
	if err2 == nil {
		t.Fatal("expected error decoding IDR without parameter sets")
	}
}

func minimalIDRAccessUnit(ppsID, spsID uint32) []byte {
	sps := decode.BaselineSPSRBSP(66, 30, 0, 0, 0, false)
	pps := decode.BaselinePPSRBSP(ppsID, spsID, false)
	spsNAL := append([]byte{0x67}, sps...)
	ppsNAL := append([]byte{0x68}, pps...)
	idr := minimalIDRRBSP(ppsID)
	idrNAL := append([]byte{0x65}, idr...)
	return avccPack(spsNAL, ppsNAL, idrNAL)
}

func idrOnlyAccessUnit(ppsID uint32) []byte {
	idr := minimalIDRRBSP(ppsID)
	idrNAL := append([]byte{0x65}, idr...)
	return avccPack(idrNAL)
}

func minimalIDRRBSP(ppsID uint32) []byte {
	sps, err := decode.ParseSPS(decode.BaselineSPSRBSP(66, 30, 0, 0, 0, false))
	if err != nil {
		panic(err)
	}
	return decode.BuildIPCMSliceRBSP(sps, ppsID)
}

func avccPack(nalus ...[]byte) []byte {
	var out []byte
	for _, n := range nalus {
		ln := len(n)
		out = append(out, byte(ln>>24), byte(ln>>16), byte(ln>>8), byte(ln))
		out = append(out, n...)
	}
	return out
}
