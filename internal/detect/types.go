package detect

import "time"

type DurationResult struct {
	Duration time.Duration
	Source   string
}

type StreamType string

const (
	StreamAudio    StreamType = "audio"
	StreamVideo    StreamType = "video"
	StreamSubtitle StreamType = "subtitle"
)

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
