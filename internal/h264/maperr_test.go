package h264

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
)

// An open-GOP picture that still holds P/B slices must fail fast as
// unsupported, not as a generic decode error.
func TestMapDecodeErrInterSliceUnsupported(t *testing.T) {
	err := mapDecodeErr(fmt.Errorf("slice 1: %w", decode.ErrInterSlice))
	if !errors.Is(err, errs.UnsupportedProfile) {
		t.Fatalf("got %v, want ErrUnsupportedProfile", err)
	}
}
