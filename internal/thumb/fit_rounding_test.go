package thumb

import "testing"

// consoletest.mp4 is 1698x828. ffmpeg fits it into 256 wide as 256x125, not 256x124.
func TestFitThumbSizeRoundsToNearest(t *testing.T) {
	w, h := fitThumbSize(1698, 828, 256, 1080)
	if w != 256 || h != 125 {
		t.Fatalf("fitThumbSize(1698,828,256,1080) = %dx%d, want 256x125", w, h)
	}
}
