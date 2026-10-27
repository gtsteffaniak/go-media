package media_test

import (
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// TestExternalSubtitleTracksStemMatch is a test.
func TestExternalSubtitleTracksStemMatch(t *testing.T) {
	subs := media.ExternalSubtitleTracks("/media/Movie.mkv", []string{"Movie.srt", "Movie 2.srt", "Other.en.srt"})
	if len(subs) != 1 || subs[0].Name != "Movie.srt" {
		t.Fatalf("got %v", subs)
	}
}

// TestProbeWebMStreams is a test.
func TestProbeWebMStreams(t *testing.T) {
	f, size := openFixture(t, "blue.webm")
	defer f.Close()
	info, err := media.Probe(f, size)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Streams) == 0 {
		t.Fatal("expected video stream")
	}
}
