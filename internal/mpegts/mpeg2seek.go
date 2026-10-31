package ts

import (
	"bytes"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// mpeg2PictureUnit is one video PES (one coded picture as muxed by common
// TS writers) with its presentation timestamp.
type mpeg2PictureUnit struct {
	pts    int64
	hasPTS bool
	data   []byte
}

// mpeg2SeekKeyframeES returns an elementary stream holding a sequence header
// and the last I-picture whose PTS is at or before seekSec (relative to the
// first video PTS). It reads the file in large chunks and stops at the first
// picture past the target, so only the head of a long recording is scanned.
func mpeg2SeekKeyframeES(r io.ReaderAt, size int64, meta *Meta, seekSec float64) ([]byte, error) {
	es, err := seekKeyframeES(r, size, meta, seekSec, sequenceHeaderSpan, func(b []byte) bool { return pictureCodingType(b) == 1 })
	if err != nil {
		return nil, errs.Unsup("thumbnail", "mpegts: no MPEG-2 I-picture with sequence header")
	}
	// Terminate with sequence_end_code so the decoder sees the end of the
	// last slice instead of running into EOF.
	return append(es, 0, 0, 1, 0xB7), nil
}

// h264SeekKeyframeES is mpeg2SeekKeyframeES for H.264 Annex B: it returns
// SPS/PPS plus the last IDR access unit at or before seekSec.
func h264SeekKeyframeES(r io.ReaderAt, size int64, meta *Meta, seekSec float64) ([]byte, error) {
	es, err := seekKeyframeES(r, size, meta, seekSec, h264ParamSets, h264HasIDR)
	if err != nil {
		return nil, errs.Unsup("thumbnail", "mpegts: no H.264 IDR with SPS/PPS")
	}
	return es, nil
}

var errNoKeyframe = errs.Unsup("thumbnail", "mpegts: no keyframe")

// seekKeyframeES scans video PES units in file order. header extracts the
// decoder configuration carried in a unit (nil when absent); isKey reports
// a unit that decodes on its own.
func seekKeyframeES(r io.ReaderAt, size int64, meta *Meta, seekSec float64,
	header func([]byte) []byte, isKey func([]byte) bool) ([]byte, error) {
	lay := meta.Layout
	if lay.Stride <= 0 {
		return nil, errs.Unsup("thumbnail", "mpegts: unknown packet layout")
	}
	const chunkPackets = 4096
	chunk := make([]byte, lay.Stride*chunkPackets)

	var (
		seqHdr   []byte // latest sequence header (+ extensions) seen
		best     []byte // ES of the chosen I-picture (with sequence header)
		cur      []byte // PES being assembled
		firstPTS int64
		havePTS0 bool
		target   int64
	)
	finish := func() bool { // returns true when the scan can stop
		if len(cur) == 0 {
			return false
		}
		u := parsePictureUnit(cur)
		cur = cur[:0]
		if u.hasPTS && !havePTS0 {
			firstPTS, havePTS0 = u.pts, true
			target = firstPTS + int64(seekSec*90000)
		}
		if sh := header(u.data); sh != nil {
			seqHdr = append(seqHdr[:0], sh...)
		}
		if u.hasPTS && havePTS0 && best != nil && u.pts > target {
			return true
		}
		if seqHdr != nil && isKey(u.data) {
			es := make([]byte, 0, len(seqHdr)+len(u.data))
			if header(u.data) == nil {
				es = append(es, seqHdr...)
			}
			best = append(es, u.data...)
			if u.hasPTS && u.pts >= target {
				return true
			}
		}
		return false
	}

scan:
	for off := int64(0); off < size; off += int64(len(chunk)) {
		n, _ := ioat.ReadRange(r, size, off, chunk)
		n -= n % lay.Stride
		if n <= 0 {
			break
		}
		for p := 0; p+lay.Stride <= n; p += lay.Stride {
			pkt := chunk[p+lay.Offset : p+lay.Offset+packetSize]
			if pkt[0] != SyncByte {
				continue
			}
			pid := uint16(pkt[1]&0x1f)<<8 | uint16(pkt[2])
			if pid != meta.VideoPID {
				continue
			}
			afc := (pkt[3] >> 4) & 3
			if afc&1 == 0 {
				continue
			}
			start := 4
			if afc&2 != 0 {
				start += 1 + int(pkt[4])
				if start >= packetSize {
					continue
				}
			}
			if pkt[1]&0x40 != 0 {
				if finish() {
					break scan
				}
			} else if len(cur) == 0 {
				continue
			}
			cur = append(cur, pkt[start:]...)
		}
	}
	if best == nil {
		finish()
	}
	if best == nil {
		return nil, errNoKeyframe
	}
	return best, nil
}

// h264NALs calls fn for each Annex B NAL unit (without its start code).
func h264NALs(es []byte, fn func(nal []byte)) {
	i := 0
	next := func(from int) int { // index of next 00 00 01, or -1
		k := bytes.Index(es[from:], []byte{0, 0, 1})
		if k < 0 {
			return -1
		}
		return from + k
	}
	i = next(0)
	for i >= 0 {
		start := i + 3
		j := next(start)
		end := len(es)
		if j >= 0 {
			end = j
			for end > start && es[end-1] == 0 { // trailing zero of a 4-byte start code
				end--
			}
		}
		if start < end {
			fn(es[start:end])
		}
		i = j
	}
}

// h264ParamSets returns the unit's SPS and PPS NALs as Annex B, or nil when
// it carries no SPS.
func h264ParamSets(es []byte) []byte {
	var out []byte
	sps := false
	h264NALs(es, func(n []byte) {
		if t := n[0] & 0x1f; t == 7 || t == 8 {
			sps = sps || t == 7
			out = append(out, 0, 0, 0, 1)
			out = append(out, n...)
		}
	})
	if !sps {
		return nil
	}
	return out
}

// h264HasIDR reports whether the access unit holds an IDR slice.
func h264HasIDR(es []byte) bool {
	idr := false
	h264NALs(es, func(n []byte) { idr = idr || n[0]&0x1f == 5 })
	return idr
}

// parsePictureUnit splits a PES packet into its PTS and payload.
func parsePictureUnit(pes []byte) mpeg2PictureUnit {
	if len(pes) < 9 || pes[0] != 0 || pes[1] != 0 || pes[2] != 1 {
		return mpeg2PictureUnit{data: append([]byte(nil), pes...)}
	}
	hlen := 9 + int(pes[8])
	if hlen > len(pes) {
		return mpeg2PictureUnit{}
	}
	u := mpeg2PictureUnit{data: append([]byte(nil), pes[hlen:]...)}
	if pes[7]&0x80 != 0 && len(pes) >= 14 {
		b := pes[9:14]
		u.pts = int64(b[0]>>1&0x07)<<30 | int64(b[1])<<22 | int64(b[2]>>1)<<15 |
			int64(b[3])<<7 | int64(b[4]>>1)
		u.hasPTS = true
	}
	return u
}

var (
	scSeq     = []byte{0, 0, 1, 0xB3}
	scPicture = []byte{0, 0, 1, 0x00}
)

// sequenceHeaderSpan returns the sequence header and any extensions up to the
// next GOP or picture start code, or nil when es has no sequence header.
func sequenceHeaderSpan(es []byte) []byte {
	i := bytes.Index(es, scSeq)
	if i < 0 {
		return nil
	}
	for j := i + 4; j+3 < len(es); j++ {
		if es[j] == 0 && es[j+1] == 0 && es[j+2] == 1 && (es[j+3] == 0xB8 || es[j+3] == 0x00) {
			return es[i:j]
		}
	}
	return nil
}

// pictureCodingType returns the first picture's coding type (1=I, 2=P, 3=B),
// or 0 when es has no picture header.
func pictureCodingType(es []byte) int {
	i := bytes.Index(es, scPicture)
	if i < 0 || i+6 > len(es) {
		return 0
	}
	return int(es[i+5]>>3) & 7
}
