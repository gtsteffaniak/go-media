package main

import (
	"os/exec"
	"strings"
)

// ffmpegHasDemuxer reports whether ffmpeg can read files in the given format name.
func ffmpegHasDemuxer(ffmpeg, format string) bool {
	set, err := ffmpegFormatSet(ffmpeg)
	if err != nil {
		return false
	}
	if set[format] {
		return true
	}
	demux, err := ffmpegDemuxerSet(ffmpeg)
	if err != nil {
		return false
	}
	return demux[format]
}

// ffmpegHEIFContainer returns a mux short name when ffmpeg lists HEIC/HEIF support.
func ffmpegHEIFContainer(ffmpeg string) (muxFmt string, ok bool) {
	formats, _ := ffmpegFormatSet(ffmpeg)
	muxers, _ := ffmpegMuxerSet(ffmpeg)
	for _, name := range []string{"heic", "heif"} {
		if formats[name] || muxers[name] {
			return name, true
		}
	}
	return "", false
}

func ffmpegFormatSet(ffmpeg string) (map[string]bool, error) {
	return ffmpegFlagListingSet(ffmpeg, "-formats", "DE")
}

func ffmpegDemuxerSet(ffmpeg string) (map[string]bool, error) {
	return ffmpegFlagListingSet(ffmpeg, "-demuxers", "D")
}

func ffmpegMuxerSet(ffmpeg string) (map[string]bool, error) {
	return ffmpegFlagListingSet(ffmpeg, "-muxers", "E")
}

// ffmpegFlagListingSet parses ffmpeg -formats / -demuxers / -muxers output.
// Those listings are written to stderr on most builds; use CombinedOutput.
func ffmpegFlagListingSet(ffmpeg, flag, enabledIn string) (map[string]bool, error) {
	out, err := exec.Command(ffmpeg, "-hide_banner", flag).CombinedOutput()
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		flags := fields[0]
		if strings.ContainsAny(flags, enabledIn) {
			set[fields[1]] = true
		}
	}
	return set, nil
}
