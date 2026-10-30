package media_test

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// TestMPEG4Part2Thumbnail runs the public thumbnail path on MPEG-4 Part 2
// (mp4v in MP4, V_MPEG4/ISO/ASP in Matroska) at several seeks.
func TestMPEG4Part2Thumbnail(t *testing.T) {
	for _, file := range []string{"mpeg4_asp_176x144.mp4", "mpeg4_asp_176x144.mkv"} {
		for _, seek := range []float64{0, 10, 50, 100} {
			f, err := os.Open("../../testdata/" + file)
			if err != nil {
				t.Fatal(err)
			}
			st, _ := f.Stat()
			if _, err := media.ThumbnailSmall(f, st.Size(), seek); err != nil {
				t.Errorf("%s@%v: %v", file, seek, err)
			}
			f.Close()
		}
	}
}
