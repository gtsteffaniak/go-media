package main

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// The checks below run the diff functions in diffgate.go. A check is:
//   - Skipped when neither go-media nor ffprobe has the data (nothing to compare);
//   - Mismatch when any difference is reported (the gate fails);
//   - Supported when both sides report the same data.

// fixtureHasSubtitleSidecars reports whether the asset lists a subtitle sidecar.
func fixtureHasSubtitleSidecars(a Asset) bool {
	for _, s := range a.Sidecars {
		if subtitleSidecarExts[strings.ToLower(filepath.Ext(s))] {
			return true
		}
	}
	return false
}

// hasAnyTag reports whether any compared metadata field (title, artist, ...)
// is set. Encoder and brand tags do not count.
func hasAnyTag(tags map[string]string) bool {
	for _, field := range tagFields {
		if ffTag(tags, field.keys...) != "" {
			return true
		}
	}
	return false
}

// checkTags compares all normalized tags with ffprobe for every asset.
func checkTags(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckTags}
	ff, err := probeFFprobe(a.Path)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	start := time.Now()
	tags, gErr := media.ReadTags(f, f.Size())
	res.GoMediaMS = msSince(start)
	if gErr != nil {
		if a.ID == "asf_wmv3" || !hasAnyTag(ff.Format.Tags) {
			res.Status = StatusSkipped
			res.Detail = "no tags in fixture"
			return res
		}
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	if !hasAnyTag(ff.Format.Tags) && tags.Title == "" && tags.Artist == "" && tags.Album == "" {
		res.Status = StatusSkipped
		res.Detail = "no tags in fixture"
		return res
	}
	diffs := diffTags(tags, ff.Format)
	res.FieldDiffs = diffs
	finishDiffResult(&res, diffs, "tags match ffprobe")
	return res
}

// checkArtwork compares artwork presence, decodability and size with ffprobe.
func checkArtwork(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckArtwork}
	ff, err := probeFFprobe(a.Path)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	start := time.Now()
	has := media.HasArtwork(f, f.Size())
	art, _, _, artErr := media.EmbeddedArtwork(f, f.Size())
	res.GoMediaMS = msSince(start)

	if !has && attachedPic(ff.Streams) == nil {
		res.Status = StatusSkipped
		res.Detail = "no embedded cover in fixture"
		return res
	}
	if artErr != nil && has {
		res.Status = classifyGoErr(artErr)
		res.Detail = artErr.Error()
		return res
	}
	diffs := diffArtwork(has, art, ff.Streams)
	finishDiffResult(&res, diffs, "artwork matches ffprobe")
	return res
}

// checkLyrics compares embedded lyrics text with the ffprobe lyrics tag, and
// sidecar .lrc text with the file.
func checkLyrics(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckLyrics}
	ff, err := probeFFprobe(a.Path)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	start := time.Now()
	text, _, lErr := media.LyricsFromReader(f, f.Size())
	res.GoMediaMS = msSince(start)

	var sidecarPath string
	for _, s := range a.Sidecars {
		if strings.EqualFold(filepath.Ext(s), ".lrc") {
			sidecarPath = sidecarFullPath(a, s)
		}
	}
	ffHasEmbedded := ffTag(ff.Format.Tags, "lyrics", "unsyncedlyrics", "unsynced lyrics", "lyric") != ""
	if sidecarPath == "" && !ffHasEmbedded && strings.TrimSpace(text) == "" {
		res.Status = StatusSkipped
		res.Detail = "no lyrics in fixture"
		return res
	}
	if lErr != nil && !errors.Is(lErr, media.ErrUnsupported) && ffHasEmbedded {
		res.Status = classifyGoErr(lErr)
		res.Detail = lErr.Error()
		return res
	}
	diffs := diffLyrics(text, ff.Format)
	if sidecarPath != "" {
		diffs = append(diffs, diffSidecarLyrics(a.Path, sidecarPath)...)
	}
	finishDiffResult(&res, diffs, "lyrics match ffprobe")
	return res
}

// checkSubtitles compares embedded tracks (codec, language, cue count of the
// WebVTT conversion) and sidecar subtitle files with ffmpeg.
func checkSubtitles(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckSubtitles}
	switch a.ID {
	case "heic_jpeg_thumb", "raw_dng_preview":
		res.Status = StatusSkipped
		res.Detail = "still image fixture"
		return res
	}
	ff, err := probeFFprobe(a.Path)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	hasEmbedded := false
	for _, s := range ff.Streams {
		if s.CodecType == "subtitle" {
			hasEmbedded = true
		}
	}
	if !hasEmbedded && !fixtureHasSubtitleSidecars(a) {
		res.Status = StatusSkipped
		res.Detail = "no subtitles in fixture"
		return res
	}
	diffs := diffEmbeddedSubtitles(a, ff)
	if fixtureHasSubtitleSidecars(a) {
		diffs = append(diffs, diffSidecarSubtitles(a)...)
	}
	finishDiffResult(&res, diffs, "subtitles match ffmpeg")
	return res
}

// finishDiffResult sets the status from a list of differences.
func finishDiffResult(res *CheckResult, diffs []string, okDetail string) {
	if len(diffs) > 0 {
		res.Status = StatusMismatch
		res.Detail = strings.Join(diffs, "; ")
		return
	}
	res.Status = StatusSupported
	res.Detail = okDetail
}

// sidecarFullPath returns the sidecar path. Fixture lists may hold absolute
// paths or names relative to the media file.
func sidecarFullPath(a Asset, s string) string {
	if filepath.IsAbs(s) {
		return s
	}
	return filepath.Join(filepath.Dir(a.Path), s)
}
