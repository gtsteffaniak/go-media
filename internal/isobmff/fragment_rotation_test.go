package isobmff

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func matrixBytes(m [9]int32) []byte {
	b := make([]byte, 36)
	for i, v := range m {
		binary.BigEndian.PutUint32(b[4*i:], uint32(v))
	}
	return b
}

var identityMatrix = [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000}

// fragTestOpts describes a small fragmented MP4 for tests.
type fragTestOpts struct {
	movieTS   uint32
	mvhdDur   uint32
	mehdDur   *uint32
	trackID   uint32
	trackTS   uint32
	matrix    [9]int32
	trexDur   uint32
	moofTrafs [][]byte
}

func buildFragMP4(o fragTestOpts) []byte {
	ftyp := synthBox("ftyp", []byte("isom"), u32s(0x200), []byte("isomiso2mp41"))
	mvhd := synthBox("mvhd",
		u32s(0, 0, 0, o.movieTS, o.mvhdDur),
		u32s(0x00010000),
		[]byte{1, 0},
		make([]byte, 10),
		matrixBytes(identityMatrix),
		make([]byte, 24),
		u32s(o.trackID+1),
	)
	tkhd := synthBox("tkhd",
		u32s(0x7, 0, 0, o.trackID, 0, 0),
		make([]byte, 16),
		matrixBytes(o.matrix),
		u32s(1920<<16, 1080<<16),
	)
	mdhd := synthBox("mdhd", u32s(0, 0, 0, o.trackTS, 0), []byte{0x55, 0xc4, 0, 0})
	hdlr := synthBox("hdlr", u32s(0), u32s(0), []byte("vide"), make([]byte, 12), []byte{0})
	trak := synthBox("trak", tkhd, synthBox("mdia", mdhd, hdlr))

	trex := synthBox("trex", u32s(0, o.trackID, 1, o.trexDur, 0, 0))
	mvexParts := [][]byte{trex}
	if o.mehdDur != nil {
		mvexParts = append([][]byte{synthBox("mehd", u32s(0, *o.mehdDur))}, mvexParts...)
	}
	mvex := synthBox("mvex", mvexParts...)

	moovParts := [][]byte{mvhd, trak, mvex}
	out := append(ftyp, synthBox("moov", moovParts...)...)
	for _, traf := range o.moofTrafs {
		out = append(out, synthBox("moof", synthBox("traf", traf))...)
	}
	return out
}

// tfhdWithDefault builds a tfhd with an optional default sample duration.
func tfhdWithDefault(trackID, def uint32) []byte {
	if def == 0 {
		return synthBox("tfhd", u32s(0, trackID))
	}
	return synthBox("tfhd", u32s(0x000008, trackID, def))
}

// trunWithDurations builds a trun that carries explicit per-sample durations.
func trunWithDurations(durs ...uint32) []byte {
	return synthBox("trun", u32s(append([]uint32{0x000100, uint32(len(durs))}, durs...)...))
}

// trunCount builds a trun with only a sample count, so durations come from tfhd or trex.
func trunCount(count uint32) []byte {
	return synthBox("trun", u32s(0, count))
}

func durationSeconds(t *testing.T, data []byte) float64 {
	t.Helper()
	d, err := Duration(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Duration: %v", err)
	}
	return d.Duration.Seconds()
}

func TestFragmentDurationSumsTrunSamples(t *testing.T) {
	data := buildFragMP4(fragTestOpts{
		movieTS: 1000, mvhdDur: 0, trackID: 1, trackTS: 1000, matrix: identityMatrix,
		moofTrafs: [][]byte{
			append(tfhdWithDefault(1, 0), trunWithDurations(500, 500)...),
			append(tfhdWithDefault(1, 0), trunWithDurations(1000)...),
		},
	})
	if got := durationSeconds(t, data); math.Abs(got-2.0) > 0.001 {
		t.Fatalf("duration = %.3f, want 2.000", got)
	}
}

func TestFragmentDurationUsesDefaultSampleDuration(t *testing.T) {
	// 4 samples, 250 ms each, from the tfhd default duration (media timescale 1000).
	data := buildFragMP4(fragTestOpts{
		movieTS: 1000, mvhdDur: 0, trackID: 1, trackTS: 1000, matrix: identityMatrix,
		moofTrafs: [][]byte{append(tfhdWithDefault(1, 250), trunCount(4)...)},
	})
	if got := durationSeconds(t, data); math.Abs(got-1.0) > 0.001 {
		t.Fatalf("duration = %.3f, want 1.000", got)
	}
}

func TestFragmentDurationPrefersMehd(t *testing.T) {
	mehd := uint32(3000)
	data := buildFragMP4(fragTestOpts{
		movieTS: 1000, mvhdDur: 0, mehdDur: &mehd, trackID: 1, trackTS: 1000, matrix: identityMatrix,
		moofTrafs: [][]byte{append(tfhdWithDefault(1, 0), trunWithDurations(1000)...)},
	})
	if got := durationSeconds(t, data); math.Abs(got-3.0) > 0.001 {
		t.Fatalf("duration = %.3f, want 3.000 from mehd", got)
	}
}

