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
	Index      int
	Type       StreamType
	Codec      string
	Width      int
	Height     int
	FrameRate  float64
	SampleRate int
	Channels   int
	Bitrate    int
	Language   string
	Title      string
}
