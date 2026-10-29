package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // artwork decode for dimension checks
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// Diff gate: compares go-media with ffprobe and ffmpeg for metadata tags,
// lyrics, embedded artwork and subtitles. Each diff function returns one
// human-readable entry per difference, like the filebrowser shadow differs.
// An empty result means the two agree. The gate fails on any entry.

// tagFields lists the normalized fields compared, with the ffprobe keys that carry them.
var tagFields = []struct {
	name string
	keys []string
}{
	{"title", []string{"title"}},
	{"artist", []string{"artist"}},
	{"album", []string{"album"}},
	{"genre", []string{"genre"}},
	{"year", []string{"date", "year", "originaldate"}},
	{"track", []string{"track", "tracknumber"}},
}

// ffTag returns the first non-empty tag matching one of keys (case-insensitive).
func ffTag(tags map[string]string, keys ...string) string {
	for _, k := range keys {
		for tk, v := range tags {
			if strings.EqualFold(tk, k) && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

var (
	yearRe   = regexp.MustCompile(`^\d{4}`)
	spacesRe = regexp.MustCompile(`\s+`)
)

// normalizeTagValue trims and collapses whitespace so formatting noise is ignored.
func normalizeTagValue(s string) string {
	return strings.TrimSpace(spacesRe.ReplaceAllString(s, " "))
}

// diffTags compares title, artist, album, genre, year and track.
// ffprobe reports dates as "2020-01-02" and track numbers as "3/12"; both are
// reduced to the year and the track number before comparing.
func diffTags(goTags media.Tags, format FFProbeFormat) []string {
	goVals := map[string]string{
		"title":  goTags.Title,
		"artist": goTags.Artist,
		"album":  goTags.Album,
		"genre":  goTags.Genre,
	}
	if goTags.Year > 0 {
		goVals["year"] = fmt.Sprintf("%d", goTags.Year)
	}
	if goTags.Track > 0 {
		goVals["track"] = fmt.Sprintf("%d", goTags.Track)
	}

	var diffs []string
	for _, field := range tagFields {
		goVal := normalizeTagValue(goVals[field.name])
		ffVal := normalizeTagValue(ffTag(format.Tags, field.keys...))
		switch field.name {
		case "year":
			ffVal = yearRe.FindString(ffVal)
		case "track":
			ffVal = strings.TrimSpace(strings.SplitN(ffVal, "/", 2)[0])
		}
		switch {
		case goVal == "" && ffVal == "":
		case goVal == "":
			diffs = append(diffs, fmt.Sprintf("%s: go-media=absent ffmpeg=%q", field.name, ffVal))
		case ffVal == "":
			diffs = append(diffs, fmt.Sprintf("%s: go-media=%q ffmpeg=absent", field.name, goVal))
		case !strings.EqualFold(goVal, ffVal):
			diffs = append(diffs, fmt.Sprintf("%s: go-media=%q ffmpeg=%q", field.name, goVal, ffVal))
		}
	}
	return diffs
}

// attachedPic returns the first attached picture stream reported by ffprobe.
func attachedPic(streams []FFProbeStream) *FFProbeStream {
	for i := range streams {
		if streams[i].AttachedPic == 1 {
			return &streams[i]
		}
	}
	return nil
}

// diffArtwork compares artwork presence, that go-media's bytes decode, and that
// the decoded size matches the ffprobe attached picture size.
func diffArtwork(goHas bool, goArt []byte, streams []FFProbeStream) []string {
	pic := attachedPic(streams)
	ffHas := pic != nil
	if goHas != ffHas {
		return []string{fmt.Sprintf("artwork presence: go-media=%t ffmpeg=%t", goHas, ffHas)}
	}
	if !goHas {
		return nil
	}
	if len(goArt) == 0 {
		return []string{"artwork: go-media reports a cover but returned no bytes"}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(goArt))
	if err != nil {
		return []string{fmt.Sprintf("artwork: go-media bytes do not decode: %v", err)}
	}
	if pic.Width > 0 && pic.Height > 0 && (cfg.Width != pic.Width || cfg.Height != pic.Height) {
		return []string{fmt.Sprintf("artwork size: go-media=%dx%d ffmpeg=%dx%d", cfg.Width, cfg.Height, pic.Width, pic.Height)}
	}
	return nil
}

var lrcTimestampRe = regexp.MustCompile(`\[[^\]]*\]`)

// normalizeLyrics removes LRC timestamps, blank lines and line-ending differences.
// Timestamps are dropped because ffprobe reports the lyrics tag as plain text.
func normalizeLyrics(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = lrcTimestampRe.ReplaceAllString(s, "")
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// diffLyrics compares embedded lyrics text between go-media and the ffprobe lyrics tag.
func diffLyrics(goText string, format FFProbeFormat) []string {
	ffText := ffTag(format.Tags, "lyrics", "unsyncedlyrics", "unsynced lyrics", "lyric")
	goHas := strings.TrimSpace(goText) != ""
	ffHas := ffText != ""
	if goHas != ffHas {
		return []string{fmt.Sprintf("embedded lyrics presence: go-media=%t ffmpeg=%t", goHas, ffHas)}
	}
	if !goHas {
		return nil
	}
	if normalizeLyrics(goText) != normalizeLyrics(ffText) {
		return []string{fmt.Sprintf("embedded lyrics text: go-media=%d chars ffmpeg=%d chars",
			len(normalizeLyrics(goText)), len(normalizeLyrics(ffText)))}
	}
	return nil
}

// diffSidecarLyrics checks that go-media reads a sidecar .lrc file with the same text as the file.
func diffSidecarLyrics(mediaPath, sidecarPath string) []string {
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return []string{fmt.Sprintf("sidecar lyrics unreadable: %v", err)}
	}
	text, _, ok := media.LyricsSidecar(mediaPath)
	if !ok {
		return []string{"sidecar lyrics: go-media did not find the .lrc file"}
	}
	if normalizeLyrics(text) != normalizeLyrics(string(raw)) {
		return []string{fmt.Sprintf("sidecar lyrics text: go-media=%d chars file=%d chars",
			len(normalizeLyrics(text)), len(normalizeLyrics(string(raw))))}
	}
	return nil
}

// subtitleCodecAliases maps go-media codec names to the ffprobe names for the same format.
var subtitleCodecAliases = map[string]string{
	"tx3g": "mov_text",
	"sbtt": "mov_text",
	"wvtt": "webvtt",
	"stpp": "ttml",
}

// imageSubtitleCodecs are bitmap subtitles. ffmpeg cannot convert them to WebVTT,
// so only their codec and language are compared.
var imageSubtitleCodecs = map[string]bool{
	"dvd_subtitle":      true,
	"hdmv_pgs_subtitle": true,
	"dvb_subtitle":      true,
}

func normalizeSubtitleCodec(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if alias, ok := subtitleCodecAliases[name]; ok {
		return alias
	}
	return name
}

// webVTTCueCount counts cues in WebVTT text.
func webVTTCueCount(vtt string) int {
	return strings.Count(vtt, "-->")
}

// ffmpegWebVTT converts one input (optionally one stream via args) to WebVTT with ffmpeg.
func ffmpegWebVTT(input string, streamSpec string) (string, error) {
	ffmpeg, _, err := ffmpegPaths()
	if err != nil {
		return "", err
	}
	args := []string{"-v", "error", "-i", input}
	if streamSpec != "" {
		args = append(args, "-map", streamSpec)
	}
	args = append(args, "-f", "webvtt", "-")
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(ffmpeg, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg webvtt: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// diffEmbeddedSubtitles compares the embedded subtitle tracks listed by go-media
// and by ffprobe (codec, language, cue count of the WebVTT conversion).
func diffEmbeddedSubtitles(a Asset, ff FFProbeResult) []string {
	var ffSubs []FFProbeStream
	for _, s := range ff.Streams {
		if s.CodecType == "subtitle" {
			ffSubs = append(ffSubs, s)
		}
	}
	f, err := openAsset(a)
	if err != nil {
		return []string{fmt.Sprintf("open: %v", err)}
	}
	tracks, listErr := media.ListSubtitleTracks(f, f.Size())
	if listErr != nil {
		if len(ffSubs) > 0 {
			return []string{fmt.Sprintf("go-media lists no tracks (%v), ffmpeg found %d", listErr, len(ffSubs))}
		}
		return nil
	}

	var diffs []string
	if len(tracks) != len(ffSubs) {
		diffs = append(diffs, fmt.Sprintf("embedded track count: go-media=%d ffmpeg=%d", len(tracks), len(ffSubs)))
	}
	for i := 0; i < len(tracks) && i < len(ffSubs); i++ {
		tr, ffs := tracks[i], ffSubs[i]
		goCodec := normalizeSubtitleCodec(tr.Codec)
		ffCodec := normalizeSubtitleCodec(ffs.CodecName)
		if goCodec != ffCodec {
			diffs = append(diffs, fmt.Sprintf("track %d codec: go-media=%q ffmpeg=%q", i, goCodec, ffCodec))
		}
		ffLang := ffTag(ffs.Tags, "language")
		if ffLang != "" && !strings.EqualFold(tr.Language, ffLang) {
			diffs = append(diffs, fmt.Sprintf("track %d language: go-media=%q ffmpeg=%q", i, tr.Language, ffLang))
		}
		if imageSubtitleCodecs[ffCodec] {
			continue
		}
		vtt, err := media.ExtractEmbeddedSubtitle(f, f.Size(), tr.Index)
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("track %d extract: go-media error %v", i, err))
			continue
		}
		ffVTT, err := ffmpegWebVTT(a.Path, fmt.Sprintf("0:s:%d", i))
		if err != nil {
			if strings.Contains(err.Error(), "no decoder found") {
				// This ffmpeg build cannot decode the codec, so there is no reference
				// cue count. Codec and language were still compared above.
				continue
			}
			diffs = append(diffs, fmt.Sprintf("track %d extract: ffmpeg error %v", i, err))
			continue
		}
		if goCues, ffCues := webVTTCueCount(vtt), webVTTCueCount(ffVTT); goCues != ffCues {
			diffs = append(diffs, fmt.Sprintf("track %d cues: go-media=%d ffmpeg=%d", i, goCues, ffCues))
		}
	}
	return diffs
}

// subtitleSidecarExts are the sidecar formats go-media lists.
var subtitleSidecarExts = map[string]bool{".srt": true, ".vtt": true, ".ass": true, ".ssa": true}

// diffSidecarSubtitles checks that go-media finds the sidecar subtitle files
// the fixture expects, and that each converts to the same number of cues as ffmpeg.
func diffSidecarSubtitles(a Asset) []string {
	dir := filepath.Dir(a.Path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("read dir: %v", err)}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := map[string]bool{}
	for _, s := range a.Sidecars {
		if subtitleSidecarExts[strings.ToLower(filepath.Ext(s))] {
			want[filepath.Base(s)] = true
		}
	}
	got := map[string]bool{}
	var diffs []string
	for _, tr := range media.ExternalSubtitleTracks(a.Path, names) {
		got[tr.Name] = true
		goVTT, err := media.SubtitleToWebVTT(filepath.Join(dir, tr.Name))
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("sidecar %s: go-media convert error %v", tr.Name, err))
			continue
		}
		ffVTT, err := ffmpegWebVTT(filepath.Join(dir, tr.Name), "")
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("sidecar %s: ffmpeg error %v", tr.Name, err))
			continue
		}
		if goCues, ffCues := webVTTCueCount(goVTT), webVTTCueCount(ffVTT); goCues != ffCues {
			diffs = append(diffs, fmt.Sprintf("sidecar %s cues: go-media=%d ffmpeg=%d", tr.Name, goCues, ffCues))
		}
	}
	for name := range want {
		if !got[name] {
			diffs = append(diffs, fmt.Sprintf("sidecar %s: expected but go-media did not list it", name))
		}
	}
	for name := range got {
		if !want[name] {
			diffs = append(diffs, fmt.Sprintf("sidecar %s: listed by go-media but not in the fixture", name))
		}
	}
	return diffs
}
