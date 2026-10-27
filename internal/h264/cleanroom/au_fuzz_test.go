package cleanroom

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
)

// FuzzDecodeIDRAVC exercises AVC length-prefixed IDR access unit decoding.
func FuzzDecodeIDRAVC(f *testing.F) {
	f.Add(seedMinimalIDRAVC())
	f.Fuzz(func(t *testing.T, data []byte) {
		d := New()
		_, _ = d.DecodeIDRAVC(data)
	})
}

func seedMinimalIDRAVC() []byte {
	sps := decode.BaselineSPSRBSP(66, 30, 0, 0, 0, false)
	pps := decode.BaselinePPSRBSP(0, 0, false)
	spsNAL := append([]byte{0x67}, sps...)
	ppsNAL := append([]byte{0x68}, pps...)
	// Minimal IDR slice header only (no slice data); decode may fail without picture.
	idr := []byte{0x65, 0x88, 0x80}
	return packAVCCSample(spsNAL, ppsNAL, idr)
}

func packAVCCSample(nalus ...[]byte) []byte {
	var out []byte
	for _, n := range nalus {
		out = append(out, byte(len(n)>>24), byte(len(n)>>16), byte(len(n)>>8), byte(len(n)))
		out = append(out, n...)
	}
	return out
}
