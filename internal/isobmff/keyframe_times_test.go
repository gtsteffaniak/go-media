package isobmff

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func kfTimes(t *testing.T, data []byte, window float64) []float64 {
	t.Helper()
	got, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), window)
	if err != nil {
		t.Fatalf("VideoKeyframeTimes: %v", err)
	}
	return got
}

// frames converts synthetic sample indexes (60 fps) to seconds.
func frames(idx ...int) []float64 {
	out := make([]float64, len(idx))
	for i, n := range idx {
		out[i] = float64(n*synthDelta) / synthTimescale
	}
	return out
}

func sameTimes(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("times = %v, want %v", got, want)
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("times = %v, want %v", got, want)
		}
	}
}

// elst builds an edts box with version-0 entries (duration, media time).
func elst(entries ...[2]int64) []byte {
	vals := []uint32{0, uint32(len(entries))}
	for _, e := range entries {
		vals = append(vals, uint32(e[0]), uint32(int32(e[1])), 0x00010000)
	}
	return synthBox("edts", synthBox("elst", u32s(vals...)))
}

// ctts builds a ctts box giving every one of n samples the same offset.
func ctts(n int, offset int32) []byte {
	return synthBox("ctts", u32s(0, 1, uint32(n), uint32(offset)))
}

func TestVideoKeyframeTimesSyncSamples(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 50, GOP: 10})
	sameTimes(t, kfTimes(t, data, 0), frames(0, 10, 20, 30, 40))
	sameTimes(t, kfTimes(t, data, float64(20*synthDelta)/synthTimescale), frames(0, 10, 20))
}

func TestVideoKeyframeTimesNoStssAllKey(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 4})
	sameTimes(t, kfTimes(t, data, 0), frames(0, 1, 2, 3))
}

func TestVideoKeyframeTimesEmptyStssFirstOnly(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 6, EmptyStss: true})
	sameTimes(t, kfTimes(t, data, 0), frames(0))
}

// sbgp builds a version-0 sample-to-group box of grouping type typ from
// (sample_count, group_description_index) runs.
func sbgp(typ string, runs ...[2]uint32) []byte {
	b := append(u32s(0), []byte(typ)...)
	b = append(b, u32s(uint32(len(runs)))...)
	for _, r := range runs {
		b = append(b, u32s(r[0], r[1])...)
	}
	return synthBox("sbgp", b)
}

// TestVideoKeyframeTimesRapGroups checks that samples in 'rap ' sample
// groups are keyframes, as in ffmpeg's mov_build_index: with an empty stss
// they replace the first-sample-only rule, and with stss they add to it.
// Other grouping types are ignored.
func TestVideoKeyframeTimesRapGroups(t *testing.T) {
	runs := [][2]uint32{{1, 1}, {9, 0}, {1, 1}, {9, 0}, {1, 1}, {9, 0}}
	data := buildSynthMP4(synthMP4Opts{Samples: 30, EmptyStss: true, Stbl: [][]byte{sbgp("rap ", runs...)}})
	sameTimes(t, kfTimes(t, data, 0), frames(0, 10, 20))

	data = buildSynthMP4(synthMP4Opts{Samples: 30, GOP: 10, Stbl: [][]byte{sbgp("rap ", [2]uint32{5, 0}, [2]uint32{1, 1})}})
	sameTimes(t, kfTimes(t, data, 0), frames(0, 5, 10, 20))

	data = buildSynthMP4(synthMP4Opts{Samples: 30, EmptyStss: true, Stbl: [][]byte{sbgp("roll", runs...)}})
	sameTimes(t, kfTimes(t, data, 0), frames(0))

	// Version 1 carries a grouping_type_parameter before the entry count.
	v1 := append(u32s(1<<24), []byte("rap ")...)
	v1 = append(v1, u32s(0, 2, 15, 0, 1, 1)...)
	data = buildSynthMP4(synthMP4Opts{Samples: 30, EmptyStss: true, Stbl: [][]byte{synthBox("sbgp", v1)}})
	sameTimes(t, kfTimes(t, data, 0), frames(15))
}

// failingReaderAt fails every read at or past off with err.
type failingReaderAt struct {
	r   *bytes.Reader
	off int64
	err error
}

func (f failingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off+int64(len(p)) > f.off {
		return 0, f.err
	}
	return f.r.ReadAt(p, off)
}

// TestVideoKeyframeTimesReadError checks that a reader error while reading
// the sample tables is returned as is, not as unsupported.
func TestVideoKeyframeTimesReadError(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 3000, GOP: 10})
	for _, box := range []string{"stss", "stts", "moov"} {
		off := int64(bytes.Index(data, []byte(box)))
		if off < 0 {
			t.Fatalf("no %s box", box)
		}
		// Fail just past the box header so the payload read fails.
		r := failingReaderAt{r: bytes.NewReader(data), off: off + 8, err: context.Canceled}
		_, err := VideoKeyframeTimes(r, int64(len(data)), 0)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: err = %v, want context.Canceled", box, err)
		}
	}
	// A file that simply ends early is still unsupported.
	short := data[:bytes.Index(data, []byte("stss"))+12]
	if _, err := VideoKeyframeTimes(bytes.NewReader(short), int64(len(data)), 0); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("truncated: err = %v, want unsupported", err)
	}
}

// TestVideoKeyframeTimesCttsNoEdit checks pts = dts + ctts without an edit
// list (ffmpeg keeps the composition offset as the start time).
func TestVideoKeyframeTimesCttsNoEdit(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 30, GOP: 10, Ctts: ctts(30, 2*synthDelta)})
	sameTimes(t, kfTimes(t, data, 0), frames(2, 12, 22))
}

