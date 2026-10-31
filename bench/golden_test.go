package bench

// Golden output checks. Every public operation is run on every fixture and
// the output (or the error text) is hashed. The hashes live in
// bench/golden/<GOOS>_<GOARCH>.json. A performance change is only acceptable
// when this test still passes, which proves the bytes, pixels and metadata
// did not change. Hashes are per platform because the YCbCr and H.264 kernels
// have architecture-specific code paths.
//
// The test also records the SHA-256 of each input fixture. If an input file
// differs from the recorded one (a different ffmpeg build, for example), the
// test reports that instead of comparing outputs of a different input.
//
// Update the goldens only for intentional output changes, with reasons in the
// commit message:
//
//	GOMEDIA_BENCH_UPDATE_GOLDEN=1 go test ./bench/ -run TestGoldenOutputs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/gtsteffaniak/go-media/bench/manifest"
	"github.com/gtsteffaniak/go-media/pkg/media"
)

const envUpdateGolden = "GOMEDIA_BENCH_UPDATE_GOLDEN"

// goldenFile is the on-disk format: fixture name -> op -> hash.
type goldenFile struct {
	Platform string                       `json:"platform"`
	Fixtures map[string]map[string]string `json:"fixtures"`
}

func goldenPath() string {
	return filepath.Join("golden", runtime.GOOS+"_"+runtime.GOARCH+".json")
}

func digest(parts ...any) string {
	h := sha256.New()
	for _, p := range parts {
		switch v := p.(type) {
		case []byte:
			h.Write(v)
		case string:
			h.Write([]byte(v))
		default:
			b, err := json.Marshal(v)
			if err != nil {
				fmt.Fprintf(h, "marshal-error:%v", err)
				continue
			}
			h.Write(b)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// errOr returns the error text, so error behaviour is part of the golden.
func errOr(err error) string {
	if err == nil {
		return ""
	}
	return "err:" + err.Error()
}

// computeGolden runs every operation on one fixture.
func computeGolden(t *testing.T, fx manifest.Fixture, path string, data []byte) map[string]string {
	t.Helper()
	out := map[string]string{"input": digest(data)}
	r := bytes.NewReader(data)
	size := int64(len(data))
	add := func(op string, parts ...any) { out[op] = digest(parts...) }

	info, err := media.Probe(r, size)
	add("probe", info, errOr(err))
	d, err := media.Duration(r, size)
	add("duration", d, errOr(err))
	tags, err := media.ReadTags(r, size)
	add("tags", tags, errOr(err))
	add("has_artwork", media.HasArtwork(r, size))
	art, mime, src, err := media.EmbeddedArtwork(r, size)
	add("artwork", art, mime, src, errOr(err))
	lyr, hint, err := media.LyricsFromReader(r, size)
	add("lyrics", lyr, hint, errOr(err))
	add("has_lyrics", media.HasLyrics(r, size, path))
	sl, shint, sok := media.LyricsSidecar(path)
	add("lyrics_sidecar", sl, shint, sok)
	ai, err := media.InspectAudio(r, size, media.AudioOptions{WithArtwork: true})
	add("inspect_audio", ai, errOr(err))
	tracks, err := media.ListSubtitleTracks(r, size)
	add("subtitle_tracks", tracks, errOr(err))
	for _, tr := range tracks {
		vtt, err := media.ExtractEmbeddedSubtitle(r, size, tr.Index)
		add(fmt.Sprintf("subtitle_vtt_%d", tr.Index), vtt, errOr(err))
	}
	kf, err := media.MP4KeyframeSamples(r, size, 0)
	add("mp4_keyframes", kf, errOr(err))

	if fx.Thumb {
		for _, preset := range []media.ThumbnailPreset{media.ThumbnailPresetSmall, media.ThumbnailPresetLarge} {
			opt := media.OptionsForPreset(preset, 10)
			res, err := media.Thumbnail(r, size, opt)
			add("thumb_"+string(preset), res.MIME, res.Data, res.Width, res.Height, res.Source, errOr(err))
		}
		opt := media.OptionsForPreset(media.ThumbnailPresetSmall, 10)
		opt.Format = "webp-lossless"
		res, err := media.Thumbnail(r, size, opt)
		add("thumb_webp", res.MIME, res.Data, res.Width, res.Height, errOr(err))
		png, err := media.ThumbnailRasterPNG(r, size, opt)
		add("thumb_png_raster", png, errOr(err))
	}
	if isMP4H264Thumb(fx) {
		yuv, w, h, err := media.H264KeyframeYUV420(r, size, 10, true)
		add("h264_yuv", yuv, w, h, errOr(err))
		sec, err := media.H264KeyframeTimeSec(r, size, 10)
		add("h264_time", fmt.Sprintf("%.9f", sec), errOr(err))
	}
	if fx.Kind == manifest.KindSubtitle {
		vtt, err := media.SubtitleToWebVTT(path)
		add("sidecar_vtt", vtt, errOr(err))
	}
	return out
}

// TestGoldenOutputs compares every fixture's outputs with the recorded goldens.
func TestGoldenOutputs(t *testing.T) {
	m := loadManifest(t)
	gp := goldenPath()
	update := os.Getenv(envUpdateGolden) == "1"

	var recorded goldenFile
	if b, err := os.ReadFile(gp); err == nil {
		if err := json.Unmarshal(b, &recorded); err != nil {
			t.Fatalf("parse %s: %v", gp, err)
		}
	} else if !update {
		// Goldens are a local before/after check, not a CI gate: record them
		// on the base commit, then rerun after a change.
		t.Skipf("no golden file %s for %s/%s; record one on the base commit with %s=1", gp, runtime.GOOS, runtime.GOARCH, envUpdateGolden)
	}
	if recorded.Fixtures == nil {
		recorded.Fixtures = map[string]map[string]string{}
	}

	current := goldenFile{Platform: runtime.GOOS + "/" + runtime.GOARCH, Fixtures: map[string]map[string]string{}}
	ran := 0
	for _, fx := range m.Fixtures {
		path := filepath.Join(dataDir(), fx.File)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Logf("skip %s: fixture missing (%v)", fx.Name, err)
			continue
		}
		ran++
		got := computeGolden(t, fx, path, data)
		current.Fixtures[fx.Name] = got
		if update {
			continue
		}
		want, ok := recorded.Fixtures[fx.Name]
		if !ok {
			t.Errorf("%s: no golden entry; regenerate with %s=1", fx.Name, envUpdateGolden)
			continue
		}
		if want["input"] != got["input"] {
			t.Errorf("%s: input fixture changed (generator or ffmpeg differs); regenerate fixtures and goldens", fx.Name)
			continue
		}
		for _, op := range sortedKeys(got) {
			if want[op] != got[op] {
				t.Errorf("%s: output %q changed (golden %s, got %s)", fx.Name, op, short(want[op]), short(got[op]))
			}
		}
		for _, op := range sortedKeys(want) {
			if _, ok := got[op]; !ok {
				t.Errorf("%s: golden op %q no longer produced", fx.Name, op)
			}
		}
	}
	if ran == 0 {
		t.Skip("no fixtures generated; run: go run ./cmd/gomedia-bench gen")
	}
	if update {
		if err := os.MkdirAll(filepath.Dir(gp), 0o755); err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(gp, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d fixtures)", gp, len(current.Fixtures))
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
