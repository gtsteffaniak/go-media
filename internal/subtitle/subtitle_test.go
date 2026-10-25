package subtitle

import (
	"strings"
	"testing"
)

// TestParseSRT is a test.
func TestParseSRT(t *testing.T) {
	in := `1
00:00:01,000 --> 00:00:02,000
Hello

2
00:00:03,000 --> 00:00:04,500
World
`
	out, err := ParseSRT(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "WEBVTT") || !strings.Contains(out, "Hello") {
		t.Fatalf("bad vtt: %s", out)
	}
	if strings.Contains(out, "00:00:01,000") {
		t.Fatalf("expected WebVTT dot separator, got: %s", out)
	}
	if !strings.Contains(out, "00:00:01.000") {
		t.Fatalf("expected WebVTT timestamp: %s", out)
	}
}

// FuzzParseSRT supports media inspection.
func FuzzParseSRT(f *testing.F) {
	f.Add("1\n00:00:00,000 --> 00:00:01,000\nx\n")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseSRT(strings.NewReader(s))
	})
}
