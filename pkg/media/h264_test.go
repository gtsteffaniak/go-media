package media_test

import (
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// TestDecodeH264KeyframeInvalid is a test.
func TestDecodeH264KeyframeInvalid(t *testing.T) {
	_, _, _, err := media.DecodeH264Keyframe([]byte{0x00}, 16, 16)
	if err == nil {
		t.Fatal("expected error")
	}
}
