package avi

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

const (
	aviIFKeyframe = 0x00000010
)

// mainHeader is the avih chunk body.
type mainHeader struct {
	microSecPerFrame uint32
	maxBytesPerSec   uint32
	paddingGranularity uint32
	flags            uint32
	totalFrames      uint32
	initialFrames    uint32
	streams          uint32
	suggestedBufferSize uint32
	width            uint32
	height           uint32
}

func parseMainHeader(b []byte) (mainHeader, error) {
	if len(b) < 56 {
		return mainHeader{}, errs.Corrupt
	}
	return mainHeader{
		microSecPerFrame:    binary.LittleEndian.Uint32(b[0:4]),
		maxBytesPerSec:      binary.LittleEndian.Uint32(b[4:8]),
		paddingGranularity:  binary.LittleEndian.Uint32(b[8:12]),
		flags:               binary.LittleEndian.Uint32(b[12:16]),
		totalFrames:         binary.LittleEndian.Uint32(b[16:20]),
		initialFrames:       binary.LittleEndian.Uint32(b[20:24]),
		streams:             binary.LittleEndian.Uint32(b[24:28]),
		suggestedBufferSize: binary.LittleEndian.Uint32(b[28:32]),
		width:               binary.LittleEndian.Uint32(b[32:36]),
		height:              binary.LittleEndian.Uint32(b[36:40]),
	}, nil
}

// streamInfo describes one AVI stream from strh/strf.
type streamInfo struct {
	index   int
	typ     detect.StreamType
	handler [4]byte // strh fccHandler (video fourcc)
	scale   uint32
	rate    uint32
	length  uint32
	strf    []byte
	width   int
	height  int
	codec   string
}

// fileIndex holds parsed AVI metadata and sample index for one file.
type fileIndex struct {
	r          io.ReaderAt
	size       int64
	avih       mainHeader
	streams    []streamInfo
	video      int // stream index in streams slice, -1 if none
	moviOff    int64 // offset of the 'movi' list type fourcc
	index      []sampleIndex
	totalFrames uint32
}

type sampleIndex struct {
	stream    int
	keyframe  bool
	chunkOff  int64 // absolute offset of chunk fourcc
	chunkSize uint32
}

// openIndex parses hdrl, locates movi, and loads idx1 / OpenDML indexes.
func openIndex(r io.ReaderAt, size int64) (*fileIndex, error) {
	if size < 12 {
		return nil, errs.Corrupt
	}
	hdr, err := readChunkHeader(r, size, 0)
	if err != nil {
		return nil, err
	}
	if string(hdr.id[:]) != "RIFF" || hdr.sz+8 > uint32(size) && uint64(hdr.sz)+8 != uint64(size) {
		// RIFF size may be file size - 8; allow either.
	}
	var form [4]byte
	if _, err := r.ReadAt(form[:], 8); err != nil {
		return nil, err
	}
	if string(form[:]) != "AVI " && string(form[:]) != "AVIX" {
		return nil, errs.NotMedia
	}
	f := &fileIndex{r: r, size: size, video: -1}
	if err := f.parseRIFFChildren(12, size); err != nil {
		return nil, err
	}
	if f.moviOff == 0 {
		return nil, errs.Unsup("avi", "no movi list")
	}
	if len(f.index) == 0 {
		if err := f.scanMovi(); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (f *fileIndex) parseRIFFChildren(start, end int64) error {
	for off := start; off+8 <= end && off+8 <= f.size; {
		h, err := readChunkHeader(f.r, f.size, off)
		if err != nil {
			return err
		}
		id := chunkIDString(h.id)
		dataEnd := chunkDataEnd(h)
		switch id {
		case "LIST":
			lt, err := listType(f.r, h)
			if err != nil {
				return err
			}
			listEnd := h.off + 8 + int64(h.sz)
			switch string(lt[:]) {
			case "hdrl":
				if err := f.parseHdrl(h.off + 12, listEnd); err != nil {
					return err
				}
			case "movi":
				f.moviOff = h.off + 8
				if f.totalFrames == 0 {
					f.totalFrames = f.avih.totalFrames
				}
			}
		case "avih":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return err
			}
			f.avih, err = parseMainHeader(body)
			if err != nil {
				return err
			}
			f.totalFrames = f.avih.totalFrames
		case "idx1":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return err
			}
			f.parseIdx1(body)
		case "indx":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return err
			}
			_ = f.parseSuperIndex(body, h.off)
		}
		off = dataEnd
	}
	return nil
}

