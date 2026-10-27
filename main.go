// Demo CLI for go-media. Library import: github.com/gtsteffaniak/go-media/pkg/media
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}
	path := os.Args[2]
	f, err := os.Open(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		fatal(err)
	}
	size := st.Size()

	switch os.Args[1] {
	case "probe":
		info, err := media.Probe(f, size)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("format=%s duration=%s streams=%d\n", info.Format, info.Duration, len(info.Streams))
		for _, s := range info.Streams {
			fmt.Printf("  [%d] %s codec=%s", s.Index, s.Type, s.Codec)
			if s.Width > 0 {
				fmt.Printf(" %dx%d", s.Width, s.Height)
			}
			fmt.Println()
		}
	case "duration":
		d, err := media.Duration(f, size)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("%s (source=%s)\n", d.Duration, d.Source)
	case "thumb":
		seek := 10.0
		preset := media.ThumbnailPresetLarge
		out := "thumb.jpg"
		for i := 3; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--small":
				preset = media.ThumbnailPresetSmall
			case "--large":
				preset = media.ThumbnailPresetLarge
			case "--seek":
				if i+1 >= len(os.Args) {
					fatal(errors.New("--seek requires a value"))
				}
				i++
				seek, err = strconv.ParseFloat(os.Args[i], 64)
				if err != nil {
					fatal(err)
				}
			default:
				if !strings.HasPrefix(os.Args[i], "-") {
					out = os.Args[i]
				}
			}
		}
		opt := media.OptionsForPreset(preset, seek)
		tr, err := media.Thumbnail(f, size, opt)
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(out, tr.Data, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s (%dx%d, %s)\n", out, tr.Width, tr.Height, tr.MIME)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <probe|duration|thumb> <file> [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  thumb [--small|--large] [--seek N] [out.jpg]\n")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
