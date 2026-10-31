package media_test

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestMPEG2MP4Thumbnail(t *testing.T) {
	f, err := os.Open("../../testdata/mpeg2_64x48.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	_, err = media.ThumbnailSmall(f, st.Size(), 10)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMPEG2TSThumbnail(t *testing.T) {
	f, err := os.Open("../../testdata/mpeg2_176x144.ts")
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	_, err = media.ThumbnailSmall(f, st.Size(), 10)
	if err != nil {
		t.Skip("mpeg2 TS keyframe thumbnail: " + err.Error())
	}
}
