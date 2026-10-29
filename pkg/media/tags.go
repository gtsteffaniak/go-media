package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/tagread"
)

// ReadTags returns normalized tags when the container is supported.
func ReadTags(r io.ReaderAt, size int64) (Tags, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return Tags{}, mapErr(err)
	}
	t, err := tagread.ReadTags(r, size, id)
	return mapTags(t), mapErr(err)
}

// HasArtwork reports whether embedded cover art is present.
func HasArtwork(r io.ReaderAt, size int64) bool {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return false
	}
	if id == format.MP4 {
		return isobmff.HasCover(r, size)
	}
	return tagread.HasArtwork(r, size, id)
}

// EmbeddedArtwork returns cover art bytes and MIME type.
func EmbeddedArtwork(r io.ReaderAt, size int64) ([]byte, string, Source, error) {
	data, mime, err := tagread.Artwork(r, size)
	if err != nil {
		return nil, "", "", mapErr(err)
	}
	return data, mime, SourceEmbedded, nil
}

// AudioOptions controls what InspectAudio returns beyond tags and lyrics.
type AudioOptions struct {
	// WithArtwork copies the embedded cover bytes into AudioInfo.Artwork.
	// HasArtwork is set either way.
	WithArtwork bool
}

// AudioInfo is the result of InspectAudio.
type AudioInfo struct {
	Tags              Tags
	HasEmbeddedLyrics bool
	HasArtwork        bool
	Artwork           []byte
	ArtworkMIME       string
}

// InspectAudio returns tags, embedded lyrics presence, and optionally cover art
// from one container parse. Sidecar lyrics files are not read here; use
// LyricsSidecar for those.
func InspectAudio(r io.ReaderAt, size int64, opts AudioOptions) (AudioInfo, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return AudioInfo{}, mapErr(err)
	}
	a, err := tagread.Inspect(r, size, id, opts.WithArtwork)
	if err != nil {
		return AudioInfo{}, mapErr(err)
	}
	return AudioInfo{
		Tags:              mapTags(a.Tags),
		HasEmbeddedLyrics: a.HasEmbeddedLyrics,
		HasArtwork:        a.HasArtwork,
		Artwork:           a.Artwork,
		ArtworkMIME:       a.ArtworkMIME,
	}, nil
}
