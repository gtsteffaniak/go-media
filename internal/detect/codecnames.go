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
	"S_TEXT/UTF8":      "subrip",
	"S_TEXT/ASS":       "ass",
	"S_TEXT/SSA":       "ass",
	"S_TEXT/WEBVTT":    "webvtt",
	"S_VOBSUB":         "dvd_subtitle",
	"S_HDMV/PGS":       "hdmv_pgs_subtitle",
}

// MatroskaCodecName returns the ffprobe-style name for a Matroska CodecID.
// Unknown IDs are returned unchanged so no information is lost. PCM CodecIDs
// need the track's bit depth, so they are returned unchanged here; use
// MatroskaAudioCodecName for audio tracks.
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

// vfwFourCCNames maps the FourCC of a Video for Windows track to an ffprobe
// codec name. The FourCC is the biCompression field of the BITMAPINFOHEADER.
var vfwFourCCNames = map[string]string{
	"MP41": "msmpeg4v1",
	"MPG4": "msmpeg4v1",
	"MP42": "msmpeg4v2",
	"MP43": "msmpeg4v3",
	"DIV3": "msmpeg4v3",
	"DIVX": "mpeg4",
	"XVID": "mpeg4",
	"MJPG": "mjpeg",
	"H264": "h264",
	"X264": "h264",
	"AVC1": "h264",
}

// vfwCodecID is the Matroska CodecID for VFW tracks, whose codec is named by
// the FourCC in CodecPrivate.
const vfwCodecID = "V_MS/VFW/FOURCC"

// VFWCodecName returns the ffprobe codec name for a V_MS/VFW/FOURCC track.
// CodecPrivate holds a BITMAPINFOHEADER. Bytes 16 to 19 are biCompression, the
// FourCC, which ffmpeg also reads from there. Unknown FourCCs return the raw
// FourCC string, matching ffprobe.
func VFWCodecName(codecPrivate []byte) string {
	if len(codecPrivate) < 20 {
		return vfwCodecID
	}
	if name, ok := vfwFourCCNames[string(codecPrivate[16:20])]; ok {
		return name
	}
	return string(codecPrivate[16:20])
}

// MatroskaAudioCodecName is MatroskaCodecName for an audio track whose
// Audio.BitDepth is bitDepth (0 when absent). The A_PCM CodecIDs take their
// sample format from BitDepth, as ffmpeg's matroska demuxer does; when the
// depth is missing or not one ffmpeg maps, the CodecID is returned unchanged
// rather than reporting a wrong bit depth.
func MatroskaAudioCodecName(id string, bitDepth int) string {
	switch id {
	case "A_PCM/INT/LIT":
		switch bitDepth {
		case 8:
			return "pcm_u8"
		case 16:
			return "pcm_s16le"
		case 24:
			return "pcm_s24le"
		case 32:
			return "pcm_s32le"
		}
		return id
	case "A_PCM/INT/BIG":
		switch bitDepth {
		case 8:
			return "pcm_u8"
		case 16:
			return "pcm_s16be"
		case 24:
			return "pcm_s24be"
		case 32:
			return "pcm_s32be"
		}
		return id
	case "A_PCM/FLOAT/IEEE":
		switch bitDepth {
		case 32:
			return "pcm_f32le"
		case 64:
			return "pcm_f64le"
		}
		return id
	}
	return MatroskaCodecName(id)
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
// ffprobe names. The generic PCM entries lpcm, sowt and twos are not listed:
// their sample format depends on the entry's fields, see MP4PCMCodecName.
var mp4AudioCodecNames = map[string]string{
	"ac-3": "ac3",
	"ec-3": "eac3",
	"Opus": "opus",
	"opus": "opus",
	"fLaC": "flac",
	"alac": "alac",
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

// mp4PCMFourCCs are the PCM sample entries whose format comes from their fields.
var mp4PCMFourCCs = []string{"lpcm", "sowt", "twos"}

// IsMP4PCMFourCC reports whether fourCC is a PCM sample entry (lpcm, sowt,
// twos) whose codec name needs MP4PCMCodecName.
func IsMP4PCMFourCC(fourCC string) bool {
	switch strings.TrimSpace(fourCC) {
	case "lpcm", "sowt", "twos":
		return true
	}
	return false
}

// MP4PCMFlag* are the QuickTime LPCM formatSpecificFlags bits read from a
// version 2 sound sample entry.
const (
	MP4PCMFlagFloat     = 0x1 // samples are IEEE floats
	MP4PCMFlagBigEndian = 0x2 // samples are big-endian
	MP4PCMFlagSigned    = 0x4 // integer samples are signed
)

// MP4PCMCodecName returns the ffprobe name for a PCM sample entry, following
// ffmpeg's mov demuxer. bits is the sample size: the version 0/1 samplesize
// field for sowt and twos, or constBitsPerChannel for a version 2 lpcm entry,
// whose flags are its formatSpecificFlags. ok is false when the fields do not
// name a format ffmpeg maps; the caller should then keep the FourCC.
func MP4PCMCodecName(fourCC string, bits int, flags uint32) (string, bool) {
	switch strings.TrimSpace(fourCC) {
	case "sowt", "twos":
		be := strings.TrimSpace(fourCC) == "twos"
		switch bits {
		case 8:
			return "pcm_s8", true
		case 16:
			return pcmName("s16", be), true
		case 24:
			return pcmName("s24", be), true
		case 32:
			return pcmName("s32", be), true
		}
	case "lpcm":
		be := flags&MP4PCMFlagBigEndian != 0
		if flags&MP4PCMFlagFloat != 0 {
			switch bits {
			case 32:
				return pcmName("f32", be), true
			case 64:
				return pcmName("f64", be), true
			}
			return "", false
		}
		switch bits {
		case 8:
			if flags&MP4PCMFlagSigned != 0 {
				return "pcm_s8", true
			}
			return "pcm_u8", true
		case 16:
			return pcmName("s16", be), true
		case 24:
			return pcmName("s24", be), true
		case 32:
			return pcmName("s32", be), true
		}
	}
	return "", false
}

// pcmName builds an ffprobe PCM codec name such as pcm_s24le.
func pcmName(format string, bigEndian bool) string {
	if bigEndian {
		return "pcm_" + format + "be"
	}
	return "pcm_" + format + "le"
}

// MP4SampleEntryFourCCs lists every sample entry type that has an ffprobe name
// in these tables, PCM entries included. The MP4 trak reader must extract these boxes to see them.
func MP4SampleEntryFourCCs() []string {
	out := make([]string, 0, len(mp4VideoCodecNames)+len(mp4AudioCodecNames)+len(mp4PCMFourCCs))
	for k := range mp4VideoCodecNames {
		out = append(out, k)
	}
	for k := range mp4AudioCodecNames {
		out = append(out, k)
	}
	return append(out, mp4PCMFourCCs...)
}
