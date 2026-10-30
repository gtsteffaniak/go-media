package isobmff

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Bounds for the moov index. Exceeding any of them returns errs.Unsup so the
// caller can fall back to ffmpeg instead of spending time on a pathological file.
const (
	// maxMoovSize caps the declared moov box size.
	maxMoovSize = 64 << 20
	// maxTopLevelBoxes caps how many top-level boxes are skipped looking for moov.
	maxTopLevelBoxes = 4096
	// maxRunTableBytes caps an in-memory run-length table (stts or stsc).
	maxRunTableBytes = 16 << 20
	// maxStsdBytes caps the stsd payload read for the sample entry.
	maxStsdBytes = 1 << 20
	// maxChunkSamples caps stsz entries read to locate one sample in a chunk.
	maxChunkSamples = 1 << 20
	// maxSamples caps sample and sync-sample counts where per-sample slices are
	// materialized (sampleSizes, syncNumbers, SampleRefs), so a crafted stsz
	// with a tiny fixed size cannot force gigabytes of memory. It is not
	// checked at index time: uncompressed PCM audio stores one sample per
	// audio frame (8M samples is about 175 s at 48 kHz) and seeking a video
	// track never expands its tables. 1<<23 is about 39 hours at 60 fps.
	maxSamples = 1 << 23
	// maxTrakContainerDepth caps mdia/minf/stbl nesting inside a trak. A valid
	// trak needs three levels; deeper nesting is rejected to bound recursion.
	maxTrakContainerDepth = 4
)

// movieIndex is a lazily-read view of moov: per-track headers plus file
// offsets of the large sample tables. Run-length tables (stts, stsc) are held
// in memory; per-sample and per-chunk tables (stsz, stco/co64, stss) are only
// read at the entries a seek touches, so cost does not grow with duration.
type movieIndex struct {
	r      io.ReaderAt
	size   int64
	tracks []trackIndex
	// fragmented is set when moov holds an mvex box, i.e. samples live in
	// moof fragments. It comes from the moov walk the index already does.
	fragmented bool
}

// deferredTable is one stts or stsc box left unparsed until first use.
type deferredTable struct {
	typ  [4]byte
	s, e int64
}

// deferTablesFor reports whether a trak with this hdlr handler type can skip
// reading stts/stsc while indexing. Video, cover-art and subtitle handlers are
// parsed eagerly because thumbnail, seek and subtitle paths read them.
func deferTablesFor(handler [4]byte) bool {
	switch string(handler[:]) {
	case "soun", "tmcd", "hint", "meta":
		return true
	}
	return false
}

// trackIndex holds one trak's headers and sample table locations.
//
// err is set when the trak failed to index. The entry keeps its position so
// track indexes still match trak order, and keeps whatever headers (handler,
// sample entry) were read before the failure, so a corrupt video trak is still
// found and reported as unsupported instead of silently picking another.
type trackIndex struct {
	err error
	// deferred holds stts/stsc box bounds for traks whose handler says no
	// seek path reads them (audio, timecode, hint, metadata). They are
	// parsed on first track() access instead of on every moov index.
	deferred []deferredTable
	once     *sync.Once

	handler  [4]byte
	entry    [4]byte // first stsd sample entry type, e.g. "avc1"
	trackID  uint32  // tkhd track_ID
	rotation int     // tkhd display rotation, ffprobe sign convention
	avcC     []byte  // avcC payload for avc1/encv entries
	// fragmented copies movieIndex.fragmented so seek errors can say why the
	// moov sample tables are empty.
	fragmented bool
	timescale  uint32
	duration   uint64
	st         sampleTable
}

// sampleTable locates samples of one track without expanding per-sample state.
type sampleTable struct {
	r           io.ReaderAt
	size        int64
	sampleCount uint32
	defaultSize uint32 // stsz sample_size; 0 means a per-sample table at stszOff
	stszOff     int64
	hasStsz     bool
	hasStss     bool
	stssOff     int64
	stssCount   uint32
	chunkOff    int64
	chunkCount  uint32
	co64        bool
	hasChunks   bool
	stts        []uint32 // (sample_count, sample_delta) pairs
	stsc        []uint32 // (first_chunk, samples_per_chunk, desc_index) triples
}

