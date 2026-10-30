package isobmff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

func yccI420(t *h264.ThumbYCbCr) []byte {
	y := t.YCbCr
	w, h := y.Rect.Dx(), y.Rect.Dy()
	var out []byte
	for r := 0; r < h; r++ {
		out = append(out, y.Y[r*y.YStride:r*y.YStride+w]...)
	}
	for _, p := range [][]byte{y.Cb, y.Cr} {
		for r := 0; r < (h+1)/2; r++ {
			out = append(out, p[r*y.CStride:r*y.CStride+(w+1)/2]...)
		}
	}
	return out
}

// TestMPEG4ThumbnailMatchesFFmpeg checks the mp4v keyframe picked at each
// seek against ffmpeg. The file was made with
//
//	ffmpeg -i src -t 2 -vf scale=176:144 -c:v mpeg4 -q:v 5 -g 12 -bf 1 out.mp4
//
// and each hash equals one I-frame of
//
//	ffmpeg -i out.mp4 -vf "select='eq(pict_type,I)'" -f rawvideo -pix_fmt yuv420p
func TestMPEG4ThumbnailMatchesFFmpeg(t *testing.T) {
	f, err := os.Open("../../testdata/mpeg4_asp_176x144.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	for _, tc := range []struct {
		seek float64
		want string
	}{
		{0, "a5e740399108bb284a707a1cfa09b4e7dbbb330d06c7ff43c23fcee313ae26d2"},
		{50, "d7668be3a9b852a7af192b6b1f66a6ff1368e64d2f2945d1dd7328ed024414b4"},
		{100, "9c49ba8d70771aad42af7d8f9e1290c3bc43ae61e544e6c97480b1efee507b82"},
	} {
		src, err := OpenThumbSourceErr(f, st.Size())
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := src.MJPEGFrame(tc.seek); ok {
			t.Fatal("MPEG-4 Part 2 track taken as MJPEG")
		}
		img, err := src.MPEG4ThumbnailYCbCr(tc.seek)
		if err != nil || img == nil {
			t.Fatalf("seek %v: %v", tc.seek, err)
		}
		sum := sha256.Sum256(yccI420(img))
		if got := hex.EncodeToString(sum[:]); got != tc.want {
			t.Errorf("seek %v: sha256 %s, want %s", tc.seek, got, tc.want)
		}
	}
}

// TestMPEG4OtherObjectType checks an mp4v track holding MPEG-2 video
// (objectTypeIndication 0x61) names that codec.
func TestMPEG4OtherObjectType(t *testing.T) {
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
	_, err = src.MPEG4ThumbnailYCbCr(10)
	if !errors.Is(err, errs.Unsupported) || !strings.Contains(err.Error(), "MPEG-2 video") {
		t.Fatalf("got %v", err)
	}
}

func TestMPEG4NoTrack(t *testing.T) {
	f, err := os.Open("../../testdata/red.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	src, err := OpenThumbSourceErr(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if img, err := src.MPEG4ThumbnailYCbCr(10); img != nil || err != nil {
		t.Fatalf("H.264 file: got %v, %v", img, err)
	}
}

func TestESDSConfig(t *testing.T) {
	// version/flags, ES_Descriptor(ES_ID=1, flags 0), DecoderConfig(oti 0x20,
	// 12 more bytes), DecoderSpecificInfo {1,2,3}, with a 2-byte size form.
	esds := []byte{0, 0, 0, 0,
		0x03, 0x80, 23, 0, 1, 0,
		0x04, 18, 0x20, 0x11, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0x05, 3, 1, 2, 3,
	}
	oti, dsi, err := esdsConfig(esds)
	if err != nil || oti != 0x20 || string(dsi) != "\x01\x02\x03" {
		t.Fatalf("got %x %v %v", oti, dsi, err)
	}
	for i := 0; i < len(esds); i++ {
		_, _, _ = esdsConfig(esds[:i]) // must not panic
	}
	if _, _, err := esdsConfig([]byte{0, 0, 0, 0, 0x03, 0x7f}); err == nil {
		t.Fatal("oversized descriptor accepted")
	}
}
