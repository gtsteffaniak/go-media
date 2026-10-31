package asf

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

type fileMeta struct {
	playDuration time.Duration
	preroll      time.Duration
	dataOffset   int64
	dataEnd      int64
	minPkt       uint32
	maxPkt       uint32
	broadcast    bool
	video        *videoStream
	streams      map[uint8]*streamEntry
}

type streamEntry struct {
	id        uint8
	typ       detect.StreamType
	codec     string
	width     int
	height    int
	extradata []byte
}

type videoStream struct {
	id        uint8
	codec     string
	width     int
	height    int
	extradata []byte
}

func openMeta(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (*fileMeta, error) {
	if size < 30 {
		return nil, errs.NotMedia
	}
	rd := newReader(r, size)
	hdr, err := rd.readAt(0, 30)
	if err != nil || len(hdr) < 30 {
		return nil, errs.NotMedia
	}
	g, _ := readGUID(hdr)
	if !guidEqual(g, guidHeader) {
		return nil, errs.NotMedia
	}
	headerSize := int64(leU64(hdr[16:24]))
	if headerSize < 30 || headerSize > size {
		return nil, errs.Corrupt
	}
	if headerSize > maxHeaderScan {
		headerSize = maxHeaderScan
	}
	header, err := rd.readAt(0, int(headerSize))
	if err != nil {
		return nil, err
	}
	meta := &fileMeta{streams: make(map[uint8]*streamEntry)}
	if err := parseHeaderChildren(meta, header[30:]); err != nil {
		return nil, err
	}
	// Data object follows the header object.
	dataOff, dataEnd, err := locateDataObject(rd, headerSize, size)
	if err != nil {
		return nil, err
	}
	meta.dataOffset = dataOff
	meta.dataEnd = dataEnd
	if meta.video == nil {
		for _, s := range meta.streams {
			if s.typ == detect.StreamVideo {
				meta.video = &videoStream{
					id: s.id, codec: s.codec, width: s.width, height: s.height, extradata: s.extradata,
				}
				break
			}
		}
	}
	if meta.video == nil {
		return nil, errs.Unsup("asf", "no video stream")
	}
	return meta, nil
}

func parseHeaderChildren(meta *fileMeta, b []byte) error {
	for off := 0; off+24 <= len(b); {
		sub, _ := readGUID(b[off:])
		subSize := int64(leU64(b[off+16 : off+24]))
		if subSize < 24 || off+int(subSize) > len(b) {
			break
		}
		body := b[off+24 : off+int(subSize)]
		switch {
		case guidEqual(sub, guidFileProps):
			if err := parseFileProps(meta, body); err != nil {
				return err
			}
		case guidEqual(sub, guidStreamProps):
			se, err := parseStreamProps(body)
			if err != nil {
				return err
			}
			if se != nil {
				meta.streams[se.id] = se
				if se.typ == detect.StreamVideo && meta.video == nil {
					meta.video = &videoStream{
						id: se.id, codec: se.codec, width: se.width, height: se.height, extradata: se.extradata,
					}
				}
			}
		case guidEqual(sub, guidHeaderExt):
			if err := parseHeaderChildren(meta, body[16:]); err != nil {
				return err
			}
		}
		off += int(subSize)
	}
	return nil
}

func parseFileProps(meta *fileMeta, b []byte) error {
	if len(b) < 76 {
		return errs.Corrupt
	}
	play := leU64(b[40:48])
	prerollMs := leU64(b[56:64])
	flags := leU32(b[64:68])
	meta.minPkt = leU32(b[68:72])
	meta.maxPkt = leU32(b[72:76])
	meta.broadcast = flags&0x01 != 0
	meta.playDuration = time.Duration(play) * 100 * time.Nanosecond
	meta.preroll = time.Duration(prerollMs) * time.Millisecond
	return nil
}

func parseStreamProps(b []byte) (*streamEntry, error) {
	if len(b) < 54 {
		return nil, errs.Corrupt
	}
	streamType, _ := readGUID(b[0:16])
	var typ detect.StreamType
	switch {
	case guidEqual(streamType, guidVideoStream):
		typ = detect.StreamVideo
	case guidEqual(streamType, guidAudioStream):
		typ = detect.StreamAudio
	default:
		return nil, nil
	}
	off := 40
	if off+14 > len(b) {
		return nil, errs.Corrupt
	}
	typeSpecificSize := int(leU32(b[off : off+4]))
	streamID := uint8(leU16(b[off+8:]) & 0x7f)
	specStart := off + 14
	if specStart+typeSpecificSize > len(b) {
		return nil, errs.Corrupt
	}
	spec := b[specStart : specStart+typeSpecificSize]
	se := &streamEntry{id: streamID, typ: typ}
	if typ == detect.StreamVideo {
		codec, w, h, extra, err := parseVideoFormat(spec)
		if err != nil {
			return nil, err
		}
		se.codec = codec
		se.width = w
		se.height = h
		se.extradata = extra
	} else {
		se.codec = "wmav2" // probe only; audio decode out of scope
	}
	return se, nil
}

func parseVideoFormat(spec []byte) (codec string, width, height int, extra []byte, err error) {
	// ASF video type-specific data (ASF spec 9.2): encoded width u32, encoded
	// height u32, reserved u8, format data size u16, then a BITMAPINFOHEADER
	// starting at byte 11. Codec-specific data follows the 40-byte header.
	const bih = 11
	if len(spec) < bih+40 {
		return "", 0, 0, nil, errs.Corrupt
	}
	width = int(int32(leU32(spec[bih+4:])))
	height = int(int32(leU32(spec[bih+8:])))
	biSize := int(leU32(spec[bih:]))
	if biSize > 40 {
		end := bih + biSize
		if end > len(spec) {
			end = len(spec)
		}
		extra = append([]byte(nil), spec[bih+40:end]...)
	}
	if idx := bytes.Index(spec, []byte("WMV3")); idx >= 0 {
		codec = "wmv3"
	} else if idx := bytes.Index(spec, []byte("WMV2")); idx >= 0 {
		codec = "wmv2"
	} else if idx := bytes.Index(spec, []byte("WMV1")); idx >= 0 {
		codec = "wmv1"
	}
	if codec == "" {
		tag := binary.LittleEndian.Uint32(spec[bih+16:])
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], tag)
		codec = detectVideoFourCC(string(b[:]))
	}
	if codec == "" || width <= 0 || height <= 0 {
		return "", 0, 0, nil, errs.Corrupt
	}
	return codec, width, height, extra, nil
}

