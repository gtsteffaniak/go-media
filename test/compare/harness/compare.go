package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gtsteffaniak/go-media"
)

// runComparisons executes all checks for every asset and writes thumbnail media under reportDir.
func runComparisons(assets []Asset, reportDir string) ([][]CheckResult, error) {
	thumbDir := filepath.Join(reportDir, "media", "thumbs")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return nil, err
	}
	var results [][]CheckResult
	for _, a := range assets {
		results = append(results, compareAsset(a, thumbDir))
	}
	return results, nil
}

// compareAsset runs the full check matrix for one asset.
func compareAsset(a Asset, thumbDir string) []CheckResult {
	var out []CheckResult
	out = append(out, checkDuration(a))
	out = append(out, checkProbe(a))
	out = append(out, checkTags(a))
	out = append(out, checkArtwork(a))
	out = append(out, checkLyrics(a))
	out = append(out, checkSubtitles(a))
	out = append(out, checkThumbnail(a, thumbDir))
	return out
}

func openAsset(a Asset) (*fileAt, error) {
	return openFileAt(a.Path)
}

// checkDuration compares media.Duration with ffprobe.
func checkDuration(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckDuration}
	start := time.Now()
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	dr, gErr := media.Duration(f, f.Size())
	res.GoMediaMS = msSince(start)

	ffStart := time.Now()
	want, fErr := ffprobeDuration(a.Path)
	res.FFmpegMS = msSince(ffStart)

	if gErr != nil {
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	if fErr != nil {
		res.Status = StatusError
		res.Detail = fErr.Error()
		return res
	}
	got := dr.Duration.Seconds()
	res.DurationDelta = got - want
	if math.Abs(res.DurationDelta) > 0.35 {
		res.Status = StatusMismatch
		res.Detail = fmt.Sprintf("go=%.3fs ffprobe=%.3fs src=%s", got, want, dr.Source)
	} else {
		res.Status = StatusSupported
		res.Detail = fmt.Sprintf("delta=%.3fs src=%s", res.DurationDelta, dr.Source)
	}
	return res
}

// checkProbe compares high-level probe fields with ffprobe.
func checkProbe(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckProbe}
	start := time.Now()
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	info, gErr := media.Probe(f, f.Size())
	res.GoMediaMS = msSince(start)

	ffStart := time.Now()
	ff, fErr := probeFFprobe(a.Path)
	res.FFmpegMS = msSince(ffStart)

	if gErr != nil {
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	if fErr != nil {
		res.Status = StatusError
		res.Detail = fErr.Error()
		return res
	}
	var diffs []string
	if ff.Format.Duration > 0 {
		delta := info.Duration.Seconds() - ff.Format.Duration
		if math.Abs(delta) > 0.35 {
			diffs = append(diffs, fmt.Sprintf("duration: go=%.3f ff=%.3f", info.Duration.Seconds(), ff.Format.Duration))
		}
	}
	vidGo, audGo := streamCounts(info.Streams)
	vidFF, audFF := ffStreamCounts(ff.Streams, true)
	if vidGo != vidFF {
		diffs = append(diffs, fmt.Sprintf("video streams: go=%d ff=%d", vidGo, vidFF))
	}
	if audGo != audFF {
		diffs = append(diffs, fmt.Sprintf("audio streams: go=%d ff=%d", audGo, audFF))
	}
	res.FieldDiffs = diffs
	if len(diffs) > 0 {
		res.Status = StatusMismatch
		res.Detail = strings.Join(diffs, "; ")
	} else {
		res.Status = StatusSupported
		res.Detail = fmt.Sprintf("format=%s streams=%d", info.FormatName, len(info.Streams))
	}
	return res
}

