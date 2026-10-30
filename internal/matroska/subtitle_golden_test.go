package matroska

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
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

// TestSubtitleFFmpegGoldens compares every Matroska fixture that has a
// <fixture>.ffmpeg.vtt golden (ffmpeg 7.1.5 output, see
// testdata/gen-subtitles.sh at the repository root) byte for byte.
func TestSubtitleFFmpegGoldens(t *testing.T) {
	goldens, err := filepath.Glob("testdata/sub_*.mkv.ffmpeg.vtt")
	if err != nil {
		t.Fatal(err)
	}
	if len(goldens) < 14 {
		t.Fatalf("found %d goldens, want at least 14", len(goldens))
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

// TestSubtitleGoMediaOnly covers S_TEXT/WEBVTT (mkvmerge's WebVTT codec
// ID), which ffmpeg 7.1 cannot read, so its expected output is hand-written:
// header lines before the cue text are dropped, as for D_WEBVTT.
func TestSubtitleGoMediaOnly(t *testing.T) {
	got, err := extractFile(t, "testdata/sub_hand_stextwebvtt.mkv")
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\n\n00:01.000 --> 00:02.000\nWith header lines\n\n00:03.000 --> 00:04.000\nText only <i>cue</i>\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestSubtitleBitmapUnsupported checks that bitmap and other unconvertible
// codecs return a descriptive errs.Unsupported error naming the format
// instead of empty or garbage WebVTT.
func TestSubtitleBitmapUnsupported(t *testing.T) {
	for fixture, needle := range map[string]string{
		"sub_hand_pgs.mkv":     "PGS",
		"sub_hand_vobsub.mkv":  "VobSub",
		"sub_hand_dvbsub.mkv":  "DVB",
		"sub_hand_textst.mkv":  "TEXTST",
		"sub_hand_aribsub.mkv": "ARIB",
	} {
		t.Run(fixture, func(t *testing.T) {
			got, err := extractFile(t, filepath.Join("testdata", fixture))
			if !errors.Is(err, errs.Unsupported) {
				t.Fatalf("err = %v (output %q), want errs.Unsupported", err, got)
			}
			if !strings.Contains(err.Error(), needle) {
				t.Errorf("error %q does not name %s", err, needle)
			}
		})
	}
}

// TestSubtitleLiveFFmpeg re-runs the installed ffmpeg on every golden
// fixture and compares go-media with that output. It is skipped when ffmpeg
// is not on PATH; a failure here with passing goldens means the local ffmpeg
// version behaves differently from the one the goldens were captured with.
func TestSubtitleLiveFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	goldens, _ := filepath.Glob("testdata/sub_*.mkv.ffmpeg.vtt")
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