// TestVideoKeyframeTimesCttsEdit checks the usual B-frame layout: the edit
// starts at the composition offset, so keyframes land on dts.
func TestVideoKeyframeTimesCttsEdit(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{
		Samples: 30, GOP: 10,
		Ctts: ctts(30, 2*synthDelta),
		Edts: elst([2]int64{30 * synthDelta, 2 * synthDelta}),
	})
	sameTimes(t, kfTimes(t, data, 0), frames(0, 10, 20))
}

// TestVideoKeyframeTimesEmptyEdit checks that a leading empty edit delays
// every keyframe by its duration.
func TestVideoKeyframeTimesEmptyEdit(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{
		Samples: 30, GOP: 10,
		Edts: elst([2]int64{6 * synthDelta, -1}, [2]int64{30 * synthDelta, 0}),
	})
	sameTimes(t, kfTimes(t, data, 0), frames(6, 16, 26))
}

// TestVideoKeyframeTimesCutEdit checks an edit starting mid-GOP: decoding
// restarts at the previous keyframe (negative, dropped) and later keyframes
// are rebased on the first frame inside the edit.
func TestVideoKeyframeTimesCutEdit(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{
		Samples: 40, GOP: 10,
		Edts: elst([2]int64{15 * synthDelta, 25 * synthDelta}),
	})
	// Edit covers samples 25..39; the index ends at the first keyframe at or
	// past the edit end (sample 40 does not exist, so 30 is the last key).
	sameTimes(t, kfTimes(t, data, 0), frames(5))

	data = buildSynthMP4(synthMP4Opts{
		Samples: 60, GOP: 10,
		Edts: elst([2]int64{20 * synthDelta, 25 * synthDelta}),
	})
	// Samples 25..44 are in the edit; keys 30, 40 are kept, and 50 is the
	// first key at the edit end, which ffmpeg also keeps in its index.
	sameTimes(t, kfTimes(t, data, 0), frames(5, 15, 25))
}

func TestVideoKeyframeTimesUnsupportedEdits(t *testing.T) {
	cases := map[string]synthMP4Opts{
		"two media edits": {Samples: 20, GOP: 10, Edts: elst([2]int64{5 * synthDelta, 0}, [2]int64{5 * synthDelta, 10 * synthDelta})},
		"zero length":     {Samples: 20, GOP: 10, Edts: elst([2]int64{0, 0})},
		"only empty":      {Samples: 20, GOP: 10, Edts: elst([2]int64{5 * synthDelta, -1})},
		"fragmented":      {Samples: 20, GOP: 10, Mvex: true},
	}
	for name, o := range cases {
		data := buildSynthMP4(o)
		_, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), 0)
		if !errors.Is(err, errs.Unsupported) {
			t.Errorf("%s: err = %v, want unsupported", name, err)
		}
	}
}

func TestVideoKeyframeTimesSkipsNonVideoTraks(t *testing.T) {
	side := synthSideTrak("soun", "mp4a", synthBox("stts", u32s(0, 1, 1, 1024)))
	data := buildSynthMP4(synthMP4Opts{Samples: 20, GOP: 10, ExtraTraks: [][]byte{side}})
	sameTimes(t, kfTimes(t, data, 0), frames(0, 10))
}

func TestParseElstCountFromSize(t *testing.T) {
	var tr kfTrack
	// Header claims 5 entries but holds one: ffmpeg trusts the size.
	tr.parseElst(u32s(0, 5, 1000, 0, 0x00010000))
	if len(tr.edits) != 1 || tr.edits[0].duration != 1000 || tr.edits[0].mediaTime != 0 {
		t.Fatalf("edits = %+v", tr.edits)
	}
	tr = kfTrack{}
	tr.parseElst(u32s(0, 0))
	if tr.hasElst {
		t.Fatal("empty elst must count as no edit list")
	}
}

func TestRescale(t *testing.T) {
	if got := rescale(1001, 90000, 1000); got != 90090 {
		t.Fatalf("rescale = %d", got)
	}
	if got := rescale(1, 3, 2); got != 2 { // 1.5 rounds half up
		t.Fatalf("rescale half = %d", got)
	}
	if got := rescale(math.MaxUint64, 90000, 1); got != 1<<62 {
		t.Fatalf("rescale overflow = %d", got)
	}
}

// FuzzVideoKeyframeTimes checks that arbitrary input never panics and that
// results are ascending and non-negative.
func FuzzVideoKeyframeTimes(f *testing.F) {
	f.Add(buildSynthMP4(synthMP4Opts{Samples: 12, GOP: 4}))
	f.Add(buildSynthMP4(synthMP4Opts{Samples: 12, GOP: 4, Ctts: ctts(12, 2*synthDelta), Edts: elst([2]int64{12 * synthDelta, 2 * synthDelta})}))
	f.Add(buildSynthMP4(synthMP4Opts{Samples: 12, GOP: 4, Edts: elst([2]int64{2 * synthDelta, -1}, [2]int64{8 * synthDelta, 5 * synthDelta})}))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := VideoKeyframeTimes(bytes.NewReader(data), int64(len(data)), 0)
		if err != nil {
			return
		}
		for i, v := range got {
			if v < 0 || math.IsNaN(v) || (i > 0 && v <= got[i-1]) {
				t.Fatalf("times not ascending and non-negative: %v", got)
			}
		}
	})
}
