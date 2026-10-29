package isobmff

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/detect"
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

// TestVideoRotationReadsTkhd checks that VideoRotation reads the tkhd matrix.
func TestVideoRotationReadsTkhd(t *testing.T) {
	quarter := [9]int32{0, 0x10000, 0, -0x10000, 0, 0, 0, 0, 0x40000000}
	data := buildFragMP4(fragTestOpts{
		movieTS: 1000, mvhdDur: 1000, trackID: 1, trackTS: 1000, matrix: quarter,
	})
	rot, ok := VideoRotation(bytes.NewReader(data), int64(len(data)))
	if !ok || rot != -90 {
		t.Fatalf("VideoRotation = (%d, %v), want (-90, true)", rot, ok)
	}
}

// quarterTurn is a tkhd matrix that rotates 90 degrees (ffprobe rotation -90).
var quarterTurn = [9]int32{0, 0x10000, 0, -0x10000, 0, 0, 0, 0, 0x40000000}

// namedTrak describes one synthetic trak: its handler, tkhd matrix, and the
// sample entries in stsd order.
type namedTrak struct {
	handler string
	matrix  [9]int32
	entries []namedEntry
}

// namedEntry is one stsd sample entry: a FourCC and an optional payload. A
// nil payload gets a zeroed entry of the handler's minimum size.
type namedEntry struct {
	fourCC  string
	payload []byte
}

// soundEntryV0 builds a version 0 sound sample entry payload with samplesize bits.
func soundEntryV0(bits uint16) []byte {
	p := make([]byte, 28)
	binary.BigEndian.PutUint16(p[18:], bits)
	return p
}

// soundEntryV2 builds a version 2 (LPCM) sound sample entry payload.
func soundEntryV2(bits, flags uint32) []byte {
	p := make([]byte, 64)
	binary.BigEndian.PutUint16(p[8:], 2)
	binary.BigEndian.PutUint32(p[48:], bits)
	binary.BigEndian.PutUint32(p[52:], flags)
	return p
}

// buildNamedEntryMP4 builds a moov with the given traks in order.
func buildNamedEntryMP4(t *testing.T, traks []namedTrak) []byte {
	t.Helper()
	ftyp := synthBox("ftyp", []byte("isom"), u32s(0x200), []byte("isomiso2mp41"))
	mvhd := synthBox("mvhd", u32s(0, 0, 0, 1000, 1000), u32s(0x00010000), []byte{1, 0}, make([]byte, 10), matrixBytes(identityMatrix), make([]byte, 24), u32s(3))
	moov := [][]byte{mvhd}
	for i, tr := range traks {
		id := uint32(i + 1)
		tkhd := synthBox("tkhd", u32s(0x7, 0, 0, id, 0, 0), make([]byte, 16), matrixBytes(tr.matrix), u32s(1920<<16, 1080<<16))
		mdhd := synthBox("mdhd", u32s(0, 0, 0, 1000, 0), []byte{0x55, 0xc4, 0, 0})
		hdlr := synthBox("hdlr", u32s(0), u32s(0), []byte(tr.handler), make([]byte, 12), []byte{0})
		stsdBody := [][]byte{u32s(0, uint32(len(tr.entries)))}
		for _, e := range tr.entries {
			p := e.payload
			if p == nil {
				if tr.handler == "soun" {
					p = make([]byte, 28)
				} else {
					p = make([]byte, 78)
					binary.BigEndian.PutUint16(p[24:], 1920)
					binary.BigEndian.PutUint16(p[26:], 1080)
				}
			}
			stsdBody = append(stsdBody, synthBox(e.fourCC, p))
		}
		stsd := synthBox("stsd", stsdBody...)
		stbl := synthBox("stbl", stsd)
		minf := synthBox("minf", stbl)
		moov = append(moov, synthBox("trak", tkhd, synthBox("mdia", mdhd, hdlr, minf)))
	}
	return append(ftyp, synthBox("moov", moov...)...)
}

// streamsOf runs Streams over data and fails the test on error.
func streamsOf(t *testing.T, data []byte) []detect.Stream {
	t.Helper()
	streams, err := Streams(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Streams: %v", err)
	}
	return streams
}

// TestNamedSampleEntriesAreReported checks that undecoded sample entries are
// reported with ffprobe codec names, PCM formats read from the entry fields.
func TestNamedSampleEntriesAreReported(t *testing.T) {
	streams := streamsOf(t, buildNamedEntryMP4(t, []namedTrak{
		{"vide", identityMatrix, []namedEntry{{"av01", nil}}},
		{"soun", identityMatrix, []namedEntry{{"lpcm", soundEntryV2(24, 0x4)}}},
		{"soun", identityMatrix, []namedEntry{{"sowt", soundEntryV0(16)}}},
		{"soun", identityMatrix, []namedEntry{{"twos", soundEntryV0(24)}}},
		{"soun", identityMatrix, []namedEntry{{"lpcm", nil}}},
	}))
	want := []string{"av1", "pcm_s24le", "pcm_s16le", "pcm_s24be", "lpcm"}
	if len(streams) != len(want) {
		t.Fatalf("streams = %+v, want codecs %v", streams, want)
	}
	for i, s := range streams {
		if s.Codec != want[i] {
			t.Errorf("stream %d codec = %q, want %q", i, s.Codec, want[i])
		}
	}
	if s := streams[0]; s.Width != 1920 || s.Height != 1080 {
		t.Fatalf("av1 dims = %dx%d, want 1920x1080", s.Width, s.Height)
	}
}

// TestNamedSampleEntryReportsRotation checks Stream.Rotation through Streams
// for a rotated named sample entry, with coded dimensions unchanged.
func TestNamedSampleEntryReportsRotation(t *testing.T) {
	streams := streamsOf(t, buildNamedEntryMP4(t, []namedTrak{
		{"vide", quarterTurn, []namedEntry{{"av01", nil}}},
	}))
	if len(streams) != 1 || streams[0].Codec != "av1" {
		t.Fatalf("streams = %+v, want one av1 stream", streams)
	}
	s := streams[0]
	if s.Rotation != -90 {
		t.Fatalf("Rotation = %d, want -90", s.Rotation)
	}
	if s.Width != 1920 || s.Height != 1080 {
		t.Fatalf("dims = %dx%d, want 1920x1080", s.Width, s.Height)
	}
}

// TestNamedSampleEntryUsesStsdOrder checks that with several named entries
// in one stsd the first one is reported, every run.
func TestNamedSampleEntryUsesStsdOrder(t *testing.T) {
	data := buildNamedEntryMP4(t, []namedTrak{
		{"vide", identityMatrix, []namedEntry{{"hvc1", nil}, {"av01", nil}}},
	})
	for i := 0; i < 20; i++ {
		streams := streamsOf(t, data)
		if len(streams) != 1 || streams[0].Codec != "hevc" {
			t.Fatalf("run %d: streams = %+v, want one hevc stream", i, streams)
		}
	}
}

// TestAttachedCoverFourCCsAreTrimmed checks that padded still-image FourCCs
// ("png ", "jpg ", "bmp ") are recognised; the comparison trims them first.
func TestAttachedCoverFourCCsAreTrimmed(t *testing.T) {
	for _, fc := range []string{"jpeg", "jpg ", "png ", "bmp "} {
		if !isAttachedCoverSampleEntry(fc, "vide") {
			t.Errorf("%q on vide: not an attached cover", fc)
		}
	}
	if isAttachedCoverSampleEntry("png ", "soun") {
		t.Errorf("png on soun must not be a cover")
	}
}
