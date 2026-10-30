package matroska

import (
	"os"
	"strings"
	"testing"
)

// TestExtractSubtitleMatchesFFmpeg checks cue text and end times against
// ffmpeg 9.0.2 `-map 0:s:0 out.vtt` for WebM WebVTT (cue id + settings in the
// block), SRT and ASS tracks. Cue settings, identifiers, ASS Dialogue fields
// and override tags must not leak into the cue text.
func TestExtractSubtitleMatchesFFmpeg(t *testing.T) {
	for _, tc := range []struct{ file, first string }{
		{"sub_vtt.mkv", "Hello <i>there</i>"},
		{"sub_srt.mkv", "<i>Hello</i>"},
		{"sub_ass.mkv", "Hello"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			f, err := os.Open("testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			st, _ := f.Stat()
			got, err := ExtractSubtitleWebVTT(f, st.Size(), 0)
			if err != nil {
				t.Fatal(err)
			}
			want := "WEBVTT\n\n" +
				"00:01.000 --> 00:03.000\n" + tc.first + "\n\n" +
				"00:04.000 --> 00:06.000\nSecond\n\n" +
				"00:10.000 --> 00:11.000\nThird\n"
			if got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
			for _, bad := range []string{"align:", "position:", "line:", "c1", "Default,", "{\\"} {
				if strings.Contains(got, bad) {
					t.Errorf("cue text leaked %q", bad)
				}
			}
		})
	}
}
