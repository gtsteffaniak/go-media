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