// readFullAt reads len(b) bytes at off. Per the io.ReaderAt contract, a read
// that fills b may still return io.EOF, which is accepted here. It reports
// whether b was fully filled. Callers must not share buffers across goroutines.
func readFullAt(r io.ReaderAt, b []byte, off int64) bool {
	n, err := r.ReadAt(b, off)
	return n == len(b) && (err == nil || err == io.EOF)
}

// openMovieIndex finds moov with bounded top-level header reads and indexes its traks.
func openMovieIndex(r io.ReaderAt, size int64) (*movieIndex, error) {
	var off int64
	var hdr [16]byte
	for i := 0; off+8 <= size; i++ {
		if i >= maxTopLevelBoxes {
			return nil, errs.Unsup("mp4 index", "too many top-level boxes")
		}
		typ, hdrLen, boxSize, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			return nil, err
		}
		if typ == [4]byte{'m', 'o', 'o', 'v'} {
			if boxSize > maxMoovSize {
				return nil, errs.Unsup("mp4 index", "moov exceeds 64 MiB")
			}
			return indexMoov(r, size, off+hdrLen, off+boxSize)
		}
		off += boxSize
	}
	return nil, errs.Unsup("mp4 index", "no moov box")
}

// readBoxHeader reads the box header at off, bounded by end.
func readBoxHeader(r io.ReaderAt, size, off, end int64, buf []byte) (typ [4]byte, hdrLen, boxSize int64, err error) {
	if end-off < 8 {
		return typ, 0, 0, errs.Unsup("mp4 index", "truncated box header")
	}
	if !readFullAt(r, buf[:8], off) {
		return typ, 0, 0, errs.Unsup("mp4 index", "short box header read")
	}
	copy(typ[:], buf[4:8])
	boxSize = int64(binary.BigEndian.Uint32(buf[:4]))
	hdrLen = 8
	switch boxSize {
	case 0:
		boxSize = end - off
	case 1:
		if end-off < 16 {
			return typ, 0, 0, errs.Unsup("mp4 index", "truncated largesize")
		}
		if !readFullAt(r, buf[8:16], off+8) {
			return typ, 0, 0, errs.Unsup("mp4 index", "short largesize read")
		}
		ls := binary.BigEndian.Uint64(buf[8:16])
		if ls > uint64(end-off) {
			return typ, 0, 0, errs.Unsup("mp4 index", fmt.Sprintf("%q box at offset %d: largesize %d exceeds parent (%d bytes left)", typ[:], off, ls, end-off))
		}
		boxSize = int64(ls)
		hdrLen = 16
	}
	if boxSize < hdrLen || boxSize > end-off {
		return typ, 0, 0, errs.Unsup("mp4 index", fmt.Sprintf("%q box at offset %d: size %d invalid (header %d, %d bytes left in parent; truncated file?)", typ[:], off, boxSize, hdrLen, end-off))
	}
	return typ, hdrLen, boxSize, nil
}

// walkBoxes calls fn for each child box in [start,end).
func walkBoxes(r io.ReaderAt, size, start, end int64, fn func(typ [4]byte, payload, payloadEnd int64) error) error {
	var hdr [16]byte
	for off := start; off+8 <= end; {
		typ, hdrLen, boxSize, err := readBoxHeader(r, size, off, end, hdr[:])
		if err != nil {
			return err
		}
		if err := fn(typ, off+hdrLen, off+boxSize); err != nil {
			return err
		}
		off += boxSize
	}
	return nil
}

func readPayload(r io.ReaderAt, start, end int64, limit int64) ([]byte, error) {
	n := end - start
	if n < 0 || n > limit {
		return nil, errs.Unsup("mp4 index", "box payload too large")
	}
	b := make([]byte, n)
	if n > 0 && !readFullAt(r, b, start) {
		return nil, errs.Unsup("mp4 index", "short box payload read")
	}
	return b, nil
}

