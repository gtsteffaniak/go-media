package media_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// keyframeGoldens are ffprobe 7.1/9.0 key packet pts_time values for the
// fixtures ("ffprobe -select_streams v:0 -show_entries packet=pts_time,flags").
var keyframeGoldens = map[string][]float64{
	"baseline_cavlc_128x128.mp4": {0},
	"frame.mjpeg.mp4":            {0},
	"high10_cavlc_96x64.mp4":     {0, 0.2},
	"mpeg4_asp_176x144.mp4":      {0, 0.4, 0.8, 1.2, 1.6, 1.966667},
	"mpeg4_asp_176x144.mkv":      {0, 0.4, 0.8, 1.2, 1.6, 1.967},
	"opengop_h264.mp4":           {0, 2, 4},
	"blue.webm":                  {0, 0.04, 0.08, 0.12, 0.16},
}

func assertTimes(t *testing.T, name string, got, want []float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d keyframes %v, want %d %v", name, len(got), got, len(want), want)
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol {
			t.Fatalf("%s: keyframe %d = %.6f, want %.6f (all: %v)", name, i, got[i], want[i], got)
		}
	}
}

func TestKeyframeTimesGolden(t *testing.T) {
	for name, want := range keyframeGoldens {
		t.Run(name, func(t *testing.T) {
			f, size := openFixture(t, name)
			defer f.Close()
			got, err := media.KeyframeTimes(f, size)
			if err != nil {
				t.Fatal(err)
			}
			assertTimes(t, name, got, want, 1e-6)
			within, err := media.KeyframeTimesWithin(f, size, 0.5)
			if err != nil {
				t.Fatal(err)
			}
			n := sort.SearchFloat64s(want, want[0]+0.5+1e-9)
			assertTimes(t, name+" within 0.5s", within, want[:n], 1e-6)
		})
	}
}

