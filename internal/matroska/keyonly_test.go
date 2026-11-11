package matroska

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// testBlock is one SimpleBlock for buildVP8TestMKV.
type testBlock struct {
	ms    int16
	key   bool
	laces [][]byte // more than one: fixed-size lacing (equal lengths)
	raw   []byte   // when set, appended to the cluster as is
}

// fixedLacedBlock encodes a SimpleBlock with fixed-size lacing.
func fixedLacedBlock(track uint64, rel int16, key bool, laces [][]byte) []byte {
	b := []byte{0x80 | byte(track)}
	var tc [2]byte
	binary.BigEndian.PutUint16(tc[:], uint16(rel))
	b = append(b, tc[:]...)
	flags := byte(0x04) // fixed-size lacing
	if key {
		flags |= 0x80
	}
	b = append(b, flags, byte(len(laces)-1))
	for _, l := range laces {
		b = append(b, l...)
	}
	return ebmlElem(idSimpleBlockTest, b)
}

// buildVP8TestMKV builds a one-cluster, cue-less V_VP8 file from blocks.
func buildVP8TestMKV(blocks []testBlock, durMs float64) []byte {
	parts := [][]byte{u64Elem(idTimecodeTest, 0)}
	for _, b := range blocks {
		switch {
		case b.raw != nil:
			parts = append(parts, b.raw)
		case len(b.laces) > 1:
			parts = append(parts, fixedLacedBlock(1, b.ms, b.key, b.laces))
		default:
			parts = append(parts, simpleBlock(1, b.ms, b.key, b.laces[0]))
		}
	}
	body := append(infoTest(1_000_000, durMs), videoTrackTest("V_VP8")...)
	body = append(body, ebmlElem(idClusterTest, parts...)...)
	return append(ebmlHeaderTest(), ebmlElem(idSegmentTest, body)...)
}

func seekFrames(t *testing.T, data []byte, targetMs int64, keyOnly bool) ([]videoFrame, error) {
	t.Helper()
	r := bytes.NewReader(data)
	ix, err := indexSegment(r, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return collectSeekFrames(r, ix, 1, 1_000_000, targetMs, true, keyOnly)
}

// TestKeyOnlyLacedKeyBlockKeepsLastLace checks that a laced key block before
// the target yields its last lace, as the full window does.
func TestKeyOnlyLacedKeyBlockKeepsLastLace(t *testing.T) {
	data := buildVP8TestMKV([]testBlock{
		{ms: 0, key: true, laces: [][]byte{{'A'}, {'B'}, {'C'}}},
		{ms: 100, laces: [][]byte{{'d'}}},
		{ms: 200, laces: [][]byte{{'e'}}},
	}, 300)
	for _, keyOnly := range []bool{false, true} {
		fr, err := seekFrames(t, data, 150, keyOnly)
		if err != nil || len(fr) == 0 {
			t.Fatalf("keyOnly=%v: %d frames, err %v", keyOnly, len(fr), err)
		}
		if got := fr[0].payload[0]; got != 'C' {
			t.Errorf("keyOnly=%v: first frame %q, want last lace 'C'", keyOnly, got)
		}
	}
	// At or after the target the first lace is kept and collection stops.
	fr, err := seekFrames(t, data, 0, true)
	if err != nil || len(fr) != 1 || fr[0].payload[0] != 'A' {
		t.Fatalf("target 0: %v err %v, want [A]", fr, err)
	}
}

// TestKeyOnlyFirstKeyframeAfterTarget checks that a keyframe shortly after
// the target is still chosen when none precedes it.
func TestKeyOnlyFirstKeyframeAfterTarget(t *testing.T) {
	data := buildVP8TestMKV([]testBlock{
		{ms: 0, laces: [][]byte{{'a'}}},
		{ms: 100, laces: [][]byte{{'b'}}},
		{ms: 200, key: true, laces: [][]byte{{'K'}}},
	}, 300)
	fr, err := seekFrames(t, data, 50, true)
	if err != nil || len(fr) != 1 || fr[0].payload[0] != 'K' {
		t.Fatalf("got %v err %v, want [K]", fr, err)
	}
}

// TestKeyOnlyLookaheadBoundedAndFallback checks that the keyframe-only pass
// stops maxKeyLookahead blocks past the target, and that a malformed block
// after the target does not stop the full-window fallback.
func TestKeyOnlyLookaheadBoundedAndFallback(t *testing.T) {
	var blocks []testBlock
	for i := 0; i <= 500+maxKeyLookahead+10; i++ {
		blocks = append(blocks, testBlock{ms: int16(i), laces: [][]byte{{'n'}}})
	}
	far := append(blocks, testBlock{ms: int16(len(blocks)), key: true, laces: [][]byte{{'K'}}})
	fr, err := seekFrames(t, buildVP8TestMKV(far, float64(len(far))), 500, true)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(fr); n == 0 || fr[n-1].payload[0] == 'K' || fr[n-1].timestampMs != 500 {
		t.Fatalf("want full window ending at 500 ms, got %d frames ending %v", n, fr[max(n-1, 0):])
	}

	// An unknown-size element inside the cluster is corrupt to the walker.
	bad := append(append([]testBlock(nil), blocks[:600]...), testBlock{raw: []byte{0xA3, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}})
	fr, err = seekFrames(t, buildVP8TestMKV(bad, 600), 500, true)
	if err != nil || len(fr) == 0 || fr[len(fr)-1].timestampMs != 500 {
		t.Fatalf("malformed tail: %d frames err %v, want full window to 500 ms", len(fr), err)
	}
}

// TestVP8InvisibleKeyframeFallsBack checks that keyframe mode still returns a
// picture when the selected keyframe is invisible and a later frame shows it.
func TestVP8InvisibleKeyframeFallsBack(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "blue.webm"))
	if err != nil {
		t.Skip(err)
	}
	r := bytes.NewReader(src)
	ix, err := indexSegment(r, int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	track, _, ok := videoTrackNumber(ix.tracks, "V_VP8")
	if !ok {
		t.Fatal("no VP8 track")
	}
	fr, err := collectSeekFrames(r, ix, track, 1_000_000, 0, true, false)
	if err != nil || len(fr) == 0 {
		t.Fatalf("fixture frames: %v", err)
	}
	visible := fr[0].payload
	if visible[0]&0x01 != 0 || visible[0]&0x10 == 0 {
		t.Fatal("fixture frame is not a shown VP8 keyframe")
	}
	hidden := append([]byte(nil), visible...)
	hidden[0] &^= 0x10 // show_frame = 0

	// The hidden keyframe is flagged as the key; the shown frame after it is
	// not, so keyframe-only collection returns just the hidden one.
	data := buildVP8TestMKV([]testBlock{
		{ms: 0, key: true, laces: [][]byte{hidden}},
		{ms: 40, laces: [][]byte{visible}},
		{ms: 80, laces: [][]byte{visible}},
	}, 120)
	if fr, _ := seekFrames(t, data, 60, true); len(fr) != 1 {
		t.Fatalf("keyframe-only window has %d frames, want 1", len(fr))
	}
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := VP8PictureAtSeek(bytes.NewReader(data), int64(len(data)), h, 50, false); !ok {
		t.Error("VP8PictureAtSeek: no picture for invisible keyframe")
	}
	img, ok := ThumbnailRGBA(bytes.NewReader(data), int64(len(data)), h, ThumbnailParams{SeekPercent: 50, MaxWidth: 16, MaxHeight: 16})
	if !ok || img == nil {
		t.Error("ThumbnailRGBA: no picture for invisible keyframe")
	}
}
