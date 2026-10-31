package asf

import (
	"os"
	"testing"
)

func TestProbeWMV3Fixture(t *testing.T) {
	b, err := os.ReadFile("testdata/wmv3_360x288.wmv")
	if err != nil {
		t.Fatal(err)
	}
	r := bytesReaderAt{b: b}
	meta, err := openMeta(r, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if meta.video == nil || meta.video.codec != "wmv3" {
		t.Fatalf("video %+v", meta.video)
	}
	if meta.video.width != 360 || meta.video.height != 288 {
		t.Fatalf("size %dx%d", meta.video.width, meta.video.height)
	}
	if len(meta.video.extradata) != 4 || meta.video.extradata[0] != 0x4d {
		t.Fatalf("extradata %x", meta.video.extradata)
	}
	d, err := Duration(r, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Duration <= 0 {
		t.Fatalf("duration %v", d.Duration)
	}
}

func TestVideoKeyframePayload(t *testing.T) {
	b, err := os.ReadFile("testdata/wmv3_360x288.wmv")
	if err != nil {
		t.Fatal(err)
	}
	r := bytesReaderAt{b: b}
	extra, sample, err := videoKeyframe(r, int64(len(b)), 0)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := os.ReadFile("../vc1/testdata/frame1_360x288.bin")
	if len(sample) == 0 {
		t.Fatal("empty sample")
	}
	t.Logf("extra=%x sample=%d ref=%d", extra, len(sample), len(ref))
}

type bytesReaderAt struct {
	b []byte
}

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.b)) {
		return 0, os.ErrInvalid
	}
	n := copy(p, b.b[off:])
	return n, nil
}
