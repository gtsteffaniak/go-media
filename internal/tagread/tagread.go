// Package tagread supports go-media parsers.

package tagread

import (
	"io"
	"strings"

	"github.com/dhowden/tag"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

// Tags is a data type used by this package.
type Tags struct {
	Title  string
	Artist string
	Album  string
	Year   int
	Genre  string
	Track  int
	Raw    map[string]string
}

// ReadTags supports media inspection.
func ReadTags(r io.ReaderAt, size int64, fmt format.ID) (Tags, error) {
	t, _, _, err := InspectTags(r, size, fmt)
	return t, err
}

// InspectTags reads container metadata once and returns tags plus artwork/lyrics flags.
func InspectTags(r io.ReaderAt, size int64, fmt format.ID) (Tags, bool, bool, error) {
	m, err := readMetadata(r, size, fmt)
	if err != nil {
		return Tags{}, false, false, err
	}
	track, _ := m.Track()
	t := Tags{
		Title:  m.Title(),
		Artist: m.Artist(),
		Album:  m.Album(),
		Year:   m.Year(),
		Genre:  m.Genre(),
		Track:  track,
		Raw:    map[string]string{},
	}
	lyrics := strings.TrimSpace(m.Lyrics())
	if lyrics != "" {
		t.Raw["lyrics"] = lyrics
	}
	p := m.Picture()
	hasArt := p != nil && len(p.Data) > 0
	return t, hasArt, lyrics != "", nil
}

// HasEmbeddedLyrics supports media inspection.
func HasEmbeddedLyrics(r io.ReaderAt, size int64, fmt format.ID) bool {
	m, err := readMetadata(r, size, fmt)
	if err != nil {
		return false
	}
	return strings.TrimSpace(m.Lyrics()) != ""
}

// Lyrics supports media inspection.
func Lyrics(r io.ReaderAt, size int64) (string, string, error) {
	fmt, err := sniff.Detect(r, size)
	if err != nil {
		return "", "", err
	}
	m, err := readMetadata(r, size, fmt)
	if err != nil {
		return "", "", err
	}
	text := strings.TrimSpace(m.Lyrics())
	if text == "" {
		return "", "", errs.Unsupported
	}
	return text, "embedded", nil
}

// HasArtwork supports media inspection.
func HasArtwork(r io.ReaderAt, size int64, fmt format.ID) bool {
	m, err := readMetadata(r, size, fmt)
	if err != nil {
		return false
	}
	p := m.Picture()
	return p != nil && len(p.Data) > 0
}

// Artwork supports media inspection.
func Artwork(r io.ReaderAt, size int64) ([]byte, string, error) {
	fmt, err := sniff.Detect(r, size)
	if err != nil {
		return nil, "", err
	}
	m, err := readMetadata(r, size, fmt)
	if err != nil {
		return nil, "", err
	}
	p := m.Picture()
	if p == nil || len(p.Data) == 0 {
		return nil, "", errs.Unsupported
	}
	mime := p.MIMEType
	if mime == "" {
		mime = "image/jpeg"
	}
	return p.Data, mime, nil
}

// readMetadata reads tags via dhowden/tag when supported.
func readMetadata(r io.ReaderAt, size int64, fmt format.ID) (tag.Metadata, error) {
	switch fmt {
	case format.MP3, format.FLAC, format.OGG, format.MP4:
		rs := ioat.New(r, size)
		m, err := tag.ReadFrom(rs)
		if err != nil {
			return nil, err
		}
		return m, nil
	default:
		return nil, errs.Unsup("tags", string(fmt))
	}
}

// Audio is everything FileBrowser-style callers need from one container parse.
type Audio struct {
	Tags              Tags
	HasEmbeddedLyrics bool
	HasArtwork        bool
	Artwork           []byte
	ArtworkMIME       string
}

// Inspect reads tags, embedded lyrics presence, and optionally cover art from a
// single readMetadata call. Artwork bytes are copied only when withArt is true.
func Inspect(r io.ReaderAt, size int64, fmt format.ID, withArt bool) (Audio, error) {
	m, err := readMetadata(r, size, fmt)
	if err != nil {
		return Audio{}, err
	}
	track, _ := m.Track()
	t := Tags{
		Title:  m.Title(),
		Artist: m.Artist(),
		Album:  m.Album(),
		Year:   m.Year(),
		Genre:  m.Genre(),
		Track:  track,
		Raw:    map[string]string{},
	}
	lyrics := strings.TrimSpace(m.Lyrics())
	if lyrics != "" {
		t.Raw["lyrics"] = lyrics
	}
	out := Audio{Tags: t, HasEmbeddedLyrics: lyrics != ""}
	if p := m.Picture(); p != nil && len(p.Data) > 0 {
		out.HasArtwork = true
		if withArt {
			out.Artwork = p.Data
			out.ArtworkMIME = p.MIMEType
			if out.ArtworkMIME == "" {
				out.ArtworkMIME = "image/jpeg"
			}
		}
	}
	return out, nil
}
