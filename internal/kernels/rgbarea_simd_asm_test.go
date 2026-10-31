//go:build goexperiment.simd && amd64 && !purego

package kernels

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestRGBAccumRow420SIMDAsm checks the compiled AVX2 row kernel uses no AVX-512
// instruction (it is reachable on AVX2-only CPUs) and clears the upper YMM bits
// before returning.
func TestRGBAccumRow420SIMDAsm(t *testing.T) {
	bin := t.TempDir() + "/k.test"
	cmd := exec.Command("go", "test", "-c", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOEXPERIMENT=simd", "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}
	out, err := exec.Command("go", "tool", "objdump", "-s", `kernels\.rgbAccumRow420SIMD$`, bin).CombinedOutput()
	if err != nil {
		t.Skipf("objdump: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "TEXT") {
		t.Fatal("rgbAccumRow420SIMD not found")
	}
	if re := regexp.MustCompile(`(?i)\bzmm\d+|\bZ\d+\b|\{k[0-7]\}|VPMOVUSDB|VPMOVDB`); re.MatchString(text) {
		t.Fatalf("AVX-512 instruction in AVX2 kernel: %s", re.FindString(text))
	}
	if !strings.Contains(text, "VZEROUPPER") {
		t.Fatal("rgbAccumRow420SIMD never executes VZEROUPPER")
	}
}
