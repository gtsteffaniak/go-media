package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestImagePSNRSSIMOnIdenticalFrame(t *testing.T) {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	cmd := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=red:s=64x64", "-frames:v", "1", a)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, data, 0o644); err != nil {
		t.Fatal(err)
	}
	psnr, ssim, err := imagePSNRSSIM(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if ssim < 0.99 {
		t.Fatalf("expected ssim ~1 for identical images, got %.4f", ssim)
	}
	if psnr < 40 && psnr > 0 {
		t.Fatalf("expected high psnr for identical images, got %.2f", psnr)
	}
	// Identical JPEGs may omit PSNR lines; SSIM must still be ~1.
	if psnr == 0 && ssim < 0.99 {
		t.Fatalf("psnr missing but ssim %.4f too low", ssim)
	}
}