func indexMoov(r io.ReaderAt, size, start, end int64) (*movieIndex, error) {
	idx := &movieIndex{r: r, size: size}
	err := walkBoxes(r, size, start, end, func(typ [4]byte, ps, pe int64) error {
		if typ == [4]byte{'m', 'v', 'e', 'x'} {
			idx.fragmented = true
			return nil
		}
		if typ != [4]byte{'t', 'r', 'a', 'k'} {
			return nil
		}
		// A bad trak (corrupt timecode/subtitle tables, oversized audio
		// tables) only makes that trak unusable; the file fails only when
		// the caller selects it.
		tr, err := indexTrak(r, size, ps, pe)
		tr.err = err
		idx.tracks = append(idx.tracks, tr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range idx.tracks {
		idx.tracks[i].fragmented = idx.fragmented
	}
	return idx, nil
}

func indexTrak(r io.ReaderAt, size, start, end int64) (trackIndex, error) {
	tr := trackIndex{st: sampleTable{r: r, size: size}}
	var visit func(ps, pe int64, depth int) error
	visit = func(ps, pe int64, depth int) error {
		return walkBoxes(r, size, ps, pe, func(typ [4]byte, s, e int64) error {
			switch string(typ[:]) {
			case "mdia", "minf", "stbl":
				// Bound nesting so a crafted moov cannot overflow the goroutine stack.
				if depth+1 > maxTrakContainerDepth {
					return errs.Unsup("mp4 index", "trak nesting too deep")
				}
				return visit(s, e, depth+1)
			case "tkhd":
				// Read only through the matrix (88 bytes for version 1).
				b, err := readPayload(r, s, min(e, s+88), 88)
				if err != nil {
					return err
				}
				tr.parseTkhd(b)
			case "mdhd":
				b, err := readPayload(r, s, e, 64)
				if err != nil {
					return err
				}
				return tr.parseMdhd(b)
			case "hdlr":
				b, err := readPayload(r, s, min(e, s+12), 12)
				if err != nil {
					return err
				}
				if len(b) >= 12 {
					copy(tr.handler[:], b[8:12])
				}
			case "stsd":
				b, err := readPayload(r, s, e, maxStsdBytes)
				if err != nil {
					return err
				}
				tr.parseStsd(b)
			case "stts", "stsc":
				if deferTablesFor(tr.handler) {
					tr.deferred = append(tr.deferred, deferredTable{typ, s, e})
					return nil
				}
				return tr.st.indexTable(typ, s, e)
			default:
				return tr.st.indexTable(typ, s, e)
			}
			return nil
		})
	}
	if err := visit(start, end, 0); err != nil {
		return tr, err
	}
	if len(tr.deferred) > 0 {
		tr.once = new(sync.Once)
	}
	return tr, nil
}

// parseTkhd records the track ID and display rotation from a tkhd payload.
// A truncated tkhd leaves both at zero (no rotation), matching listTrakMeta.
func (tr *trackIndex) parseTkhd(b []byte) {
	if len(b) < 4 {
		return
	}
	idOff, matOff := 12, 40
	if b[0] == 1 {
		idOff, matOff = 20, 52
	}
	if len(b) >= idOff+4 {
		tr.trackID = binary.BigEndian.Uint32(b[idOff:])
	}
	if len(b) < matOff+36 {
		return
	}
	var m [9]int32
	for i := range m {
		m[i] = int32(binary.BigEndian.Uint32(b[matOff+4*i:]))
	}
	tr.rotation = rotationFromMatrix(m)
}

// videoRotation returns the rotation of the first motion-video track (cover
// art traks are skipped). ok is false when there is no video track.
func (m *movieIndex) videoRotation() (int, bool) {
	for i := range m.tracks {
		tr := &m.tracks[i]
		if string(tr.handler[:]) != "vide" || isAttachedCoverSampleEntry(string(tr.entry[:]), "vide") {
			continue
		}
		return tr.rotation, true
	}
	return 0, false
}

// load parses deferred stts/stsc boxes once; concurrent callers share the result.
func (tr *trackIndex) load() error {
	if tr.once == nil {
		return tr.err
	}
	tr.once.Do(func() {
		for _, d := range tr.deferred {
			if err := tr.st.indexTable(d.typ, d.s, d.e); err != nil {
				tr.err = err
				return
			}
		}
	})
	return tr.err
}

func (tr *trackIndex) parseMdhd(b []byte) error {
	if len(b) < 20 {
		return errs.Unsup("mp4 index", "short mdhd")
	}
	if b[0] == 1 {
		if len(b) < 32 {
			return errs.Unsup("mp4 index", "short mdhd v1")
		}
		tr.timescale = binary.BigEndian.Uint32(b[20:24])
		tr.duration = binary.BigEndian.Uint64(b[24:32])
		return nil
	}
	tr.timescale = binary.BigEndian.Uint32(b[12:16])
	tr.duration = uint64(binary.BigEndian.Uint32(b[16:20]))
	return nil
}

// parseStsd records the first sample entry type and, for H.264, its avcC payload.
func (tr *trackIndex) parseStsd(b []byte) {
	if len(b) < 16 {
		return
	}
	e := b[8:]
	esz := int(binary.BigEndian.Uint32(e[:4]))
	if esz < 8 || esz > len(e) {
		return
	}
	copy(tr.entry[:], e[4:8])
	if t := string(tr.entry[:]); t != "avc1" && t != "encv" {
		return
	}
	const visualEntryLen = 8 + 78
	if esz < visualEntryLen {
		return
	}
	for c := e[visualEntryLen:esz]; len(c) >= 8; {
		csz := int(binary.BigEndian.Uint32(c[:4]))
		if csz < 8 || csz > len(c) {
			return
		}
		if string(c[4:8]) == "avcC" {
			tr.avcC = c[8:csz]
			return
		}
		c = c[csz:]
	}
}

// indexTable records the location of a sample table box payload [s,e).
func (st *sampleTable) indexTable(typ [4]byte, s, e int64) error {
	switch string(typ[:]) {
	case "stts", "stsc":
		b, err := readPayload(st.r, s, e, maxRunTableBytes)
		if err != nil {
			return err
		}
		width := 2
		if typ[2] == 's' && typ[3] == 'c' {
			width = 3
		}
		vals, err := parseRunTable(b, width)
		if err != nil {
			return err
		}
		if width == 2 {
			st.stts = vals
		} else {
			st.stsc = vals
		}
	case "stsz":
		h, err := st.readFullBoxHeader(s, e, 12)
		if err != nil {
			return err
		}
		st.defaultSize = binary.BigEndian.Uint32(h[4:8])
		st.sampleCount = binary.BigEndian.Uint32(h[8:12])
		st.stszOff = s + 12
		st.hasStsz = true
		if st.defaultSize == 0 && int64(st.sampleCount)*4 > e-st.stszOff {
			return errs.Unsup("mp4 index", "stsz table truncated")
		}
		// A fixed sample size must still fit the file, which bounds sampleCount
		// before callers materialize per-sample slices.
		if st.defaultSize != 0 && uint64(st.sampleCount)*uint64(st.defaultSize) > uint64(st.size) {
			return errs.Unsup("mp4 index", "stsz samples exceed file size")
		}
	case "stss":
		h, err := st.readFullBoxHeader(s, e, 8)
		if err != nil {
			return err
		}
		st.stssCount = binary.BigEndian.Uint32(h[4:8])
		st.stssOff = s + 8
		st.hasStss = true
		if int64(st.stssCount)*4 > e-st.stssOff {
			return errs.Unsup("mp4 index", "stss table truncated")
		}
	case "stco", "co64":
		h, err := st.readFullBoxHeader(s, e, 8)
		if err != nil {
			return err
		}
		st.chunkCount = binary.BigEndian.Uint32(h[4:8])
		st.chunkOff = s + 8
		st.co64 = typ[0] == 'c' && typ[1] == 'o'
		st.hasChunks = true
		w := int64(4)
		if st.co64 {
			w = 8
		}
		if int64(st.chunkCount)*w > e-st.chunkOff {
			return errs.Unsup("mp4 index", "chunk offset table truncated")
		}
	}
	return nil
}

func (st *sampleTable) readFullBoxHeader(s, e int64, n int) ([]byte, error) {
	if e-s < int64(n) {
		return nil, errs.Unsup("mp4 index", "short table header")
	}
	b := make([]byte, n)
	if !readFullAt(st.r, b, s) {
		return nil, errs.Unsup("mp4 index", "short table header read")
	}
	return b, nil
}

// parseRunTable decodes a full-box table of width uint32 columns per entry.
func parseRunTable(b []byte, width int) ([]uint32, error) {
	if len(b) < 8 {
		return nil, errs.Unsup("mp4 index", "short run table")
	}
	n := uint64(binary.BigEndian.Uint32(b[4:8]))
	if n*uint64(width)*4 > uint64(len(b)-8) {
		return nil, errs.Unsup("mp4 index", "run table truncated")
	}
	out := make([]uint32, int(n)*width)
	for i := range out {
		out[i] = binary.BigEndian.Uint32(b[8+4*i:])
	}
	return out, nil
}

// u32At reads one big-endian uint32. It allocates its own buffer so a cached
// index can serve concurrent thumbnail calls.
func (st *sampleTable) u32At(off int64) (uint32, error) {
	var b [4]byte
	if !readFullAt(st.r, b[:], off) {
		return 0, errs.Unsup("mp4 seek", "short table read")
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

// usable reports whether the table can locate samples (non-fragmented moov).
func (st *sampleTable) usable() bool {
	return st.hasStsz && st.hasChunks && st.sampleCount > 0 && st.chunkCount > 0 && len(st.stsc) > 0
}

// totalTicks sums stts durations.
func (st *sampleTable) totalTicks() uint64 {
	var t uint64
	for i := 0; i+1 < len(st.stts); i += 2 {
		t += uint64(st.stts[i]) * uint64(st.stts[i+1])
	}
	return t
}

// sampleAtTime returns the sample whose decode interval contains t (clamped).
func (st *sampleTable) sampleAtTime(t uint64) int {
	var base, acc uint64
	for i := 0; i+1 < len(st.stts); i += 2 {
		cnt, d := uint64(st.stts[i]), uint64(st.stts[i+1])
		if d > 0 && t < acc+cnt*d {
			return st.clampSample(base + (t-acc)/d)
		}
		base += cnt
		acc += cnt * d
	}
	return st.clampSample(base)
}

func (st *sampleTable) clampSample(i uint64) int {
	if st.sampleCount == 0 {
		return 0
	}
	if i >= uint64(st.sampleCount) {
		return int(st.sampleCount - 1)
	}
	return int(i)
}

// sampleTime returns the decode timestamp of sample i.
func (st *sampleTable) sampleTime(i int) uint64 {
	var base, acc uint64
	for k := 0; k+1 < len(st.stts); k += 2 {
		cnt, d := uint64(st.stts[k]), uint64(st.stts[k+1])
		if uint64(i) < base+cnt {
			return acc + (uint64(i)-base)*d
		}
		base += cnt
		acc += cnt * d
	}
	return acc
}

// syncAtOrBefore returns the latest sync sample index <= i (ffmpeg -ss
// keyframe semantics). Without stss every sample is a sync sample.
func (st *sampleTable) syncAtOrBefore(i int) (int, error) {
	if !st.hasStss {
		return i, nil
	}
	k, err := st.syncPosAtOrBefore(i)
	if err != nil {
		return 0, err
	}
	return st.syncSampleAt(k)
}

// syncPosAtOrBefore returns the 0-based stss position of the latest sync
// sample <= i, or 0 when i precedes the first sync sample. It requires stss.
func (st *sampleTable) syncPosAtOrBefore(i int) (int, error) {
	if st.stssCount == 0 {
		return 0, errs.Unsup("mp4 seek", "empty stss")
	}
	want := uint32(i + 1)
	var rerr error
	k := sort.Search(int(st.stssCount), func(j int) bool {
		v, err := st.u32At(st.stssOff + 4*int64(j))
		if err != nil {
			rerr = err
		}
		return v > want
	}) - 1
	if rerr != nil {
		return 0, rerr
	}
	if k < 0 {
		k = 0
	}
	return k, nil
}

// syncSampleAt returns the 0-based sample index stored at stss position pos.
func (st *sampleTable) syncSampleAt(pos int) (int, error) {
	n, err := st.u32At(st.stssOff + 4*int64(pos))
	if err != nil {
		return 0, err
	}
	if n == 0 || n > st.sampleCount {
		return 0, errs.Unsup("mp4 seek", "stss entry out of range")
	}
	return int(n - 1), nil
}

// syncPosNearest returns the stss position of the sync sample closest in
// decode time to the requested tick target, looking at the sync sample at or
// before sample i (the sample whose interval holds target) and the one after
// it. Comparing against target rather than sample i's start time keeps a seek
// that lands mid-sample from favouring the earlier keyframe. A tie goes to the
// earlier one. It requires stss.
func (st *sampleTable) syncPosNearest(i int, target uint64) (int, error) {
	pos, err := st.syncPosAtOrBefore(i)
	if err != nil {
		return 0, err
	}
	if uint32(pos+1) >= st.stssCount {
		return pos, nil
	}
	prev, err := st.syncSampleAt(pos)
	if err != nil {
		return 0, err
	}
	next, err := st.syncSampleAt(pos + 1)
	if err != nil {
		return 0, err
	}
	tp, tn := st.sampleTime(prev), st.sampleTime(next)
	if tp > target { // i precedes the first sync sample
		return pos, nil
	}
	if tn-target < target-tp {
		return pos + 1, nil
	}
	return pos, nil
}

// sampleSize returns the byte size of sample i from stsz.
func (st *sampleTable) sampleSize(i int) (uint32, error) {
	if i < 0 || uint32(i) >= st.sampleCount {
		return 0, errs.Unsup("mp4 seek", "sample out of range")
	}
	if st.defaultSize != 0 {
		return st.defaultSize, nil
	}
	return st.u32At(st.stszOff + 4*int64(i))
}

const (
	// keyframeSkipWindow is how many consecutive sync samples, starting at the
	// nearest one, are compared by size when looking for an empty keyframe. It
	// bounds the extra table reads at a few 4-byte stss and stsz lookups.
	keyframeSkipWindow = 5
	// emptyKeyframeDivisor marks a keyframe as empty when its size is below
	// 1/divisor of the largest keyframe in the window. A black or flat
	// first frame compresses to under 1% of a real frame (906 B against
	// 56-217 KB in one test clip), while real content stayed above 25%.
	emptyKeyframeDivisor = 10
)

// pickThumbnailKeyframe returns the sync sample to decode for a thumbnail at
// sample i. It starts from the sync sample nearest in time (before or after),
// then moves forward past keyframes whose encoded size is tiny next to their
// neighbours: those are black, fade, or flat frames that make a useless
// thumbnail. It reads only table entries, never sample data, so it adds no
// decode work. target is the requested decode tick and i the sample whose
// interval holds it. Without stss every sample is a sync sample and i is
// returned.
func (st *sampleTable) pickThumbnailKeyframe(i int, target uint64) (int, error) {
	if !st.hasStss {
		return i, nil
	}
	pos, err := st.syncPosNearest(i, target)
	if err != nil {
		return 0, err
	}
	first, err := st.syncSampleAt(pos)
	if err != nil {
		return 0, err
	}
	end := pos + keyframeSkipWindow
	if end > int(st.stssCount) {
		end = int(st.stssCount)
	}
	if end-pos < 2 {
		return first, nil
	}
	var samples [keyframeSkipWindow]int
	var sizes [keyframeSkipWindow]uint32
	var maxSize uint32
	n := 0
	for p := pos; p < end; p++ {
		s, err := st.syncSampleAt(p)
		if err != nil {
			return first, nil
		}
		sz, err := st.sampleSize(s)
		if err != nil {
			return first, nil
		}
		samples[n], sizes[n] = s, sz
		if sz > maxSize {
			maxSize = sz
		}
		n++
	}
	for j := 0; j < n; j++ {
		if uint64(sizes[j])*emptyKeyframeDivisor >= uint64(maxSize) {
			return samples[j], nil
		}
	}
	return first, nil
}

// chunkOf returns the 0-based chunk holding sample i and that chunk's first sample.
func (st *sampleTable) chunkOf(i int) (chunk uint32, first int, err error) {
	var base uint64
	for k := 0; k+2 < len(st.stsc); k += 3 {
		fc, spc := st.stsc[k], uint64(st.stsc[k+1])
		if fc == 0 || fc > st.chunkCount {
			return 0, 0, errs.Unsup("mp4 seek", "stsc first_chunk out of range")
		}
		next := st.chunkCount + 1
		if k+5 < len(st.stsc) {
			next = st.stsc[k+3]
			if next <= fc {
				return 0, 0, errs.Unsup("mp4 seek", "stsc not increasing")
			}
			if next > st.chunkCount+1 {
				next = st.chunkCount + 1
			}
		}
		run := uint64(next-fc) * spc
		if spc > 0 && uint64(i) < base+run {
			c := (uint64(i) - base) / spc
			return fc - 1 + uint32(c), int(base + c*spc), nil
		}
		base += run
	}
	return 0, 0, errs.Unsup("mp4 seek", "sample beyond stsc")
}

// locate returns the file offset and size of sample i.
func (st *sampleTable) locate(i int) (int64, int64, error) {
	if i < 0 || uint32(i) >= st.sampleCount {
		return 0, 0, errs.Unsup("mp4 seek", "sample out of range")
	}
	chunk, first, err := st.chunkOf(i)
	if err != nil {
		return 0, 0, err
	}
	var off uint64
	if st.co64 {
		var b [8]byte
		if !readFullAt(st.r, b[:], st.chunkOff+8*int64(chunk)) {
			return 0, 0, errs.Unsup("mp4 seek", "short co64 read")
		}
		off = binary.BigEndian.Uint64(b[:])
	} else {
		v, err := st.u32At(st.chunkOff + 4*int64(chunk))
		if err != nil {
			return 0, 0, err
		}
		off = uint64(v)
	}
	var sz uint64
	if st.defaultSize != 0 {
		off += uint64(i-first) * uint64(st.defaultSize)
		sz = uint64(st.defaultSize)
	} else {
		n := i - first + 1
		if n > maxChunkSamples {
			return 0, 0, errs.Unsup("mp4 seek", "chunk too large")
		}
		b := make([]byte, 4*n) // at most 4 MiB, bounded by maxChunkSamples
		if !readFullAt(st.r, b, st.stszOff+4*int64(first)) {
			return 0, 0, errs.Unsup("mp4 seek", "short stsz read")
		}
		for j := 0; j < n-1; j++ {
			off += uint64(binary.BigEndian.Uint32(b[4*j:]))
		}
		sz = uint64(binary.BigEndian.Uint32(b[4*(n-1):]))
	}
	if off > uint64(st.size) || sz > uint64(st.size)-off {
		return 0, 0, errs.Unsup("mp4 seek", "sample outside file")
	}
	return int64(off), int64(sz), nil
}

// sampleSizes reads the full stsz table (used only by whole-track iteration).
func (st *sampleTable) sampleSizes() ([]uint32, error) {
	if st.sampleCount > maxSamples {
		return nil, errs.Unsup("sample table", "sample count exceeds limit")
	}
	out := make([]uint32, st.sampleCount)
	if st.defaultSize != 0 {
		for i := range out {
			out[i] = st.defaultSize
		}
		return out, nil
	}
	b := make([]byte, 4*int64(st.sampleCount))
	if !readFullAt(st.r, b, st.stszOff) {
		return nil, errs.Unsup("sample table", "short stsz read")
	}
	for i := range out {
		out[i] = binary.BigEndian.Uint32(b[4*i:])
	}
	return out, nil
}

// track returns track i, or an unsupported error when it is out of range or
// failed to index.
func (m *movieIndex) track(i int) (*trackIndex, error) {
	if i < 0 || i >= len(m.tracks) {
		return nil, errs.Unsup("sample table", "track index out of range")
	}
	if err := m.tracks[i].load(); err != nil {
		return nil, err
	}
	return &m.tracks[i], nil
}

// firstAVC returns the first avc1/encv track (mp4.Probe CodecAVC1 parity).
// A trak that failed to index is still returned when its sample entry was
// read, so track() then reports it as unsupported.
func (m *movieIndex) firstAVC() (int, bool) {
	for i := range m.tracks {
		if t := string(m.tracks[i].entry[:]); t == "avc1" || t == "encv" {
			return i, true
		}
	}
	return 0, false
}

// avcConfig decodes the track's avcC box.
func (tr *trackIndex) avcConfig() (*mp4.AVCDecoderConfiguration, bool) {
	cfg, err := tr.avcConfigErr()
	return cfg, err == nil
}

// avcConfigErr decodes the track's avcC box, or says why it cannot.
func (tr *trackIndex) avcConfigErr() (*mp4.AVCDecoderConfiguration, error) {
	if len(tr.avcC) == 0 {
		return nil, errs.Unsup("thumbnail", "mp4: avcC box missing from "+string(tr.entry[:])+" sample entry")
	}
	var cfg mp4.AVCDecoderConfiguration
	cfg.SetType(mp4.BoxTypeAvcC())
	if _, err := mp4.Unmarshal(bytes.NewReader(tr.avcC), uint64(len(tr.avcC)), &cfg, mp4.Context{}); err != nil {
		return nil, errs.Unsup("thumbnail", "mp4: avcC invalid: "+err.Error())
	}
	return &cfg, nil
}

// durationTicks returns mdhd duration, or the stts sum when mdhd has none.
func (tr *trackIndex) durationTicks() uint64 {
	if tr.duration != 0 {
		return tr.duration
	}
	return tr.st.totalTicks()
}

// normalizeSeekPercent applies the thumbnail default (10%) and clamps to 100.
func normalizeSeekPercent(p float64) float64 {
	// NaN fails both comparisons below and would reach uint64(NaN), which is
	// platform-dependent, so it takes the default explicitly.
	if math.IsNaN(p) || p <= 0 {
		return 10
	}
	if p > 100 {
		return 100
	}
	return p
}

// targetTick maps seekPercent of the track duration to a decode tick.
func (tr *trackIndex) targetTick(seekPercent float64) uint64 {
	return uint64(float64(tr.durationTicks()) * normalizeSeekPercent(seekPercent) / 100.0)
}

// targetSample maps seekPercent of the track duration to a sample index.
func (tr *trackIndex) targetSample(seekPercent float64) int {
	return tr.st.sampleAtTime(tr.targetTick(seekPercent))
}

// keyframeAt returns the sync sample to decode for a thumbnail at seekPercent
// of the track duration: the nearest sync sample in time, moved forward past
// empty (black or flat) keyframes. See pickThumbnailKeyframe.
func (tr *trackIndex) keyframeAt(seekPercent float64) (int, error) {
	if !tr.st.usable() {
		if tr.fragmented {
			return 0, errs.Unsup("mp4 seek", "fragmented MP4 (samples in moof, moov sample tables empty)")
		}
		return 0, errs.Unsup("mp4 seek", "no usable moov sample tables (empty stbl)")
	}
	target := tr.targetTick(seekPercent)
	return tr.st.pickThumbnailKeyframe(tr.st.sampleAtTime(target), target)
}