// checkTags compares normalized tags when ffprobe reports tag fields.
func checkTags(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckTags}
	if a.Category != "audio" && !strings.Contains(a.ID, "m4a") {
		res.Status = StatusSkipped
		res.Detail = "not a tagged audio fixture"
		return res
	}
	start := time.Now()
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	tags, gErr := media.ReadTags(f, f.Size())
	res.GoMediaMS = msSince(start)

	ff, fErr := probeFFprobe(a.Path)
	res.FFmpegMS = 0
	if fErr != nil {
		res.Status = StatusError
		res.Detail = fErr.Error()
		return res
	}
	if gErr != nil {
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	if len(ff.Format.Tags) == 0 && tags.Title == "" && tags.Artist == "" {
		res.Status = StatusSkipped
		res.Detail = "no tags in fixture"
		return res
	}
	var diffs []string
	if ffTitle := ff.Format.Tags["title"]; ffTitle != "" && !strings.EqualFold(tags.Title, ffTitle) {
		diffs = append(diffs, fmt.Sprintf("title: go=%q ff=%q", tags.Title, ffTitle))
	}
	if ffArtist := ff.Format.Tags["artist"]; ffArtist != "" && !strings.EqualFold(tags.Artist, ffArtist) {
		diffs = append(diffs, fmt.Sprintf("artist: go=%q ff=%q", tags.Artist, ffArtist))
	}
	res.FieldDiffs = diffs
	if len(diffs) > 0 {
		res.Status = StatusMismatch
	} else {
		res.Status = StatusSupported
	}
	return res
}

// checkArtwork compares HasArtwork / EmbeddedArtwork with ffprobe stream disposition.
func checkArtwork(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckArtwork}
	start := time.Now()
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	has := media.HasArtwork(f, f.Size())
	_, _, _, artErr := media.EmbeddedArtwork(f, f.Size())
	res.GoMediaMS = msSince(start)

	ff, fErr := probeFFprobe(a.Path)
	if fErr != nil {
		res.Status = StatusError
		res.Detail = fErr.Error()
		return res
	}
	ffHas := false
	for _, s := range ff.Streams {
		if s.AttachedPic == 1 {
			ffHas = true
		}
	}
	if !has && !ffHas {
		res.Status = StatusSkipped
		res.Detail = "no embedded cover in fixture"
		return res
	}
	if artErr != nil {
		res.Status = classifyGoErr(artErr)
		res.Detail = artErr.Error()
		return res
	}
	if has != ffHas {
		res.Status = StatusMismatch
		res.Detail = fmt.Sprintf("hasArtwork go=%v ff=%v", has, ffHas)
	} else {
		res.Status = StatusSupported
	}
	return res
}

// checkLyrics compares HasLyrics with sidecar / embedded expectations.
func checkLyrics(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckLyrics}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	start := time.Now()
	has := media.HasLyrics(f, f.Size(), a.Path)
	res.GoMediaMS = msSince(start)

	expect := len(a.Sidecars) > 0 && strings.HasSuffix(a.Sidecars[0], ".lrc")
	if !expect {
		res.Status = StatusSkipped
		res.Detail = "no lyrics sidecar on fixture"
		return res
	}
	if has {
		res.Status = StatusSupported
	} else {
		res.Status = StatusMismatch
		res.Detail = "expected sidecar lyrics"
	}
	return res
}

// checkSubtitles compares subtitle listing and WebVTT conversion paths.
func checkSubtitles(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckSubtitles}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	start := time.Now()
	tracks, listErr := media.ListSubtitleTracks(f, f.Size())
	res.GoMediaMS = msSince(start)

	if a.ID == "mp4_sidecar_subs" {
		dir := filepath.Dir(a.Path)
		names, _ := os.ReadDir(dir)
		var sibs []string
		for _, e := range names {
			sibs = append(sibs, e.Name())
		}
		ext := media.ExternalSubtitleTracks(a.Path, sibs)
		if len(ext) >= 2 {
			vtt, err := media.SubtitleToWebVTT(filepath.Join(dir, ext[0].Name))
			if err != nil || !strings.Contains(vtt, "WEBVTT") {
				res.Status = StatusMismatch
				res.Detail = "sidecar WebVTT conversion failed"
				return res
			}
			res.Status = StatusSupported
			res.Detail = fmt.Sprintf("%d sidecar tracks", len(ext))
			return res
		}
		res.Status = StatusMismatch
		res.Detail = fmt.Sprintf("expected >=2 sidecar tracks, got %d", len(ext))
		return res
	}

	if a.ID == "mp4_embedded_subs" {
		if listErr != nil {
			res.Status = classifyGoErr(listErr)
			res.Detail = listErr.Error()
			return res
		}
		if len(tracks) < 2 {
			res.Status = StatusMismatch
			res.Detail = fmt.Sprintf("expected >=2 embedded tracks, got %d", len(tracks))
			return res
		}
		f2, err := openAsset(a)
		if err != nil {
			res.Status = StatusError
			return res
		}
		for _, tr := range tracks[:2] {
			vtt, exErr := media.ExtractEmbeddedSubtitle(f2, f2.Size(), tr.Index)
			if exErr != nil || !strings.Contains(vtt, "WEBVTT") {
				res.Status = StatusMismatch
				res.Detail = fmt.Sprintf("track %d (%s): %v", tr.Index, tr.Codec, exErr)
				return res
			}
		}
		res.Status = StatusSupported
		res.Detail = fmt.Sprintf("%d embedded tracks", len(tracks))
		return res
	}

	if a.ID == "mkv_subtitles" {
		if listErr != nil {
			res.Status = classifyGoErr(listErr)
			res.Detail = listErr.Error()
			return res
		}
		if len(tracks) == 0 {
			res.Status = StatusMismatch
			res.Detail = "expected embedded subtitle track"
			return res
		}
		f2, err := openAsset(a)
		if err != nil {
			res.Status = StatusError
			return res
		}
		vtt, exErr := media.ExtractEmbeddedSubtitle(f2, f2.Size(), tracks[0].Index)
		if exErr != nil || !strings.Contains(vtt, "WEBVTT") {
			res.Status = StatusMismatch
			res.Detail = fmt.Sprintf("extract: %v", exErr)
			return res
		}
		res.Status = StatusSupported
		return res
	}

	if listErr != nil && errors.Is(listErr, media.ErrUnsupported) {
		res.Status = StatusTypedUnsupported
		res.Detail = listErr.Error()
		return res
	}
	res.Status = StatusSkipped
	res.Detail = "subtitles check not applicable for this fixture"
	return res
}

