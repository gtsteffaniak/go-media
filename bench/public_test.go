package bench

// End-to-end benchmarks for the public API in pkg/media. Every function that
// takes media input gets one sub-benchmark per applicable fixture, so a new
// manifest entry is measured automatically. Names follow
// BenchmarkOp/<fixture>; the runner and compare tool group them by op and
// by container format.

import (
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/bench/manifest"
	"github.com/gtsteffaniak/go-media/pkg/media"
)

func BenchmarkProbe(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		info, err := media.Probe(in.r, int64(len(in.data)))
		check(b, in, err, "Probe")
		if info == nil && !in.fx.Corrupt {
			b.Fatal("Probe returned nil info")
		}
		for b.Loop() {
			info, err := media.Probe(in.r, int64(len(in.data)))
			if !tolerated(in, err) {
				b.Fatal(err)
			}
			sinkAny = info
		}
	})
}

func BenchmarkDuration(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		_, err := media.Duration(in.r, size)
		check(b, in, err, "Duration")
		for b.Loop() {
			d, err := media.Duration(in.r, size)
			if !tolerated(in, err) {
				b.Fatal(err)
			}
			sinkInt64 = int64(d.Duration)
		}
	})
}

func BenchmarkReadTags(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		for b.Loop() {
			t, err := media.ReadTags(in.r, size)
			if !tolerated(in, err) {
				b.Fatal(err)
			}
			sinkStr = t.Title
		}
	})
}

func BenchmarkHasArtwork(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		for b.Loop() {
			sinkBool = media.HasArtwork(in.r, size)
		}
	})
}

func BenchmarkEmbeddedArtwork(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Artwork }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		data, _, _, err := media.EmbeddedArtwork(in.r, size)
		check(b, in, err, "EmbeddedArtwork")
		if len(data) == 0 {
			b.Fatalf("EmbeddedArtwork(%s): no artwork bytes", in.fx.Name)
		}
		for b.Loop() {
			d, m, _, err := media.EmbeddedArtwork(in.r, size)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = d
			sinkStr = m
		}
	})
}

func BenchmarkInspectAudio(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		opts := media.AudioOptions{WithArtwork: true}
		_, err := media.InspectAudio(in.r, size, opts)
		check(b, in, err, "InspectAudio")
		for b.Loop() {
			info, err := media.InspectAudio(in.r, size, opts)
			if !tolerated(in, err) {
				b.Fatal(err)
			}
			sinkAny = info
		}
	})
}

func BenchmarkLyricsFromReader(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		for b.Loop() {
			text, _, _ := media.LyricsFromReader(in.r, size)
			sinkStr = text
		}
	})
}

func BenchmarkHasLyrics(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		for b.Loop() {
			sinkBool = media.HasLyrics(in.r, size, in.path)
		}
	})
}

func BenchmarkLyricsSidecar(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		for b.Loop() {
			text, _, ok := media.LyricsSidecar(in.path)
			sinkStr = text
			sinkBool = ok
		}
	})
}

func BenchmarkListSubtitleTracks(b *testing.B) {
	forEach(b, isMedia, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		tracks, err := media.ListSubtitleTracks(in.r, size)
		check(b, in, err, "ListSubtitleTracks")
		if in.fx.Subtitles && len(tracks) == 0 {
			b.Fatalf("ListSubtitleTracks(%s): fixture marked subtitles but none found", in.fx.Name)
		}
		for b.Loop() {
			t, err := media.ListSubtitleTracks(in.r, size)
			if !tolerated(in, err) {
				b.Fatal(err)
			}
			sinkInt = len(t)
		}
	})
}

func BenchmarkExtractEmbeddedSubtitle(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Subtitles }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		tracks, err := media.ListSubtitleTracks(in.r, size)
		check(b, in, err, "ListSubtitleTracks")
		if len(tracks) == 0 {
			b.Fatalf("no subtitle tracks in %s", in.fx.Name)
		}
		idx := tracks[0].Index
		vtt, err := media.ExtractEmbeddedSubtitle(in.r, size, idx)
		check(b, in, err, "ExtractEmbeddedSubtitle")
		if vtt == "" {
			b.Fatalf("ExtractEmbeddedSubtitle(%s): empty output", in.fx.Name)
		}
		for b.Loop() {
			s, err := media.ExtractEmbeddedSubtitle(in.r, size, idx)
			if err != nil {
				b.Fatal(err)
			}
			sinkStr = s
		}
	})
}

// BenchmarkSubtitleToWebVTT converts sidecar text subtitles (SRT, ASS, VTT).
func BenchmarkSubtitleToWebVTT(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Kind == manifest.KindSubtitle }, func(b *testing.B, in *input) {
		vtt, err := media.SubtitleToWebVTT(in.path)
		if in.fx.Container != "vtt" {
			check(b, in, err, "SubtitleToWebVTT")
		}
		if err == nil && vtt == "" {
			b.Fatalf("SubtitleToWebVTT(%s): empty output", in.fx.Name)
		}
		for b.Loop() {
			s, err := media.SubtitleToWebVTT(in.path)
			if !tolerated(in, err) && in.fx.Container != "vtt" {
				b.Fatal(err)
			}
			sinkStr = s
		}
	})
}

