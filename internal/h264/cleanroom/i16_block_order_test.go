package cleanroom

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
)

// annexBToAVC converts an Annex B elementary stream into 4-byte length-prefixed NAL units.
func annexBToAVC(b []byte) []byte {
	var out []byte
	for _, p := range bytes.Split(b, []byte{0, 0, 1}) {
		p = bytes.TrimRight(p, "\x00")
		if len(p) == 0 {
			continue
		}
		out = binary.BigEndian.AppendUint32(out, uint32(len(p)))
		out = append(out, p...)
	}
	return out
}

// TestIntra16x16BlockOrderMatchesFFmpeg decodes high-QP x264 IDRs (mostly
// I_16x16 macroblocks) and requires bit-exact output against ffmpeg. The 4x4
// residual blocks must be placed in raster order; placing them in
// luma4x4BlkIdx order swapped blocks 2/3, 4/8, ... and darkened thumbnails.
func TestIntra16x16BlockOrderMatchesFFmpeg(t *testing.T) {
	cases := map[string]string{
		// ffmpeg -f lavfi -i testsrc2=s=128x64:r=1:d=1,format=yuv420p -c:v libx264 -qp 44 -coder {1,0}
		// -bsf:v filter_units=remove_types=6 -f h264; sha256 of ffmpeg's yuv420p decode.
		"testdata/i16_qp44_cabac.h264": "d3afbe154b2e864ec38ef3249b4270a8367571a28f72ef7bb4f91a661d88b41b",
		"testdata/i16_qp44_cavlc.h264": "91df85e56eef648576022f545187de462979504e7c1f0d638a08d97205ca9826",
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			frames, err := New().DecodeIDRAVC(annexBToAVC(raw))
			if err != nil {
				t.Fatal(err)
			}
			y, cb, cr := frames[0].CroppedYUV()
			h := sha256.New()
			h.Write(y)
			h.Write(cb)
			h.Write(cr)
			if got := hex.EncodeToString(h.Sum(nil)); got != want {
				t.Fatalf("decoded YUV sha256 = %s, want ffmpeg %s", got, want)
			}
		})
	}
}
