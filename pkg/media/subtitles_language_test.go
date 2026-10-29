package media

import "testing"

// TestExternalSubtitleTracksInfersLanguage checks sidecar matching and language suffixes.
func TestExternalSubtitleTracksInfersLanguage(t *testing.T) {
	tracks := ExternalSubtitleTracks("/movies/video.mkv", []string{"video.en.srt", "video.srt", "video.fra.ass", "other.srt"})
	want := map[string]string{"video.en.srt": "en", "video.srt": "", "video.fra.ass": "fra"}
	if len(tracks) != len(want) {
		t.Fatalf("got %d tracks, want %d: %+v", len(tracks), len(want), tracks)
	}
	for _, tr := range tracks {
		if tr.Language != want[tr.Name] {
			t.Errorf("%s language = %q, want %q", tr.Name, tr.Language, want[tr.Name])
		}
	}
}

// TestExternalSubtitleTracksLanguageNeedsAddedLetterSuffix checks that a
// language comes only from a letter suffix added after the video's own stem.
func TestExternalSubtitleTracksLanguageNeedsAddedLetterSuffix(t *testing.T) {
	cases := []struct {
		video, sub, want string
	}{
		{"/m/video.en.mkv", "video.en.srt", ""},      // stem already ends in .en
		{"/m/video.en.mkv", "video.en.de.srt", "de"}, // added after the stem
		{"/m/video.mkv", "video.01.srt", ""},         // digits are not a language
		{"/m/video.mkv", "video.e1.srt", ""},
		{"/m/video.mkv", "video.EN.srt", "EN"},
	}
	for _, c := range cases {
		tracks := ExternalSubtitleTracks(c.video, []string{c.sub})
		if len(tracks) != 1 {
			t.Fatalf("%s/%s: got %d tracks", c.video, c.sub, len(tracks))
		}
		if tracks[0].Language != c.want {
			t.Errorf("%s/%s: language %q, want %q", c.video, c.sub, tracks[0].Language, c.want)
		}
	}
}
