package matroska

import (
	"encoding/binary"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Synchronous, bounded Matroska block demuxer used by the thumbnail and
// subtitle paths. It replaces ebml-go's mkvcore.SimpleBlockReader, whose
// background goroutine could not be stopped and panicked on blocks for
// undeclared track numbers. Everything here runs on the caller's goroutine,
// reads only element headers plus the payloads it returns, and fails with
// errs.Unsup or errs.Corrupt instead of panicking.

// Element IDs for the cluster and index walk.
const (
	idSeekHead          = 0x114D9B74
	idSeek              = 0x4DBB
	idSeekID            = 0x53AB
	idSeekPosition      = 0x53AC
	idCues              = 0x1C53BB6B
	idCuePoint          = 0xBB
	idCueTime           = 0xB3
	idCueTrackPositions = 0xB7
	idCueTrack          = 0xF7
	idCueClusterPos     = 0xF1
	idClusterTimecode   = 0xE7
	idSimpleBlock       = 0xA3
	idBlockGroup        = 0xA0
	idBlock             = 0xA1
	idReferenceBlock    = 0xFB

	// maxCuesElement bounds the Cues read. A 2-hour file with one cue per
	// second is about 150 KB; 8 MiB leaves ample room and still fails fast.
	maxCuesElement = 8 << 20
	// maxBlockPayload bounds a single frame payload.
	maxBlockPayload = 32 << 20
	// maxCollectBytes bounds the payload bytes kept for one seek (the VP8
	// keyframe through the target frame).
	maxCollectBytes = 256 << 20
	// maxBlockHeader is the largest block header read before the payload:
	// track vint (up to 8), timecode (2), flags (1), lace count (1).
	maxBlockHeader = 12
	// maxLaces bounds the lace count of one block (the field is one byte).
	maxLaces = 256
)

// blockInfo is one demuxed block header. payloadOff/payloadLen cover the
// block's frame data, which may be laced.
type blockInfo struct {
	track      uint64
	timecode   int64 // absolute, in timecode ticks
	keyframe   bool
	lacing     byte // 0 none, 1 Xiph, 2 fixed, 3 EBML
	payloadOff int64
	payloadLen int64
}

// segmentIndex is the top-level layout needed to demux one track.
type segmentIndex struct {
	segStart, segEnd int64
	firstCluster     int64 // -1 when no Cluster was found
	cuesOff          int64 // -1 when unknown
	tracks           []trackEntry
}

// indexSegment walks the Segment's top-level children by size until the first
// Cluster, collecting Tracks, the SeekHead Cues pointer, and the first Cluster
// offset. Cluster bodies are never read.
func indexSegment(r io.ReaderAt, size int64) (*segmentIndex, error) {
	segStart, segEnd, found, err := findSegment(r, size)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errs.Unsup("matroska", "no segment")
	}
	ix := &segmentIndex{segStart: segStart, segEnd: segEnd, firstCluster: -1, cuesOff: -1}
	off := segStart
	for off < segEnd {
		id, hdrLen, dataSize, err := readElementHeader(r, off, segEnd)
		if err != nil {
			break
		}
		if id == idCluster {
			ix.firstCluster = off
			break
		}
		if dataSize < 0 {
			break
		}
		end := off + hdrLen + dataSize
		switch id {
		case idTracks:
			if ix.tracks == nil {
				var el tracksElement
				if err := decodeElement(r, off, end, &el); err != nil {
					return nil, err
				}
				ix.tracks = el.Tracks.TrackEntry
				if ix.tracks == nil {
					ix.tracks = []trackEntry{}
				}
			}
		case idSeekHead:
			if ix.cuesOff < 0 && dataSize <= maxHeaderElement {
				buf, err := readBody(r, off+hdrLen, dataSize)
				if err != nil {
					return nil, err
				}
				if pos, ok := parseSeekHeadCues(buf); ok && pos < uint64(segEnd-segStart) {
					ix.cuesOff = segStart + int64(pos)
				}
			}
		case idCues:
			ix.cuesOff = off
		}
		off = end
	}
	if ix.tracks == nil {
		return nil, errs.Unsup("matroska", "no tracks")
	}
	return ix, nil
}

