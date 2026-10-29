package media

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func openFixture(t *testing.T, name string) (*os.File, int64) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Skipf("fixture %s: %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return f, st.Size()
}

// TestInspectAudioMatchesSeparateCalls checks that one InspectAudio call gives the
// same tags, lyrics flag and artwork as the three older calls.
func TestInspectAudioMatchesSeparateCalls(t *testing.T) {
	for _, name := range []string{"tone.mp3", "tone.flac", "red.mp4"} {
		t.Run(name, func(t *testing.T) {
			f, size := openFixture(t, name)

			wantTags, tagErr := ReadTags(f, size)
			wantLyrics := false
			if _, _, err := LyricsFromReader(f, size); err == nil {
				wantLyrics = true
			}
			wantArt, _, _, artErr := EmbeddedArtwork(f, size)

			got, err := InspectAudio(f, size, AudioOptions{WithArtwork: true})
			if (err != nil) != (tagErr != nil) {
				t.Fatalf("error mismatch: inspect=%v separate=%v", err, tagErr)
			}
			if err != nil {
				return
			}
			if got.Tags.Title != wantTags.Title || got.Tags.Artist != wantTags.Artist ||
				got.Tags.Album != wantTags.Album || got.Tags.Year != wantTags.Year ||
				got.Tags.Genre != wantTags.Genre || got.Tags.Track != wantTags.Track {
				t.Errorf("tags differ: inspect=%+v separate=%+v", got.Tags, wantTags)
			}
			if got.HasEmbeddedLyrics != wantLyrics {
				t.Errorf("lyrics flag: inspect=%v separate=%v", got.HasEmbeddedLyrics, wantLyrics)
			}
			hasArt := artErr == nil && len(wantArt) > 0
			if got.HasArtwork != hasArt {
				t.Errorf("artwork flag: inspect=%v separate=%v", got.HasArtwork, hasArt)
			}
			if hasArt && !bytes.Equal(got.Artwork, wantArt) {
				t.Errorf("artwork bytes differ (%d vs %d)", len(got.Artwork), len(wantArt))
			}
		})
	}
}

// TestInspectAudioWithoutArtworkReturnsNoBytes checks WithArtwork=false.
func TestInspectAudioWithoutArtworkReturnsNoBytes(t *testing.T) {
	f, size := openFixture(t, "tone.mp3")
	got, err := InspectAudio(f, size, AudioOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Artwork) != 0 || got.ArtworkMIME != "" {
		t.Errorf("expected no artwork bytes, got %d bytes mime=%q", len(got.Artwork), got.ArtworkMIME)
	}
}

// TestInspectAudioReadsLessThanSeparateCalls shows that one InspectAudio call
// reads fewer bytes than the old ReadTags + HasLyrics + EmbeddedArtwork sequence.
func TestInspectAudioReadsLessThanSeparateCalls(t *testing.T) {
	f, size := openFixture(t, "tone.mp3")

	sep := &countingReaderAt{r: f}
	_, _ = ReadTags(sep, size)
	_, _, _ = LyricsFromReader(sep, size)
	_, _, _, _ = EmbeddedArtwork(sep, size)

	one := &countingReaderAt{r: f}
	if _, err := InspectAudio(one, size, AudioOptions{WithArtwork: true}); err != nil {
		t.Fatal(err)
	}
	if one.bytes >= sep.bytes {
		t.Errorf("single parse read %d bytes, separate calls read %d; expected fewer", one.bytes, sep.bytes)
	}
	t.Logf("bytes read: single=%d separate=%d", one.bytes, sep.bytes)
}

// TestInspectAudioUnsupported returns an error for a non-audio input.
func TestInspectAudioUnsupported(t *testing.T) {
	r := bytes.NewReader([]byte("this is not media at all, just text"))
	if _, err := InspectAudio(r, int64(r.Len()), AudioOptions{}); err == nil {
		t.Fatal("expected error for unsupported input")
	}
}

func BenchmarkAudioSeparateCalls(b *testing.B) {
	f, err := os.Open(filepath.Join("..", "..", "testdata", "tone.mp3"))
	if err != nil {
		b.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ReadTags(f, st.Size())
		_, _, _ = LyricsFromReader(f, st.Size())
		_, _, _, _ = EmbeddedArtwork(f, st.Size())
	}
}

func BenchmarkInspectAudio(b *testing.B) {
	f, err := os.Open(filepath.Join("..", "..", "testdata", "tone.mp3"))
	if err != nil {
		b.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = InspectAudio(f, st.Size(), AudioOptions{WithArtwork: true})
	}
}