func TestKeyframeTimesUnsupported(t *testing.T) {
	for _, name := range []string{"mpeg2_176x144.ts", "mpeg2_64x48.mpg", "silence.wav", "tone.mp3"} {
		f, size := openFixture(t, name)
		_, err := media.KeyframeTimes(f, size)
		f.Close()
		if !errors.Is(err, media.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	if _, err := media.KeyframeTimes(bytes.NewReader(nil), 0); !errors.Is(err, media.ErrCorrupt) {
		t.Errorf("empty input: err = %v, want ErrCorrupt", err)
	}
	junk := bytes.Repeat([]byte{0x5a}, 4096)
	if _, err := media.KeyframeTimes(bytes.NewReader(junk), int64(len(junk))); err == nil {
		t.Error("junk input: want an error")
	}
}

// TestKeyframeTimesMatchFFprobe encodes a matrix of MP4/MOV/MKV/WebM files
// with the ffmpeg on PATH and compares KeyframeTimes with ffprobe's key
// packets. It covers B-frames with and without edit lists, mid-GOP cuts,
// leading empty edits, trimmed edits, negative ctts, a 90 kHz timescale,
// NTSC rates, Matroska with and without Cues, and a shifted start time.
// Cases whose encoder is missing are skipped.
func TestKeyframeTimesMatchFFprobe(t *testing.T) {
	if testing.Short() {
		t.Skip("encodes test files with ffmpeg")
	}
	ff, err1 := exec.LookPath("ffmpeg")
	fp, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("ffmpeg/ffprobe not on PATH")
	}
	dir := t.TempDir()
	src := []string{"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "12"}
	x264 := []string{"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3", "-g", "48", "-keyint_min", "12", "-pix_fmt", "yuv420p", "-c:a", "aac"}
	run := func(args ...string) error {
		cmd := exec.Command(ff, append([]string{"-v", "error", "-y"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New(strings.TrimSpace(string(out)))
		}
		return nil
	}
	p := func(name string) string { return filepath.Join(dir, name) }
	cat := func(parts ...[]string) []string {
		var out []string
		for _, s := range parts {
			out = append(out, s...)
		}
		return out
	}
	steps := []struct {
		out  string
		args []string
	}{
		{"bf.mp4", cat(src, x264, []string{p("bf.mp4")})},
		{"bf.mov", cat(src, x264, []string{p("bf.mov")})},
		{"noelst.mp4", cat(src, x264, []string{"-use_editlist", "0", p("noelst.mp4")})},
		{"ts90k.mp4", cat(src, x264, []string{"-video_track_timescale", "90000", p("ts90k.mp4")})},
		{"bf.mkv", cat(src, x264, []string{p("bf.mkv")})},
		{"off.mkv", cat(src, x264, []string{"-output_ts_offset", "10", p("off.mkv")})},
		{"ntsc.mp4", []string{"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24000/1001", "-t", "10", "-c:v", "libx264", "-preset", "ultrafast", "-bf", "3", "-g", "24", p("ntsc.mp4")}},
		{"vp9.webm", cat(src, []string{"-c:v", "libvpx-vp9", "-deadline", "realtime", "-g", "60", "-c:a", "libopus", p("vp9.webm")})},
		{"mp4v.mkv", cat(src, []string{"-c:v", "mpeg4", "-g", "25", "-bf", "2", "-c:a", "aac", p("mp4v.mkv")})},
		{"cut.mp4", []string{"-ss", "1.3", "-i", p("bf.mp4"), "-c", "copy", p("cut.mp4")}},
		{"cut.mov", []string{"-ss", "2.2", "-i", p("bf.mov"), "-c", "copy", p("cut.mov")}},
		{"cut.mkv", []string{"-ss", "1.3", "-i", p("bf.mp4"), "-c", "copy", p("cut.mkv")}},
		{"trim.mp4", []string{"-i", p("bf.mp4"), "-t", "7.3", "-c", "copy", p("trim.mp4")}},
		{"negcts.mp4", []string{"-i", p("bf.mp4"), "-c", "copy", "-movflags", "negative_cts_offsets", p("negcts.mp4")}},
		{"empty.mp4", []string{"-itsoffset", "2", "-i", p("bf.mp4"), "-i", p("bf.mp4"), "-map", "0:v", "-map", "1:a", "-c", "copy", p("empty.mp4")}},
	}
	var made []string
	for _, s := range steps {
		if err := run(s.args...); err != nil {
			t.Logf("skip %s: %v", s.out, err)
			continue
		}
		made = append(made, s.out)
	}
	// Matroska without Cues (non-seekable output), to exercise the Cluster walk.
	for _, s := range []struct{ in, out, format string }{{"bf.mp4", "nocues.mkv", "matroska"}, {"vp9.webm", "nocues.webm", "webm"}} {
		if _, err := os.Stat(p(s.in)); err != nil {
			continue
		}
		cmd := exec.Command(ff, "-v", "error", "-i", p(s.in), "-c", "copy", "-f", s.format, "-")
		out, err := cmd.Output()
		if err != nil || os.WriteFile(p(s.out), out, 0o644) != nil {
			t.Logf("skip %s: %v", s.out, err)
			continue
		}
		made = append(made, s.out)
	}
	if len(made) == 0 {
		t.Skip("no test file could be encoded")
	}
	for _, name := range made {
		t.Run(name, func(t *testing.T) {
			want := ffprobeKeyTimes(t, fp, p(name))
			f, err := os.Open(p(name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			st, _ := f.Stat()
			got, err := media.KeyframeTimes(f, st.Size())
			if err != nil {
				t.Fatal(err)
			}
			assertTimes(t, name, got, want, 2e-6)
		})
	}
}

// ffprobeKeyTimes lists key packet times of v:0 the way go-ffmpeg's probe
// does: pts_time, else dts_time; negative times dropped; sorted, unique.
func ffprobeKeyTimes(t *testing.T, ffprobe, path string) []float64 {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,dts_time,flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var times []float64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(strings.TrimRight(strings.TrimSpace(line), ","), ",")
		if len(fields) < 3 || !strings.Contains(fields[2], "K") {
			continue
		}
		raw := fields[0]
		if raw == "N/A" {
			raw = fields[1]
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			continue
		}
		times = append(times, v)
	}
	sort.Float64s(times)
	var uniq []float64
	for _, v := range times {
		if len(uniq) == 0 || v > uniq[len(uniq)-1] {
			uniq = append(uniq, v)
		}
	}
	return uniq
}