// checkThumbnail compares go-media Thumbnail with an ffmpeg frame when video is present.
func checkThumbnail(a Asset, thumbDir string) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckThumbnail}
	if a.Category != "video" {
		res.Status = StatusSkipped
		res.Detail = "audio fixture"
		return res
	}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	start := time.Now()
	tr, gErr := media.Thumbnail(f, f.Size(), media.ThumbnailOptions{
		MaxWidth: 320, MaxHeight: 240, SeekPercent: 10, Format: "jpeg",
	})
	res.GoMediaMS = msSince(start)

	goPath := filepath.Join(thumbDir, a.ID+"_go.jpg")
	ffPath := filepath.Join(thumbDir, a.ID+"_ff.jpg")
	res.ThumbGoPath = relReportPath(goPath)
	res.ThumbFFPath = relReportPath(ffPath)

	ffStart := time.Now()
	ffErr := ffmpegThumbnail(a.Path, ffPath, 10)
	res.FFmpegMS = msSince(ffStart)

	if gErr != nil {
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	if err := os.WriteFile(goPath, tr.Data, 0o644); err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	if ffErr != nil {
		res.Status = StatusError
		res.Detail = ffErr.Error()
		return res
	}
	psnr, ssim, mErr := imagePSNRSSIM(ffPath, goPath)
	res.PSNR = psnr
	res.SSIM = ssim
	if mErr != nil {
		res.Detail = mErr.Error()
	}
	if ssim >= 0.85 || psnr >= 25 {
		res.Status = StatusSupported
	} else if tr.Width > 0 {
		res.Status = StatusMismatch
		res.Detail = fmt.Sprintf("visual delta PSNR=%.1f SSIM=%.3f", psnr, ssim)
	} else {
		res.Status = StatusSupported
	}
	return res
}

func classifyGoErr(err error) CapabilityStatus {
	if errors.Is(err, media.ErrUnsupported) || errors.Is(err, media.ErrUnsupportedProfile) {
		return StatusTypedUnsupported
	}
	return StatusError
}

func streamCounts(streams []media.Stream) (video, audio int) {
	for _, s := range streams {
		switch s.Type {
		case media.StreamVideo:
			video++
		case media.StreamAudio:
			audio++
		}
	}
	return video, audio
}

func ffStreamCounts(streams []FFProbeStream, skipAttachedPic bool) (video, audio int) {
	for _, s := range streams {
		if skipAttachedPic && s.AttachedPic == 1 {
			continue
		}
		switch s.CodecType {
		case "video":
			video++
		case "audio":
			audio++
		}
	}
	return video, audio
}

func msSince(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func relReportPath(abs string) string {
	// Stored relative to report_site root for static hosting.
	const marker = "report_site/"
	if i := strings.Index(abs, marker); i >= 0 {
		return abs[i+len(marker):]
	}
	return filepath.Base(abs)
}
