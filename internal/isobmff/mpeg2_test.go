package isobmff

import (
	"os"
	"testing"
)

func TestMPEG2MP4Thumbnail(t *testing.T) {
	f, err := os.Open("../../testdata/mpeg2_64x48.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	src, err := OpenThumbSourceErr(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	img, err := src.MPEG2ThumbnailYCbCr(10)
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("nil")
	}
}
