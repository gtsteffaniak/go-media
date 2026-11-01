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
	return set[format]
}

// ffmpegHEIFContainer returns demuxer/muxer short names for Apple HEIC/HEIF stills.
// ffmpeg 9.x often registers only "heif" even for .heic files.
func ffmpegHEIFContainer(ffmpeg string) (demux, mux string, ok bool) {
	set, err := ffmpegFormatSet(ffmpeg)
	if err != nil {
		return "", "", false
	}
	if set["heic"] {
		return "heic", "heic", true
	}
	if set["heif"] {
		return "heif", "heif", true
	}
	return "", "", false
}

func ffmpegFormatSet(ffmpeg string) (map[string]bool, error) {
	out, err := exec.Command(ffmpeg, "-hide_banner", "-formats").Output()
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
		if strings.ContainsAny(flags, "DE") {
			set[fields[1]] = true
		}
	}
	return set, nil
}
