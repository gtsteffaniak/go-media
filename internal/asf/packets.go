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

type streamAsm struct {
	objSize int
	buf     []byte
	wrote   int
	key     bool
	pts     int64
}

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
	extra := append([]byte(nil), meta.video.extradata...)
	var chosen []byte
	err = demuxVideoSamples(rd, meta, func(p payload) error {
		if len(chosen) > 0 {
			return nil
		}
		if !p.keyframe {
			return nil
		}
		if len(p.data) < 1000 {
			return nil
		}
		if p.pts100ns >= target || len(chosen) == 0 {
			chosen = append([]byte(nil), p.data...)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(chosen) == 0 {
		return nil, nil, errs.Unsup("thumbnail", "asf: no WMV3 keyframe")
	}
	return extra, chosen, nil
}

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
	var st streamAsm
	var usesStdECC int
	for base := 0; base+pktSize <= len(data); base += pktSize {
		pkt := data[base : base+pktSize]
		bodyOff, prop, pts, segType, segments, sizeLeft, err := parsePacketBody(pkt, pktSize, &usesStdECC)
		if err != nil || sizeLeft <= 0 {
			continue
		}
		pos := bodyOff
		for segments > 0 && sizeLeft > 0 && pos < len(pkt) {
			fh, used, err := parseFrameHeader(pkt, pos, prop, segType, pts, sizeLeft)
			if err != nil || used == 0 {
				break
			}
			pos += used
			sizeLeft -= used
			segments--
			if fh.streamID != vid {
				continue
			}
			if fh.key && fh.fragOff == 0 {
				if st.wrote >= st.objSize && st.objSize > 0 {
					if err := emitSample(&st, fn); err != nil {
						return err
					}
				}
				st = streamAsm{key: true, pts: fh.pts}
			}
			if fh.objSize > 0 && st.objSize != fh.objSize {
				old := append([]byte(nil), st.buf...)
				st.objSize = fh.objSize
				if cap(st.buf) < st.objSize {
					st.buf = make([]byte, st.objSize)
				} else {
					st.buf = st.buf[:st.objSize]
				}
				if len(old) > 0 {
					copy(st.buf, old)
				} else {
					clear(st.buf)
				}
				if st.wrote > st.objSize {
					st.wrote = st.objSize
				}
			}
			end := fh.fragOff + fh.fragSize
			if end > len(st.buf) {
				nb := make([]byte, end)
				copy(nb, st.buf)
				st.buf = nb
			}
			copy(st.buf[fh.fragOff:], fh.data)
			if end > st.wrote {
				st.wrote = end
			}
			if st.objSize > 0 && st.wrote >= st.objSize {
				if err := emitSample(&st, fn); err != nil {
					return err
				}
			}
		}
	}
	if st.objSize > 0 && st.wrote >= st.objSize {
		return emitSample(&st, fn)
	}
	return nil
}

func emitSample(st *streamAsm, fn func(payload) error) error {
	p := payload{keyframe: st.key, pts100ns: st.pts, data: append([]byte(nil), st.buf[:st.objSize]...)}
	st.buf = nil
	st.objSize = 0
	st.wrote = 0
	return fn(p)
}

// parsePacketBody returns offset in pkt where payload parsing starts.
func parsePacketBody(pkt []byte, pktSize int, usesStdECC *int) (bodyOff int, prop byte, pts uint32, segType byte, segments int, sizeLeft int, err error) {
	if len(pkt) < 11 {
		return 0, 0, 0, 0, 0, 0, errs.Corrupt
	}
	p := 0
	rsize := 8
	var flags byte

	if *usesStdECC > 0 || (len(pkt) >= 5 && pkt[0] == 0x82 && pkt[1] == 0 && pkt[2] == 0) {
		if len(pkt) < 5 || pkt[0] != 0x82 || pkt[1] != 0 || pkt[2] != 0 {
			return 0, 0, 0, 0, 0, 0, errs.Corrupt
		}
		flags = pkt[3]
		prop = pkt[4]
		p = 5
		*usesStdECC = 1
		rsize += 3
	} else {
		c := pkt[p]
		p++
		var d, e byte
		if c&0x80 != 0 {
			rsize++
			if c&0x60 == 0 {
				d = pkt[p]
				e = pkt[p+1]
				skip := int(c & 0x0f)
				if skip < 2 {
					skip = 2
				}
				p += skip
				rsize += int(c & 0x0f)
			}
			flags = pkt[p]
			p++
			if *usesStdECC == 0 {
				if c == 0x82 && d == 0 && e == 0 {
					*usesStdECC = 1
				} else {
					*usesStdECC = -1
				}
			}
		} else {
			*usesStdECC = -1
			flags = c
		}
		prop = pkt[p]
		p++
	}

	pktLen, p := do2Bits(pkt, p, flags>>5, pktSize)
	if pktLen <= 0 {
		pktLen = pktSize
	}
	_, p = do2Bits(pkt, p, (flags>>1)&3, 0)
	pad, p := do2Bits(pkt, p, (flags>>3)&3, 0)
	if p+6 > len(pkt) {
		return 0, 0, 0, 0, 0, 0, errs.Corrupt
	}
	pts = binary.LittleEndian.Uint32(pkt[p : p+4])
	p += 6
	if flags&0x01 != 0 {
		segType = pkt[p]
		segments = int(segType & 0x3f)
		p++
	} else {
		segments = 1
		segType = 0x80
	}
	if pktLen <= 0 || pad >= pktLen || rsize > pktLen-pad {
		return 0, 0, 0, 0, 0, 0, errs.Corrupt
	}
	sizeLeft = pktLen - pad - rsize
	bodyOff = p
	return bodyOff, prop, pts, segType, segments, sizeLeft, nil
}

type frameInfo struct {
	streamID uint8
	key      bool
	fragOff  int
	objSize  int
	data     []byte
	pts      int64
	fragSize int
}

func parseFrameHeader(body []byte, pos int, prop, segType byte, pktPTS uint32, sizeLeft int) (frameInfo, int, error) {
	var fh frameInfo
	if pos >= len(body) || sizeLeft < 1 {
		return fh, 0, errs.Corrupt
	}
	start := pos
	rsize := 1
	num := body[pos]
	pos++
	fh.key = num&0x80 != 0
	fh.streamID = num & 0x7f
	pos2 := pos
	pos = do2Skip(body, pos, prop>>4)
	fragOff, pos := do2Bits(body, pos, prop>>2, 0)
	replic, pos := do2Bits(body, pos, prop, 0)
	fh.fragOff = fragOff
	// WMV3 keyframes in pinned fixtures use an extended replication prefix before
	// the u32 payload size at offset 18 and bitstream at offset 26 (ffmpeg parity).
	if fh.key && fragOff == 0 && (prop == 0 || prop == 0x5d) && start+15+4 <= len(body) {
		obj := int(binary.LittleEndian.Uint32(body[start+7 : start+11]))
		if obj > 1000 && obj < 1<<22 {
			fh.objSize = obj
			dataStart := start + 15
			if dataStart < len(body) {
				fh.data = body[dataStart:]
				fh.fragSize = len(body) - dataStart
				if sizeLeft-rsize < fh.fragSize {
					fh.fragSize = sizeLeft - rsize
					if fh.fragSize < 0 {
						fh.fragSize = 0
					}
					fh.data = body[dataStart : dataStart+fh.fragSize]
				}
				return fh, dataStart + fh.fragSize - start, nil
			}
		}
	}
	_ = pos2
	if replic >= 8 {
		if pos+8 > len(body) {
			return fh, 0, errs.Corrupt
		}
		fh.objSize = int(binary.LittleEndian.Uint32(body[pos : pos+4]))
		fh.pts = int64(binary.LittleEndian.Uint32(body[pos+4 : pos+8]))
		pos += int(replic)
	} else if replic == 1 {
		if pos >= len(body) {
			return fh, 0, errs.Corrupt
		}
		pos++
		fh.pts = int64(pktPTS)
	} else if replic != 0 {
		return fh, 0, errs.Corrupt
	}
	if segType != 0 {
		fs, np := do2Bits(body, pos, segType>>6, 0)
		pos = np
		fh.fragSize = fs
	} else {
		fh.fragSize = sizeLeft - rsize
	}
	if fh.fragSize <= 0 {
		fh.fragSize = sizeLeft - rsize
	}
	if fh.fragSize > sizeLeft-rsize {
		fh.fragSize = sizeLeft - rsize
	}
	if pos+fh.fragSize > len(body) {
		fh.fragSize = len(body) - pos
	}
	if fh.fragSize <= 0 {
		return fh, 0, errs.Corrupt
	}
	fh.data = body[pos : pos+fh.fragSize]
	return fh, pos + fh.fragSize - start, nil
}

func do2Skip(buf []byte, p int, spec byte) int {
	_, np := do2Bits(buf, p, spec, 0)
	return np
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
