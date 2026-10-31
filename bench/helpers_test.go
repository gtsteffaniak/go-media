package bench

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gtsteffaniak/go-media/bench/manifest"
	"github.com/gtsteffaniak/go-media/pkg/media"
)

// Environment overrides:
//
//	GOMEDIA_BENCH_DATA      fixture directory (default bench/.data)
//	GOMEDIA_BENCH_MANIFEST  manifest path (default bench/manifest.json)
const (
	envData     = "GOMEDIA_BENCH_DATA"
	envManifest = "GOMEDIA_BENCH_MANIFEST"
)

var (
	manOnce sync.Once
	man     *manifest.Manifest
	manErr  error
)

// loadManifest parses the manifest once per test binary.
func loadManifest(tb testing.TB) *manifest.Manifest {
	tb.Helper()
	manOnce.Do(func() {
		path := os.Getenv(envManifest)
		if path == "" {
			path = "manifest.json"
		}
		man, manErr = manifest.Load(path)
	})
	if manErr != nil {
		tb.Fatalf("load manifest: %v", manErr)
	}
	return man
}

// dataDir returns the fixture directory.
func dataDir() string {
	if d := os.Getenv(envData); d != "" {
		return d
	}
	return ".data"
}

// input is one fixture loaded fully into memory. Benchmarks measure CPU
// work on the bytes (the same in-memory preload the compare harness uses),
// so disk latency does not leak into the numbers.
type input struct {
	fx   manifest.Fixture
	path string
	data []byte
	r    *bytes.Reader
}

var inputs sync.Map // fixture name -> *input

// loadInput returns the loaded fixture, skipping the benchmark when the
// generated file is missing (run `go run ./cmd/gomedia-bench gen`).
func loadInput(b *testing.B, fx manifest.Fixture) *input {
	b.Helper()
	if v, ok := inputs.Load(fx.Name); ok {
		return v.(*input)
	}
	path := filepath.Join(dataDir(), fx.File)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("fixture %s missing (%v); run: go run ./cmd/gomedia-bench gen", fx.Name, err)
	}
	in := &input{fx: fx, path: path, data: data, r: bytes.NewReader(data)}
	actual, _ := inputs.LoadOrStore(fx.Name, in)
	return actual.(*input)
}

// fixturesWhere returns manifest entries matching keep.
func fixturesWhere(tb testing.TB, keep func(manifest.Fixture) bool) []manifest.Fixture {
	tb.Helper()
	return loadManifest(tb).Where(keep)
}

// isMP4H264Thumb selects fixtures for the H.264 keyframe APIs. Those read the
// avcC track from an MP4 sample table, so AVI and MPEG-TS H.264 fixtures (which
// still get the Thumbnail benchmarks) are excluded here.
func isMP4H264Thumb(fx manifest.Fixture) bool {
	return fx.Thumb && fx.Codec == "h264" && fx.Container == "mp4"
}

func isMedia(fx manifest.Fixture) bool {
	return fx.Kind == manifest.KindVideo || fx.Kind == manifest.KindAudio
}

// forEach runs one sub-benchmark per matching fixture.
func forEach(b *testing.B, keep func(manifest.Fixture) bool, fn func(b *testing.B, in *input)) {
	b.Helper()
	fxs := fixturesWhere(b, keep)
	if len(fxs) == 0 {
		b.Skip("no fixtures match this benchmark in the manifest")
	}
	for _, fx := range fxs {
		b.Run(fx.Name, func(b *testing.B) {
			fn(b, loadInput(b, fx))
		})
	}
}

// sinks keep results alive so the compiler cannot drop the measured work.
var (
	sinkInt   int
	sinkInt64 int64
	sinkStr   string
	sinkBytes []byte
	sinkBool  bool
	sinkAny   any
)

// tolerated reports whether err is an acceptable outcome for in: nil, a
// corrupt fixture (its error path is the measurement), or a typed
// unsupported result (for example tags on MKV, which go-media does not read).
func tolerated(in *input, err error) bool {
	return err == nil || in.fx.Corrupt ||
		errors.Is(err, media.ErrUnsupported) || errors.Is(err, media.ErrUnsupportedProfile)
}

// check fails the benchmark on an error that is not tolerated.
func check(b *testing.B, in *input, err error, what string) {
	b.Helper()
	if !tolerated(in, err) {
		b.Fatalf("%s(%s): %v", what, in.fx.Name, err)
	}
}

func describe(in *input) string {
	return fmt.Sprintf("%s (%s, %d bytes)", in.fx.Name, in.fx.Container, len(in.data))
}
