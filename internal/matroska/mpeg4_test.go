package matroska

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/mpeg4"
)

func picI420(p *mpeg4.Picture) []byte {
	var out []byte
	for y := 0; y < p.Height; y++ {
		out = append(out, p.Y[y*p.YStride:y*p.YStride+p.Width]...)
	}
	for _, c := range [][]byte{p.Cb, p.Cr} {
		for y := 0; y < (p.Height+1)/2; y++ {
			out = append(out, c[y*p.CStride:y*p.CStride+(p.Width+1)/2]...)
		}
	}
	return out
}

// TestMPEG4PictureAtSeek checks V_MPEG4/ISO/ASP keyframes (VOL from
// CodecPrivate) against ffmpeg's decode of the same I-frames. The file is
// testdata/mpeg4_asp_176x144.mp4 remuxed with ffmpeg -c copy.
func TestMPEG4PictureAtSeek(t *testing.T) {
	f, err := os.Open("../../testdata/mpeg4_asp_176x144.mkv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	h, err := ProbeHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		seek float64
		want string
	}{
		{1, "f3b4581e3eae1e3b779602ee5f98209c096d08a7cefc7304d36362934f5e8a14"},
		{50, "6579ac6675ca92a889b8de997590f60f8fc0d4bd5944b9aa73b4f24a5a8b5da1"},
		{100, "9c49ba8d70771aad42af7d8f9e1290c3bc43ae61e544e6c97480b1efee507b82"},
	} {
		pic, err := MPEG4PictureAtSeek(f, st.Size(), h, tc.seek)
		if err != nil || pic == nil {
			t.Fatalf("seek %v: %v", tc.seek, err)
		}
		sum := sha256.Sum256(picI420(pic))
		if got := hex.EncodeToString(sum[:]); got != tc.want {
			t.Errorf("seek %v: sha256 %s, want %s", tc.seek, got, tc.want)
		}
	}
	if !IsMPEG4CodecID("V_MPEG4/ISO/ASP") || IsMPEG4CodecID("V_MPEG4/ISO/AVC") {
		t.Fatal("IsMPEG4CodecID")
	}
}
