// Package matroska supports go-media parsers.

package matroska

import (
	"bytes"
	"io"
	"math/bits"
	"time"

	"github.com/at-wat/ebml-go"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// segmentInfo holds the Segment Info element fields go-media uses.
type segmentInfo struct {
	TimecodeScale uint64  `ebml:"TimecodeScale"`
	Duration      float64 `ebml:"Duration"`
}

// segmentTracks holds the Segment Tracks element.
type segmentTracks struct {
	TrackEntry []trackEntry `ebml:"TrackEntry"`
}

// segmentHead is a data type used by this package.
type segmentHead struct {
	Segment struct {
		Info   segmentInfo   `ebml:"Info"`
		Tracks segmentTracks `ebml:"Tracks"`
	}
}

// infoElement decodes a single Segment Info element, so readHead can unmarshal
// it without walking the rest of the Segment.
type infoElement struct {
	Info segmentInfo `ebml:"Info"`
}

// tracksElement decodes a single Segment Tracks element.
type tracksElement struct {
	Tracks segmentTracks `ebml:"Tracks"`
}

// trackEntry is a data type used by this package.
type trackEntry struct {
	TrackNumber uint64 `ebml:"TrackNumber"`
	TrackType   uint64 `ebml:"TrackType"`
	CodecID     string `ebml:"CodecID"`
	Language    string `ebml:"Language"`
	Name        string `ebml:"Name"`
	Video       *struct {
		PixelWidth  uint64 `ebml:"PixelWidth"`
		PixelHeight uint64 `ebml:"PixelHeight"`
	} `ebml:"Video"`
	Audio *struct {
		SamplingFrequency float64 `ebml:"SamplingFrequency"`
		Channels          uint64  `ebml:"Channels"`
	} `ebml:"Audio"`
}

// Duration reads container length from headers in internal.
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	head, err := readHead(r, size)
	if err != nil {
		return detect.DurationResult{}, err
	}
	if head.Segment.Info.Duration <= 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "mkv missing duration")
	}
	scale := head.Segment.Info.TimecodeScale
	if scale == 0 {
		scale = 1_000_000
	}
	sec := head.Segment.Info.Duration * float64(scale) / 1e9
	d := time.Duration(sec * float64(time.Second))
	return detect.DurationResult{Duration: d, Source: "header"}, nil
}

// Streams returns elementary stream descriptors from container headers.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	head, err := readHead(r, size)
	if err != nil {
		return nil, err
	}
	var out []detect.Stream
	for i, t := range head.Segment.Tracks.TrackEntry {
		s := detect.Stream{Index: i}
		switch t.TrackType {
		case 1:
			s.Type = detect.StreamVideo
			if t.Video != nil {
				s.Width = int(t.Video.PixelWidth)
				s.Height = int(t.Video.PixelHeight)
			}
			s.Codec = normalizeCodec(t.CodecID)
		case 2:
			s.Type = detect.StreamAudio
			if t.Audio != nil {
				s.SampleRate = int(t.Audio.SamplingFrequency)
				s.Channels = int(t.Audio.Channels)
			}
			s.Codec = normalizeCodec(t.CodecID)
		case 17:
			s.Type = detect.StreamSubtitle
			s.Codec = normalizeCodec(t.CodecID)
		default:
			continue
		}
		s.Language = t.Language
		s.Title = t.Name
		out = append(out, s)
	}
	return out, nil
}

// EBML element IDs used by the header walker (IDs include their length marker).
const (
	idEBMLHeader = 0x1A45DFA3
	idSegment    = 0x18538067
	idInfo       = 0x1549A966
	idTracks     = 0x1654AE6B
	idCluster    = 0x1F43B675

	// maxHeaderElement bounds how much of Info or Tracks is read. Real headers
	// are a few KB, and CodecPrivate fits well inside 1 MiB. The cap stops a
	// corrupt size from forcing a large allocation and read before it fails.
	maxHeaderElement = 1 << 20
)

