package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// Minimal EBML builders for gate tests. They need no ffmpeg, so these run
// everywhere. Element IDs are written out in full.

func gateElem(id []byte, body ...[]byte) []byte {
	var payload []byte
	for _, p := range body {
		payload = append(payload, p...)
	}
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], uint64(len(payload)))
	out := append(append([]byte{}, id...), 0x01)
	out = append(out, sz[1:]...)
	return append(out, payload...)
}

func gateUint(id []byte, v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return gateElem(id, b[:])
}

var (
	idEBMLGate    = []byte{0x1A, 0x45, 0xDF, 0xA3}
	idSegmentGate = []byte{0x18, 0x53, 0x80, 0x67}
	idInfoGate    = []byte{0x15, 0x49, 0xA9, 0x66}
	idTracksGate  = []byte{0x16, 0x54, 0xAE, 0x6B}
	idClusterGate = []byte{0x1F, 0x43, 0xB6, 0x75}
)

func gateHeader() []byte {
	return gateElem(idEBMLGate, gateElem([]byte{0x42, 0x82}, []byte("webm")))
}

// gateFile builds an EBML header and a Segment with the given children.
func gateFile(children ...[]byte) []byte {
	return append(gateHeader(), gateElem(idSegmentGate, children...)...)
}

func gateInfo(withDuration bool) []byte {
	parts := [][]byte{gateUint([]byte{0x2A, 0xD7, 0xB1}, 1_000_000)}
	if withDuration {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], 0x40AF400000000000) // float64 4000.0
		parts = append(parts, gateElem([]byte{0x44, 0x89}, b[:]))
	}
	return gateElem(idInfoGate, parts...)
}

func gateVideoTrack(codec string) []byte {
	entry := gateElem([]byte{0xAE},
		gateUint([]byte{0xD7}, 1),
		gateUint([]byte{0x83}, 1),
		gateElem([]byte{0x86}, []byte(codec)),
		gateElem([]byte{0xE0}, gateUint([]byte{0xB0}, 320), gateUint([]byte{0xBA}, 240)),
	)
	return gateElem(idTracksGate, entry)
}

// gateCounter serves b and counts the bytes read from it.
type gateCounter struct {
	b     []byte
	bytes int64
}

func (g *gateCounter) ReadAt(p []byte, off int64) (int, error) {
	n := copy(p, g.b[min(off, int64(len(g.b))):])
	g.bytes += int64(n)
	if n < len(p) {
		return n, errEOFGate
	}
	return n, nil
}

var errEOFGate = errors.New("eof")

func thumbOpts() ThumbnailOptions {
	return ThumbnailOptions{MaxWidth: 160, MaxHeight: 160, SeekPercent: 10, Format: "jpeg", Quality: 72}
}

// A Matroska file with no video track is rejected from the header alone, with
// the explicit "no video track" reason.
func TestMatroskaNoVideoTrackFailsFast(t *testing.T) {
	data := gateFile(gateInfo(true))
	c := &gateCounter{b: data}
	_, err := Thumbnail(c, int64(len(data)), thumbOpts())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Thumbnail error = %v, want ErrUnsupported", err)
	}
	if c.bytes > 64<<10 {
		t.Fatalf("read %d bytes for a file with no video track", c.bytes)
	}
}

// A VP8 file without Duration cannot be seeked, so it is rejected before any
// demux pass. The 1 MiB cluster body must not be read.
func TestMatroskaVP8WithoutDurationFailsFast(t *testing.T) {
	cluster := gateElem(idClusterGate, make([]byte, 1<<20))
	data := gateFile(gateInfo(false), gateVideoTrack("V_VP8"), cluster)
	c := &gateCounter{b: data}
	_, err := Thumbnail(c, int64(len(data)), thumbOpts())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Thumbnail error = %v, want ErrUnsupported", err)
	}
	if c.bytes > 64<<10 {
		t.Fatalf("read %d bytes for a VP8 file without duration", c.bytes)
	}
}

// H.264 keeps its profile error when it is the only video track.
func TestMatroskaH264OnlyKeepsProfileError(t *testing.T) {
	data := gateFile(gateInfo(true), gateVideoTrack("V_MPEG4/ISO/AVC"))
	_, err := Thumbnail(bytes.NewReader(data), int64(len(data)), thumbOpts())
	if !errors.Is(err, ErrUnsupportedProfile) {
		t.Fatalf("Thumbnail error = %v, want ErrUnsupportedProfile", err)
	}
}
