// Package detect supports go-media parsers.

package detect

import "time"

// DurationResult is a data type used by this package.
type DurationResult struct {
	Duration time.Duration
	Source   string
}

// StreamType is a data type used by this package.
type StreamType string

const (
	StreamAudio    StreamType = "audio"
	StreamVideo    StreamType = "video"
	StreamSubtitle StreamType = "subtitle"
)

// Stream is a data type used by this package.
type Stream struct {
	Index  int
	Type   StreamType
	Codec  string
	Width  int
	Height int
	// Rotation is the display rotation in degrees (ffprobe sign convention).
	// Width and Height are coded dimensions; apply Rotation for display.
	Rotation   int
	FrameRate  float64
	SampleRate int
	Channels   int
	Bitrate    int
	Language   string
	Title      string
}
