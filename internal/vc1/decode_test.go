package vc1

import "testing"

func FuzzDecodeKeyframe(f *testing.F) {
	f.Add([]byte{0x4d, 0xf9, 0x0a, 0x11}, []byte{0x10, 0x08})
	f.Fuzz(func(t *testing.T, config, sample []byte) {
		_, _ = DecodeKeyframe(config, sample)
	})
}
