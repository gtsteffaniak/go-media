package matroska

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// cat concatenates byte slices.
func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// rawElem encodes an element from literal ID and size bytes, followed by body.
func rawElem(id, size []byte, body ...[]byte) []byte {
	return cat(id, size, cat(body...))
}

// shortElem encodes an element with a one-byte size (body under 127 bytes).
func shortElem(id uint32, body ...[]byte) []byte {
	b := cat(body...)
	return cat(ebmlIDBytes(id), []byte{0x80 | byte(len(b))}, b)
}

// nested wraps body in depth BlockGroup elements.
func nested(depth int, body []byte) []byte {
	for range depth {
		body = ebmlElem(0xA0, body)
	}
	return body
}

// ebmlCase is one decodeEBML input. check, when set, inspects the result of
// decoding buf as an Info element and as a Tracks element.
type ebmlCase struct {
	name    string
	buf     []byte
	wantErr bool
	check   func(t *testing.T, info segmentInfo, tracks segmentTracks)
}

// ebmlCases covers the decoder rules: field storage, type and size checks,
// hand-backs of top-level elements, unknown sizes, non-canonical IDs, block
// lacing and nesting depth. Each case was checked against ebml-go v0.19.4.
func ebmlCases() []ebmlCase {
	f32 := make([]byte, 4)
	binary.BigEndian.PutUint32(f32, math.Float32bits(2.5))
	fullEntry := ebmlElem(eidTrackEntry,
		u64Elem(eidTrackNumber, 2),
		u64Elem(eidTrackType, 2),
		ebmlElem(eidCodecID, []byte("A_OPUS")),
		ebmlElem(eidCodecPrivate, []byte("OpusHead")),
		ebmlElem(eidLanguage, []byte("jpn")),
		ebmlElem(eidName, []byte("Main")),
		u64Elem(eidDefaultDuration, 20_000_000),
		u64Elem(eidCodecDelay, 6_500_000),
		f64Elem(eidTrackTimestampScale, 1),
		ebmlElem(eidVideo, u64Elem(eidPixelWidth, 1)),
		ebmlElem(eidVideo, u64Elem(eidPixelHeight, 2)),
		ebmlElem(eidAudio, f64Elem(eidSamplingFrequency, 48000), u64Elem(eidChannels, 2), u64Elem(eidBitDepth, 16)),
		ebmlElem(eidContentEncodings,
			ebmlElem(eidContentEncoding, u64Elem(eidContentEncodingScope, 1), ebmlElem(eidContentCompression, u64Elem(eidContentCompAlgo, 3), ebmlElem(eidContentCompSettings, []byte{0xAA}))),
			ebmlElem(eidContentEncoding, u64Elem(eidContentEncodingType, 0)),
		),
	)
	simpleBlock := func(payload ...byte) []byte { return shortElem(0xA3, payload) }
	return []ebmlCase{
		{name: "info", buf: infoTest(1_000_000, 5000), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 1_000_000 || info.Duration != 5000 {
				t.Errorf("info = %+v", info)
			}
		}},
		{name: "float32 duration", buf: ebmlElem(idInfo, ebmlElem(eidDuration, f32)), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.Duration != 2.5 {
				t.Errorf("duration = %v", info.Duration)
			}
		}},
		{name: "3-byte float", buf: ebmlElem(idInfo, ebmlElem(eidDuration, []byte{1, 2, 3})), wantErr: true},
		{name: "9-byte uint", buf: ebmlElem(idInfo, ebmlElem(eidTimecodeScale, make([]byte, 9))), wantErr: true},
		{name: "empty uint", buf: ebmlElem(idInfo, ebmlElem(eidTimecodeScale)), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 0 {
				t.Errorf("scale = %d", info.TimecodeScale)
			}
		}},
		{name: "last scalar wins", buf: ebmlElem(idInfo, u64Elem(eidTimecodeScale, 1), u64Elem(eidTimecodeScale, 2)), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 2 {
				t.Errorf("scale = %d", info.TimecodeScale)
			}
		}},
		{name: "unknown ID", buf: ebmlElem(idInfo, ebmlElem(0x5A5A, []byte{1})), wantErr: true},
		{name: "unused known elements", buf: ebmlElem(idInfo, ebmlElem(0x7BA9, []byte("t")), ebmlElem(0x4461, make([]byte, 8)), ebmlElem(0xEC, make([]byte, 5)), u64Elem(eidTimecodeScale, 7)), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 7 {
				t.Errorf("scale = %d", info.TimecodeScale)
			}
		}},
		{name: "9-byte date", buf: ebmlElem(idInfo, ebmlElem(0x4461, make([]byte, 9))), wantErr: true},
		{name: "child overflows parent", buf: rawElem(ebmlIDBytes(idInfo), []byte{0x83}, u64Elem(eidTimecodeScale, 1)), wantErr: true},
		{name: "unknown-size leaf", buf: ebmlElem(idInfo, ebmlUnknown(eidTimecodeScale, []byte{1})), wantErr: true},
		{name: "unknown-size master in sized parent", buf: ebmlElem(idTracks, ebmlElem(eidTrackEntry, ebmlUnknown(eidVideo))), wantErr: true},
		{name: "partial child header", buf: ebmlElem(idInfo, u64Elem(eidTimecodeScale, 1), []byte{0x2A}), wantErr: true},
		{name: "partial header after element", buf: cat(infoTest(1, 0), []byte{0x42}), wantErr: true},
		{name: "string ends at NUL, empty binary kept", buf: ebmlElem(idTracks, ebmlElem(eidTrackEntry, ebmlElem(eidCodecID, []byte("V_VP8\x00junk")), ebmlElem(eidCodecPrivate))), check: func(t *testing.T, _ segmentInfo, tracks segmentTracks) {
			if len(tracks.TrackEntry) != 1 {
				t.Fatalf("tracks = %+v", tracks)
			}
			e := tracks.TrackEntry[0]
			if e.CodecID != "V_VP8" || e.CodecPrivate == nil || len(e.CodecPrivate) != 0 {
				t.Errorf("entry = %+v", e)
			}
		}},
		{name: "full track entry", buf: ebmlElem(idTracks, fullEntry, ebmlElem(eidTrackEntry, u64Elem(eidTrackNumber, 3))), check: func(t *testing.T, _ segmentInfo, tracks segmentTracks) {
			if len(tracks.TrackEntry) != 2 || tracks.TrackEntry[1].TrackNumber != 3 {
				t.Fatalf("tracks = %+v", tracks)
			}
			e := tracks.TrackEntry[0]
			if e.TrackNumber != 2 || e.TrackType != 2 || e.CodecID != "A_OPUS" || string(e.CodecPrivate) != "OpusHead" ||
				e.Language != "jpn" || e.Name != "Main" || e.DefaultDuration != 20_000_000 || e.CodecDelay != 6_500_000 || e.TrackTimestampScale != 1 {
				t.Errorf("entry = %+v", e)
			}
			if e.Video == nil || e.Video.PixelWidth != 0 || e.Video.PixelHeight != 2 {
				t.Errorf("video = %+v (a repeated Video replaces the first)", e.Video)
			}
			if e.Audio == nil || e.Audio.SamplingFrequency != 48000 || e.Audio.Channels != 2 || e.Audio.BitDepth != 16 {
				t.Errorf("audio = %+v", e.Audio)
			}
			ce := e.ContentEncodings
			if ce == nil || len(ce.ContentEncoding) != 2 || ce.ContentEncoding[0].ContentCompression == nil ||
				ce.ContentEncoding[0].ContentCompression.ContentCompAlgo != 3 || !bytes.Equal(ce.ContentEncoding[0].ContentCompression.ContentCompSettings, []byte{0xAA}) {
				t.Errorf("content encodings = %+v", ce)
			}
		}},
		{name: "top-level element ends Info", buf: ebmlElem(idInfo, u64Elem(eidTimecodeScale, 1000), ebmlElem(idCluster, u64Elem(0xE7, 0)), f64Elem(eidDuration, 7)), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 1000 || info.Duration != 0 {
				t.Errorf("info = %+v (Duration after the Cluster is outside Info)", info)
			}
		}},
		{name: "Info inside Info merges", buf: ebmlElem(idInfo, u64Elem(eidTimecodeScale, 1000), ebmlElem(idInfo, f64Elem(eidDuration, 9))), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 1000 || info.Duration != 9 {
				t.Errorf("info = %+v", info)
			}
		}},
		{name: "handed-back header overflows by declared size", buf: ebmlElem(idTracks, ebmlElem(eidTrackEntry, u64Elem(eidTrackType, 1), ebmlElem(idInfo, ebmlElem(idCluster), u64Elem(eidTimecodeScale, 1)))), wantErr: true},
		{name: "EBML header ends unknown-size element", buf: cat(infoTest(1000, 0), ebmlUnknown(idSegment, ebmlElem(idEBMLHeader), infoTest(0, 3))), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 1000 || info.Duration != 3 {
				t.Errorf("info = %+v", info)
			}
		}},
		{name: "unknown-size element swallows the rest", buf: cat(infoTest(1000, 0), ebmlUnknown(idSegment, infoTest(0, 3))), check: func(t *testing.T, info segmentInfo, _ segmentTracks) {
			if info.TimecodeScale != 1000 || info.Duration != 0 {
				t.Errorf("info = %+v", info)
			}
		}},
		{name: "non-canonical 2-byte ID", buf: ebmlElem(idTracks, ebmlElem(eidTrackEntry, rawElem([]byte{0x40, 0x03}, []byte{0x81}, []byte{1}))), check: func(t *testing.T, _ segmentInfo, tracks segmentTracks) {
			if len(tracks.TrackEntry) != 1 || tracks.TrackEntry[0].TrackType != 1 {
				t.Errorf("tracks = %+v", tracks)
			}
		}},
		{name: "8-byte ID keeps low 32 bits", buf: ebmlElem(idTracks, ebmlElem(eidTrackEntry, rawElem([]byte{0x01, 0x00, 0x00, 0x77, 0x00, 0x00, 0x00, 0x03}, []byte{0x81}, []byte{2}))), check: func(t *testing.T, _ segmentInfo, tracks segmentTracks) {
			if len(tracks.TrackEntry) != 1 || tracks.TrackEntry[0].TrackType != 2 {
				t.Errorf("tracks = %+v", tracks)
			}
		}},
		{name: "zero ID byte reads as ChapterDisplay", buf: ebmlElem(idInfo, []byte{0x00, 0x80}, u64Elem(eidTimecodeScale, 5))},
		{name: "zero size byte", buf: ebmlElem(idInfo, rawElem(ebmlIDBytes(eidTimecodeScale), []byte{0x00}))},
		{name: "block, no lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x80, 'a', 'b'))},
		{name: "block too short", buf: ebmlElem(idTracks, simpleBlock(0x81, 0)), wantErr: true},
		{name: "block, Xiph lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x02, 1, 2, 'a', 'b', 'c'))},
		{name: "block, bad Xiph lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x02, 1, 0xFF, 0x10, 'a')), wantErr: true},
		{name: "block, fixed lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x04, 1, 'a', 'b', 'c', 'd'))},
		{name: "block, bad fixed lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x04, 1, 'a', 'b', 'c')), wantErr: true},
		{name: "block, EBML lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x06, 2, 0x82, 0xBF, 'a', 'b', 'c', 'd', 'e', 'f'))},
		{name: "block, bad EBML lacing", buf: ebmlElem(idTracks, simpleBlock(0x81, 0, 0, 0x06, 2, 0x82, 0xC5, 'a', 'b', 'c')), wantErr: true},
		{name: "block ends at end of stream", buf: cat(infoTest(1, 0), ebmlUnknown(idCluster, []byte{0xA3, 0x8A, 0x81, 0, 0, 0x80}))},
		{name: "block cut inside a frame", buf: cat(infoTest(1, 0), ebmlUnknown(idCluster, []byte{0xA3, 0x8A, 0x81, 0, 0, 0x80, 'a'})), wantErr: true},
		{name: "nesting at the limit", buf: ebmlElem(idInfo, nested(ebmlMaxDepth-1, nil))},
		{name: "nesting past the limit", buf: ebmlElem(idInfo, nested(ebmlMaxDepth, nil)), wantErr: true},
	}
}

// TestDecodeEBML runs every ebmlCase through decodeEBML.
func TestDecodeEBML(t *testing.T) {
	if _, ok := ebmlElements[0x5A5A]; ok {
		t.Fatal("0x5A5A must be absent from the schema for the unknown-ID case")
	}
	for _, tc := range ebmlCases() {
		t.Run(tc.name, func(t *testing.T) {
			var info infoElement
			var tracks tracksElement
			ie, te := decodeEBML(tc.buf, &info), decodeEBML(tc.buf, &tracks)
			if (ie != nil) != tc.wantErr || (te != nil) != tc.wantErr {
				t.Fatalf("errors = %v, %v; want error %v", ie, te, tc.wantErr)
			}
			if ie != nil && !errors.Is(ie, errs.Corrupt) {
				t.Errorf("error %v does not unwrap to errs.Corrupt", ie)
			}
			if tc.check != nil && ie == nil {
				tc.check(t, info.Info, tracks.Tracks)
			}
		})
	}
}

// TestEBMLSchema checks the schema table: 234 elements, IDs with a valid length
// marker, and no two IDs sharing a marker-free value.
func TestEBMLSchema(t *testing.T) {
	if len(ebmlElements) != 234 || len(ebmlByValue) != 234 {
		t.Fatalf("schema has %d elements (%d by value), want 234", len(ebmlElements), len(ebmlByValue))
	}
	for value, el := range ebmlByValue {
		first := byte(el.id >> (8 * (el.idLen - 1)))
		if leadingZeros8(first)+1 != el.idLen {
			t.Errorf("ID %#x: length marker says %d bytes, ID has %d", el.id, leadingZeros8(first)+1, el.idLen)
		}
		if value&^(1<<(8*el.idLen-el.idLen)) != value || value|1<<(8*el.idLen-el.idLen) != el.id {
			t.Errorf("ID %#x: value %#x", el.id, value)
		}
	}
}

// FuzzDecodeEBML decodes arbitrary bytes as an Info element and as a Tracks
// element. Both must give the same error result, and any error must unwrap to
// errs.Corrupt.
func FuzzDecodeEBML(f *testing.F) {
	for _, tc := range ebmlCases() {
		f.Add(tc.buf)
	}
	f.Add(videoTrackTest("V_MPEG4/ISO/AVC", ebmlElem(eidCodecPrivate, []byte{1, 2, 3})))
	f.Fuzz(func(t *testing.T, data []byte) {
		var info infoElement
		var tracks tracksElement
		ie, te := decodeEBML(data, &info), decodeEBML(data, &tracks)
		if (ie == nil) != (te == nil) {
			t.Fatalf("info error %v, tracks error %v", ie, te)
		}
		if ie != nil && !errors.Is(ie, errs.Corrupt) {
			t.Fatalf("error %v does not unwrap to errs.Corrupt", ie)
		}
	})
}
