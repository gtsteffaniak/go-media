package media

import "time"

// Format identifies the detected container.
type Format string

const (
	FormatMP4  Format = "mp4"
	FormatMKV  Format = "mkv"
	FormatWebM Format = "webm"
	FormatWAV  Format = "wav"
	FormatFLAC Format = "flac"
	FormatOGG  Format = "ogg"
	FormatMP3  Format = "mp3"
	FormatAIFF Format = "aiff"
	FormatAU   Format = "au"
)

// StreamType is audio or video.
type StreamType string

const (
	StreamAudio StreamType = "audio"
	StreamVideo StreamType = "video"
	StreamSubtitle StreamType = "subtitle"
)

// Stream describes one elementary stream.
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

// Tags holds common metadata fields.
type Tags struct {
	Title  string
	Artist string
	Album  string
	Year   int
	Genre  string
	Track  int
	// Raw maps tag keys to values when callers need unnormalized fields.
	Raw map[string]string
}

// ProbeInfo is the result of Probe.
type ProbeInfo struct {
	Format       Format
	FormatName   string
	Duration     time.Duration
	DurationSrc  Source
	Streams      []Stream
	Tags         Tags
	HasLyrics    bool
	HasArtwork   bool
	SubtitleTracks []SubtitleTrack
}

// DurationResult pairs a duration with its source.
type DurationResult struct {
	Duration time.Duration
	Source   Source
}

// SubtitleTrack describes an embedded or sidecar subtitle stream.
type SubtitleTrack struct {
	Index    int
	Codec    string
	Language string
	Title    string
	Name     string
	Embedded bool
}

// ThumbnailOptions configures Thumbnail.
type ThumbnailOptions struct {
	MaxWidth       int
	MaxHeight      int
	SeekPercent    float64 // 0–100, default ~10
	Format         string  // "jpeg" or "png"
	JPEGQuality    int     // 1–100, default 85
	Fast           bool    // skip expensive steps when supported
	PreferEmbedded bool    // try embedded cover art before video keyframe
}

// ThumbnailResult holds encoded image bytes and how they were obtained.
type ThumbnailResult struct {
	MIME   string
	Data   []byte
	Source Source
	Width  int
	Height int
}