func TestMvhdDurationStillWins(t *testing.T) {
	data := buildFragMP4(fragTestOpts{
		movieTS: 1000, mvhdDur: 5000, trackID: 1, trackTS: 1000, matrix: identityMatrix,
		moofTrafs: [][]byte{append(tfhdWithDefault(1, 0), trunWithDurations(1000)...)},
	})
	if got := durationSeconds(t, data); math.Abs(got-5.0) > 0.001 {
		t.Fatalf("duration = %.3f, want 5.000 from mvhd", got)
	}
}

func TestRotationFromMatrix(t *testing.T) {
	cases := []struct {
		name string
		m    [9]int32
		want int
	}{
		{"identity", identityMatrix, 0},
		{"quarter turn one way", [9]int32{0, 0x10000, 0, -0x10000, 0, 0, 0, 0, 0x40000000}, -90},
		{"quarter turn other way", [9]int32{0, -0x10000, 0, 0x10000, 0, 0, 0, 0, 0x40000000}, 90},
		{"half turn", [9]int32{-0x10000, 0, 0, 0, -0x10000, 0, 0, 0, 0x40000000}, 180},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rotationFromMatrix(tc.m); got != tc.want {
				t.Fatalf("rotationFromMatrix = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestVideoRotationReadsTkhd(t *testing.T) {
	quarter := [9]int32{0, 0x10000, 0, -0x10000, 0, 0, 0, 0, 0x40000000}
	data := buildFragMP4(fragTestOpts{
		movieTS: 1000, mvhdDur: 1000, trackID: 1, trackTS: 1000, matrix: quarter,
	})
	rot, ok := VideoRotation(bytes.NewReader(data), int64(len(data)))
	if !ok || rot != -90 {
		t.Fatalf("VideoRotation = (%d, %v), want (-90, true)", rot, ok)
	}
	// Rotation is reported on the stream and width/height stay coded values.
	streams, err := Streams(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Streams: %v", err)
	}
	if len(streams) != 0 {
		// The synthetic track has no sample entry, so no stream is expected.
		t.Fatalf("unexpected streams: %+v", streams)
	}
}

// buildNamedEntryMP4 builds a moov with one trak per sample entry FourCC.
func buildNamedEntryMP4(t *testing.T, traks map[string]string) []byte {
	t.Helper()
	ftyp := synthBox("ftyp", []byte("isom"), u32s(0x200), []byte("isomiso2mp41"))
	mvhd := synthBox("mvhd", u32s(0, 0, 0, 1000, 1000), u32s(0x00010000), []byte{1, 0}, make([]byte, 10), matrixBytes(identityMatrix), make([]byte, 24), u32s(3))
	moov := [][]byte{mvhd}
	id := uint32(1)
	for fourCC, handler := range traks {
		tkhd := synthBox("tkhd", u32s(0x7, 0, 0, id, 0, 0), make([]byte, 16), matrixBytes(identityMatrix), u32s(1920<<16, 1080<<16))
		mdhd := synthBox("mdhd", u32s(0, 0, 0, 1000, 0), []byte{0x55, 0xc4, 0, 0})
		hdlr := synthBox("hdlr", u32s(0), u32s(0), []byte(handler), make([]byte, 12), []byte{0})
		entryPayload := make([]byte, 78)
		if handler == "soun" {
			entryPayload = make([]byte, 28)
		} else {
			binary.BigEndian.PutUint16(entryPayload[24:], 1920)
			binary.BigEndian.PutUint16(entryPayload[26:], 1080)
		}
		stsd := synthBox("stsd", u32s(0, 1), synthBox(fourCC, entryPayload))
		stbl := synthBox("stbl", stsd)
		minf := synthBox("minf", stbl)
		moov = append(moov, synthBox("trak", tkhd, synthBox("mdia", mdhd, hdlr, minf)))
		id++
	}
	return append(ftyp, synthBox("moov", moov...)...)
}

func TestNamedSampleEntriesAreReported(t *testing.T) {
	data := buildNamedEntryMP4(t, map[string]string{
		"av01": "vide",
		"lpcm": "soun",
	})
	streams, err := Streams(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Streams: %v", err)
	}
	got := map[string]bool{}
	for _, s := range streams {
		got[s.Codec] = true
		if s.Codec == "av1" && (s.Width != 1920 || s.Height != 1080) {
			t.Fatalf("av1 dims = %dx%d, want 1920x1080", s.Width, s.Height)
		}
	}
	for _, want := range []string{"av1", "pcm_s16le"} {
		if !got[want] {
			t.Fatalf("missing stream codec %q in %+v", want, streams)
		}
	}
}
