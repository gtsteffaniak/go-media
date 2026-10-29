package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestDiffTagsMatchAndMismatch(t *testing.T) {
	ff := FFProbeFormat{Tags: map[string]string{
		"title": "Song", "artist": "Band", "album": "LP", "genre": "Rock",
		"date": "2020-05-01", "track": "3/12",
	}}
	good := media.Tags{Title: "song", Artist: "Band", Album: "LP", Genre: "Rock", Year: 2020, Track: 3}
	if d := diffTags(good, ff); len(d) != 0 {
		t.Fatalf("expected no diffs, got %v", d)
	}

	bad := good
	bad.Album = "Other"
	bad.Year = 1999
	d := diffTags(bad, ff)
	if len(d) != 2 {
		t.Fatalf("expected album and year diffs, got %v", d)
	}
}

func TestDiffTagsAbsentOnOneSide(t *testing.T) {
	ff := FFProbeFormat{Tags: map[string]string{"title": "Only ffmpeg"}}
	d := diffTags(media.Tags{}, ff)
	if len(d) != 1 {
		t.Fatalf("expected one diff, got %v", d)
	}
	d = diffTags(media.Tags{Title: "Only go"}, FFProbeFormat{})
	if len(d) != 1 {
		t.Fatalf("expected one diff, got %v", d)
	}
}

func solidPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDiffArtwork(t *testing.T) {
	art := solidPNG(t, 10, 20)
	withPic := []FFProbeStream{{CodecType: "video", AttachedPic: 1, Width: 10, Height: 20}}
	if d := diffArtwork(true, art, withPic); len(d) != 0 {
		t.Fatalf("expected match, got %v", d)
	}
	if d := diffArtwork(false, nil, withPic); len(d) != 1 {
		t.Fatalf("expected presence diff, got %v", d)
	}
	if d := diffArtwork(true, art, []FFProbeStream{{CodecType: "video"}}); len(d) != 1 {
		t.Fatalf("expected presence diff, got %v", d)
	}
	wrongSize := []FFProbeStream{{CodecType: "video", AttachedPic: 1, Width: 30, Height: 20}}
	if d := diffArtwork(true, art, wrongSize); len(d) != 1 {
		t.Fatalf("expected size diff, got %v", d)
	}
	if d := diffArtwork(true, []byte("junk"), withPic); len(d) != 1 {
		t.Fatalf("expected undecodable diff, got %v", d)
	}
}

func TestDiffLyrics(t *testing.T) {
	ff := FFProbeFormat{Tags: map[string]string{"lyrics": "Line one\nLine two"}}
	synced := "[00:01.00]Line one\r\n[00:02.00]Line two\r\n"
	if d := diffLyrics(synced, ff); len(d) != 0 {
		t.Fatalf("timestamps and line endings should not differ, got %v", d)
	}
	if d := diffLyrics("Something else", ff); len(d) != 1 {
		t.Fatalf("expected text diff, got %v", d)
	}
	if d := diffLyrics("", ff); len(d) != 1 {
		t.Fatalf("expected presence diff, got %v", d)
	}
	if d := diffLyrics("", FFProbeFormat{}); len(d) != 0 {
		t.Fatalf("no lyrics on either side must match, got %v", d)
	}
}

func TestDiffSidecarLyricsMatchesFile(t *testing.T) {
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "song.mp3")
	lrc := filepath.Join(dir, "song.lrc")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lrc, []byte("[00:00.00]Hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := diffSidecarLyrics(mediaPath, lrc); len(d) != 0 {
		t.Fatalf("expected match, got %v", d)
	}
}

func TestNormalizeSubtitleCodec(t *testing.T) {
	cases := map[string]string{"tx3g": "mov_text", "wvtt": "webvtt", "subrip": "subrip", "SRT": "srt"}
	for in, want := range cases {
		if got := normalizeSubtitleCodec(in); got != want {
			t.Errorf("normalizeSubtitleCodec(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWebVTTCueCount(t *testing.T) {
	vtt := "WEBVTT\n\n00:00.000 --> 00:01.000\nhi\n\n00:02.000 --> 00:03.000\nthere\n"
	if got := webVTTCueCount(vtt); got != 2 {
		t.Fatalf("cues = %d, want 2", got)
	}
}