// readBody reads n bytes at off. A short read is an error.
func readBody(r io.ReaderAt, off, n int64) ([]byte, error) {
	buf := make([]byte, n)
	if m, err := r.ReadAt(buf, off); int64(m) != n {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return buf, nil
}

// bufElem parses one element header from b, returning id, header length and
// data size. Unknown sizes and sizes past the buffer are rejected.
func bufElem(b []byte) (id uint32, hdrLen int, size int, ok bool) {
	if len(b) < 2 {
		return 0, 0, 0, false
	}
	idLen := vintLength(b[0])
	if idLen == 0 || idLen > 4 || len(b) < idLen+1 {
		return 0, 0, 0, false
	}
	for _, c := range b[:idLen] {
		id = id<<8 | uint32(c)
	}
	v, n, ok := readVint(b[idLen:])
	if !ok || v < 0 || v > int64(len(b)-idLen-n) {
		return 0, 0, 0, false
	}
	return id, idLen + n, int(v), true
}

// readVint decodes an EBML size/number vint (marker stripped). The all-ones
// value (unknown size) is returned as -1.
func readVint(b []byte) (int64, int, bool) {
	if len(b) == 0 {
		return 0, 0, false
	}
	n := vintLength(b[0])
	if n == 0 || n > 8 || len(b) < n {
		return 0, 0, false
	}
	v := uint64(b[0]) & (0xFF >> n)
	allOnes := v == uint64(0xFF>>n)
	for _, c := range b[1:n] {
		v = v<<8 | uint64(c)
		allOnes = allOnes && c == 0xFF
	}
	if allOnes {
		return -1, n, true
	}
	if v > 1<<62 {
		return 0, 0, false
	}
	return int64(v), n, true
}

// bufUint decodes a big-endian unsigned integer element body (max 8 bytes).
func bufUint(b []byte) (uint64, bool) {
	if len(b) > 8 {
		return 0, false
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, true
}

// parseSeekHeadCues returns the Cues SeekPosition (relative to the Segment
// data start) from a SeekHead body.
func parseSeekHeadCues(b []byte) (uint64, bool) {
	for len(b) > 0 {
		id, h, n, ok := bufElem(b)
		if !ok {
			return 0, false
		}
		body := b[h : h+n]
		b = b[h+n:]
		if id != idSeek {
			continue
		}
		var sid []byte
		var pos uint64
		var havePos bool
		for len(body) > 0 {
			cid, ch, cn, ok := bufElem(body)
			if !ok {
				break
			}
			switch cid {
			case idSeekID:
				sid = body[ch : ch+cn]
			case idSeekPosition:
				pos, havePos = bufUint(body[ch : ch+cn])
			}
			body = body[ch+cn:]
		}
		if havePos && len(sid) == 4 && binary.BigEndian.Uint32(sid) == idCues {
			return pos, true
		}
	}
	return 0, false
}

// pickCueCluster parses a Cues body and returns the cluster position
// (relative to the Segment data start) of the cue for track whose time is the
// latest at or before targetTicks, like ffmpeg's fast -ss. When every cue is
// after the target, the earliest cue is used.
func pickCueCluster(b []byte, track uint64, targetTicks int64) (uint64, bool) {
	var bestPos, firstPos uint64
	bestTime, firstTime := int64(-1), int64(-1)
	prevTime, sorted := int64(-1), true
	for len(b) > 0 {
		id, h, n, ok := bufElem(b)
		if !ok {
			break
		}
		body := b[h : h+n]
		b = b[h+n:]
		if id != idCuePoint {
			continue
		}
		var t uint64
		var haveTime bool
		var pos uint64
		var havePos bool
		for len(body) > 0 {
			cid, ch, cn, ok := bufElem(body)
			if !ok {
				break
			}
			cb := body[ch : ch+cn]
			body = body[ch+cn:]
			switch cid {
			case idCueTime:
				t, haveTime = bufUint(cb)
			case idCueTrackPositions:
				var tr, cp uint64
				var haveTr, haveCp bool
				for len(cb) > 0 {
					gid, gh, gn, ok := bufElem(cb)
					if !ok {
						break
					}
					switch gid {
					case idCueTrack:
						tr, haveTr = bufUint(cb[gh : gh+gn])
					case idCueClusterPos:
						cp, haveCp = bufUint(cb[gh : gh+gn])
					}
					cb = cb[gh+gn:]
				}
				if haveTr && haveCp && tr == track && !havePos {
					pos, havePos = cp, true
				}
			}
		}
		if !haveTime || t > 1<<62 {
			continue
		}
		ti := int64(t)
		if ti < prevTime {
			sorted = false
		}
		prevTime = ti
		if !havePos {
			continue
		}
		// CuePoints are ordered by CueTime (RFC 9559). Once a cue past the
		// target has been seen in a sorted run that already has a candidate,
		// no later cue can be a better pick.
		if sorted && ti > targetTicks && bestTime >= 0 {
			break
		}
		if firstTime < 0 || ti < firstTime {
			firstTime, firstPos = ti, pos
		}
		if ti <= targetTicks && ti > bestTime {
			bestTime, bestPos = ti, pos
		}
	}
	if bestTime >= 0 {
		return bestPos, true
	}
	if firstTime >= 0 {
		return firstPos, true
	}
	return 0, false
}

// isTopLevelID reports whether id is a Segment child, which ends an
// unknown-size Cluster.
func isTopLevelID(id uint32) bool {
	switch id {
	case idCluster, idCues, idSeekHead, idInfo, idTracks,
		0x1941A469, // Attachments
		0x1043A770, // Chapters
		0x1254C367: // Tags
		return true
	}
	return false
}

// clusterWalker iterates the blocks of consecutive Clusters starting at a
// Cluster offset. It holds no goroutines and keeps no per-block state.
type clusterWalker struct {
	r      io.ReaderAt
	segEnd int64
	off    int64 // next element offset
	// cluster state
	inCluster  bool
	clusterEnd int64
	timecode   int64
}

// clusterHeader reads the Cluster header at off and returns its body start,
// body end and Timecode. Only the children before the first block are read.
func clusterHeader(r io.ReaderAt, off, segEnd int64) (bodyStart, end, timecode int64, err error) {
	id, hdrLen, dataSize, err := readElementHeader(r, off, segEnd)
	if err != nil {
		return 0, 0, 0, err
	}
	if id != idCluster {
		return 0, 0, 0, errs.Corrupt
	}
	bodyStart = off + hdrLen
	end = segEnd
	if dataSize >= 0 {
		end = bodyStart + dataSize
	}
	// Timecode is the first child in practice; scan the leading non-block
	// children for it.
	p := bodyStart
	for p < end {
		cid, ch, cn, err := readElementHeader(r, p, end)
		if err != nil || cn < 0 {
			break
		}
		if cid == idClusterTimecode {
			if cn > 8 {
				return 0, 0, 0, errs.Corrupt
			}
			var b [8]byte
			if m, _ := r.ReadAt(b[:cn], p+ch); int64(m) != cn {
				return 0, 0, 0, io.ErrUnexpectedEOF
			}
			v, _ := bufUint(b[:cn])
			if v > 1<<62 {
				return 0, 0, 0, errs.Corrupt
			}
			return bodyStart, end, int64(v), nil
		}
		if cid == idSimpleBlock || cid == idBlockGroup || isTopLevelID(cid) {
			break
		}
		p += ch + cn
	}
	return bodyStart, end, 0, nil
}

// next returns the next block. io.EOF means the walk reached the end of the
// Segment. The caller filters by track number; blocks for tracks that are not
// declared are just skipped by the caller instead of crashing.
func (w *clusterWalker) next() (blockInfo, error) {
	for {
		if w.off >= w.segEnd {
			return blockInfo{}, io.EOF
		}
		if !w.inCluster || w.off >= w.clusterEnd {
			w.inCluster = false
			id, hdrLen, dataSize, err := readElementHeader(w.r, w.off, w.segEnd)
			if err != nil {
				return blockInfo{}, io.EOF
			}
			if id != idCluster {
				if dataSize < 0 {
					return blockInfo{}, io.EOF
				}
				w.off += hdrLen + dataSize
				continue
			}
			w.inCluster = true
			w.timecode = 0
			w.clusterEnd = w.segEnd
			if dataSize >= 0 {
				w.clusterEnd = w.off + hdrLen + dataSize
			}
			w.off += hdrLen
			continue
		}
		id, hdrLen, dataSize, err := readElementHeader(w.r, w.off, w.clusterEnd)
		if err != nil {
			// Truncated tail: end the walk cleanly.
			return blockInfo{}, io.EOF
		}
		if isTopLevelID(id) {
			// Next Segment child inside an unknown-size Cluster.
			w.inCluster = false
			continue
		}
		if dataSize < 0 {
			return blockInfo{}, errs.Corrupt
		}
		elemOff := w.off
		w.off += hdrLen + dataSize
		switch id {
		case idClusterTimecode:
			if dataSize > 8 {
				return blockInfo{}, errs.Corrupt
			}
			var b [8]byte
			if m, _ := w.r.ReadAt(b[:dataSize], elemOff+hdrLen); int64(m) != dataSize {
				return blockInfo{}, io.ErrUnexpectedEOF
			}
			v, _ := bufUint(b[:dataSize])
			if v > 1<<62 {
				return blockInfo{}, errs.Corrupt
			}
			w.timecode = int64(v)
		case idSimpleBlock:
			bi, err := readBlockHeader(w.r, elemOff+hdrLen, dataSize, w.timecode, true)
			if err != nil {
				return blockInfo{}, err
			}
			return bi, nil
		case idBlockGroup:
			bi, ok, err := readBlockGroup(w.r, elemOff+hdrLen, dataSize, w.timecode)
			if err != nil {
				return blockInfo{}, err
			}
			if ok {
				return bi, nil
			}
		}
	}
}

// readBlockGroup finds the Block inside a BlockGroup. The frame is a keyframe
// when the group has no ReferenceBlock (Matroska spec).
func readBlockGroup(r io.ReaderAt, off, n, clusterTC int64) (blockInfo, bool, error) {
	end := off + n
	var bi blockInfo
	var have bool
	key := true
	for p := off; p < end; {
		id, h, sz, err := readElementHeader(r, p, end)
		if err != nil || sz < 0 {
			return blockInfo{}, false, errs.Corrupt
		}
		switch id {
		case idBlock:
			b, err := readBlockHeader(r, p+h, sz, clusterTC, false)
			if err != nil {
				return blockInfo{}, false, err
			}
			bi, have = b, true
		case idReferenceBlock:
			key = false
		}
		p += h + sz
	}
	bi.keyframe = key
	return bi, have, nil
}

// readBlockHeader parses the (Simple)Block header at off: track vint, signed
// 16-bit relative timecode, and flags. Only the header bytes are read.
func readBlockHeader(r io.ReaderAt, off, n, clusterTC int64, simple bool) (blockInfo, error) {
	var hb [maxBlockHeader]byte
	want := min(n, int64(len(hb)))
	m, _ := r.ReadAt(hb[:want], off)
	if int64(m) != want {
		return blockInfo{}, io.ErrUnexpectedEOF
	}
	return parseBlockHeader(hb[:m], off, n, clusterTC, simple)
}

// parseBlockHeader decodes a block header from b (the first bytes of a block
// of total length n at off).
func parseBlockHeader(b []byte, off, n, clusterTC int64, simple bool) (blockInfo, error) {
	tr, tl, ok := readVint(b)
	if !ok || tr <= 0 || len(b) < tl+3 {
		return blockInfo{}, errs.Corrupt
	}
	rel := int16(binary.BigEndian.Uint16(b[tl:]))
	flags := b[tl+2]
	hdr := int64(tl + 3)
	if hdr > n {
		return blockInfo{}, errs.Corrupt
	}
	bi := blockInfo{
		track:      uint64(tr),
		timecode:   clusterTC + int64(rel),
		lacing:     (flags >> 1) & 3,
		payloadOff: off + hdr,
		payloadLen: n - hdr,
	}
	if simple {
		bi.keyframe = flags&0x80 != 0
	}
	return bi, nil
}

// readFrames reads the block payload and splits it into frames per its
// lacing. Total payload size is capped by maxBlockPayload.
func readFrames(r io.ReaderAt, bi blockInfo) ([][]byte, error) {
	if bi.payloadLen <= 0 {
		return nil, nil
	}
	if bi.payloadLen > maxBlockPayload {
		return nil, errs.Unsup("matroska", "block too large")
	}
	buf, err := readBody(r, bi.payloadOff, bi.payloadLen)
	if err != nil {
		return nil, err
	}
	return splitLaces(buf, bi.lacing)
}

// splitLaces splits a block payload into frames. Lacing values follow the
// Matroska spec: 0 none, 1 Xiph, 2 fixed-size, 3 EBML. The returned slices
// alias buf.
func splitLaces(buf []byte, lacing byte) ([][]byte, error) {
	if lacing == 0 {
		return [][]byte{buf}, nil
	}
	if len(buf) < 1 {
		return nil, errs.Corrupt
	}
	count := int(buf[0]) + 1
	p := 1
	sizes := make([]int, count)
	switch lacing {
	case 1: // Xiph
		total := 0
		for i := 0; i < count-1; i++ {
			s := 0
			for {
				if p >= len(buf) {
					return nil, errs.Corrupt
				}
				c := buf[p]
				p++
				s += int(c)
				if c != 0xFF {
					break
				}
			}
			sizes[i] = s
			total += s
			if total > len(buf) {
				return nil, errs.Corrupt
			}
		}
		last := len(buf) - p - total
		if last < 0 {
			return nil, errs.Corrupt
		}
		sizes[count-1] = last
	case 2: // fixed
		rem := len(buf) - p
		if rem%count != 0 {
			return nil, errs.Corrupt
		}
		for i := range sizes {
			sizes[i] = rem / count
		}
	case 3: // EBML
		first, n, ok := readVint(buf[p:])
		if !ok || first < 0 || first > int64(len(buf)) {
			return nil, errs.Corrupt
		}
		p += n
		sizes[0] = int(first)
		total := int64(first)
		prev := first
		for i := 1; i < count-1; i++ {
			raw, n, ok := readVint(buf[p:])
			if !ok || raw < 0 {
				return nil, errs.Corrupt
			}
			p += n
			// Signed difference: subtract the bias 2^(7n-1)-1.
			bias := int64(1)<<(7*uint(n)-1) - 1
			cur := prev + (raw - bias)
			if cur < 0 || cur > int64(len(buf)) {
				return nil, errs.Corrupt
			}
			sizes[i] = int(cur)
			total += cur
			if total > int64(len(buf)) {
				return nil, errs.Corrupt
			}
			prev = cur
		}
		if count > 1 {
			last := int64(len(buf)-p) - total
			if last < 0 {
				return nil, errs.Corrupt
			}
			sizes[count-1] = int(last)
		}
	default:
		return nil, errs.Corrupt
	}
	out := make([][]byte, 0, count)
	for _, s := range sizes {
		if s > len(buf)-p {
			return nil, errs.Corrupt
		}
		out = append(out, buf[p:p+s:p+s])
		p += s
	}
	return out, nil
}

// videoTrackNumber returns the track number of the first video track whose
// CodecID is in codecs, and that CodecID.
func videoTrackNumber(tracks []trackEntry, codecs ...string) (uint64, string, bool) {
	for _, c := range codecs {
		for i, t := range tracks {
			if t.TrackType != 1 || t.CodecID != c {
				continue
			}
			n := t.TrackNumber
			if n == 0 {
				n = uint64(i + 1)
			}
			return n, c, true
		}
	}
	return 0, "", false
}

// seekStartCluster returns the Cluster offset to start demuxing track from for
// targetTicks. With Cues it is the cue at or before the target. Without Cues
// it walks Cluster headers by size (reading only each Timecode) and returns
// the last Cluster whose Timecode is at or before the target.
//
// When needKey is set (VP8), the fallback only accepts Clusters whose first
// block for track is a keyframe, so decoding never starts on a delta frame.
func seekStartCluster(r io.ReaderAt, ix *segmentIndex, track uint64, targetTicks int64, needKey bool) (int64, error) {
	if ix.firstCluster < 0 {
		return 0, errs.Unsup("matroska", "no clusters")
	}
	if ix.cuesOff >= 0 {
		if pos, ok := cuesClusterPos(r, ix, track, targetTicks); ok {
			off := ix.segStart + int64(pos)
			if off < ix.segEnd {
				if id, _, _, err := readElementHeader(r, off, ix.segEnd); err == nil && id == idCluster {
					return off, nil
				}
			}
		}
	}
	best := ix.firstCluster
	off := ix.firstCluster
	for off < ix.segEnd {
		id, hdrLen, dataSize, err := readElementHeader(r, off, ix.segEnd)
		if err != nil {
			break
		}
		if id != idCluster {
			if dataSize < 0 {
				break
			}
			off += hdrLen + dataSize
			continue
		}
		if dataSize < 0 {
			// Unknown-size clusters cannot be skipped by size; demux from
			// the last known cluster instead.
			break
		}
		_, end, tc, err := clusterHeader(r, off, ix.segEnd)
		if err != nil {
			break
		}
		if tc > targetTicks {
			break
		}
		if !needKey || firstBlockIsKey(r, off, end, track) {
			best = off
		}
		off = end
	}
	return best, nil
}

// firstBlockIsKey reports whether the first block for track in the Cluster at
// [off, end) is a keyframe. Only block headers are read.
func firstBlockIsKey(r io.ReaderAt, off, end int64, track uint64) bool {
	w := clusterWalker{r: r, segEnd: end, off: off}
	for {
		bi, err := w.next()
		if err != nil {
			return false
		}
		if bi.track == track {
			return bi.keyframe
		}
	}
}

// cuesClusterPos reads the Cues element and picks the cue for track.
func cuesClusterPos(r io.ReaderAt, ix *segmentIndex, track uint64, targetTicks int64) (uint64, bool) {
	id, hdrLen, dataSize, err := readElementHeader(r, ix.cuesOff, ix.segEnd)
	if err != nil || id != idCues || dataSize <= 0 || dataSize > maxCuesElement {
		return 0, false
	}
	buf, err := readBody(r, ix.cuesOff+hdrLen, dataSize)
	if err != nil {
		return 0, false
	}
	return pickCueCluster(buf, track, targetTicks)
}

// msToTicks converts milliseconds to timecode ticks for the given scale.
func msToTicks(ms int64, timecodeScale uint64) int64 {
	if timecodeScale == 0 {
		timecodeScale = 1_000_000
	}
	return ms * 1_000_000 / int64(timecodeScale)
}

// collectSeekFrames demuxes the frames of track needed to decode near
// targetMs, starting at the cue (or cluster) at or before the target. For
// VP8, frames from the last keyframe at or before target through the first
// frame at or after target are kept. For MJPEG, only the latest frame at or
// before target is kept (or the first frame when all are later).
func collectSeekFrames(r io.ReaderAt, ix *segmentIndex, track uint64, scale uint64, targetMs int64, vp8 bool) ([]videoFrame, error) {
	start, err := seekStartCluster(r, ix, track, msToTicks(targetMs, scale), vp8)
	if err != nil {
		return nil, err
	}
	w := clusterWalker{r: r, segEnd: ix.segEnd, off: start}
	var out []videoFrame
	var mjpegPick, mjpegFirst *videoFrame
	var kept int64
	for {
		bi, err := w.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if bi.track != track || bi.payloadLen <= 0 {
			continue
		}
		tsMs := timecodeTicksToMs(bi.timecode, scale)
		if !vp8 && tsMs > targetMs && mjpegPick != nil {
			break
		}
		frames, err := readFrames(r, bi)
		if err != nil {
			return nil, err
		}
		done := false
		for _, fb := range frames {
			if len(fb) == 0 {
				continue
			}
			f := videoFrame{timestampMs: tsMs, keyframe: bi.keyframe, payload: fb}
			if vp8 {
				if f.keyframe && tsMs <= targetMs {
					out = out[:0]
					kept = 0
				}
				kept += int64(len(fb))
				if kept > maxCollectBytes {
					return nil, errs.Unsup("matroska", "seek window too large")
				}
				out = append(out, f)
				if tsMs >= targetMs {
					done = true
					break
				}
				continue
			}
			if mjpegFirst == nil {
				ff := f
				mjpegFirst = &ff
			}
			if tsMs <= targetMs {
				ff := f
				mjpegPick = &ff
			}
		}
		if done {
			break
		}
	}
	if vp8 {
		return out, nil
	}
	if mjpegPick != nil {
		return []videoFrame{*mjpegPick}, nil
	}
	if mjpegFirst != nil {
		return []videoFrame{*mjpegFirst}, nil
	}
	return nil, nil
}
