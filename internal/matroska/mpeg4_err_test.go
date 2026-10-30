package matroska

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// TestMPEG4ThumbErr checks that corrupt decoder errors become Unsupported
// (so callers fall back to ffmpeg) while ProfileError identity is kept.
func TestMPEG4ThumbErr(t *testing.T) {
	c := mpeg4ThumbErr(fmt.Errorf("%w: mpeg4: truncated macroblock", errs.Corrupt))
	var ue *errs.UnsupportedError
	if !errors.As(c, &ue) || !strings.Contains(ue.Reason, "truncated macroblock") {
		t.Fatalf("corrupt: got %v, want Unsupported with reason", c)
	}
	p := errs.UnsupProfile("mpeg4", "B-VOP")
	if got := mpeg4ThumbErr(p); got != p {
		t.Fatalf("profile error identity lost: %v", got)
	}
}
