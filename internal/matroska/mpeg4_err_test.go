package matroska

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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

// TestMPEG4PictureAtSeekTruncated cuts the MPEG-4 fixture at several points
// after the header and checks that MPEG4PictureAtSeek never returns a raw
// demux error: either a picture, or errs.Unsupported so callers fall back to
// ffmpeg.
func TestMPEG4PictureAtSeekTruncated(t *testing.T) {
	data, err := os.ReadFile("../../testdata/mpeg4_asp_176x144.mkv")
	if err != nil {
		t.Fatal(err)
	}
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	sawErr := false
	for cut := len(data) / 8; cut < len(data); cut += len(data) / 16 {
		r := bytes.NewReader(data[:cut])
		_, err := MPEG4PictureAtSeek(r, int64(cut), h, 50)
		if err == nil {
			continue
		}
		sawErr = true
		var ue *errs.UnsupportedError
		if !errors.As(err, &ue) {
			t.Fatalf("cut %d: got %T %v, want errs.Unsupported", cut, err, err)
		}
	}
	if !sawErr {
		t.Fatal("no truncation produced an error; test exercises nothing")
	}
}
