package media_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// TestH264High10KeyframeMatchesFFmpeg decodes 10-bit High10 keyframes and
// checks the 8-bit 4:2:0 planes against ffmpeg's 10-bit output rounded with
// (v+2)>>2. The hashes come from
//
//	ffmpeg -skip_frame nokey -ss T -i F -frames:v 1 -pix_fmt yuv420p10le
//
// so a match means the 10-bit reconstruction is bit-exact with ffmpeg.
func TestH264High10KeyframeMatchesFFmpeg(t *testing.T) {
	cases := []struct {
		file string
		seek float64
		want string
	}{
		{"high10_cavlc_96x64.mp4", 0, "bdad709decd142cc74a4302c7c9684aa1d77f6faed79286d29cacd0569403be8"},
		{"high10_cavlc_96x64.mp4", 50, "fdba090d9156f90cff02ac7826186bd99f45b540c9e1be9f4807b17b454ce099"},
		{"high10_cabac8x8_96x64.mp4", 0, "e84a9bb82c99510f65fd84dbaf3c69caaa71e8cbbbcd0ff8a4101e2d8ecac1a6"},
		{"high10_cabac8x8_96x64.mp4", 50, "4ee44bca52346c1bb2ee3b9132ec406faf23f447f9d754d51d40b02f9ca5cfbb"},
	}
	for _, tc := range cases {
		f, err := os.Open("../../testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		st, _ := f.Stat()
		yuv, w, h, err := media.H264KeyframeYUV420(f, st.Size(), tc.seek, false)
		f.Close()
		if err != nil {
			t.Fatalf("%s@%v: %v", tc.file, tc.seek, err)
		}
		if w != 96 || h != 64 {
			t.Fatalf("%s: %dx%d", tc.file, w, h)
		}
		sum := sha256.Sum256(yuv)
		if got := hex.EncodeToString(sum[:]); got != tc.want {
			t.Errorf("%s@%v%%: yuv sha256 %s, want %s", tc.file, tc.seek, got, tc.want)
		}
		f, _ = os.Open("../../testdata/" + tc.file)
		if _, err := media.ThumbnailSmall(f, st.Size(), tc.seek); err != nil {
			t.Errorf("%s thumbnail: %v", tc.file, err)
		}
		f.Close()
	}
}
