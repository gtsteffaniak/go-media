package media_test

import (
	"errors"
	"testing"

	"github.com/gtsteffaniak/go-media"
)

func TestDecodeH264KeyframeStub(t *testing.T) {
	_, _, _, err := media.DecodeH264Keyframe([]byte{0x00}, 16, 16)
	if !errors.Is(err, media.ErrUnsupportedProfile) {
		t.Fatalf("got %v", err)
	}
}
