//go:build goexperiment.simd && (amd64 || arm64)

package h264

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSIMDBinaryContainsVectorOps builds this package's tests and checks objdump for archsimd lowering.
func TestSIMDBinaryContainsVectorOps(t *testing.T) {
	if os.Getenv("GOEXPERIMENT") == "" && !simdExperimentEnabled() {
		t.Skip("requires GOEXPERIMENT=simd build")
	}
	SetSIMDEnabledForTest(true)
	SetKernelSIMDForTest("idct4x4", true)
	if !KernelSIMD("idct4x4") {
		t.Skip("CPU lacks AVX2/ASIMD")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "h264k.test")
	cmd := exec.Command("go", "test", "-c", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOEXPERIMENT=simd", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}
	out, err := exec.Command("objdump", "-d", bin).CombinedOutput()
	if err != nil {
		t.Skipf("objdump unavailable: %v", err)
	}
	text := string(out)
	var needle string
	switch runtime.GOARCH {
	case "amd64":
		needle = "vmov"
	case "arm64":
		needle = "ldr"
	default:
		t.Skip(runtime.GOARCH)
	}
	if !strings.Contains(strings.ToLower(text), needle) {
		t.Fatalf("objdump missing %q (SIMD not linked?)", needle)
	}
}
