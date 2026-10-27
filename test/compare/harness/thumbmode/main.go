// Command thumbmode benchmarks media.Thumbnail for one asset/preset (stdout: median write ms).
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/gtsteffaniak/go-media"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

func main() {
	asset := flag.String("asset", "", "path to media file")
	preset := flag.String("preset", "large", "small|large")
	seek := flag.Float64("seek", 10, "seek percent")
	warmup := flag.Int("warmup", 11, "warmup iterations")
	runs := flag.Int("runs", 15, "timed iterations")
	measureGated := flag.Bool("measure-gated", false, "decode profiles rejected only for missing the timing gate (measurement only)")
	flag.Parse()
	h264.AllowGatedProfiles(*measureGated)
	if *asset == "" {
		fmt.Fprintln(os.Stderr, "thumbmode: -asset required")
		os.Exit(2)
	}
	f, err := os.Open(*asset)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	st, _ := f.Stat()
	opt := media.OptionsForPreset(media.ThumbnailPreset(*preset), *seek)
	var samples []float64
	tmp := os.TempDir() + "/thumbmode-out.jpg"
	for i := 0; i < *warmup+*runs; i++ {
		f.Seek(0, 0)
		start := time.Now()
		tr, err := media.Thumbnail(f, st.Size(), opt)
		elapsed := time.Since(start).Seconds() * 1000
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		w0 := time.Now()
		_ = os.WriteFile(tmp, tr.Data, 0o644)
		writeMs := elapsed + time.Since(w0).Seconds()*1000
		if i >= *warmup {
			samples = append(samples, writeMs)
		}
	}
	sort.Float64s(samples)
	med := samples[len(samples)/2]
	fmt.Printf("%.3f\n", med)
}
