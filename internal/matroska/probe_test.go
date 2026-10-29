package matroska

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// Synthetic Matroska builders. They produce just enough EBML for the header
// walker, so these tests run without ffmpeg.

// ebmlIDBytes encodes an element ID with its minimal byte length.
func ebmlIDBytes(id uint32) []byte {
	n := 4
	for n > 1 && byte(id>>(8*(n-1))) == 0 {
		n--
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(id >> (8 * (n - 1 - i)))
	}
	return out
}

// ebmlElem encodes an element with an 8-byte size vint.
func ebmlElem(id uint32, body ...[]byte) []byte {
	var payload []byte
	for _, p := range body {
		payload = append(payload, p...)
	}
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], uint64(len(payload)))
	out := append(ebmlIDBytes(id), 0x01)
	out = append(out, sz[1:]...)
	return append(out, payload...)
}

// ebmlUnknown encodes an element with the unknown-size marker, followed by body.
func ebmlUnknown(id uint32, body ...[]byte) []byte {
	out := append(ebmlIDBytes(id), 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
	for _, p := range body {
		out = append(out, p...)
	}
	return out
}

func u64Elem(id uint32, v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return ebmlElem(id, b[:])
}

func f64Elem(id uint32, v float64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	return ebmlElem(id, b[:])
}

const (
	idInfoTest     = 0x1549A966
	idTracksTest   = 0x1654AE6B
	idSegmentTest  = 0x18538067
	idClusterTest  = 0x1F43B675
	idTrackEntry   = 0xAE
	idCodecIDTest  = 0x86
	idTrackTypeTst = 0x83
	idTrackNumTst  = 0xD7
	idVideoTest    = 0xE0
)

func ebmlHeaderTest() []byte {
	return ebmlElem(idEBMLHeader, ebmlElem(0x4282, []byte("webm")))
}

// infoTest builds Segment Info. A zero duration or scale omits that field.
func infoTest(scale uint64, duration float64) []byte {
	var parts [][]byte
	if scale > 0 {
		parts = append(parts, u64Elem(0x2AD7B1, scale))
	}
	if duration > 0 {
		parts = append(parts, f64Elem(0x4489, duration))
	}
	return ebmlElem(idInfoTest, parts...)
}

// videoTrackTest builds one Tracks element holding a single video TrackEntry.
func videoTrackTest(codec string) []byte {
	entry := ebmlElem(idTrackEntry,
		u64Elem(idTrackNumTst, 1),
		u64Elem(idTrackTypeTst, 1),
		ebmlElem(idCodecIDTest, []byte(codec)),
		ebmlElem(idVideoTest, u64Elem(0xB0, 320), u64Elem(0xBA, 240)),
	)
	return ebmlElem(idTracksTest, entry)
}

// countingReader records bytes and calls, like the thumbnail budget tests.
type countingReader struct {
	r     io.ReaderAt
	calls int
	bytes int64
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	n, err := c.r.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err
}

func TestProbeHeaderUnknownSizeSegmentAndCluster(t *testing.T) {
	data := bytes.Join([][]byte{
		ebmlHeaderTest(),
		ebmlUnknown(idSegmentTest),
		infoTest(1_000_000, 4000),
		videoTrackTest("V_VP8"),
		ebmlUnknown(idClusterTest),
	}, nil)
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.VideoCodecIDs) != 1 || h.VideoCodecIDs[0] != "V_VP8" {
		t.Fatalf("VideoCodecIDs = %v, want [V_VP8]", h.VideoCodecIDs)
	}
	if !h.HasDuration() || h.Duration != 4000 {
		t.Fatalf("Duration = %v, want 4000", h.Duration)
	}
}

// An unknown-size Cluster before Tracks ends the walk, so the tracks are not
// found. The thumbnail gate reports this as "no video track in bounded header".
func TestProbeHeaderUnknownSizeClusterBeforeTracks(t *testing.T) {
	data := bytes.Join([][]byte{
		ebmlHeaderTest(),
		ebmlUnknown(idSegmentTest),
		infoTest(1_000_000, 4000),
		ebmlUnknown(idClusterTest),
		videoTrackTest("V_VP8"),
	}, nil)
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.VideoCodecIDs) != 0 {
		t.Fatalf("VideoCodecIDs = %v, want none", h.VideoCodecIDs)
	}
}

