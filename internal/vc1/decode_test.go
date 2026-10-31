package vc1

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestDecodeKeyframe360x288(t *testing.T) {
	config := []byte{0x4d, 0xf9, 0x0a, 0x11}
	sample, err := os.ReadFile("testdata/frame1_360x288.bin")
	if err != nil {
		t.Fatal(err)
	}
	pic, err := DecodeKeyframe(config, sample, 360, 288)
	if err != nil {
		t.Fatal(err)
	}
	if pic.Width != 360 || pic.Height != 288 {
		t.Fatalf("size %dx%d", pic.Width, pic.Height)
	}
	// Golden hashes are of ffmpeg 9.0.2's decode of the same frame
	// (-pix_fmt yuv420p rawvideo); the decoder is bit-exact with it.
	crop := func(p []byte, stride, w, h int) []byte {
		out := make([]byte, 0, w*h)
		for y := 0; y < h; y++ {
			out = append(out, p[y*stride:y*stride+w]...)
		}
		return out
	}
	planes := []struct {
		name string
		data []byte
		want string
	}{
		{"Y", crop(pic.Y, pic.YStride, 360, 288), "7d6a4e00c6333aeddeb52dfac4d965510aa97a60d734630089d812b80facd454"},
		{"Cb", crop(pic.Cb, pic.CStride, 180, 144), "45c0e1bef201b2ac42e9b5c1d25076fe569c1e11dcab7a88cb626e36889517b3"},
		{"Cr", crop(pic.Cr, pic.CStride, 180, 144), "76259ac1bf2a732c48650b66d3a9091aed56e750521a67e86fef41dcc1fd2c3c"},
	}
	for _, p := range planes {
		sum := sha256.Sum256(p.data)
		if got := hex.EncodeToString(sum[:]); got != p.want {
			t.Errorf("%s SHA256\ngot  %s\nwant %s", p.name, got, p.want)
		}
	}
}

func FuzzDecodeKeyframe(f *testing.F) {
	f.Add([]byte{0x4d, 0xf9, 0x0a, 0x11}, []byte{0x10, 0x08})
	f.Fuzz(func(t *testing.T, config, sample []byte) {
		_, _ = DecodeKeyframe(config, sample, 360, 288)
	})
}
