package media

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/subtitle"
)

// SubtitleToWebVTT converts a sidecar subtitle file to WebVTT.
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
		var out []SubtitleTrack
		for _, s := range streams {
			if s.Type != detect.StreamSubtitle {
				continue
			}
			out = append(out, SubtitleTrack{
				Index:    s.Index,
				Codec:    s.Codec,
				Language: s.Language,
				Title:    s.Title,
				Embedded: true,
			})
		}
		return out, nil
	case format.MP4:
		return nil, Unsupported("subtitle list", "mp4 embedded tracks not yet listed")
	default:
		return nil, nil
	}
}

// ExtractEmbeddedSubtitle returns WebVTT for an embedded subtitle stream index.
func ExtractEmbeddedSubtitle(r io.ReaderAt, size int64, streamIndex int) (string, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return "", mapErr(err)
	}
	switch id {
	case format.MKV, format.WebM:
		vtt, err := matroska.ExtractSubtitleWebVTT(r, size, streamIndex)
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
		out = append(out, SubtitleTrack{
			Name:     name,
			Embedded: false,
			Codec:    strings.TrimPrefix(low, "."),
		})
	}
	return out
}
