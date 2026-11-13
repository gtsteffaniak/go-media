package matroska

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func keyTimes(t *testing.T, data []byte, window float64) []float64 {
	t.Helper()
	got, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), window)
	if err != nil {
		t.Fatalf("VideoKeyframeTimes: %v", err)
	}
	return got
}

func equalTimes(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if d := a[i] - b[i]; d > 1e-9 || d < -1e-9 {
			return false
		}
	}
	return true
}

// TestVideoKeyframeTimesClusterWalk checks the Cue-less walk: SimpleBlock key
// flags, BlockGroups without ReferenceBlock, laced key blocks at the block
// timestamp, and blocks of other tracks ignored.
func TestVideoKeyframeTimesClusterWalk(t *testing.T) {
	group := func(rel int16, ref bool) []byte {
		block := ebmlElem(idBlock, []byte{0x81, byte(uint16(rel) >> 8), byte(rel), 0x00, 'x'})
		if ref {
			return ebmlElem(idBlockGroup, block, u64Elem(idReferenceBlock, 1))
		}
		return ebmlElem(idBlockGroup, block)
	}
	data := buildVP8TestMKV([]testBlock{
		{ms: 0, key: true, laces: [][]byte{{'A'}, {'B'}}},
		{ms: 40, laces: [][]byte{{'b'}}},
		{raw: simpleBlock(2, 60, true, []byte{'a'})}, // audio-like other track
		{raw: group(80, true)},
		{raw: group(120, false)},
		{ms: 160, laces: [][]byte{{'c'}}},
		{ms: 200, key: true, laces: [][]byte{{'D'}}},
	}, 240)
	got := keyTimes(t, data, 0)
	want := []float64{0, 0.12, 0.2}
	if !equalTimes(got, want) {
		t.Fatalf("times = %v, want %v", got, want)
	}
	if got := keyTimes(t, data, 0.15); !equalTimes(got, []float64{0, 0.12}) {
		t.Fatalf("window 0.15 = %v, want [0 0.12]", got)
	}
}

// TestVideoKeyframeTimesPrefersCues checks that Cues are used when present:
// the builder writes one CuePoint per Cluster, so the Cue list is the Cluster
// starts while the walk sees every (all-key) frame.
func TestVideoKeyframeTimesPrefersCues(t *testing.T) {
	o := mkvOpts{clusters: 3, framesPer: 4, frameStepMs: 100, jpegForFrame: func(int) []byte { return []byte{1} }}
	walk := keyTimes(t, buildMJPEGMKV(t, o), 0)
	if len(walk) != 12 {
		t.Fatalf("walk found %d keyframes, want 12: %v", len(walk), walk)
	}
	o.cues = true
	cues := keyTimes(t, buildMJPEGMKV(t, o), 0)
	if !equalTimes(cues, []float64{0, 0.4, 0.8}) {
		t.Fatalf("cue times = %v, want [0 0.4 0.8]", cues)
	}
	if got := keyTimes(t, buildMJPEGMKV(t, o), 0.5); !equalTimes(got, []float64{0, 0.4}) {
		t.Fatalf("cue window = %v, want [0 0.4]", got)
	}
}

// TestVideoKeyframeTimesTimecodeScale checks scaling by TimecodeScale.
func TestVideoKeyframeTimesTimecodeScale(t *testing.T) {
	parts := [][]byte{u64Elem(idTimecodeTest, 1000), simpleBlock(1, 0, true, []byte{1}), simpleBlock(1, 500, true, []byte{1})}
	body := append(infoTest(100_000, 2000), videoTrackTest("V_VP8")...) // 0.1 ms ticks
	body = append(body, ebmlElem(idClusterTest, parts...)...)
	data := append(ebmlHeaderTest(), ebmlElem(idSegmentTest, body)...)
	if got := keyTimes(t, data, 0); !equalTimes(got, []float64{0.1, 0.15}) {
		t.Fatalf("times = %v, want [0.1 0.15]", got)
	}
}

// Track element IDs used by the keyframe tests.
const (
	idCodecDelayTest          = 0x56AA
	idTrackTimestampScaleTest = 0x23314F
)

