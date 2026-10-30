// Package ffmpegtest holds helpers shared by the subtitle golden tests in
// internal/subtitle, internal/matroska and internal/isobmff. Only _test.go
// files import it, so it never reaches the library.
package ffmpegtest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// LiveFFmpeg returns the path of the ffmpeg on PATH for tests that diff
// go-media against a live ffmpeg, or skips t when there is none or when its
// first "ffmpeg -version" line differs from the one recorded in
// versionFile (testdata/subtitle-ffmpeg-version.txt, written by
// testdata/gen-subtitles.sh). Subtitle output changes between ffmpeg
// releases (7.1 and 9.0 differ on WebVTT tags and on ASS events with
// non-positive durations), so only the version the goldens pin is a valid
// reference; the golden tests run regardless.
func LiveFFmpeg(t testing.TB, versionFile string) string {
	t.Helper()
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("live ffmpeg comparison skipped: ffmpeg not on PATH")
	}
	want, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatalf("read recorded ffmpeg version: %v", err)
	}
	out, err := exec.Command(ff, "-version").Output()
	if err != nil {
		t.Skipf("live ffmpeg comparison skipped: %s -version: %v", ff, err)
	}
	first, _, _ := strings.Cut(string(out), "\n")
	got, w := strings.TrimSpace(first), strings.TrimSpace(string(want))
	if got != w {
		t.Skipf("live ffmpeg comparison skipped: %s is %q but the goldens were made with %q (scripts/install-ffmpeg.sh installs it)", ff, got, w)
	}
	return ff
}
