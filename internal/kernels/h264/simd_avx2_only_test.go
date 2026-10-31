//go:build goexperiment.simd && amd64 && !purego

package h264

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestSIMDKernelsAVX2Only disassembles the compiled test binary and fails if any
// kernel that is reachable on AVX2-only CPUs contains an AVX-512 (EVEX / zmm /
// k-mask) instruction. KernelsUseSIMD gates on AVX2 only, so such an instruction
// would raise SIGILL on those machines while passing on AVX-512 hardware.
func TestSIMDKernelsAVX2Only(t *testing.T) {
	bin := t.TempDir() + "/h264k.test"
	cmd := exec.Command("go", "test", "-c", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOEXPERIMENT=simd", "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}
	// AVX2-tier kernel symbols: everything except the explicit AVX-512 ones.
	out, err := exec.Command("go", "tool", "objdump", "-s",
		`h264\.(idctAdd4x4PairSIMD|transpose4x4x2|butterfly4x8|addRow8|join2|inverseTransform4x4ResidualSIMD)$`, bin).CombinedOutput()
	if err != nil {
		t.Skipf("objdump: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "TEXT") {
		t.Skip("kernel symbols inlined away; nothing to check")
	}
	if re := regexp.MustCompile(`(?i)\bzmm\d+|\bk[0-7]\b|\{k[0-7]\}|VPMOVUSDB|VPMOVDB`); re.MatchString(text) {
		t.Fatalf("AVX-512 instruction in AVX2-tier kernel:\n%s", re.FindString(text))
	}
}
