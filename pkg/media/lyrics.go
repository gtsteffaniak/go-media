package media

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/tagread"
)

var lyricsSidecarExts = []string{".elrc", ".lrc", ".srt", ".vtt"}

// LyricsFromReader returns embedded lyrics and a format hint ("embedded").
func LyricsFromReader(r io.ReaderAt, size int64) (string, string, error) {
	return tagread.Lyrics(r, size)
}

// LyricsSidecar reads a sidecar lyrics file next to mediaPath if present.
func LyricsSidecar(mediaPath string) (string, string, bool) {
	dir := filepath.Dir(mediaPath)
	base := filepath.Base(mediaPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	for _, ext := range lyricsSidecarExts {
		path := filepath.Join(dir, name+ext)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 15*1024*1024 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		return text, strings.TrimPrefix(ext, "."), true
	}
	return "", "", false
}

// HasLyrics reports embedded lyrics or a readable sidecar.
func HasLyrics(r io.ReaderAt, size int64, mediaPath string) bool {
	if _, _, ok := LyricsSidecar(mediaPath); ok {
		return true
	}
	fmt, err := sniff.Detect(r, size)
	if err != nil {
		return false
	}
	return tagread.HasEmbeddedLyrics(r, size, fmt)
}