func TestProbeHeaderMissingDuration(t *testing.T) {
	data := bytes.Join([][]byte{
		ebmlHeaderTest(),
		ebmlElem(idSegmentTest, infoTest(1_000_000, 0), videoTrackTest("V_VP8")),
	}, nil)
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if h.HasDuration() {
		t.Fatal("HasDuration = true without a Duration element")
	}
	if len(h.VideoCodecIDs) != 1 {
		t.Fatalf("VideoCodecIDs = %v, want one track", h.VideoCodecIDs)
	}
}

func TestProbeHeaderNoTracks(t *testing.T) {
	data := bytes.Join([][]byte{
		ebmlHeaderTest(),
		ebmlElem(idSegmentTest, infoTest(1_000_000, 4000)),
	}, nil)
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.VideoCodecIDs) != 0 {
		t.Fatalf("VideoCodecIDs = %v, want none", h.VideoCodecIDs)
	}
}

// The header walk must skip large cluster bodies by size. Both placements are
// checked: clusters after the headers, and clusters between Info and Tracks.
func TestProbeHeaderSkipsClusterBodies(t *testing.T) {
	cluster := ebmlElem(idClusterTest, make([]byte, 1<<20))
	layouts := map[string][][]byte{
		"cluster after headers": {
			ebmlHeaderTest(),
			ebmlElem(idSegmentTest, infoTest(1_000_000, 4000), videoTrackTest("V_VP8"), cluster),
		},
		"cluster between info and tracks": {
			ebmlHeaderTest(),
			ebmlElem(idSegmentTest, infoTest(1_000_000, 4000), cluster, videoTrackTest("V_VP8")),
		},
	}
	for name, parts := range layouts {
		t.Run(name, func(t *testing.T) {
			data := bytes.Join(parts, nil)
			c := &countingReader{r: bytes.NewReader(data)}
			h, err := ProbeHeader(c, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if len(h.VideoCodecIDs) != 1 {
				t.Fatalf("VideoCodecIDs = %v", h.VideoCodecIDs)
			}
			if c.bytes > 64<<10 {
				t.Fatalf("header probe read %d bytes of a %d byte file", c.bytes, len(data))
			}
		})
	}
}

// A corrupt Info size larger than the file must not panic or be trusted.
func TestProbeHeaderCorruptOversizedSize(t *testing.T) {
	// Info claims 2 MiB, but the file only holds a few bytes after the header.
	bad := append(ebmlIDBytes(idInfoTest), 0x01)
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], 2<<20)
	bad = append(bad, sz[1:]...)
	bad = append(bad, 0, 0, 0)
	data := bytes.Join([][]byte{ebmlHeaderTest(), ebmlElem(idSegmentTest, bad)}, nil)
	h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if h.HasDuration() || len(h.VideoCodecIDs) != 0 {
		t.Fatalf("corrupt header produced %+v", h)
	}
}

// An Info element inside the file but larger than maxHeaderElement is rejected
// before any large allocation.
func TestProbeHeaderOversizedElementRejected(t *testing.T) {
	claimed := uint64(maxHeaderElement + 512<<10)
	bad := append(ebmlIDBytes(idInfoTest), 0x01)
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], claimed)
	bad = append(bad, sz[1:]...)
	bad = append(bad, make([]byte, claimed)...)
	data := bytes.Join([][]byte{ebmlHeaderTest(), ebmlElem(idSegmentTest, bad, videoTrackTest("V_VP8"))}, nil)
	if _, err := ProbeHeader(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected error for Info above maxHeaderElement")
	}
}

// FuzzReadHead walks headers from arbitrary bytes. It must never panic, and the
// results it returns must be consistent with the input.
func FuzzReadHead(f *testing.F) {
	seeds := [][]byte{
		bytes.Join([][]byte{ebmlHeaderTest(), ebmlUnknown(idSegmentTest), infoTest(1_000_000, 4000), videoTrackTest("V_VP8"), ebmlUnknown(idClusterTest)}, nil),
		bytes.Join([][]byte{ebmlHeaderTest(), ebmlUnknown(idSegmentTest), infoTest(0, 0), ebmlUnknown(idClusterTest)}, nil),
		bytes.Join([][]byte{ebmlHeaderTest(), ebmlElem(idSegmentTest, infoTest(1_000_000, 0), videoTrackTest("V_VP8"))}, nil),
		{0x1A, 0x45, 0xDF, 0xA3, 0x01, 0xFF},
		{},
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		h, err := ProbeHeader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		if h.HasDuration() && h.durationMs() < 1 {
			t.Fatalf("durationMs %d for duration %v", h.durationMs(), h.Duration)
		}
	})
}
