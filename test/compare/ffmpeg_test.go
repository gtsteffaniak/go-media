package compare_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gtsteffaniak/go-media"
)

// ffprobeDuration reads duration seconds from ffprobe JSON.
func ffprobeDuration(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", path).Output()
	if err != nil {
		return 0, err
	}
	var payload struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(payload.Format.Duration, 64)
}

// TestDurationMatchesFFprobe is a test.
func TestDurationMatchesFFprobe(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	root := filepath.Join("..", "..", "testdata")
	cases := []string{"silence.wav", "tone.flac", "tone.mp3", "red.mp4", "blue.webm"}
	for _, name := range cases {
		path := filepath.Join(root, name)
		f, err := os.Open(path)
		if err != nil {
			t.Skip("fixtures missing, run scripts/generate-fixtures.sh")
		}
		st, _ := f.Stat()
		dr, err := media.Duration(f, st.Size())
		f.Close()
		if err != nil {
			t.Errorf("%s: go-media: %v", name, err)
			continue
		}
		want, err := ffprobeDuration(path)
		if err != nil {
			t.Errorf("%s: ffprobe: %v", name, err)
			continue
		}
		got := dr.Duration.Seconds()
		if math.Abs(got-want) > 0.25 {
			t.Errorf("%s: go-media=%.3fs ffprobe=%.3fs", name, got, want)
		}
	}
}

// TestH264KeyframeUnsupported is a test.
func TestH264KeyframeUnsupported(t *testing.T) {
	_, _, _, err := media.DecodeH264Keyframe(nil, 64, 64)
	if !errors.Is(err, media.ErrUnsupportedProfile) {
		t.Fatalf("expected ErrUnsupportedProfile, got %v", err)
	}
}
