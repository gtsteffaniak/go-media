package detect

import "strings"

// Codec names follow ffprobe's codec_name so callers can compare go-media
// output with ffprobe directly.

// matroskaCodecNames maps Matroska CodecID values to ffprobe codec names.
var matroskaCodecNames = map[string]string{
	"V_VP8":            "vp8",
	"V_VP9":            "vp9",
	"V_AV1":            "av1",
	"V_MPEG4/ISO/AVC":  "h264",
	"V_MPEGH/ISO/HEVC": "hevc",
	"V_MPEG4/ISO/ASP":  "mpeg4",
	"V_MPEG4/ISO/SP":   "mpeg4",
	"V_MPEG2":          "mpeg2video",
	"V_MPEG1":          "mpeg1video",
	"V_THEORA":         "theora",
	"V_MJPEG":          "mjpeg",
	"V_PRORES":         "prores",
	"A_AAC":            "aac",
	"A_AC3":            "ac3",
	"A_EAC3":           "eac3",
	"A_DTS":            "dts",
	"A_TRUEHD":         "truehd",
	"A_MPEG/L1":        "mp1",
	"A_MPEG/L2":        "mp2",
	"A_MPEG/L3":        "mp3",
	"A_FLAC":           "flac",
	"A_ALAC":           "alac",
	"A_VORBIS":         "vorbis",
	"A_OPUS":           "opus",
	"A_PCM/INT/LIT":    "pcm_s16le",
	"A_PCM/INT/BIG":    "pcm_s16be",
	"A_PCM/FLOAT/IEEE": "pcm_f32le",
	"S_TEXT/UTF8":      "subrip",
	"S_TEXT/ASS":       "ass",
	"S_TEXT/SSA":       "ssa",
	"S_TEXT/WEBVTT":    "webvtt",
	"S_VOBSUB":         "dvd_subtitle",
	"S_HDMV/PGS":       "hdmv_pgs_subtitle",
}

// MatroskaCodecName returns the ffprobe-style name for a Matroska CodecID.
// Unknown IDs are returned unchanged so no information is lost.
func MatroskaCodecName(id string) string {
	if name, ok := matroskaCodecNames[id]; ok {
		return name
	}
	// AAC variants are published as A_AAC/MPEG4/LC and friends.
	if strings.HasPrefix(id, "A_AAC") {
		return "aac"
	}
	if strings.HasPrefix(id, "A_DTS") {
		return "dts"
	}
	return id
}

// mp4VideoCodecNames maps ISO BMFF visual sample entry FourCCs to ffprobe names.
// avc1/avc3 and mp4v are handled by the existing stream code, so they are not listed.
var mp4VideoCodecNames = map[string]string{
	"hvc1": "hevc",
	"hev1": "hevc",
	"dvh1": "hevc",
	"dvhe": "hevc",
	"av01": "av1",
	"vp09": "vp9",
	"apch": "prores",
	"apcn": "prores",
	"apcs": "prores",
	"apco": "prores",
	"ap4h": "prores",
	"ap4x": "prores",
}

// mp4AudioCodecNames maps ISO BMFF / QuickTime audio sample entry FourCCs to
// ffprobe names. PCM bit depth is not read from the sample entry, so the
// 16-bit default is used for the generic lpcm/sowt/twos tags.
var mp4AudioCodecNames = map[string]string{
	"ac-3": "ac3",
	"ec-3": "eac3",
	"Opus": "opus",
	"opus": "opus",
	"fLaC": "flac",
	"alac": "alac",
	"lpcm": "pcm_s16le",
	"sowt": "pcm_s16le",
	"twos": "pcm_s16be",
	"in24": "pcm_s24be",
	"in32": "pcm_s32be",
	"fl32": "pcm_f32le",
	"fl64": "pcm_f64le",
}

// MP4VideoCodecName returns the ffprobe name for a visual sample entry FourCC.
func MP4VideoCodecName(fourCC string) (string, bool) {
	name, ok := mp4VideoCodecNames[strings.TrimSpace(fourCC)]
	return name, ok
}

// MP4AudioCodecName returns the ffprobe name for an audio sample entry FourCC.
func MP4AudioCodecName(fourCC string) (string, bool) {
	name, ok := mp4AudioCodecNames[strings.TrimSpace(fourCC)]
	return name, ok
}

// MP4SampleEntryFourCCs lists every sample entry type that has an ffprobe name
// in this table. The MP4 trak reader must extract these boxes to see them.
func MP4SampleEntryFourCCs() []string {
	out := make([]string, 0, len(mp4VideoCodecNames)+len(mp4AudioCodecNames))
	for k := range mp4VideoCodecNames {
		out = append(out, k)
	}
	for k := range mp4AudioCodecNames {
		out = append(out, k)
	}
	return out
}
