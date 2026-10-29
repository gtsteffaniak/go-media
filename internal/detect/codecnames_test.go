package detect

import "testing"

// TestMatroskaCodecName checks CodecID to ffprobe name mapping.
func TestMatroskaCodecName(t *testing.T) {
	cases := map[string]string{
		"A_AAC":            "aac",
		"A_AAC/MPEG4/LC":   "aac",
		"A_AC3":            "ac3",
		"A_EAC3":           "eac3",
		"A_MPEG/L3":        "mp3",
		"V_MPEGH/ISO/HEVC": "hevc",
		"V_MPEG4/ISO/AVC":  "h264",
		"V_AV1":            "av1",
		"S_TEXT/UTF8":      "subrip",
		"S_TEXT/SSA":       "ass",
		"A_PCM/INT/LIT":    "A_PCM/INT/LIT",
		"V_UNKNOWN/THING":  "V_UNKNOWN/THING",
	}
	for id, want := range cases {
		if got := MatroskaCodecName(id); got != want {
			t.Errorf("MatroskaCodecName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestVFWCodecName(t *testing.T) {
	priv := make([]byte, 40)
	copy(priv[16:20], []byte("MP42"))
	if got := VFWCodecName(priv); got != "msmpeg4v2" {
		t.Errorf("MP42 = %q, want msmpeg4v2", got)
	}
	copy(priv[16:20], []byte("NOPE"))
	if got := VFWCodecName(priv); got != "NOPE" {
		t.Errorf("unknown FourCC = %q, want NOPE", got)
	}
	if got := VFWCodecName(nil); got != vfwCodecID {
		t.Errorf("short private = %q, want %q", got, vfwCodecID)
	}
}

// TestMP4CodecNames checks sample entry FourCC to ffprobe name mapping.
func TestMP4CodecNames(t *testing.T) {
	if name, ok := MP4VideoCodecName("av01"); !ok || name != "av1" {
		t.Errorf("av01 = (%q, %v), want av1", name, ok)
	}
	if name, ok := MP4VideoCodecName("apch"); !ok || name != "prores" {
		t.Errorf("apch = (%q, %v), want prores", name, ok)
	}
	if _, ok := MP4AudioCodecName("lpcm"); ok {
		t.Errorf("lpcm must go through MP4PCMCodecName")
	}
	if _, ok := MP4AudioCodecName("tmcd"); ok {
		t.Errorf("tmcd should not map to an audio codec")
	}
}

// TestMatroskaAudioCodecName checks that PCM names come from Audio.BitDepth
// and that a missing depth keeps the CodecID.
func TestMatroskaAudioCodecName(t *testing.T) {
	cases := []struct {
		id    string
		depth int
		want  string
	}{
		{"A_PCM/INT/LIT", 16, "pcm_s16le"},
		{"A_PCM/INT/LIT", 24, "pcm_s24le"},
		{"A_PCM/INT/LIT", 8, "pcm_u8"},
		{"A_PCM/INT/LIT", 0, "A_PCM/INT/LIT"},
		{"A_PCM/INT/BIG", 32, "pcm_s32be"},
		{"A_PCM/FLOAT/IEEE", 64, "pcm_f64le"},
		{"A_PCM/FLOAT/IEEE", 12, "A_PCM/FLOAT/IEEE"},
		{"A_OPUS", 0, "opus"},
	}
	for _, c := range cases {
		if got := MatroskaAudioCodecName(c.id, c.depth); got != c.want {
			t.Errorf("MatroskaAudioCodecName(%q, %d) = %q, want %q", c.id, c.depth, got, c.want)
		}
	}
}

// TestMP4PCMCodecName checks PCM names from sample entry fields.
func TestMP4PCMCodecName(t *testing.T) {
	cases := []struct {
		fourCC string
		bits   int
		flags  uint32
		want   string
		ok     bool
	}{
		{"sowt", 16, 0, "pcm_s16le", true},
		{"sowt", 24, 0, "pcm_s24le", true},
		{"twos", 16, 0, "pcm_s16be", true},
		{"twos", 8, 0, "pcm_s8", true},
		{"lpcm", 24, MP4PCMFlagSigned, "pcm_s24le", true},
		{"lpcm", 16, MP4PCMFlagSigned | MP4PCMFlagBigEndian, "pcm_s16be", true},
		{"lpcm", 32, MP4PCMFlagFloat, "pcm_f32le", true},
		{"lpcm", 64, MP4PCMFlagFloat | MP4PCMFlagBigEndian, "pcm_f64be", true},
		{"lpcm", 8, 0, "pcm_u8", true},
		{"lpcm", 0, 0, "", false},
		{"sowt", 12, 0, "", false},
	}
	for _, c := range cases {
		got, ok := MP4PCMCodecName(c.fourCC, c.bits, c.flags)
		if got != c.want || ok != c.ok {
			t.Errorf("MP4PCMCodecName(%q, %d, %#x) = (%q, %v), want (%q, %v)", c.fourCC, c.bits, c.flags, got, ok, c.want, c.ok)
		}
	}
}
