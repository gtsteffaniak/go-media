//go:build ignore

// Regenerate display PNG for ffmpeg when libjxl is unavailable at compare time.
// Prefer ffmpeg (matches HEIC phone sidecar pattern):
//
//	ffmpeg -y -i test/data/zoltan_unsplash.jxl -frames:v 1 test/data/zoltan_unsplash_display.png
//
// Or: go run scripts/gen-jxl-display-sidecar.go
package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/gtsteffaniak/go-media/internal/jxldecode"
	"image/png"
)

func main() {
	const jxl = "test/data/zoltan_unsplash.jxl"
	const pngOut = "test/data/zoltan_unsplash_display.png"

	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
			"-i", jxl, "-frames:v", "1", pngOut)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			fmt.Println("wrote", pngOut, "(ffmpeg libjxl decode — use this for pinned test/data)")
			return
		}
	}

	f, err := os.Open(jxl)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	img, err := jxldecode.Decode(f, st.Size(), 0, 0)
	if err != nil {
		panic(err)
	}
	out, err := os.Create(pngOut)
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		panic(err)
	}
	fmt.Println("wrote", pngOut, "(go-media decode — temporary; re-pin with ffmpeg when libjxl is available)")
}
