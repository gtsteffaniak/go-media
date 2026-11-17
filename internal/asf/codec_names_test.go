package asf

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// videoSpec builds ASF video type-specific data (ASF spec 9.2) with the given
// BITMAPINFOHEADER compression fourcc.
func videoSpec(fourcc string, w, h int) []byte {
	const bih = 11
	spec := make([]byte, bih+40)
	binary.LittleEndian.PutUint32(spec[0:], uint32(w))
	binary.LittleEndian.PutUint32(spec[4:], uint32(h))
	binary.LittleEndian.PutUint32(spec[bih:], 40)
	binary.LittleEndian.PutUint32(spec[bih+4:], uint32(w))
	binary.LittleEndian.PutUint32(spec[bih+8:], uint32(h))
	copy(spec[bih+16:], fourcc)
	return spec
}

func TestDetectVideoFourCCVC1(t *testing.T) {
	for _, tag := range []string{"WVC1", "wvc1", "WMVA", "VC-1"} {
		if got := detectVideoFourCC(tag); got != "vc1" {
			t.Errorf("detectVideoFourCC(%q) = %q, want vc1", tag, got)
		}
	}
}

func TestParseVideoFormatVC1AdvancedProfile(t *testing.T) {
	// VC-1 Advanced Profile (WVC1) must parse as vc1, not fail as corrupt.
	codec, w, h, _, err := parseVideoFormat(videoSpec("WVC1", 1280, 720))
	if err != nil {
		t.Fatalf("parseVideoFormat WVC1: %v", err)
	}
	if codec != "vc1" || w != 1280 || h != 720 {
		t.Fatalf("got %s %dx%d, want vc1 1280x720", codec, w, h)
	}
}

func TestParseVideoFormatWMV3(t *testing.T) {
	codec, _, _, _, err := parseVideoFormat(videoSpec("WMV3", 360, 288))
	if err != nil || codec != "wmv3" {
		t.Fatalf("got %q err %v, want wmv3", codec, err)
	}
}

func TestAudioCodecNameFormatTags(t *testing.T) {
	cases := map[uint16]string{
		0x0160: "wmav1",
		0x0161: "wmav2",
		0x0162: "wmapro",
		0x0163: "wmalossless",
		0x0055: "mp3",
		0x00FF: "aac",
		0x1610: "aac",
		0x9999: "", // unknown: must not default to wmav2
	}
	for tag, want := range cases {
		if got := audioCodecName(tag); got != want {
			t.Errorf("audioCodecName(0x%04x) = %q, want %q", tag, got, want)
		}
	}
}

func TestParseStreamPropsAudioUsesFormatTag(t *testing.T) {
	// Audio type-specific data is a WAVEFORMATEX; wFormatTag is the first u16.
	spec := make([]byte, 18)
	binary.LittleEndian.PutUint16(spec[0:], 0x0162) // WMA Pro
	codec, err := audioCodecFromSpec(spec)
	if err != nil || codec != "wmapro" {
		t.Fatalf("audio codec %q err %v, want wmapro", codec, err)
	}
}

func TestParseStreamPropsAudioUnknownTagIsUnsupported(t *testing.T) {
	// An unrecognized WAVEFORMATEX tag must not yield an empty codec; it
	// must return a typed profile error so callers fall back to ffprobe.
	spec := make([]byte, 18)
	binary.LittleEndian.PutUint16(spec[0:], 0x9999)
	if _, err := audioCodecFromSpec(spec); !errors.Is(err, errs.UnsupportedProfile) {
		t.Fatalf("unknown audio tag err = %v, want UnsupportedProfile", err)
	}
}