func (f *fileIndex) parseHdrl(start, end int64) error {
	for off := start; off+8 <= end && off+8 <= f.size; {
		h, err := readChunkHeader(f.r, f.size, off)
		if err != nil {
			return err
		}
		id := chunkIDString(h.id)
		switch id {
		case "LIST":
			lt, err := listType(f.r, h)
			if err != nil {
				return err
			}
			if string(lt[:]) == "strl" {
				si, err := f.parseStreamList(h.off+12, h.off+8+int64(h.sz))
				if err != nil {
					return err
				}
				si.index = len(f.streams)
				f.streams = append(f.streams, si)
				if si.typ == detect.StreamVideo && f.video < 0 {
					f.video = si.index
				}
			}
			if string(lt[:]) == "odml" {
				if err := f.parseOdml(h.off+12, h.off+8+int64(h.sz)); err != nil {
					return err
				}
			}
		case "avih":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return err
			}
			f.avih, err = parseMainHeader(body)
			if err != nil {
				return err
			}
		}
		off = chunkDataEnd(h)
	}
	return nil
}

func (f *fileIndex) parseOdml(start, end int64) error {
	for off := start; off+8 <= end && off+8 <= f.size; {
		h, err := readChunkHeader(f.r, f.size, off)
		if err != nil {
			return err
		}
		switch chunkIDString(h.id) {
		case "indx":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return err
			}
			_ = f.parseSuperIndex(body, h.off)
		default:
			if h.id[0] == 'i' && h.id[1] == 'x' {
				body, err := readAt(f.r, h.off+8, int(h.sz))
				if err != nil {
					return err
				}
				f.parseStandardIndex(body)
			}
		}
		off = chunkDataEnd(h)
	}
	return nil
}

func (f *fileIndex) parseStreamList(start, end int64) (streamInfo, error) {
	var si streamInfo
	for off := start; off+8 <= end && off+8 <= f.size; {
		h, err := readChunkHeader(f.r, f.size, off)
		if err != nil {
			return si, err
		}
		switch chunkIDString(h.id) {
		case "strh":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return si, err
			}
			if err := parseStreamHeader(body, &si); err != nil {
				return si, err
			}
		case "strf":
			body, err := readAt(f.r, h.off+8, int(h.sz))
			if err != nil {
				return si, err
			}
			si.strf = append([]byte(nil), body...)
			parseStreamFormat(body, &si)
		}
		off = chunkDataEnd(h)
	}
	return si, nil
}

func parseStreamHeader(b []byte, si *streamInfo) error {
	if len(b) < 56 {
		return errs.Corrupt
	}
	fccType := b[0:4]
	copy(si.handler[:], b[4:8])
	si.scale = binary.LittleEndian.Uint32(b[20:24])
	si.rate = binary.LittleEndian.Uint32(b[24:28])
	si.length = binary.LittleEndian.Uint32(b[32:36])
	switch string(fccType) {
	case "vids":
		si.typ = detect.StreamVideo
	case "auds":
		si.typ = detect.StreamAudio
	default:
		si.typ = detect.StreamVideo
	}
	return nil
}

func parseStreamFormat(b []byte, si *streamInfo) {
	if si.typ == detect.StreamVideo && len(b) >= 40 {
		si.width = int(int32(binary.LittleEndian.Uint32(b[4:8])))
		si.height = int(int32(binary.LittleEndian.Uint32(b[8:12])))
		fourCC := string(b[16:20])
		_ = fourCC
		si.codec = detect.VFWCodecName(b)
		return
	}
	if si.typ == detect.StreamAudio && len(b) >= 16 {
		tag := binary.LittleEndian.Uint16(b[0:2])
		ch := binary.LittleEndian.Uint16(b[2:4])
		rate := binary.LittleEndian.Uint32(b[4:8])
		si.codec = audioCodecName(tag)
		_ = ch
		_ = rate
	}
}

