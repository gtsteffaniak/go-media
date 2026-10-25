package isobmff

import (
	"strings"
	"testing"
)

// TestExtractWVTTPlainSample verifies wvtt samples with inline WebVTT cue text.
func TestExtractWVTTPlainSample(t *testing.T) {
	cue := "00:00:00.000 --> 00:00:02.000\nHello\n"
	got, err := parseWVTSample([]byte(cue))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Hello") {
		t.Fatalf("unexpected cue: %q", got)
	}
}
