package isobmff

import (
	"encoding/binary"
	"io"
	"sort"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Keyframe timeline for HLS segment planning. VideoKeyframeTimes reproduces
// the packet timestamps ffmpeg's mov demuxer gives the key packets of the
// first video track (what "ffprobe -select_streams v:0 -show_entries
// packet=pts_time,flags" lists), from the sample tables alone:
//
//   - keyframes are stss sync samples plus stps partial sync samples plus
//     samples in 'rap ' sample groups (sbgp); with no stss box every sample
//     is a keyframe, and an empty stss with neither stps nor 'rap ' groups
//     marks only the first sample (ffmpeg mov_build_index);
//   - pts is dts + ctts, with ctts read as signed (ffmpeg reads it that way
//     for both box versions);
//   - an edit list made of leading empty edits and one media edit is applied
//     the way ffmpeg's mov_fix_index applies it: decoding restarts at the
//     keyframe at or before media_time, timestamps are rebased on the first
//     frame inside the edit plus the empty-edit offset, the index ends one
//     keyframe (two with ctts) past the edit, and video is shifted so the
//     earliest presented frame starts at the empty-edit offset.
//
// Other edit lists (several media edits, an edit of zero length, edits with
// only empty entries) and fragmented files return errs.Unsup so the caller can
// ask ffprobe instead.

const (
	// maxElstBytes caps the edts payload read for the edit list.
	maxElstBytes = 1 << 20
	// tableChunk is the read size used to stream large per-sample tables
	// (stss, stps, ctts) without loading them whole.
	tableChunk = 32 << 10
)

// boxRange is a table payload [off, end) and its entry count.
type boxRange struct {
	off, end int64
	count    uint32
	ok       bool
}

// edit is one elst entry: duration in the movie timescale, media time in the
// track timescale (-1 for an empty edit).
type edit struct {
	duration  uint64
	mediaTime int64
}

// kfTrack is the subset of one video trak needed to rebuild its timeline.
type kfTrack struct {
	timescale   uint32
	sampleCount uint32
	haveCount   bool
	stts        []uint32 // (count, delta) pairs
	ctts        boxRange // 8-byte entries after the full-box header
	stss        boxRange // 4-byte entries
	stps        boxRange // 4-byte entries
	rap         boxRange // sbgp 'rap ' entries: 8-byte (sample_count, group_description_index)
	edits       []edit
	hasElst     bool
}

// VideoKeyframeTimes returns the presentation times in seconds of the first
// video track's keyframes, ascending, without duplicates and without negative
// times (keyframes an edit list starts before). windowSec > 0 keeps only
// keyframes within windowSec of the first one.
func VideoKeyframeTimes(r io.ReaderAt, size int64, windowSec float64) ([]float64, error) {
	tr, movieScale, fragmented, err := findKeyframeTrack(r, size)
	if err != nil {
		return nil, err
	}
	if tr == nil {
		return nil, errs.Unsup("keyframe times", "no video track")
	}
	if fragmented {
		return nil, errs.Unsup("keyframe times", "fragmented mp4")
	}
	if tr.timescale == 0 {
		return nil, errs.Unsup("keyframe times", "zero track timescale")
	}
	if !tr.haveCount || tr.sampleCount == 0 {
		return nil, errs.Unsup("keyframe times", "missing sample count")
	}
	if tr.sampleCount > maxSamples {
		return nil, errs.Unsup("keyframe times", "sample count exceeds limit")
	}
	if len(tr.stts) == 0 {
		return nil, errs.Unsup("keyframe times", "missing stts")
	}
	ticks, err := keyframeTicks(r, tr, movieScale)
	if err != nil {
		return nil, err
	}
	return ticksToSeconds(ticks, tr.timescale, windowSec), nil
}

// ticksToSeconds converts, filters, sorts and de-duplicates pts ticks.
func ticksToSeconds(ticks []int64, scale uint32, windowSec float64) []float64 {
	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	out := make([]float64, 0, len(ticks))
	for _, t := range ticks {
		if t < 0 {
			continue
		}
		// Compare after conversion so ticks that differ only below float64
		// precision still give a strictly ascending list.
		v := float64(t) / float64(scale)
		if len(out) > 0 && v <= out[len(out)-1] {
			continue
		}
		out = append(out, v)
	}
	if windowSec > 0 && len(out) > 0 {
		limit := out[0] + windowSec
		n := sort.Search(len(out), func(i int) bool { return out[i] > limit })
		out = out[:n]
	}
	return out
}

// findKeyframeTrack locates moov and returns its first vide trak, the mvhd
// timescale, and whether moov declares fragments (mvex).
func findKeyframeTrack(r io.ReaderAt, size int64) (*kfTrack, uint32, bool, error) {
	var off int64
	var hdr [16]byte
	for i := 0; off+8 <= size; i++ {
		if i >= maxTopLevelBoxes {
			return nil, 0, false, errs.Unsup("keyframe times", "too many top-level boxes")
		}
		typ, hl, bs, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			return nil, 0, false, err
		}
		if typ == [4]byte{'m', 'o', 'o', 'v'} {
			if bs > maxMoovSize {
				return nil, 0, false, errs.Unsup("keyframe times", "moov exceeds 64 MiB")
			}
			return scanMoovForVideo(r, size, off+hl, off+bs)
		}
		off += bs
	}
	return nil, 0, false, errs.Unsup("keyframe times", "no moov box")
}

