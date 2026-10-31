// Package format supports go-media parsers.

package format

// ID is a data type used by this package.
type ID string

const (
	MP4  ID = "mp4"
	MKV  ID = "mkv"
	WebM ID = "webm"
	WAV  ID = "wav"
	FLAC ID = "flac"
	OGG  ID = "ogg"
	MP3  ID = "mp3"
	AIFF ID = "aiff"
	AU   ID = "au"
	AVI  ID = "avi"

	MPEGTS ID = "mpegts"
	M2TS   ID = "m2ts"
	MPEGPS ID = "mpegps"

	HEIC ID = "heic"
	HEIF ID = "heif"
	AVIF ID = "avif"

	CR2 ID = "cr2"
	CR3 ID = "cr3"
	NEF ID = "nef"
	ARW ID = "arw"
	DNG ID = "dng"
	ORF ID = "orf"
	RW2 ID = "rw2"
	RAF ID = "raf"
	PEF ID = "pef"
)

// IsHEIFImage reports ISO BMFF still-image brands handled by the HEIF thumb path.
func IsHEIFImage(id ID) bool {
	switch id {
	case HEIC, HEIF, AVIF:
		return true
	default:
		return false
	}
}

// IsCameraRAW reports camera RAW containers with embedded JPEG previews.
func IsCameraRAW(id ID) bool {
	switch id {
	case CR2, CR3, NEF, ARW, DNG, ORF, RW2, RAF, PEF:
		return true
	default:
		return false
	}
}
