package compare_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestComparisonGate runs the manifest gate when pinned ffmpeg is available.
func TestComparisonGate(t *testing.T) {
	if os.Getenv("COMPARE_GATE_TEST") != "1" {
		t.Skip("set COMPARE_GATE_TEST=1 to run the full comparison gate in go test")
	}
	root := filepath.Join("..", "..")
	ffmpegDir := os.Getenv("FFMPEG_DIR")
	if ffmpegDir == "" {
		ffmpegDir = filepath.Join(os.Getenv("HOME"), "ffmpeg-9.0.2")
	}
	ffmpeg := filepath.Join(ffmpegDir, "ffmpeg")
	if _, err := os.Stat(ffmpeg); err != nil {
		t.Skip("pinned ffmpeg not installed; run make ffmpeg")
	}
	cmd := exec.Command("bash", "scripts/run-comparison.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+ffmpegDir+":"+os.Getenv("PATH"), "COMPARE_GATE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("comparison gate: %v\n%s", err, out)
	}
}