// TestVideoKeyframeTimesCodecDelay checks that the video track's CodecDelay
// is subtracted from block and Cue times before negative times are dropped,
// as ffmpeg does.
func TestVideoKeyframeTimesCodecDelay(t *testing.T) {
	delay := u64Elem(idCodecDelayTest, 200_000_000) // 200 ms
	o := mkvOpts{clusters: 3, framesPer: 1, frameStepMs: 500, trackExtra: [][]byte{delay}}
	if got := keyTimes(t, buildMJPEGMKV(t, o), 0); !equalTimes(got, []float64{0.3, 0.8}) {
		t.Fatalf("walk = %v, want [0.3 0.8]", got)
	}
	o.cues = true
	if got := keyTimes(t, buildMJPEGMKV(t, o), 0); !equalTimes(got, []float64{0.3, 0.8}) {
		t.Fatalf("cues = %v, want [0.3 0.8]", got)
	}
}

// TestVideoKeyframeTimesTrackTimestampScale checks that a non-default
// TrackTimestampScale is left to ffprobe, and that 1.0 is accepted.
func TestVideoKeyframeTimesTrackTimestampScale(t *testing.T) {
	o := mkvOpts{clusters: 2, framesPer: 1, frameStepMs: 100, trackExtra: [][]byte{f64Elem(idTrackTimestampScaleTest, 2)}}
	data := buildMJPEGMKV(t, o)
	if _, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), 0); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("scale 2: err = %v, want unsupported", err)
	}
	o.trackExtra = [][]byte{f64Elem(idTrackTimestampScaleTest, 1)}
	if got := keyTimes(t, buildMJPEGMKV(t, o), 0); !equalTimes(got, []float64{0, 0.1}) {
		t.Fatalf("scale 1 = %v, want [0 0.1]", got)
	}
}

// TestVideoKeyframeTimesWindowBoundary checks that a keyframe exactly at the
// window is kept even when windowSec does not convert to whole ticks
// exactly (1.001 s is 1000.9999999999999 ms in float64).
func TestVideoKeyframeTimesWindowBoundary(t *testing.T) {
	parts := [][]byte{u64Elem(idTimecodeTest, 0), simpleBlock(1, 0, true, []byte{1}), simpleBlock(1, 1001, true, []byte{1}), simpleBlock(1, 1002, true, []byte{1})}
	body := append(infoTest(1_000_000, 2000), videoTrackTest("V_VP8")...)
	body = append(body, ebmlElem(idClusterTest, parts...)...)
	data := append(ebmlHeaderTest(), ebmlElem(idSegmentTest, body)...)
	if got := keyTimes(t, data, 1.001); !equalTimes(got, []float64{0, 1.001}) {
		t.Fatalf("window 1.001 = %v, want [0 1.001]", got)
	}
}

// lacedGroup builds a BlockGroup whose Block uses fixed-size lacing, with an
// optional BlockDuration (ms, 0 for none).
func lacedGroup(rel int16, durMs uint64, laces [][]byte) []byte {
	sb := fixedLacedBlock(1, rel, false, laces)
	// Re-tag the SimpleBlock body as a Block (same layout, no key flag).
	_, h, _, _ := bufElem(sb)
	kids := [][]byte{ebmlElem(idBlock, sb[h:])}
	if durMs > 0 {
		kids = append(kids, u64Elem(idBlockDuration, durMs))
	}
	return ebmlElem(idBlockGroup, kids...)
}

