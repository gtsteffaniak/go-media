package avi

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

func TestSniffAVI(t *testing.T) {
	data, err := os.ReadFile("testdata/mjpeg.avi")
	if err != nil {
		t.Fatal(err)
	}
	id, err := sniff.Detect(bytesReaderAt{data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if id != format.AVI {
		t.Fatalf("got %q want avi", id)
	}
}

type bytesReaderAt struct{ b []byte }

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.b)) {
		return 0, io.EOF
	}
	n := copy(p, b.b[off:])
	if off+int64(n) >= int64(len(b.b)) {
		return n, io.EOF
	}
	return n, nil
}

func TestProbeFixtures(t *testing.T) {
	cases := map[string]string{
		"testdata/mjpeg.avi": "mjpeg",
		"testdata/h264.avi":  "h264",
		"testdata/xvid.avi":  "mpeg4",
	}
	for path, codec := range cases {
		t.Run(path, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			st, _ := f.Stat()
			streams, err := Streams(f, st.Size())
			if err != nil {
				t.Fatal(err)
			}
			if len(streams) != 1 || streams[0].Codec != codec {
				t.Fatalf("streams=%+v want codec %s", streams, codec)
			}
			dr, err := Duration(f, st.Size())
			if err != nil {
				t.Fatal(err)
			}
			if dr.Duration < time.Second/2 || dr.Duration > 2*time.Second {
				t.Fatalf("duration %v", dr.Duration)
			}
		})
	}
}

func TestThumbSourceDecodes(t *testing.T) {
	for _, path := range []string{"testdata/mjpeg.avi", "testdata/h264.avi", "testdata/xvid.avi"} {
		t.Run(path, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			st, _ := f.Stat()
			src, err := OpenThumbSourceErr(f, st.Size())
			if err != nil {
				t.Fatal(err)
			}
			if b, ok := src.MJPEGFrame(10); ok {
				if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
					t.Fatalf("not jpeg prefix % x", b[:4])
				}
				return
			}
			if y, err := src.H264ThumbnailYCbCr(10, true); err != nil {
				t.Fatal(err)
			} else if y != nil {
				return
			}
			if y, err := src.MPEG4ThumbnailYCbCr(10); err != nil {
				t.Fatal(err)
			} else if y != nil {
				return
			}
			t.Fatal("no decoder path produced output")
		})
	}
}

func TestIdx1KeyframeSeek(t *testing.T) {
	f, err := os.Open("testdata/h264.avi")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	idx, err := openIndex(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	ent, ok := idx.pickKeyframe(2)
	if !ok {
		t.Fatal("no keyframe")
	}
	if !ent.keyframe {
		t.Fatal("wanted keyframe flag")
	}
}
