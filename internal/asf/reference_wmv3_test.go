package asf

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/vc1"
)

const referenceWMV3 = "../../test/data/reference_1080_10s_wmv3.wmv"

// TestReferenceWMV3Demux checks the ASF demuxer against values ffprobe reports
// for the checked-in 1080p reference clip (encoded by Windows MediaTranscoder).
func TestReferenceWMV3Demux(t *testing.T) {
	f, err := os.Open(referenceWMV3)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	meta, err := openMeta(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	v := meta.video
	if v == nil || v.codec != "wmv3" || v.width != 1920 || v.height != 1080 {
		t.Fatalf("video stream = %+v", v)
	}
	if want := []byte{0x4f, 0xf9, 0x48, 0x01, 0x20}; !bytes.Equal(v.extradata, want) {
		t.Fatalf("extradata = % x, want % x", v.extradata, want)
	}
	var n, keys int
	var keyPTS []int64
	err = demuxVideoSamples(newReader(f, st.Size()), meta, func(p payload) error {
		n++
		if p.keyframe {
			keys++
			keyPTS = append(keyPTS, p.pts100ns)
			if keys == 1 && !bytes.HasPrefix(p.data, []byte{0x00, 0x12, 0xb4, 0x0c}) {
				t.Errorf("first keyframe starts % x", p.data[:4])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 302 || keys != 14 {
		t.Fatalf("samples=%d keyframes=%d, want 302 and 14", n, keys)
	}
	if keyPTS[0] != 660000 || keyPTS[1] != 20660000 {
		t.Fatalf("keyframe pts = %v", keyPTS[:2])
	}
}

// TestReferenceWMV3MatchesFFmpeg decodes the keyframe at 2.066s (the last
// keyframe at or before 22% of the clip) and compares
// all three planes with ffmpeg's decode of the same frame; the decoder is
// expected to be bit-exact.
func TestReferenceWMV3MatchesFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	want, err := exec.Command(ff, "-v", "error", "-i", referenceWMV3, "-vf", `select=gte(t\,2.06)`,
		"-frames:v", "1", "-pix_fmt", "yuv420p", "-f", "rawvideo", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(referenceWMV3)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	extra, sample, err := videoKeyframe(f, st.Size(), 22)
	if err != nil {
		t.Fatal(err)
	}
	pic, err := vc1.DecodeKeyframe(extra, sample, 1920, 1080)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 1920*1080*3/2 {
		t.Fatalf("ffmpeg frame is %d bytes", len(want))
	}
	planes := []struct {
		name   string
		got    []byte
		stride int
		w, h   int
		off    int
	}{
		{"Y", pic.Y, pic.YStride, 1920, 1080, 0},
		{"Cb", pic.Cb, pic.CStride, 960, 540, 1920 * 1080},
		{"Cr", pic.Cr, pic.CStride, 960, 540, 1920*1080 + 960*540},
	}
	for _, p := range planes {
		bad := 0
		for y := 0; y < p.h; y++ {
			for x := 0; x < p.w; x++ {
				if p.got[y*p.stride+x] != want[p.off+y*p.w+x] {
					bad++
				}
			}
		}
		if bad != 0 {
			t.Errorf("%s plane: %d of %d samples differ from ffmpeg", p.name, bad, p.w*p.h)
		}
	}
}