// TestVideoKeyframeTimesTimedLaces checks that every lace of a key block is
// a keyframe, timed from BlockDuration or DefaultDuration as ffmpeg 7.1.1+
// does, and that without either only the first lace has a time.
func TestVideoKeyframeTimesTimedLaces(t *testing.T) {
	build := func(defaultDurNs uint64, blocks ...[]byte) []byte {
		var extra [][]byte
		if defaultDurNs > 0 {
			extra = append(extra, u64Elem(0x23E383, defaultDurNs))
		}
		parts := append([][]byte{u64Elem(idTimecodeTest, 0)}, blocks...)
		body := append(infoTest(1_000_000, 2000), videoTrackTest("V_REAL/RV40", extra...)...)
		body = append(body, ebmlElem(idClusterTest, parts...)...)
		return append(ebmlHeaderTest(), ebmlElem(idSegmentTest, body)...)
	}
	two := [][]byte{{'A'}, {'B'}}
	cases := []struct {
		name string
		data []byte
		want []float64
	}{
		{"BlockDuration", build(0, lacedGroup(0, 80, two), simpleBlock(1, 500, true, []byte{1})), []float64{0, 0.04, 0.5}},
		{"DefaultDuration", build(40_000_000, fixedLacedBlock(1, 1000, true, two)), []float64{1, 1.04}},
		{"BlockDuration wins", build(10_000_000, lacedGroup(0, 80, two)), []float64{0, 0.04}},
		{"untimed", build(0, fixedLacedBlock(1, 1000, true, two)), []float64{1}},
		{"three laces", build(0, lacedGroup(0, 100, [][]byte{{'A'}, {'B'}, {'C'}})), []float64{0, 0.033, 0.066}},
	}
	for _, c := range cases {
		if got := keyTimes(t, c.data, 0); !equalTimes(got, c.want) {
			t.Errorf("%s: times = %v, want %v", c.name, got, c.want)
		}
	}
}

// failAfterReader fails every read that reaches past limit.
type failAfterReader struct {
	r     io.ReaderAt
	limit int64
}

var errInjected = errors.New("injected read failure")

func (f failAfterReader) ReadAt(p []byte, off int64) (int, error) {
	if off+int64(len(p)) > f.limit {
		return 0, errInjected
	}
	return f.r.ReadAt(p, off)
}

// TestVideoKeyframeTimesReaderErrorIsReturned checks that a reader failure
// during the Cluster walk (for example a cancelled request) is returned
// instead of a truncated list that looks complete.
func TestVideoKeyframeTimesReaderErrorIsReturned(t *testing.T) {
	o := mkvOpts{clusters: 40, framesPer: 4, frameStepMs: 100, jpegForFrame: func(int) []byte { return make([]byte, 600) }}
	data := buildMJPEGMKV(t, o)
	ix, err := indexSegment(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	r := failAfterReader{r: bytes.NewReader(data), limit: ix.firstCluster + int64(len(data)-int(ix.firstCluster))/2}
	if _, err := VideoKeyframeTimes(r, int64(len(data)), 0); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want the injected read error", err)
	}
}

// TestVideoKeyframeTimesNoVideo checks the unsupported cases.
func TestVideoKeyframeTimesNoVideo(t *testing.T) {
	body := append(infoTest(1_000_000, 100), ebmlElem(idTracks, ebmlElem(0xAE, u64Elem(0xD7, 1), u64Elem(0x83, 2)))...)
	body = append(body, ebmlElem(idClusterTest, u64Elem(idTimecodeTest, 0), simpleBlock(1, 0, true, []byte{1}))...)
	data := append(ebmlHeaderTest(), ebmlElem(idSegmentTest, body)...)
	if _, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), 0); err == nil {
		t.Fatal("audio-only file: want an error")
	}
	key := buildVP8TestMKV([]testBlock{{ms: 0, laces: [][]byte{{'a'}}}}, 40)
	if _, err := VideoKeyframeTimes(bytes.NewReader(key), int64(len(key)), 0); err == nil {
		t.Fatal("no keyframes: want an error")
	}
}

// FuzzVideoKeyframeTimes checks that arbitrary input never panics and that
// any result is ascending and non-negative.
func FuzzVideoKeyframeTimes(f *testing.F) {
	f.Add(buildMJPEGMKV(f, mkvOpts{clusters: 2, framesPer: 2, frameStepMs: 100, cues: true, jpegForFrame: func(int) []byte { return []byte{1} }}))
	f.Add(buildMJPEGMKV(f, mkvOpts{clusters: 2, framesPer: 3, frameStepMs: 40, jpegForFrame: func(int) []byte { return []byte{1} }}))
	f.Add(buildVP8TestMKV([]testBlock{{ms: 0, key: true, laces: [][]byte{{'A'}, {'B'}}}, {ms: -5, laces: [][]byte{{'b'}}}}, 40))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, w := range []float64{0, 0.5} {
			got, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), w)
			if err != nil {
				continue
			}
			for i, v := range got {
				if v < 0 || (i > 0 && v <= got[i-1]) {
					t.Fatalf("times not ascending and non-negative: %v", got)
				}
			}
		}
	})
}
