package isobmff

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/tagread"
	"github.com/gtsteffaniak/go-media/internal/format"
)

// HasCover reports ilst/covr metadata or a dedicated attached-picture video track.
// Full-motion MJPEG/MP4V video tracks are not treated as cover art.
func HasCover(r io.ReaderAt, size int64) bool {
	if tagread.HasArtwork(r, size, format.MP4) {
		return true
	}
	m, err := OpenFileMeta(r, size)
	if err != nil {
		return false
	}
	return m.HasAttachedCover()
}
