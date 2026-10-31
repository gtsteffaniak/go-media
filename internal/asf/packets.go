package asf

import (
	"encoding/binary"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

type payload struct {
	keyframe bool
	pts100ns int64
	data     []byte
}

// videoKeyframe returns the next complete video sample at or after seekPercent.
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
		target = int64(float64(meta.playDuration) * seekPercent / 100.0)
	}
	rd := newReader(r, size)
	var extradata = append([]byte(nil), meta.video.extradata...)
	var got []byte
	err = walkVideoSamples(rd, meta, func(p payload) error {
		if len(got) == 0 && p.pts100ns >= target {
			got = append([]byte(nil), p.data...)
		}
		if len(got) == 0 && p.keyframe {
			got = append([]byte(nil), p.data...)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(got) == 0 {
		return nil, nil, errs.Unsup("thumbnail", "asf: no WMV3 keyframe")
	}
	return extradata, got, nil
}

func walkVideoSamples(rd reader, meta *fileMeta, fn func(payload) error) error {
	off := meta.dataOffset
	vid := meta.video.id
	var cur []byte
	var curKey bool
	var curPTS int64
	flush := func() error {
		if len(cur) == 0 {
			return nil
		}
		p := payload{keyframe: curKey, pts100ns: curPTS, data: cur}
		cur = nil
		return fn(p)
	}
	for off < meta.dataEnd {
		pkt, next, err := readASFPacket(rd, meta, off, meta.dataEnd)
		if err != nil {
			return err
		}
		if next <= off {
			break
		}
		off = next
		for _, pl := range pkt {
			if pl.streamID != vid {
				continue
			}
			if pl.isKey {
				if err := flush(); err != nil {
					return err
				}
				curKey = true
				curPTS = pl.pts100ns
			}
			cur = append(cur, pl.data...)
			if pl.complete {
				if err := flush(); err != nil {
					return err
				}
				curKey = false
			}
		}
	}
	return flush()
}

type rawPayload struct {
	streamID uint8
	isKey    bool
	pts100ns int64
	data     []byte
	complete bool
}

func readASFPacket(rd reader, meta *fileMeta, off int64, end int64) ([]rawPayload, int64, error) {
	if off >= end {
		return nil, end, nil
	}
	buf, err := rd.readAt(off, int(min64(int64(meta.maxPkt)+64, end-off)))
	if err != nil || len(buf) < 2 {
		return nil, 0, err
	}
	p := 0
	if buf[p]&0x80 != 0 {
		ecLen := int(buf[p+1])
		p = 2 + ecLen
		if p >= len(buf) {
			return nil, off + 1, errs.Corrupt
		}
	} else {
		p = 1
	}
	if p+1 >= len(buf) {
		return nil, off + int64(p), errs.Corrupt
	}
	flags := buf[p]
	p += 2
	packetLen, p := do2Bits(buf, p, flags>>5, int(meta.maxPkt))
	if packetLen <= 0 {
		return nil, off + int64(p), errs.Corrupt
	}
	_, p = do2Bits(buf, p, flags>>1, 0) // seq
	padSize, p := do2Bits(buf, p, flags>>3, 0)
	if p+6 > len(buf) {
		return nil, off + int64(packetLen), errs.Corrupt
	}
	pts := int64(binary.LittleEndian.Uint32(buf[p : p+4]))
	p += 6 // duration
	var segments int
	var segType byte
	if flags&0x01 != 0 {
		if p >= len(buf) {
			return nil, off + int64(packetLen), errs.Corrupt
		}
		segType = buf[p]
		p++
		segments = int(segType & 0x3f)
	} else {
		segments = 1
		segType = 0x80
	}
	headerEnd := p
	payloadEnd := packetLen - padSize
	if payloadEnd < headerEnd {
		return nil, off + int64(packetLen), errs.Corrupt
	}
	if off+int64(packetLen) > end {
		packetLen = int(end - off)
	}
	body, err := rd.readAt(off+int64(headerEnd), packetLen-headerEnd-padSize)
	if err != nil {
		return nil, off + int64(packetLen), err
	}
	var out []rawPayload
	pos := 0
	for seg := 0; seg < segments && pos < len(body); seg++ {
		if flags&0x01 == 0 {
			out = append(out, rawPayload{
				streamID: meta.video.id, isKey: true, pts100ns: pts, data: body, complete: true,
			})
			break
		}
		if pos >= len(body) {
			break
		}
		streamID := body[pos] & 0x7f
		key := body[pos]&0x80 != 0
		pos++
		if pos >= len(body) {
			break
		}
		repl := body[pos]
		pos++
		var payLen int
		if repl != 0 {
			if pos >= len(body) {
				break
			}
			olen := int(body[pos])
			pos++
			pos += olen
			if pos >= len(body) {
				break
			}
			payLen = int(body[pos])
			pos++
		} else {
			if pos+2 > len(body) {
				break
			}
			payLen = int(binary.LittleEndian.Uint16(body[pos : pos+2]))
			pos += 2
		}
		if payLen <= 0 || pos+payLen > len(body) {
			break
		}
		data := body[pos : pos+payLen]
		pos += payLen
		complete := segType&0x40 != 0 || repl != 0
		out = append(out, rawPayload{
			streamID: streamID, isKey: key, pts100ns: pts, data: data, complete: complete,
		})
		if segType&0x40 != 0 {
			break
		}
	}
	return out, off + int64(packetLen), nil
}

func do2Bits(buf []byte, p int, spec byte, def int) (val int, np int) {
	switch spec & 3 {
	case 3:
		if p+4 > len(buf) {
			return def, p
		}
		return int(binary.LittleEndian.Uint32(buf[p : p+4])), p + 4
	case 2:
		if p+2 > len(buf) {
			return def, p
		}
		return int(binary.LittleEndian.Uint16(buf[p : p+2])), p + 2
	case 1:
		if p+1 > len(buf) {
			return def, p
		}
		return int(buf[p]), p + 1
	default:
		return def, p
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
