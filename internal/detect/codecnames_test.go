package detect

import "testing"

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
		"V_UNKNOWN/THING":  "V_UNKNOWN/THING",
	}
	for id, want := range cases {
		if got := MatroskaCodecName(id); got != want {
			t.Errorf("MatroskaCodecName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestMP4CodecNames(t *testing.T) {
	if name, ok := MP4VideoCodecName("av01"); !ok || name != "av1" {
		t.Errorf("av01 = (%q, %v), want av1", name, ok)
	}
	if name, ok := MP4VideoCodecName("apch"); !ok || name != "prores" {
		t.Errorf("apch = (%q, %v), want prores", name, ok)
	}
	if name, ok := MP4AudioCodecName("lpcm"); !ok || name != "pcm_s16le" {
		t.Errorf("lpcm = (%q, %v), want pcm_s16le", name, ok)
	}
	if _, ok := MP4AudioCodecName("tmcd"); ok {
		t.Errorf("tmcd should not map to an audio codec")
	}
}
