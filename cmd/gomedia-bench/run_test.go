package main

import (
	"regexp"
	"strings"
	"testing"
)

// matchPattern reports whether a -bench pattern selects a benchmark name such
// as "BenchmarkThumbnailLarge/mp4_h264_1920x1080_60s_large", using the rule go
// test uses: each "/"-separated pattern level is matched against the name level
// at the same depth. A shorter pattern selects every sub-benchmark beneath a
// matching parent, so only the levels the pattern has are compared.
func matchPattern(t *testing.T, pattern, name string) bool {
	t.Helper()
	pl := strings.Split(pattern, "/")
	nl := strings.Split(name, "/")
	if len(pl) > len(nl) {
		return false
	}
	for i := range pl {
		re, err := regexp.Compile(pl[i])
		if err != nil {
			t.Fatalf("pattern level %q: %v", pl[i], err)
		}
		if !re.MatchString(nl[i]) {
			return false
		}
	}
	return true
}

func TestBuildBenchPattern(t *testing.T) {
	const (
		opA     = "BenchmarkThumbnailLarge"
		opB     = "BenchmarkThumbnailLargeWebM"
		fixA    = "mp4_h264_1920x1080_60s_large"
		fixLong = "mp4_h264_1920x1080_60s_large_extra"
	)
	cases := []struct {
		name, op, fixture string
		bench             string // expected name to match, with the Benchmark prefix
		want              bool
	}{
		{"op only matches its own sub-benchmark", "ThumbnailLarge", "", opA + "/" + fixA, true},
		{"op only does not match a longer op", "ThumbnailLarge", "", opB + "/" + fixA, false},
		{"fixture only matches that fixture", "", fixA, opA + "/" + fixA, true},
		{"fixture only skips other fixtures", "", fixA, opA + "/mp4_h264_640x360_20s", false},
		{"fixture match is exact, not substring", "", fixA, opA + "/" + fixLong, false},
		{"both selects one op and fixture", "ThumbnailLarge", fixA, opA + "/" + fixA, true},
		{"both rejects same fixture under another op", "ThumbnailLarge", fixA, opB + "/" + fixA, false},
		{"both rejects same op on another fixture", "ThumbnailLarge", fixA, opA + "/" + fixLong, false},
		{"neither matches everything", "", "", opB + "/" + fixA, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := buildBenchPattern(c.op, c.fixture)
			if err != nil {
				t.Fatal(err)
			}
			if got := matchPattern(t, p, c.bench); got != c.want {
				t.Fatalf("pattern %q on %q: got %v, want %v", p, c.bench, got, c.want)
			}
		})
	}
}

func TestBuildBenchPatternRejectsBadNames(t *testing.T) {
	for _, bad := range []string{"Thumb/Large", "a b", "x*", "(x)", "a|b"} {
		if _, err := buildBenchPattern(bad, ""); err == nil {
			t.Errorf("op %q: expected error", bad)
		}
		if _, err := buildBenchPattern("", bad); err == nil {
			t.Errorf("fixture %q: expected error", bad)
		}
	}
}
