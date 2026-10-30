package subtitle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sidecarFixtures lists every sidecar fixture with its ffmpeg input
// options. srt_structure.srt starts with garbage that defeats ffmpeg's
// probe, so ffmpeg needs -f srt; go-media picks the parser by extension.
var sidecarFixtures = []struct {
	name    string
	ffInput []string
}{
	{"srt_markup.srt", nil},
	{"srt_structure.srt", []string{"-f", "srt"}},
	{"srt_bom_crlf.srt", nil},
	{"srt_utf16le.srt", nil},
	{"vtt_features.vtt", nil},
	{"vtt_bom_crlf.vtt", nil},
	{"vtt_empty.vtt", nil},
	{"ass_v4plus.ass", nil},
	{"ssa_v4.ssa", nil},
	{"ass_format_order.ass", nil},
}

// convertFixture runs SidecarToWebVTT on a testdata fixture.
func convertFixture(t *testing.T, name string) string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := SidecarToWebVTT(filepath.Ext(name), f)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	return got
}

// TestSidecarFFmpegGoldens compares go-media with ffmpeg 7.1.5's output
// (testdata/<fixture>.ffmpeg.vtt, written by testdata/gen-subtitles.sh at
// the repository root) byte for byte for every sidecar kind: SRT, WebVTT,
// ASS and SSA.
func TestSidecarFFmpegGoldens(t *testing.T) {
	for _, tc := range sidecarFixtures {
		t.Run(tc.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".ffmpeg.vtt"))
			if err != nil {
				t.Fatal(err)
			}
			if got := convertFixture(t, tc.name); got != string(want) {
				t.Errorf("differs from ffmpeg\n got: %q\nwant: %q", got, want)
			}
		})
	}
	goldens, _ := filepath.Glob("testdata/*.ffmpeg.vtt")
	if len(goldens) != len(sidecarFixtures) {
		t.Errorf("%d goldens on disk but %d fixtures in the table", len(goldens), len(sidecarFixtures))
	}
}

// TestSidecarLiveFFmpeg compares go-media with the installed ffmpeg. It is
// skipped when ffmpeg is not on PATH.
func TestSidecarLiveFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	for _, tc := range sidecarFixtures {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-nostdin", "-v", "error"}, tc.ffInput...)
			args = append(args, "-i", filepath.Join("testdata", tc.name), "-map", "0:s:0", "-f", "webvtt", "-")
			want, err := exec.Command(ff, args...).Output()
			if err != nil {
				t.Fatalf("ffmpeg: %v", err)
			}
			if got := convertFixture(t, tc.name); got != string(want) {
				t.Errorf("differs from live ffmpeg\n got: %q\nwant: %q", got, strings.TrimSpace(string(want)))
			}
		})
	}
}
