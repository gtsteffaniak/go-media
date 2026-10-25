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
)