// readHead returns the Segment Info and Tracks headers.
//
// It reads only the Info and Tracks element bodies, plus the small header of
// each top-level Segment child. Cluster bodies are skipped by size, so the cost
// is a few reads per cluster instead of a walk over the whole file. The walk
// stops as soon as both Info and Tracks are found.
func readHead(r io.ReaderAt, size int64) (*segmentHead, error) {
	var head segmentHead
	segStart, segEnd, found, err := findSegment(r, size)
	if err != nil {
		return nil, err
	}
	if !found {
		return &head, nil
	}

	var gotInfo, gotTracks bool
	off := segStart
	for off < segEnd {
		id, hdrLen, dataSize, err := readElementHeader(r, off, segEnd)
		if err != nil {
			// A truncated header keeps whatever was already parsed.
			break
		}
		if dataSize < 0 {
			// Unknown-size element (live streams). Its end cannot be found without
			// walking its children, so stop here.
			break
		}
		elemEnd := off + hdrLen + dataSize
		if elemEnd > segEnd {
			elemEnd = segEnd
		}
		switch id {
		case idInfo:
			if !gotInfo {
				var el infoElement
				if err := decodeElement(r, off, elemEnd, &el); err != nil {
					return nil, err
				}
				head.Segment.Info = el.Info
				gotInfo = true
			}
		case idTracks:
			if !gotTracks {
				var el tracksElement
				if err := decodeElement(r, off, elemEnd, &el); err != nil {
					return nil, err
				}
				head.Segment.Tracks = el.Tracks
				gotTracks = true
			}
		}
		if gotInfo && gotTracks {
			break
		}
		off = elemEnd
	}
	return &head, nil
}

// findSegment returns the byte range of the first Segment element. Elements
// before it (the EBML header) are skipped by size.
func findSegment(r io.ReaderAt, size int64) (start, end int64, found bool, err error) {
	off := int64(0)
	for off < size {
		id, hdrLen, dataSize, err := readElementHeader(r, off, size)
		if err != nil {
			return 0, 0, false, err
		}
		if id == idSegment {
			end := size
			if dataSize >= 0 && off+hdrLen+dataSize < size {
				end = off + hdrLen + dataSize
			}
			return off + hdrLen, end, true, nil
		}
		if dataSize < 0 || off+hdrLen+dataSize > size {
			return 0, 0, false, errs.Corrupt
		}
		off += hdrLen + dataSize
	}
	return 0, 0, false, nil
}

