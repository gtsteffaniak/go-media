package main

import (
	"strings"
	"testing"
)

func TestFFmpegHEIFContainerRecognizesHeifAlias(t *testing.T) {
	set := map[string]bool{"heif": true}
	demux, mux, ok := heifContainerFromSet(set)
	if !ok || demux != "heif" || mux != "heif" {
		t.Fatalf("heifContainerFromSet() = (%q, %q, %v), want (heif, heif, true)", demux, mux, ok)
	}
	set = map[string]bool{"heic": true}
	demux, mux, ok = heifContainerFromSet(set)
	if !ok || demux != "heic" || mux != "heic" {
		t.Fatalf("heic branch = (%q, %q, %v)", demux, mux, ok)
	}
}

func heifContainerFromSet(set map[string]bool) (demux, mux string, ok bool) {
	if set["heic"] {
		return "heic", "heic", true
	}
	if set["heif"] {
		return "heif", "heif", true
	}
	return "", "", false
}

func TestFFmpegFormatSetParsesDemuxerLine(t *testing.T) {
	const sample = " D  heif            Apple HEIF (MPEG-H Part 12)\n DE mp4             MP4\n"
	set := parseFFmpegFormatSetForTest(sample)
	if !set["heif"] {
		t.Fatalf("set = %v, want heif demuxer", set)
	}
	if !set["mp4"] {
		t.Fatalf("set = %v, want mp4", set)
	}
}

func parseFFmpegFormatSetForTest(out string) map[string]bool {
	set := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		flags := fields[0]
		if strings.ContainsAny(flags, "DE") {
			set[fields[1]] = true
		}
	}
	return set
}
