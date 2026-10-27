package media

// Source describes how a value was obtained so callers can cache and compare with ffprobe.
type Source string

const (
	// SourceHeader means the value came from a container header (mvhd, STREAMINFO, etc.).
	SourceHeader Source = "header"
	// SourceEstimate means the value was estimated (for example MP3 CBR without a VBR header).
	SourceEstimate Source = "estimate"
	// SourceScan means the value required scanning the file (for example MP3 VBR frame scan).
	SourceScan Source = "scan"
	// SourceTag means the value came from embedded tags (ID3, ilst, Vorbis comments).
	SourceTag Source = "tag"
	// SourceEmbedded means payload was extracted from an embedded attachment or cover art atom.
	SourceEmbedded Source = "embedded"
	// SourceDecode means an image was produced by a pure-Go decoder.
	SourceDecode Source = "decode"
)