// decodeElement reads the raw bytes of the element at [off, end) and unmarshals
// them into v. Only that element is read, not the rest of the file.
func decodeElement(r io.ReaderAt, off, end int64, v any) error {
	n := end - off
	if n <= 0 || n > maxHeaderElement {
		return errs.Corrupt
	}
	buf := make([]byte, n)
	// io.ReaderAt may return io.EOF together with a full read, so the byte
	// count decides. A short read is an error even when err is nil.
	if m, err := r.ReadAt(buf, off); int64(m) != n {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	if err := ebml.Unmarshal(bytes.NewReader(buf), v); err != nil && err != ebml.ErrReadStopped {
		return err
	}
	return nil
}

// readElementHeader reads one EBML element ID and size at off. It returns the
// header length and the data size, or -1 for an unknown size. The read stays
// inside limit.
func readElementHeader(r io.ReaderAt, off, limit int64) (id uint32, hdrLen, dataSize int64, err error) {
	avail := limit - off
	if avail < 2 {
		return 0, 0, 0, errs.Corrupt
	}
	// One read covers the longest ID (4 bytes) plus the longest size (8 bytes).
	// The buffer lives on the stack, so a header costs one ReadAt and no allocations.
	var hdr [12]byte
	m, rerr := r.ReadAt(hdr[:min(int64(len(hdr)), avail)], off)
	buf := hdr[:m]
	if len(buf) == 0 {
		if rerr == nil {
			rerr = io.ErrUnexpectedEOF
		}
		return 0, 0, 0, rerr
	}
	idLen := vintLength(buf[0])
	if idLen == 0 || idLen > 4 {
		return 0, 0, 0, errs.Corrupt
	}
	if len(buf) < idLen+1 {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	for _, b := range buf[:idLen] {
		id = id<<8 | uint32(b)
	}

	sizeLen := vintLength(buf[idLen])
	if sizeLen == 0 || sizeLen > 8 {
		return 0, 0, 0, errs.Corrupt
	}
	if len(buf) < idLen+sizeLen {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	sizeBuf := buf[idLen : idLen+sizeLen]
	val := uint64(sizeBuf[0]) & (0xFF >> sizeLen)
	allOnes := val == uint64(0xFF>>sizeLen)
	for _, b := range sizeBuf[1:] {
		val = val<<8 | uint64(b)
		allOnes = allOnes && b == 0xFF
	}
	hdrLen = int64(idLen + sizeLen)
	if allOnes {
		return id, hdrLen, -1, nil
	}
	// The data must fit in the bytes left after this header, not just in the
	// absolute limit. Callers then never see a silently clamped element.
	remain := avail - hdrLen
	if val > uint64(remain) {
		return 0, 0, 0, errs.Corrupt
	}
	return id, hdrLen, int64(val), nil
}

// vintLength returns the byte length of an EBML variable-length integer from its
// first byte, or 0 when the byte is invalid.
func vintLength(b byte) int {
	if b == 0 {
		return 0
	}
	return bits.LeadingZeros8(b) + 1
}

// Header is the bounded view of a Matroska file used for thumbnail decisions.
// ProbeHeader fills it with one header read, so callers can pass it down
// instead of walking the headers again for each decision.
type Header struct {
	// VideoCodecIDs holds the CodecID of every video track, in track order.
	VideoCodecIDs []string
	// Duration is the raw Segment Info Duration in timecode units, or 0 when absent.
	Duration float64
	// TimecodeScale is nanoseconds per timecode unit; 0 means the 1 ms default.
	TimecodeScale uint64
}

// ProbeHeader reads the bounded Segment header once and returns its video
// tracks, duration, and timecode scale. It does not read cluster data.
func ProbeHeader(r io.ReaderAt, size int64) (*Header, error) {
	head, err := readHead(r, size)
	if err != nil {
		return nil, err
	}
	h := &Header{
		Duration:      head.Segment.Info.Duration,
		TimecodeScale: head.Segment.Info.TimecodeScale,
	}
	for _, t := range head.Segment.Tracks.TrackEntry {
		if t.TrackType == 1 {
			h.VideoCodecIDs = append(h.VideoCodecIDs, t.CodecID)
		}
	}
	return h, nil
}

// HasDuration reports whether seek percentages can be mapped to time.
func (h *Header) HasDuration() bool {
	return h.Duration > 0
}

// timecodeScale returns TimecodeScale, applying the 1 ms default when it is unset.
func (h *Header) timecodeScale() uint64 {
	if h.TimecodeScale == 0 {
		return 1_000_000
	}
	return h.TimecodeScale
}

// durationMs returns the file length in milliseconds, at least 1.
func (h *Header) durationMs() int64 {
	ms := int64(h.Duration * float64(h.timecodeScale()) / 1e6)
	if ms <= 0 {
		return 1
	}
	return ms
}

// normalizeCodec maps Matroska CodecID values to short names.
func normalizeCodec(id string) string {
	switch id {
	case "V_VP8":
		return "vp8"
	case "V_VP9":
		return "vp9"
	case "V_MPEG4/ISO/AVC":
		return "h264"
	case "A_VORBIS":
		return "vorbis"
	case "A_OPUS":
		return "opus"
	default:
		return id
	}
}
