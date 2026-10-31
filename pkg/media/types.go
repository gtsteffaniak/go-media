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

	FormatHEIC Format = "heic"
	FormatHEIF Format = "heif"
	FormatAVIF Format = "avif"

	FormatCR2 Format = "cr2"
	FormatCR3 Format = "cr3"
	FormatNEF Format = "nef"
	FormatARW Format = "arw"
	FormatDNG Format = "dng"
	FormatORF Format = "orf"
	FormatRW2 Format = "rw2"
	FormatRAF Format = "raf"
	FormatPEF Format = "pef"
)

// StreamType is audio or video.
type StreamType string

const (
	StreamAudio    StreamType = "audio"
	StreamVideo    StreamType = "video"
	StreamSubtitle StreamType = "subtitle"
)

// Stream describes one elementary stream.
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
	Format         Format
	FormatName     string
	Duration       time.Duration
	DurationSrc    Source
	Streams        []Stream
	Tags           Tags
	HasLyrics      bool
	HasArtwork     bool
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
	// MaxWidth and MaxHeight bound the output size (aspect ratio preserved).
	MaxWidth  int
	MaxHeight int
	// SeekPercent is the timeline position to capture (0–100). Alias: use Seek when set.
	SeekPercent float64
	// Seek overrides SeekPercent when > 0.
	Seek float64
	// Format is jpeg, png, or webp-lossless. JPEG and WebP use the BT.601
	// matrix like ffmpeg's YUV capture; PNG keeps the stream's tagged matrix.
	Format string
	// Quality is JPEG quality 1–100 (preferred over JPEGQuality).
	Quality int
	// JPEGQuality is the legacy JPEG quality field (1–100).
	JPEGQuality int
	// Filter selects the scaler: fast, area, balanced, or high (see ThumbnailFilter* constants).
	Filter ThumbnailFilter
	// Preset applies FileBrowser-style small/large defaults when set.
	Preset ThumbnailPreset
	// Fast requests the fast filter (deprecated: use Filter or Preset).
	Fast bool
	// PreferEmbedded tries cover art before decoding video.
	PreferEmbedded bool
	// Exact captures the frame at the seek time instead of the nearest
	// keyframe. By default (false) video thumbnails use the keyframe at or
	// before the seek time, which only needs one frame decoded; the image can
	// then sit up to one keyframe interval (GOP) before the requested time.
	// Exact decodes forward from that keyframe and is slower in proportion to
	// the distance. Only VP8 and VP9 honor it today; other codecs always use
	// the keyframe.
	Exact bool
}

// ThumbnailResult holds encoded image bytes and how they were obtained.
type ThumbnailResult struct {
	MIME   string
	Data   []byte
	Source Source
	Width  int
	Height int
}