// scanMoovForVideo walks moov children for mvhd, mvex and the first vide trak.
func scanMoovForVideo(r io.ReaderAt, size, start, end int64) (*kfTrack, uint32, bool, error) {
	var movieScale uint32
	var fragmented bool
	var found *kfTrack
	err := walkBoxes(r, size, start, end, func(typ [4]byte, ps, pe int64) error {
		switch string(typ[:]) {
		case "mvhd":
			b, err := readPayload(r, ps, min(pe, ps+24), 24)
			if err != nil {
				return err
			}
			if len(b) >= 16 && b[0] != 1 {
				movieScale = binary.BigEndian.Uint32(b[12:16])
			} else if len(b) >= 24 && b[0] == 1 {
				movieScale = binary.BigEndian.Uint32(b[20:24])
			}
		case "mvex":
			fragmented = true
		case "trak":
			if found != nil {
				return nil
			}
			tr, isVideo, err := scanTrak(r, size, ps, pe)
			if err != nil {
				// A corrupt non-video trak must not hide the video one,
				// but a corrupt video trak is the caller's answer.
				if isVideo {
					return err
				}
				return nil
			}
			if isVideo {
				found = tr
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, false, err
	}
	return found, movieScale, fragmented, nil
}

// scanTrak reads one trak's handler, edit list and timing tables.
func scanTrak(r io.ReaderAt, size, start, end int64) (*kfTrack, bool, error) {
	tr := &kfTrack{}
	var handler [4]byte
	var visit func(ps, pe int64, depth int) error
	visit = func(ps, pe int64, depth int) error {
		return walkBoxes(r, size, ps, pe, func(typ [4]byte, s, e int64) error {
			switch string(typ[:]) {
			case "mdia", "minf", "stbl", "edts":
				if depth+1 > maxTrakContainerDepth {
					return errs.Unsup("keyframe times", "trak nesting too deep")
				}
				return visit(s, e, depth+1)
			case "hdlr":
				b, err := readPayload(r, s, min(e, s+12), 12)
				if err != nil {
					return err
				}
				// mdia's hdlr comes first; a QuickTime minf hdlr (data
				// handler, e.g. "alis") must not replace it.
				if len(b) >= 12 && handler == [4]byte{} {
					copy(handler[:], b[8:12])
				}
			case "mdhd":
				b, err := readPayload(r, s, e, 64)
				if err != nil {
					return err
				}
				return tr.parseMdhd(b)
			case "elst":
				b, err := readPayload(r, s, e, maxElstBytes)
				if err != nil {
					return err
				}
				tr.parseElst(b)
			case "stts":
				b, err := readPayload(r, s, e, maxRunTableBytes)
				if err != nil {
					return err
				}
				vals, err := parseRunTable(b, 2)
				if err != nil {
					return err
				}
				tr.stts = vals
			case "ctts":
				rg, err := tableRange(r, s, e, 8)
				if err != nil {
					return err
				}
				tr.ctts = rg
			case "stss":
				rg, err := tableRange(r, s, e, 4)
				if err != nil {
					return err
				}
				tr.stss = rg
			case "stps":
				rg, err := tableRange(r, s, e, 4)
				if err != nil {
					return err
				}
				tr.stps = rg
			case "sbgp":
				rg, isRap, err := rapGroupRange(r, s, e)
				if err != nil {
					return err
				}
				if isRap {
					// ffmpeg keeps the last 'rap ' sbgp of a track.
					tr.rap = rg
				}
			case "stsz", "stz2":
				// Both carry sample_count at bytes 8..12 of the payload.
				b, err := readPayload(r, s, min(e, s+12), 12)
				if err != nil {
					return err
				}
				if len(b) < 12 {
					return errs.Unsup("keyframe times", "short stsz")
				}
				tr.sampleCount = binary.BigEndian.Uint32(b[8:12])
				tr.haveCount = true
			}
			return nil
		})
	}
	err := visit(start, end, 0)
	isVideo := handler == [4]byte{'v', 'i', 'd', 'e'}
	if err != nil {
		return nil, isVideo, err
	}
	return tr, isVideo, nil
}

// parseMdhd records the media timescale.
func (tr *kfTrack) parseMdhd(b []byte) error {
	if len(b) < 20 {
		return errs.Unsup("keyframe times", "short mdhd")
	}
	if b[0] == 1 {
		if len(b) < 24 {
			return errs.Unsup("keyframe times", "short mdhd v1")
		}
		tr.timescale = binary.BigEndian.Uint32(b[20:24])
		return nil
	}
	tr.timescale = binary.BigEndian.Uint32(b[12:16])
	return nil
}

// parseElst decodes elst like ffmpeg's mov_read_elst: the entry count is
// taken from the payload size when the two disagree, and an empty list is
// the same as none.
func (tr *kfTrack) parseElst(b []byte) {
	if len(b) < 8 {
		return
	}
	version := b[0]
	entry := 12
	if version == 1 {
		entry = 20
	}
	n := (len(b) - 8) / entry
	if n == 0 {
		return
	}
	edits := make([]edit, 0, n)
	for i := range n {
		p := b[8+i*entry:]
		var e edit
		if version == 1 {
			e.duration = binary.BigEndian.Uint64(p[0:8])
			e.mediaTime = int64(binary.BigEndian.Uint64(p[8:16]))
		} else {
			e.duration = uint64(binary.BigEndian.Uint32(p[0:4]))
			e.mediaTime = int64(int32(binary.BigEndian.Uint32(p[4:8])))
		}
		edits = append(edits, e)
	}
	tr.edits = edits
	tr.hasElst = true
}

// tableRange validates a full-box table with an entry count and fixed-size
// entries and returns where its entries live.
func tableRange(r io.ReaderAt, s, e int64, width int64) (boxRange, error) {
	if e-s < 8 {
		return boxRange{}, errs.Unsup("keyframe times", "short table header")
	}
	var h [8]byte
	if err := readFullAtErr(r, h[:], s, errs.Unsup("keyframe times", "short table header read")); err != nil {
		return boxRange{}, err
	}
	count := binary.BigEndian.Uint32(h[4:8])
	if int64(count)*width > e-s-8 {
		return boxRange{}, errs.Unsup("keyframe times", "table truncated")
	}
	return boxRange{off: s + 8, end: s + 8 + int64(count)*width, count: count, ok: true}, nil
}

// rapGroupRange reads an sbgp header and, when its grouping type is 'rap ',
// returns where its (sample_count, group_description_index) entries live.
// ffmpeg's mov_build_index marks every sample in a run with a non-zero
// description index as a keyframe, on top of stss and stps. Other grouping
// types report isRap false.
func rapGroupRange(r io.ReaderAt, s, e int64) (rg boxRange, isRap bool, err error) {
	b, err := readPayload(r, s, min(e, s+16), 16)
	if err != nil {
		return boxRange{}, false, err
	}
	if len(b) < 8 || string(b[4:8]) != "rap " {
		return boxRange{}, false, nil
	}
	hdr := int64(12)
	if b[0] == 1 {
		hdr = 16 // grouping_type_parameter
	}
	if int64(len(b)) < hdr {
		return boxRange{}, false, errs.Unsup("keyframe times", "short sbgp")
	}
	count := binary.BigEndian.Uint32(b[hdr-4 : hdr])
	if int64(count)*8 > e-s-hdr {
		return boxRange{}, false, errs.Unsup("keyframe times", "sbgp truncated")
	}
	return boxRange{off: s + hdr, end: s + hdr + int64(count)*8, count: count, ok: count > 0}, true, nil
}

// u32Reader streams big-endian uint32 values from a table payload.
type u32Reader struct {
	r        io.ReaderAt
	off, end int64
	buf      []byte
	pos      int
	err      error
}

// newU32Reader returns a streaming reader over rg.
func newU32Reader(r io.ReaderAt, rg boxRange) *u32Reader {
	return &u32Reader{r: r, off: rg.off, end: rg.end}
}

// next returns the next value; ok is false at the end of the table or on a
// read error (reported by err).
func (u *u32Reader) next() (uint32, bool) {
	if u.pos+4 > len(u.buf) {
		if u.off >= u.end || u.err != nil {
			return 0, false
		}
		n := min(int64(tableChunk), u.end-u.off)
		if cap(u.buf) < int(n) {
			u.buf = make([]byte, n)
		}
		u.buf = u.buf[:n]
		if err := readFullAtErr(u.r, u.buf, u.off, errs.Unsup("keyframe times", "short table read")); err != nil {
			u.err = err
			return 0, false
		}
		u.off += n
		u.pos = 0
	}
	v := binary.BigEndian.Uint32(u.buf[u.pos:])
	u.pos += 4
	return v, true
}

// sampleWalker yields per-sample dts, ctts and the keyframe flag in decode
// order, following ffmpeg's mov_build_index rules.
type sampleWalker struct {
	tr *kfTrack
	i  uint32
	// stts
	sttsIdx  int
	sttsLeft uint32
	dts      int64
	// ctts
	ctts      *u32Reader
	cttsLeft  uint32
	cttsDelta int32
	cttsDone  bool
	// sync samples
	stss              *u32Reader
	stssNext          uint32
	stssHave          bool
	stps              *u32Reader
	stpsNext          uint32
	stpsHave          bool
	keyOff            uint32
	allKey, firstOnly bool
	// 'rap ' sample groups: the current run's description index, its length
	// and how many of its samples were used.
	rap                         *u32Reader
	rapHave                     bool
	rapCount, rapIndex, rapUsed uint32
}

// newSampleWalker positions a walker at the first sample of tr.
func newSampleWalker(r io.ReaderAt, tr *kfTrack) (*sampleWalker, error) {
	w := &sampleWalker{tr: tr}
	if tr.ctts.ok {
		w.ctts = newU32Reader(r, tr.ctts)
	}
	switch {
	case !tr.stss.ok:
		w.allKey = true
	case tr.stss.count == 0:
		w.firstOnly = true
	default:
		w.stss = newU32Reader(r, tr.stss)
		w.stssNext, w.stssHave = w.stss.next()
	}
	if tr.stps.ok && tr.stps.count > 0 {
		w.stps = newU32Reader(r, tr.stps)
		w.stpsNext, w.stpsHave = w.stps.next()
	}
	if tr.rap.ok {
		w.rap = newU32Reader(r, tr.rap)
		w.nextRapRun()
	}
	// ffmpeg's key_off: tables are 1-based unless the first entry is 0.
	if (w.stssHave && w.stssNext > 0) || (w.stpsHave && w.stpsNext > 0) {
		w.keyOff = 1
	}
	if len(tr.stts) >= 2 {
		w.sttsLeft = tr.stts[0]
		for w.sttsLeft == 0 && w.sttsIdx+3 < len(tr.stts) {
			w.sttsIdx += 2
			w.sttsLeft = tr.stts[w.sttsIdx]
		}
	}
	if err := w.errs(); err != nil {
		return nil, err
	}
	return w, nil
}

// nextRapRun loads the next 'rap ' sample group run; rapHave is false once
// the table is used up.
func (w *sampleWalker) nextRapRun() {
	c, ok1 := w.rap.next()
	d, ok2 := w.rap.next()
	w.rapHave = ok1 && ok2
	w.rapCount, w.rapIndex, w.rapUsed = c, d, 0
}

// errs returns the first table read error seen by the walker.
func (w *sampleWalker) errs() error {
	for _, u := range []*u32Reader{w.ctts, w.stss, w.stps, w.rap} {
		if u != nil && u.err != nil {
			return u.err
		}
	}
	return nil
}

// sample describes one sample in decode order.
type sample struct {
	index uint32
	dts   int64
	ctts  int64
	dur   int64 // dts delta to the next sample
	key   bool
}

// next returns the next sample; ok is false after the last one.
func (w *sampleWalker) next() (sample, bool) {
	if w.i >= w.tr.sampleCount {
		return sample{}, false
	}
	s := sample{index: w.i, dts: w.dts}

	// ctts runs; entries with count <= 0 (as int32) are skipped like ffmpeg.
	if w.ctts != nil && !w.cttsDone {
		for w.cttsLeft == 0 && !w.cttsDone {
			c, ok1 := w.ctts.next()
			d, ok2 := w.ctts.next()
			if !ok1 || !ok2 {
				w.cttsDone = true
				w.cttsDelta = 0
				break
			}
			if int32(c) <= 0 {
				continue
			}
			w.cttsLeft, w.cttsDelta = c, int32(d)
		}
		if w.cttsLeft > 0 {
			w.cttsLeft--
		}
	}
	s.ctts = int64(w.cttsDelta)

	n := w.i + w.keyOff
	if w.allKey {
		s.key = true
	} else if w.stssHave && n == w.stssNext {
		s.key = true
		if v, ok := w.stss.next(); ok {
			w.stssNext = v
		} // else: keep the last entry, as ffmpeg does
	}
	if !s.key && w.stpsHave && n == w.stpsNext {
		s.key = true
		if v, ok := w.stps.next(); ok {
			w.stpsNext = v
		}
	}
	if w.rapHave {
		// Like ffmpeg, a run with a zero sample_count never ends.
		if w.rapIndex > 0 {
			s.key = true
		}
		w.rapUsed++
		if w.rapUsed == w.rapCount {
			w.nextRapRun()
		}
	}
	if w.firstOnly && !w.stpsHave && w.rap == nil && w.i == 0 {
		// An empty stss (and no stps or 'rap ' groups) marks only the first
		// sample.
		s.key = true
	}

	// stts: samples past the table's total reuse the last delta.
	delta := int64(0)
	if w.sttsIdx+1 < len(w.tr.stts) {
		delta = int64(w.tr.stts[w.sttsIdx+1])
	}
	s.dur = delta
	w.dts += delta
	if w.sttsLeft > 0 {
		w.sttsLeft--
	}
	for w.sttsLeft == 0 && w.sttsIdx+3 < len(w.tr.stts) {
		w.sttsIdx += 2
		w.sttsLeft = w.tr.stts[w.sttsIdx]
	}
	w.i++
	return s, true
}

// keyframeTicks walks the samples and returns keyframe pts in track ticks.
func keyframeTicks(r io.ReaderAt, tr *kfTrack, movieScale uint32) ([]int64, error) {
	if !tr.hasElst {
		return keyframeTicksNoEdit(r, tr)
	}
	var empty int64
	var media *edit
	for i := range tr.edits {
		e := tr.edits[i]
		if media == nil && e.mediaTime == -1 {
			if movieScale == 0 {
				return nil, errs.Unsup("keyframe times", "edit list with zero movie timescale")
			}
			empty += rescale(e.duration, tr.timescale, movieScale)
			continue
		}
		if media != nil {
			return nil, errs.Unsup("keyframe times", "multiple media edits")
		}
		media = &tr.edits[i]
	}
	if media == nil {
		return nil, errs.Unsup("keyframe times", "edit list has no media edit")
	}
	if movieScale == 0 {
		return nil, errs.Unsup("keyframe times", "edit list with zero movie timescale")
	}
	if media.mediaTime < 0 {
		return nil, errs.Unsup("keyframe times", "negative edit media time")
	}
	m := media.mediaTime
	d := rescale(media.duration, tr.timescale, movieScale)
	if d <= 0 {
		return nil, errs.Unsup("keyframe times", "zero-length media edit")
	}
	return keyframeTicksOneEdit(r, tr, empty, m, d)
}

// rescale is av_rescale (round half away from zero) for non-negative values.
func rescale(v uint64, to, from uint32) int64 {
	if from == 0 {
		return 0
	}
	// Split to avoid overflow for large durations.
	q, rem := v/uint64(from), v%uint64(from)
	res := q*uint64(to) + (rem*uint64(to)+uint64(from)/2)/uint64(from)
	if res > 1<<62 {
		return 1 << 62
	}
	return int64(res)
}

// keyframeTicksNoEdit returns dts+ctts for every keyframe.
func keyframeTicksNoEdit(r io.ReaderAt, tr *kfTrack) ([]int64, error) {
	w, err := newSampleWalker(r, tr)
	if err != nil {
		return nil, err
	}
	var out []int64
	for {
		s, ok := w.next()
		if !ok {
			break
		}
		if s.key {
			out = append(out, s.dts+s.ctts)
		}
	}
	if err := w.errs(); err != nil {
		return nil, err
	}
	return out, nil
}

// keyframeTicksOneEdit applies one media edit (media time m, duration d,
// after empty edits totalling empty, all in track ticks) like mov_fix_index.
func keyframeTicksOneEdit(r io.ReaderAt, tr *kfTrack, empty, m, d int64) ([]int64, error) {
	hasCtts := tr.ctts.ok && tr.ctts.count > 0

	// Pass 1: the sample decoding restarts from. ffmpeg searches backwards
	// for the last keyframe whose dts (and, with ctts, whose pts) is at or
	// before media_time; without ctts it then accepts any sample by dts;
	// failing both it starts at the first sample.
	w, err := newSampleWalker(r, tr)
	if err != nil {
		return nil, err
	}
	start, anyStart := int64(-1), int64(-1)
	for {
		s, ok := w.next()
		if !ok || s.dts > m {
			break
		}
		anyStart = int64(s.index)
		if s.key && (!hasCtts || s.dts+s.ctts <= m) {
			start = int64(s.index)
		}
	}
	if err := w.errs(); err != nil {
		return nil, err
	}
	if start < 0 {
		start = 0
		if !hasCtts && anyStart >= 0 {
			start = anyStart
		}
	}

	// Pass 2: from start, find the first frame inside the edit, the earliest
	// presented frame, and the keyframes ffmpeg keeps in its index.
	w, err = newSampleWalker(r, tr)
	if err != nil {
		return nil, err
	}
	type kf struct{ dts, ctts int64 }
	var keys []kf
	firstIn := false
	var firstInDts, minCts int64
	foundAfter := false
	end := m + d
	var prev *sample
	handle := func(s sample) {
		cts := s.dts + s.ctts
		if cts >= m && cts < end {
			if !firstIn {
				firstIn, firstInDts, minCts = true, s.dts, cts
			} else if cts < minCts {
				minCts = cts
			}
		}
		if s.key {
			keys = append(keys, kf{s.dts, s.ctts})
		}
	}
	// ffmpeg's frame_duration needs the next sample, so the break test for a
	// sample runs once its successor is known. After the last sample the
	// walk ends anyway.
	stop := func(s sample, dur int64) bool {
		cts := s.dts + s.ctts
		if cts+dur >= end && s.key {
			if hasCtts && !foundAfter {
				foundAfter = true
				return false
			}
			return true
		}
		return false
	}
	for {
		s, ok := w.next()
		if !ok {
			break
		}
		if int64(s.index) < start {
			continue
		}
		if prev != nil && stop(*prev, s.dts-prev.dts) {
			break
		}
		handle(s)
		cp := s
		prev = &cp
	}
	if err := w.errs(); err != nil {
		return nil, err
	}
	if !firstIn {
		return nil, errs.Unsup("keyframe times", "no frame inside the edit")
	}
	shift := max(int64(0), minCts-firstInDts)
	out := make([]int64, len(keys))
	for i, k := range keys {
		out[i] = k.dts + k.ctts - firstInDts + empty - shift
	}
	return out, nil
}
