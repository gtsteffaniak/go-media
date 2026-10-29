package isobmff

import (
	"bytes"
	"testing"
)

// pickTable builds a sampleTable over in-memory stss and stsz entries with one
// stts run (every sample lasts one tick). syncs holds 1-based stss numbers and
// sizes the byte size of every sample.
func pickTable(syncs []uint32, sizes []uint32) *sampleTable {
	buf := append(u32s(syncs...), u32s(sizes...)...)
	return &sampleTable{
		r:           bytes.NewReader(buf),
		size:        int64(len(buf)),
		sampleCount: uint32(len(sizes)),
		hasStsz:     true,
		stszOff:     int64(4 * len(syncs)),
		hasStss:     true,
		stssOff:     0,
		stssCount:   uint32(len(syncs)),
		stts:        []uint32{uint32(len(sizes)), 1},
	}
}

// bigBunnyLikeSizes mimics the clip that returned a black thumbnail: a tiny
// first keyframe at sample 0, real keyframes after it, and small delta frames.
func bigBunnyLikeSizes() ([]uint32, []uint32) {
	sizes := make([]uint32, 1000)
	for i := range sizes {
		sizes[i] = 100
	}
	// 0-based keyframes at 0, 250, 300, 400 and 600.
	for sample, size := range map[int]uint32{0: 906, 250: 194025, 300: 202408, 400: 66066, 600: 216517} {
		sizes[sample] = size
	}
	return []uint32{1, 251, 301, 401, 601}, sizes
}

func TestSyncPosNearest(t *testing.T) {
	syncs, sizes := bigBunnyLikeSizes()
	st := pickTable(syncs, sizes)
	cases := []struct {
		target  int
		wantPos int // stss position
	}{
		{0, 0},
		{100, 0}, // 100 from sample 0, 150 from 250
		{125, 0}, // exact tie goes to the earlier keyframe
		{126, 1}, // just past the tie
		{275, 1}, // tie between 250 and 300 goes earlier
		{276, 2}, // closer to 300
		{999, 4}, // after the last keyframe
		{400, 3}, // exact keyframe
		{499, 3}, // 99 from 400, 101 from 600
		{500, 3}, // tie between 400 and 600
		{501, 4}, // closer to 600
	}
	for _, c := range cases {
		got, err := st.syncPosNearest(c.target)
		if err != nil || got != c.wantPos {
			t.Errorf("target %d: pos %d err %v, want %d", c.target, got, err, c.wantPos)
		}
	}
}

func TestPickThumbnailKeyframeSkipsEmptyFirstKeyframe(t *testing.T) {
	syncs, sizes := bigBunnyLikeSizes()
	st := pickTable(syncs, sizes)
	cases := []struct {
		target int
		want   int // 0-based sample
	}{
		{0, 250},   // 0% request: black first keyframe skipped
		{100, 250}, // default 10% of a 1000-sample clip: nearest is the black keyframe
		{125, 250}, // still nearest-to-black, still skipped
		{200, 250}, // nearest is already a real keyframe
		{290, 300}, // real nearest keyframe is kept
		{520, 600}, // real nearest keyframe is kept
		{999, 600}, // last keyframe, nothing after it to compare
	}
	for _, c := range cases {
		got, err := st.pickThumbnailKeyframe(c.target)
		if err != nil || got != c.want {
			t.Errorf("target %d: sample %d err %v, want %d", c.target, got, err, c.want)
		}
	}
}

func TestPickThumbnailKeyframeKeepsSimilarSizes(t *testing.T) {
	// Keyframes of similar size (a flat, dark, or slow scene) are never skipped.
	sizes := make([]uint32, 100)
	for i := range sizes {
		sizes[i] = 50
	}
	for _, s := range []int{0, 20, 40, 60} {
		sizes[s] = 2000
	}
	st := pickTable([]uint32{1, 21, 41, 61}, sizes)
	for target, want := range map[int]int{0: 0, 5: 0, 12: 20, 45: 40, 99: 60} {
		got, err := st.pickThumbnailKeyframe(target)
		if err != nil || got != want {
			t.Errorf("target %d: sample %d err %v, want %d", target, got, err, want)
		}
	}
}

func TestPickThumbnailKeyframeAllEmptyReturnsNearest(t *testing.T) {
	// The window maximum is always itself non-empty, so a video that is dark
	// throughout still gets a frame: the first keyframe in the window within a
	// tenth of the largest.
	sizes := make([]uint32, 100)
	for _, s := range []int{0, 20, 40} {
		sizes[s] = 800
	}
	st := pickTable([]uint32{1, 21, 41}, sizes)
	got, err := st.pickThumbnailKeyframe(0)
	if err != nil || got != 0 {
		t.Errorf("sample %d err %v, want 0", got, err)
	}
}

func TestPickThumbnailKeyframeWithoutStss(t *testing.T) {
	st := &sampleTable{sampleCount: 10}
	got, err := st.pickThumbnailKeyframe(7)
	if err != nil || got != 7 {
		t.Errorf("sample %d err %v, want 7", got, err)
	}
}

func TestPickThumbnailKeyframeFixedSizeNeverSkips(t *testing.T) {
	st := pickTable([]uint32{1, 21}, make([]uint32, 40))
	st.defaultSize = 12
	got, err := st.pickThumbnailKeyframe(0)
	if err != nil || got != 0 {
		t.Errorf("sample %d err %v, want 0", got, err)
	}
}
