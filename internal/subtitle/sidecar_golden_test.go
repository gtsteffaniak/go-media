package subtitle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/ffmpegtest"
)

// sidecarFixtures lists every sidecar fixture with its ffmpeg input
// options. srt_structure.srt starts with garbage that defeats ffmpeg's
// probe, so ffmpeg needs -f srt; go-media picks the parser by extension.
//
// diffs lists documented, intentional differences from ffmpeg as
// (ffmpeg text, go-media text) replacements; the goldens already carry the
// go-media side and the live test applies them to ffmpeg's output.
var sidecarFixtures = []struct {
	name    string
	ffInput []string
	diffs   [][2]string
}{
	{"srt_markup.srt", nil, nil},
	// The last cue ends before it starts: ffmpeg wraps its uint32 duration
	// to ~1193 h, go-media makes it zero-length (no later cue to end at).
	{"srt_structure.srt", []string{"-f", "srt"}, [][2]string{{"00:13.000 --> 1193:02:59.296", "00:13.000 --> 00:13.000"}}},
	{"srt_bom_crlf.srt", nil, nil},
	{"srt_utf16le.srt", nil, nil},
	{"vtt_features.vtt", nil, nil},
	{"vtt_bom_crlf.vtt", nil, nil},
	{"vtt_empty.vtt", nil, nil},
	// ffmpeg 9.0 tag-prefix matching and inline-timestamp karaoke.
	{"vtt_tags_karaoke.vtt", nil, nil},
	{"ass_v4plus.ass", nil, nil},
	{"ssa_v4.ssa", nil, nil},
	{"ass_format_order.ass", nil, nil},
	// ffmpeg 9.0 drops zero- and negative-duration ASS events.
	{"ass_nonpositive.ass", nil, nil},
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

// TestSidecarFFmpegGoldens compares go-media with the pinned ffmpeg's output (see
// testdata/subtitle-ffmpeg-version.txt)
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
// skipped unless that ffmpeg is the version the goldens were made with.
func TestSidecarLiveFFmpeg(t *testing.T) {
	ff := ffmpegtest.LiveFFmpeg(t, "../../testdata/subtitle-ffmpeg-version.txt")
	for _, tc := range sidecarFixtures {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-nostdin", "-v", "error"}, tc.ffInput...)
			args = append(args, "-i", filepath.Join("testdata", tc.name), "-map", "0:s:0", "-f", "webvtt", "-")
			want, err := exec.Command(ff, args...).Output()
			if err != nil {
				t.Fatalf("ffmpeg: %v", err)
			}
			exp := string(want)
			for _, d := range tc.diffs {
				if !strings.Contains(exp, d[0]) {
					t.Errorf("documented difference %q no longer in ffmpeg output", d[0])
				}
				exp = strings.ReplaceAll(exp, d[0], d[1])
			}
			if got := convertFixture(t, tc.name); got != exp {
				t.Errorf("differs from live ffmpeg\n got: %q\nwant: %q", got, exp)
			}
		})
	}
}

// TestEndBeforeStartClamped pins the documented difference from ffmpeg for
// cues whose end is before their start, on every sidecar parser: the cue
// ends at the next later cue's start, or is zero-length if none follows,
// instead of ffmpeg's wrapped ~1193 h end that never leaves the screen.
func TestEndBeforeStartClamped(t *testing.T) {
	const want = "WEBVTT\n\n00:05.000 --> 00:07.000\nBack\n\n00:07.000 --> 00:08.000\nNext\n\n00:09.000 --> 00:09.000\nLast\n"
	const assHead = "[Script Info]\nScriptType: v4.00+\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
	// ASS/SSA never get here: like ffmpeg 9.0, the ASS demuxer drops events
	// with a zero or negative duration.
	for ext, in := range map[string]string{
		".srt": "1\n00:00:05,000 --> 00:00:02,000\nBack\n\n2\n00:00:07,000 --> 00:00:08,000\nNext\n\n3\n00:00:09,000 --> 00:00:01,000\nLast\n",
		".vtt": "WEBVTT\n\n00:05.000 --> 00:02.000\nBack\n\n00:07.000 --> 00:08.000\nNext\n\n00:09.000 --> 00:01.000\nLast\n",
	} {
		got, err := SidecarToWebVTT(ext, strings.NewReader(in))
		if err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", ext, got, want)
		}
	}
	got, err := SidecarToWebVTT(".ass", strings.NewReader(assHead+"Dialogue: 0,0:00:05.00,0:00:02.00,Default,,0,0,0,,Back\nDialogue: 0,0:00:07.00,0:00:08.00,Default,,0,0,0,,Next\nDialogue: 0,0:00:09.00,0:00:09.00,Default,,0,0,0,,Zero\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "WEBVTT\n\n00:07.000 --> 00:08.000\nNext\n"; got != want {
		t.Errorf(".ass: got %q, want %q", got, want)
	}
	got, err = TTMLOrXMLToWebVTT(`<tt><p begin="5s" end="2s">Back</p><p begin="7s" end="8s">Next</p><p begin="9s" end="1s">Last</p></tt>`)
	if err != nil {
		t.Fatal(err)
	}
	const wantTTML = "WEBVTT\n\n00:00:05.000 --> 00:00:07.000\nBack\n\n00:00:07.000 --> 00:00:08.000\nNext\n\n00:00:09.000 --> 00:00:09.000\nLast\n"
	if got != wantTTML {
		t.Errorf("ttml: got %q, want %q", got, wantTTML)
	}
}