func audioCodecName(wFormatTag uint16) string {
	switch wFormatTag {
	case 1:
		return "pcm_s16le"
	case 0x0055:
		return "mp3"
	case 0xFFFE:
		return "pcm_s16le"
	default:
		return "unknown"
	}
}

func streamIndexFromChunkID(id [4]byte) (int, bool) {
	if id[0] < '0' || id[0] > '9' || id[1] < '0' || id[1] > '9' {
		return 0, false
	}
	return int(id[0]-'0')*10 + int(id[1]-'0'), true
}

func (f *fileIndex) parseIdx1(body []byte) {
	if f.moviOff == 0 {
		return
	}
	for i := 0; i+16 <= len(body); i += 16 {
		var cid [4]byte
		copy(cid[:], body[i:i+4])
		flags := binary.LittleEndian.Uint32(body[i+4 : i+8])
		rel := binary.LittleEndian.Uint32(body[i+8 : i+12])
		sz := binary.LittleEndian.Uint32(body[i+12 : i+16])
		st, ok := streamIndexFromChunkID(cid)
		if !ok {
			continue
		}
		f.index = append(f.index, sampleIndex{
			stream:    st,
			keyframe:  flags&aviIFKeyframe != 0,
			chunkOff:  f.moviOff + int64(rel),
			chunkSize: sz,
		})
	}
}

func (f *fileIndex) scanMovi() error {
	if f.moviOff == 0 {
		return errs.Unsup("avi", "no index and no movi")
	}
	listHdr, err := readChunkHeader(f.r, f.size, f.moviOff-8)
	if err != nil {
		return err
	}
	off := f.moviOff + 4
	listEnd := listHdr.off + 8 + int64(listHdr.sz)
	for off+8 <= listEnd && off+8 <= f.size {
		h, err := readChunkHeader(f.r, f.size, off)
		if err != nil {
			break
		}
		st, ok := streamIndexFromChunkID(h.id)
		if ok {
			f.index = append(f.index, sampleIndex{
				stream:    st,
				keyframe:  true, // without flags assume keyframe for seek fallback
				chunkOff:  off,
				chunkSize: h.sz,
			})
		}
		off = chunkDataEnd(h)
	}
	return nil
}

func (f *fileIndex) videoStream() *streamInfo {
	if f.video < 0 || f.video >= len(f.streams) {
		return nil
	}
	return &f.streams[f.video]
}

func (f *fileIndex) duration() (detect.DurationResult, error) {
	if f.avih.microSecPerFrame > 0 && f.avih.totalFrames > 0 {
		us := uint64(f.avih.totalFrames) * uint64(f.avih.microSecPerFrame)
		return detect.DurationResult{
			Duration: time.Duration(us) * time.Microsecond,
			Source:   "header",
		}, nil
	}
	vs := f.videoStream()
	if vs != nil && vs.rate > 0 && vs.length > 0 {
		us := uint64(vs.length) * uint64(vs.scale) * 1_000_000 / uint64(vs.rate)
		return detect.DurationResult{
			Duration: time.Duration(us) * time.Microsecond,
			Source:   "header",
		}, nil
	}
	return detect.DurationResult{}, errs.Unsup("duration", "avi missing avih/strh timing")
}

func (f *fileIndex) readChunkPayload(ent sampleIndex) ([]byte, error) {
	if ent.chunkOff+8+int64(ent.chunkSize) > f.size {
		return nil, errs.Corrupt
	}
	return readAt(f.r, ent.chunkOff+8, int(ent.chunkSize))
}

// pickKeyframe returns the video keyframe at or before targetFrame (0-based).
func (f *fileIndex) pickKeyframe(targetFrame uint32) (sampleIndex, bool) {
	vs := f.video
	if vs < 0 {
		return sampleIndex{}, false
	}
	var (
		pick    sampleIndex
		hasPick bool
		fn      uint32
	)
	for _, ent := range f.index {
		if ent.stream != vs {
			continue
		}
		if ent.keyframe && fn <= targetFrame {
			pick = ent
			hasPick = true
		}
		fn++
	}
	return pick, hasPick
}
