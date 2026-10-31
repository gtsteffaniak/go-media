package media

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/subtitle"
)

// SubtitleToWebVTT converts a sidecar subtitle file (.srt, .vtt, .ass, .ssa)
// to WebVTT, byte-identical to ffmpeg's "-f webvtt" output for the same file.
// SRT or ASS files with no cues return an error.
func SubtitleToWebVTT(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	ext := filepath.Ext(path)
	return subtitle.SidecarToWebVTT(ext, f)
}

// ListSubtitleTracks returns embedded subtitle streams when supported.
func ListSubtitleTracks(r io.ReaderAt, size int64) ([]SubtitleTrack, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return nil, mapErr(err)
	}
	switch id {
	case format.MKV, format.WebM:
		streams, err := matroska.Streams(r, size)
		if err != nil {
			return nil, mapErr(err)
		}
		return subtitleTracksFromStreams(streams), nil
	case format.MP4:
		streams, err := isobmff.SubtitleTracks(r, size)
		if err != nil {
			return nil, mapErr(err)
		}
		var out []SubtitleTrack
		for _, s := range streams {
			out = append(out, SubtitleTrack{
				Index:    s.Index,
				Codec:    s.Codec,
				Language: s.Language,
				Title:    s.Title,
				Embedded: true,
			})
		}
		return out, nil
	default:
		return nil, nil
	}
}

// ExtractEmbeddedSubtitle returns WebVTT for an embedded subtitle stream
// index, byte-identical to "ffmpeg -map 0:<index> -f webvtt" for Matroska text
// tracks and MP4 tx3g (see the package doc for the few intentional
// differences). Bitmap tracks such as PGS, VobSub and DVB return
// ErrUnsupported with a message naming the format.
func ExtractEmbeddedSubtitle(r io.ReaderAt, size int64, streamIndex int) (string, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return "", mapErr(err)
	}
	switch id {
	case format.MKV, format.WebM:
		vtt, err := matroska.ExtractSubtitleWebVTT(r, size, streamIndex)
		return vtt, mapErr(err)
	case format.MP4:
		vtt, err := isobmff.ExtractSubtitleWebVTT(r, size, streamIndex)
		return vtt, mapErr(err)
	default:
		return "", Unsupported("subtitle extract", string(id))
	}
}

// ExternalSubtitleTracks lists sidecar subtitles sharing a base name with videoPath.
func ExternalSubtitleTracks(videoPath string, siblingNames []string) []SubtitleTrack {
	videoStem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	var out []SubtitleTrack
	for _, name := range siblingNames {
		low := strings.ToLower(filepath.Ext(name))
		if low != ".srt" && low != ".vtt" && low != ".ass" && low != ".ssa" {
			continue
		}
		fileBase := strings.TrimSuffix(name, filepath.Ext(name))
		langExt := filepath.Ext(fileBase)
		fileBase2 := strings.TrimSuffix(fileBase, langExt)
		if fileBase != videoStem && fileBase2 != videoStem {
			continue
		}
		// "video.en.srt" carries a 2 or 3 letter language suffix.
		language := ""
		// Only a suffix added after the video stem counts, and it must be letters.
		if fileBase2 == videoStem {
			if l := strings.TrimPrefix(langExt, "."); (len(l) == 2 || len(l) == 3) && isASCIILetters(l) {
				language = l
			}
		}
		out = append(out, SubtitleTrack{
			Name:     name,
			Language: language,
			Embedded: false,
			Codec:    strings.TrimPrefix(low, "."),
		})
	}
	return out
}

// isASCIILetters reports whether s is non-empty and only ASCII letters.
func isASCIILetters(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
