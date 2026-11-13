package media_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestHEIFTransformAVIFIrotImir(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "data", "avif", "irot1_imir0.avif"))
	if err != nil {
		t.Skip("missing fixture:", err)
	}
	got, err := media.HEIFTransform(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	want := media.ImageTransform{Rotation: 90, Mirror: true, MirrorHorizontal: false}
	if got != want || got.IsIdentity() {
		t.Fatalf("HEIFTransform = %+v, want %+v", got, want)
	}
}

func TestHEIFTransformIrot(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xd9}
	for irot, deg := range []int{0, 90, 180, 270} {
		data := heifthumb.SynthJPEGHEIFWithPrimaryIrot(jpeg, 2, 1, uint8(irot))
		got, err := media.HEIFTransform(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("irot %d: %v", irot, err)
		}
		if got.Rotation != deg || got.Mirror {
			t.Fatalf("irot %d: %+v, want rotation %d", irot, got, deg)
		}
		if got.IsIdentity() != (deg == 0) {
			t.Fatalf("irot %d: IsIdentity = %v", irot, got.IsIdentity())
		}
	}
}

// TestHEIFTransformMirrorFirst checks that an imir associated before irot is
// composed in association order: a top-to-bottom mirror followed by a
// quarter turn is the quarter turn followed by a left-to-right mirror.
func TestHEIFTransformMirrorFirst(t *testing.T) {
	data := heifthumb.SynthJPEGHEIFWithPrimaryMirrorFirst([]byte{0xff, 0xd8, 0xff, 0xd9}, 2, 1, 0, 1)
	got, err := media.HEIFTransform(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want := media.ImageTransform{Rotation: 90, Mirror: true, MirrorHorizontal: true}
	if got != want {
		t.Fatalf("HEIFTransform = %+v, want %+v", got, want)
	}
}

// TestHEIFTransformUnreadable checks that missing item properties are an
// error, not "no transform", and that other formats are unsupported.
func TestHEIFTransformUnreadable(t *testing.T) {
	data := heifthumb.SynthJPEGHEIF([]byte{0xff, 0xd8, 0xff, 0xd9}, 2, 1)
	if _, err := media.HEIFTransform(bytes.NewReader(data), int64(len(data))); !errors.Is(err, media.ErrUnsupported) {
		t.Fatalf("no iprp: err = %v, want ErrUnsupported", err)
	}
	f, size := openFixture(t, "red.mp4")
	defer f.Close()
	if _, err := media.HEIFTransform(f, size); !errors.Is(err, media.ErrUnsupported) {
		t.Fatalf("mp4: err = %v, want ErrUnsupported", err)
	}
	if _, err := media.HEIFTransform(bytes.NewReader(nil), 0); !errors.Is(err, media.ErrCorrupt) {
		t.Fatalf("empty: err = %v, want ErrCorrupt", err)
	}
}
