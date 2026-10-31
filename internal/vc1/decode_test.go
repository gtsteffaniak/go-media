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
	sum := sha256.Sum256(pic.Y)
	got := hex.EncodeToString(sum[:])
	const want = "27f0dc0be6207fb31cd86c90eab16fa6a6c5ee5339cd34593366125e2f298b35"
	if got != want {
		t.Fatalf("Y SHA256\ngot  %s\nwant %s", got, want)
	}
}

func FuzzDecodeKeyframe(f *testing.F) {
	f.Add([]byte{0x4d, 0xf9, 0x0a, 0x11}, []byte{0x10, 0x08})
	f.Fuzz(func(t *testing.T, config, sample []byte) {
		_, _ = DecodeKeyframe(config, sample, 360, 288)
	})
}
