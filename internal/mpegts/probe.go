package ts

import (
	"bytes"
	"fmt"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/mpeg2video/bitreader"
)

const (
	packetSize = 188
	m2tsPrefix = 4
	m2tsStride = packetSize + m2tsPrefix
	maxProbe   = 8192 // packets
)

// Layout reports how TS packets are stored in the file.
type Layout struct {
	M2TS   bool
	Stride int
	Offset int
}

// DetectLayout picks plain TS (188-byte 0x47 packets) or Blu-ray M2TS (192-byte records).
func DetectLayout(r io.ReaderAt, size int64) (Layout, bool) {
	if size < int64(packetSize) {
		return Layout{}, false
	}
	buf := make([]byte, min64(size, m2tsStride*4))
	n, _ := ioat.ReadRange(r, size, 0, buf)
	buf = buf[:n]
	if len(buf) >= packetSize && buf[0] == SyncByte {
		ok := true
		for off := packetSize; off+1 <= len(buf); off += packetSize {
			if buf[off] != SyncByte {
				ok = false
				break
			}
		}
		if ok {
			return Layout{Stride: packetSize, Offset: 0}, true
		}
	}
	if len(buf) >= m2tsStride && buf[m2tsPrefix] == SyncByte {
		ok := true
		for off := m2tsStride; off+m2tsPrefix+1 <= len(buf); off += m2tsStride {
			if buf[off+m2tsPrefix] != SyncByte {
				ok = false
				break
			}
		}
		if ok {
			return Layout{M2TS: true, Stride: m2tsStride, Offset: m2tsPrefix}, true
		}
	}
	return Layout{}, false
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// Stream describes one elementary stream from the PMT.
type Stream struct {
	PID        uint16
	StreamType byte
	Codec      string
}

// Meta holds PSI and coarse duration from a bounded packet scan.
type Meta struct {
	Layout   Layout
	Streams  []Stream
	Duration float64
	VideoPID uint16
	VideoTyp byte
}

// Probe parses PAT/PMT and estimates duration from the highest video PTS seen.
func Probe(r io.ReaderAt, size int64) (*Meta, error) {
	lay, ok := DetectLayout(r, size)
	if !ok {
		return nil, errs.NotMedia
	}
	m := &Meta{Layout: lay}
	var pmtPID uint16
	var videoPID uint16
	var videoType byte
	var maxPTS uint64
	var havePTS bool

	pktBuf := make([]byte, packetSize)
	br := bitreader.NewReader(nil)
	p := &Packet{}

	for off, i := int64(0), 0; off+int64(lay.Stride) <= size && i < maxProbe; i, off = i+1, off+int64(lay.Stride) {
		rec := make([]byte, lay.Stride)
		n, _ := ioat.ReadRange(r, size, off, rec)
		if n < lay.Stride {
			break
		}
		copy(pktBuf, rec[lay.Offset:lay.Offset+packetSize])
		if pktBuf[0] != SyncByte {
			continue
		}
		br = bitreader.NewReader(bytes.NewReader(pktBuf))
		if err := p.Next(br); err != nil {
			continue
		}
		if p.PID == 0 && p.PayloadUnitStartIndicator {
			for _, e := range parsePAT(p.Payload) {
				if e.programNumber != 0 {
					pmtPID = e.pmtPID
					break
				}
			}
		}
		if pmtPID != 0 && p.PID == uint32(pmtPID) && p.PayloadUnitStartIndicator && len(m.Streams) == 0 {
			m.Streams = parsePMT(p.Payload)
			for _, s := range m.Streams {
				if s.Codec == "h264" || s.Codec == "mpeg2video" {
					videoPID = s.PID
					videoType = s.StreamType
					break
				}
			}
		}
		if videoPID != 0 && uint16(p.PID) == videoPID && p.PayloadUnitStartIndicator {
			if pts, ok := peekPESPTS(p.Payload); ok && pts >= maxPTS {
				maxPTS = pts
				havePTS = true
			}
		}
	}
	if len(m.Streams) == 0 {
		return nil, errs.Unsup("probe", "mpegts: no program map in bounded scan")
	}
	m.VideoPID = videoPID
	m.VideoTyp = videoType
	if havePTS {
		m.Duration = float64(maxPTS) / 90000.0
	}
	return m, nil
}

type program struct {
	programNumber uint16
	pmtPID        uint16
}

func parsePAT(payload []byte) []program {
	if len(payload) < 8 || payload[0] != 0 {
		return nil
	}
	p := payload[1:]
	secLen := int(p[1]&0x0f)<<8 | int(p[2])
	if secLen+3 > len(p) {
		return nil
	}
	body := p[8 : 8+secLen-5]
	var out []program
	for i := 0; i+4 <= len(body); i += 4 {
		pn := uint16(body[i])<<8 | uint16(body[i+1])
		pid := uint16(body[i+2]&0x1f)<<8 | uint16(body[i+3])
		out = append(out, program{programNumber: pn, pmtPID: pid})
	}
	return out
}

func parsePMT(payload []byte) []Stream {
	if len(payload) < 12 || payload[0] != 0 {
		return nil
	}
	p := payload[1:]
	secLen := int(p[1]&0x0f)<<8 | int(p[2])
	if secLen+3 > len(p) {
		return nil
	}
	progInfoLen := int(p[10]&0x0f)<<8 | int(p[11])
	off := 12 + progInfoLen
	end := 8 + secLen - 4
	var streams []Stream
	for off+5 <= end && off+5 <= len(p) {
		st := p[off]
		pid := uint16(p[off+1]&0x1f)<<8 | uint16(p[off+2])
		esInfoLen := int(p[off+3]&0x0f)<<8 | int(p[off+4])
		streams = append(streams, Stream{PID: pid, StreamType: st, Codec: streamTypeName(st)})
		off += 5 + esInfoLen
	}
	return streams
}

func streamTypeName(st byte) string {
	switch st {
	case 0x01:
		return "mpeg1video"
	case 0x02:
		return "mpeg2video"
	case 0x1B:
		return "h264"
	case 0x24:
		return "hevc"
	case 0x10:
		return "mpeg4"
	default:
		return fmt.Sprintf("0x%02x", st)
	}
}

func peekPESPTS(payload []byte) (uint64, bool) {
	if len(payload) < 14 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 {
		return 0, false
	}
	if payload[7]&0x80 == 0 {
		return 0, false
	}
	p := payload[9:14]
	pts := uint64(p[0]>>1&0x07)<<30 | uint64(p[1])<<22 | uint64(p[2]>>1)<<15 | uint64(p[3])<<7 | uint64(p[4]>>1)
	return pts, true
}

// DurationSec returns the highest video PTS seen in a bounded scan (seconds).
func DurationSec(r io.ReaderAt, size int64) (float64, error) {
	m, err := Probe(r, size)
	if err != nil {
		return 0, err
	}
	return m.Duration, nil
}