// BenchmarkExternalSubtitleTracks matches sidecar names next to a video.
// Pure string work; no fixture needed.
func BenchmarkExternalSubtitleTracks(b *testing.B) {
	video := filepath.Join("movies", "Feature.Film.2024.mkv")
	siblings := []string{
		"Feature.Film.2024.mkv", "Feature.Film.2024.en.srt", "Feature.Film.2024.de.vtt",
		"Feature.Film.2024.forced.ass", "Feature.Film.2024.fr.ssa", "Feature.Film.2024.nfo",
		"Feature.Film.2024.jpg", "Feature.Film.2024.sup", "Other.Movie.srt", "Feature.Film.2024.srt",
		"Feature.Film.2024.es-419.srt", "Feature.Film.2024.hi.vtt", "poster.jpg", "readme.txt",
	}
	for b.Loop() {
		sinkInt = len(media.ExternalSubtitleTracks(video, siblings))
	}
}

// Thumbnail benchmarks cover the FileBrowser presets and the PNG/WebP paths.
// They run only on fixtures with "thumb": true; unsupported codecs (HEVC,
// VP9) are listed in the manifest with "thumb": false and skipped here.
func BenchmarkThumbnailSmall(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Thumb }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		res, err := media.ThumbnailSmall(in.r, size, 10)
		check(b, in, err, "ThumbnailSmall")
		if len(res.Data) == 0 {
			b.Fatalf("ThumbnailSmall(%s): empty output", in.fx.Name)
		}
		for b.Loop() {
			r, err := media.ThumbnailSmall(in.r, size, 10)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = r.Data
		}
	})
}

func BenchmarkThumbnailLarge(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Thumb }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		res, err := media.ThumbnailLarge(in.r, size, 10)
		check(b, in, err, "ThumbnailLarge")
		if len(res.Data) == 0 {
			b.Fatalf("ThumbnailLarge(%s): empty output", in.fx.Name)
		}
		for b.Loop() {
			r, err := media.ThumbnailLarge(in.r, size, 10)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = r.Data
		}
	})
}

// BenchmarkThumbnailOptions exercises the generic Thumbnail entry point with
// explicit options: WebP (lossless) output and the PNG raster path.
func BenchmarkThumbnailWebP(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Thumb }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		opt := media.OptionsForPreset(media.ThumbnailPresetSmall, 10)
		opt.Format = "webp-lossless"
		res, err := media.Thumbnail(in.r, size, opt)
		check(b, in, err, "Thumbnail(webp)")
		if err == nil && len(res.Data) == 0 {
			b.Fatalf("Thumbnail(webp, %s): empty output", in.fx.Name)
		}
		for b.Loop() {
			r, err := media.Thumbnail(in.r, size, opt)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = r.Data
		}
	})
}

func BenchmarkThumbnailRasterPNG(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Thumb }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		opt := media.OptionsForPreset(media.ThumbnailPresetSmall, 10)
		out, err := media.ThumbnailRasterPNG(in.r, size, opt)
		check(b, in, err, "ThumbnailRasterPNG")
		if err == nil && len(out) == 0 {
			b.Fatalf("ThumbnailRasterPNG(%s): empty output", in.fx.Name)
		}
		for b.Loop() {
			d, err := media.ThumbnailRasterPNG(in.r, size, opt)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = d
		}
	})
}

func BenchmarkH264KeyframeYUV420(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Thumb && fx.Codec == "h264" }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		out, w, h, err := media.H264KeyframeYUV420(in.r, size, 10, true)
		check(b, in, err, "H264KeyframeYUV420")
		if err == nil && (len(out) == 0 || w == 0 || h == 0) {
			b.Fatalf("H264KeyframeYUV420(%s): empty output", in.fx.Name)
		}
		for b.Loop() {
			o, _, _, err := media.H264KeyframeYUV420(in.r, size, 10, true)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = o
		}
	})
}

func BenchmarkH264KeyframeTimeSec(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Thumb && fx.Codec == "h264" }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		sec, err := media.H264KeyframeTimeSec(in.r, size, 10)
		check(b, in, err, "H264KeyframeTimeSec")
		_ = sec
		for b.Loop() {
			s, err := media.H264KeyframeTimeSec(in.r, size, 10)
			if err != nil {
				b.Fatal(err)
			}
			sinkInt64 = int64(s * 1e6)
		}
	})
}

func BenchmarkMP4KeyframeSamples(b *testing.B) {
	forEach(b, func(fx manifest.Fixture) bool { return fx.Container == "mp4" && fx.Kind == manifest.KindVideo }, func(b *testing.B, in *input) {
		size := int64(len(in.data))
		refs, err := media.MP4KeyframeSamples(in.r, size, 0)
		check(b, in, err, "MP4KeyframeSamples")
		for b.Loop() {
			r, err := media.MP4KeyframeSamples(in.r, size, 0)
			if !tolerated(in, err) {
				b.Fatal(err)
			}
			sinkInt = len(r)
		}
		_ = refs
	})
}
