package asf

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

type payload struct {
	keyframe bool
	pts100ns int64
	data     []byte
}

// objAsm reassembles one media object from its payload fragments.
type objAsm struct {
	active bool
	objNum int
	size   int
	buf    []byte
	wrote  int
	key    bool
	pts    int64
}

var errStopDemux = errors.New("asf: stop demux")

func videoKeyframe(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, seekPercent float64) ([]byte, []byte, error) {
	meta, err := openMeta(r, size)
	if err != nil {
		return nil, nil, err
	}
	if meta.video == nil || meta.video.codec != "wmv3" {
		return nil, nil, errs.Unsup("thumbnail", "asf: video codec not supported")
	}
	target := int64(0)
	if seekPercent > 0 && meta.playDuration > 0 {
		target = int64(float64(meta.playDuration-meta.preroll) * seekPercent / 100.0 / 100.0)
	}
	rd := newReader(r, size)
	extra := append([]byte(nil), meta.video.extradata...)
	// Pick the last keyframe at or before the target (the first one when the
	// target precedes every keyframe), matching how the other intra-only
	// thumbnail paths seek. Stop reading once a later keyframe shows up.
	var chosen []byte
	err = demuxVideoSamples(rd, meta, func(p payload) error {
		if !p.keyframe || len(p.data) < 1000 {
			return nil
		}
		if p.pts100ns > target && len(chosen) > 0 {
			return errStopDemux
		}
		chosen = p.data
		return nil
	})
	if err == errStopDemux {
		err = nil
	}
	if err != nil {
		return nil, nil, err
	}
	if len(chosen) == 0 {
		return nil, nil, errs.Unsup("thumbnail", "asf: no WMV3 keyframe")
	}
	return extra, chosen, nil
}

// demuxVideoSamples walks the ASF data packets (ASF spec 5.2) and calls fn
// once per complete media object of the video stream, in file order.
func demuxVideoSamples(rd reader, meta *fileMeta, fn func(payload) error) error {
	vid := meta.video.id
	pktSize := int(meta.maxPkt)
	if pktSize <= 0 {
		pktSize = int(meta.minPkt)
	}
	if pktSize <= 0 {
		return errs.Corrupt
	}
	data, err := rd.readAt(meta.dataOffset, int(meta.dataEnd-meta.dataOffset))
	if err != nil {
		return err
	}
	prerollMs := int64(meta.preroll / time.Millisecond)
	var obj objAsm
	emit := func(key bool, ptsMs int64, b []byte) error {
		return fn(payload{keyframe: key, pts100ns: (ptsMs - prerollMs) * 10000, data: b})
	}
	for base := 0; base+pktSize <= len(data); base += pktSize {
		pkt := data[base : base+pktSize]
		err := walkPacket(pkt, func(pl packetPayload) error {
			if pl.stream != vid {
				return nil
			}
			if pl.compressed {
				for _, sub := range pl.subs {
					if err := emit(pl.key, pl.ptsMs, append([]byte(nil), sub...)); err != nil {
						return err
					}
				}
				return nil
			}
			if pl.offset == 0 {
				obj = objAsm{active: true, objNum: pl.objNum, size: pl.objSize, key: pl.key, pts: pl.ptsMs}
				if obj.size <= 0 || obj.size > 1<<26 {
					obj.active = false
					return nil
				}
				obj.buf = make([]byte, obj.size)
			} else if !obj.active || obj.objNum != pl.objNum {
				return nil // fragment of an object whose start we never saw
			}
			if pl.offset+len(pl.data) > obj.size {
				obj.active = false
				return nil
			}
			copy(obj.buf[pl.offset:], pl.data)
			obj.wrote += len(pl.data)
			if obj.wrote >= obj.size {
				obj.active = false
				return emit(obj.key, obj.pts, obj.buf)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type packetPayload struct {
	stream     uint8
	key        bool
	objNum     int
	offset     int
	objSize    int
	ptsMs      int64
	data       []byte
	compressed bool
	subs       [][]byte
}

// walkPacket parses one fixed-size ASF data packet and calls fn per payload.
// Malformed packets are skipped rather than failing the whole file.
func walkPacket(pkt []byte, fn func(packetPayload) error) error {
	p := 0
	if len(pkt) < 1 {
		return nil
	}
	if pkt[0]&0x80 != 0 { // error correction present
		eccLen := int(pkt[0] & 0x0f)
		p = 1 + eccLen
	}
	if p+2 > len(pkt) {
		return nil
	}
	ltf := pkt[p]
	prop := pkt[p+1]
	p += 2
	var ok bool
	var pktLen, pad int
	if pktLen, p, ok = varField(pkt, p, ltf>>5); !ok {
		return nil
	}
	if _, p, ok = varField(pkt, p, ltf>>1); !ok { // sequence
		return nil
	}
	if pad, p, ok = varField(pkt, p, ltf>>3); !ok {
		return nil
	}
	if pktLen == 0 || pktLen > len(pkt) {
		pktLen = len(pkt)
	}
	p += 6 // send time (4) + duration (2)
	end := pktLen - pad
	if end > len(pkt) || p > end {
		return nil
	}
	multi := ltf&0x01 != 0
	count, plLenType := 1, byte(0)
	if multi {
		if p >= end {
			return nil
		}
		count = int(pkt[p] & 0x3f)
		plLenType = pkt[p] >> 6
		p++
	}
	for i := 0; i < count && p < end; i++ {
		var pl packetPayload
		sn := pkt[p]
		p++
		pl.stream = sn & 0x7f
		pl.key = sn&0x80 != 0
		var repl int
		if pl.objNum, p, ok = varField(pkt, p, prop>>4); !ok {
			return nil
		}
		if pl.offset, p, ok = varField(pkt, p, prop>>2); !ok {
			return nil
		}
		if repl, p, ok = varField(pkt, p, prop); !ok {
			return nil
		}
		if repl == 1 { // compressed payload: offset field is the presentation time
			pl.compressed = true
			pl.ptsMs = int64(pl.offset)
			p++ // presentation time delta
		} else if repl >= 8 {
			if p+repl > end {
				return nil
			}
			pl.objSize = int(binary.LittleEndian.Uint32(pkt[p:]))
			pl.ptsMs = int64(binary.LittleEndian.Uint32(pkt[p+4:]))
			p += repl
		} else {
			p += repl
		}
		n := end - p
		if multi {
			if n, p, ok = varField(pkt, p, plLenType); !ok {
				return nil
			}
		}
		if n < 0 || p+n > end {
			return nil
		}
		b := pkt[p : p+n]
		p += n
		if pl.compressed {
			for len(b) > 0 {
				sz := int(b[0])
				if 1+sz > len(b) {
					break
				}
				pl.subs = append(pl.subs, b[1:1+sz])
				b = b[1+sz:]
			}
		} else {
			pl.data = b
		}
		if err := fn(pl); err != nil {
			return err
		}
	}
	return nil
}

// varField reads a field whose width is given by a 2-bit length type
// (0: absent, 1: byte, 2: word, 3: dword).
func varField(b []byte, p int, lt byte) (int, int, bool) {
	switch lt & 3 {
	case 1:
		if p+1 > len(b) {
			return 0, p, false
		}
		return int(b[p]), p + 1, true
	case 2:
		if p+2 > len(b) {
			return 0, p, false
		}
		return int(binary.LittleEndian.Uint16(b[p:])), p + 2, true
	case 3:
		if p+4 > len(b) {
			return 0, p, false
		}
		return int(binary.LittleEndian.Uint32(b[p:])), p + 4, true
	}
	return 0, p, true
}