func detectVideoFourCC(tag string) string {
	switch tag {
	case "WMV3", "wmv3":
		return "wmv3"
	case "WMV2", "wmv2":
		return "wmv2"
	case "WMV1", "wmv1":
		return "wmv1"
	case "MSS2":
		return "mss2"
	default:
		return ""
	}
}

func locateDataObject(rd reader, after int64, size int64) (start int64, end int64, err error) {
	off := after
	for off+24 <= size {
		buf, err := rd.readAt(off, 24)
		if err != nil {
			return 0, 0, err
		}
		g, _ := readGUID(buf)
		objSize := int64(leU64(buf[16:24]))
		if objSize < 24 {
			return 0, 0, errs.Corrupt
		}
		if guidEqual(g, guidData) {
			end := off + objSize
			if end > size {
				end = size
			}
			return off + 50, end, nil
		}
		off += objSize
	}
	return 0, 0, errs.Unsup("asf", "no data object")
}

// Duration reads play duration from the File Properties object.
func Duration(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (detect.DurationResult, error) {
	meta, err := openMeta(r, size)
	if err != nil {
		return detect.DurationResult{}, err
	}
	d := meta.playDuration - meta.preroll
	if d < 0 {
		d = meta.playDuration
	}
	if d <= 0 {
		return detect.DurationResult{Source: "unknown"}, nil
	}
	return detect.DurationResult{Duration: d, Source: "header"}, nil
}

// Streams returns elementary stream descriptors.
func Streams(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) ([]detect.Stream, error) {
	meta, err := openMeta(r, size)
	if err != nil {
		return nil, err
	}
	var out []detect.Stream
	for id, s := range meta.streams {
		st := detect.Stream{
			Index:  int(id),
			Type:   s.typ,
			Codec:  s.codec,
			Width:  s.width,
			Height: s.height,
		}
		out = append(out, st)
	}
	if len(out) == 0 && meta.video != nil {
		out = append(out, detect.Stream{
			Index:  int(meta.video.id),
			Type:   detect.StreamVideo,
			Codec:  meta.video.codec,
			Width:  meta.video.width,
			Height: meta.video.height,
		})
	}
	return out, nil
}
