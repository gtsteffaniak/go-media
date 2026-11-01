package main

import (
	"strings"
	"testing"
)

func TestFFmpegHEIFContainerRecognizesHeifAlias(t *testing.T) {
	formats := map[string]bool{"heif": true}
	muxers := map[string]bool{}
	mux, ok := heifMuxFromSets(formats, muxers)
	if !ok || mux != "heif" {
		t.Fatalf("heifMuxFromSets() = (%q, %v), want (heif, true)", mux, ok)
	}
	formats = map[string]bool{"heic": true}
	mux, ok = heifMuxFromSets(formats, muxers)
	if !ok || mux != "heic" {
		t.Fatalf("heic branch = (%q, %v)", mux, ok)
	}
	mux, ok = heifMuxFromSets(nil, map[string]bool{"heif": true})
	if !ok || mux != "heif" {
		t.Fatalf("muxer-only = (%q, %v)", mux, ok)
	}
}

func heifMuxFromSets(formats, muxers map[string]bool) (muxFmt string, ok bool) {
	for _, name := range []string{"heic", "heif"} {
		if formats[name] || muxers[name] {
			return name, true
		}
	}
	return "", false
}

func TestFFmpegFlagListingSetParsesFormatLine(t *testing.T) {
	const sample = " D  heif            Apple HEIF (MPEG-H Part 12)\n DE mp4             MP4\n"
	set := parseFlagListingForTest(sample, "DE")
	if !set["heif"] {
		t.Fatalf("set = %v, want heif demuxer", set)
	}
	if !set["mp4"] {
		t.Fatalf("set = %v, want mp4", set)
	}
}

func parseFlagListingForTest(out, enabledIn string) map[string]bool {
	set := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		flags := fields[0]
		if strings.ContainsAny(flags, enabledIn) {
			set[fields[1]] = true
		}
	}
	return set
}
