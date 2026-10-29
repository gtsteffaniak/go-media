package media

import "testing"

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
