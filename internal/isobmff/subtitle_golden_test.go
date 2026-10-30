package isobmff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/ffmpegtest"
)

// extractFile runs ExtractSubtitleWebVTT on stream 0 of a fixture.
func extractFile(t *testing.T, path string) (string, error) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return ExtractSubtitleWebVTT(f, st.Size(), 0)
}

// TestSubtitleFFmpegGoldens compares every MP4 fixture that has a
// <fixture>.ffmpeg.vtt golden (output of the ffmpeg pinned in
// testdata/subtitle-ffmpeg-version.txt, see testdata/gen-subtitles.sh at the repository root) byte for byte.
func TestSubtitleFFmpegGoldens(t *testing.T) {
	goldens, err := filepath.Glob("testdata/sub_*.mp4.ffmpeg.vtt")
	if err != nil {
		t.Fatal(err)
	}
	if len(goldens) < 5 {
		t.Fatalf("found %d goldens, want at least 5", len(goldens))
	}
	for _, g := range goldens {
		fixture := strings.TrimSuffix(g, ".ffmpeg.vtt")
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			want, err := os.ReadFile(g)
			if err != nil {
				t.Fatal(err)
			}
			got, err := extractFile(t, fixture)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if got != string(want) {
				t.Errorf("differs from ffmpeg\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

// TestSubtitleGoMediaOnly covers wvtt (ISO/IEC 14496-30 WebVTT), which
// ffmpeg 7.1 only exposes as a data stream, so its expected output is
// hand-written: one cue per vttc box, cue text passed through the same
// WebVTT->ASS->WebVTT path ffmpeg uses for D_WEBVTT.
func TestSubtitleGoMediaOnly(t *testing.T) {
	got, err := extractFile(t, "testdata/sub_hand_wvtt.mp4")
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\n\n00:01.000 --> 00:02.500\n<i>Hello</i> & \\{{}x}\n\n00:03.000 --> 00:04.000\nTwo cues\n\n00:03.000 --> 00:04.000\nat once\n\n01:00:04.000 --> 01:00:05.234\nPast an hour\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestSubtitleLiveFFmpeg re-runs the installed ffmpeg on every golden
// fixture and compares go-media with that output. It is skipped unless the
// ffmpeg on PATH prints the -version line recorded in
// testdata/subtitle-ffmpeg-version.txt, since other releases legitimately
// produce different bytes.
func TestSubtitleLiveFFmpeg(t *testing.T) {
	ff := ffmpegtest.LiveFFmpeg(t, "../../testdata/subtitle-ffmpeg-version.txt")
	goldens, _ := filepath.Glob("testdata/sub_*.mp4.ffmpeg.vtt")
	for _, g := range goldens {
		fixture := strings.TrimSuffix(g, ".ffmpeg.vtt")
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			want, err := exec.Command(ff, "-nostdin", "-v", "error", "-i", fixture, "-map", "0:s:0", "-f", "webvtt", "-").Output()
			if err != nil {
				t.Fatalf("ffmpeg: %v", err)
			}
			got, err := extractFile(t, fixture)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if got != string(want) {
				t.Errorf("differs from live ffmpeg\n got: %q\nwant: %q", got, want)
			}
		})
	}
}
