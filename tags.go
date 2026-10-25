package media

import (
	"io"

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
