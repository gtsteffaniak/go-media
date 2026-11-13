package matroska

import (
	"errors"
	"io"
	"sort"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Keyframe timeline for HLS segment planning. VideoKeyframeTimes returns the
// timestamps ffmpeg's matroska demuxer gives the key packets of the first
// video track (Tracks order, as ffprobe's v:0): Cluster Timecode plus the
// block's relative timecode, scaled by TimecodeScale. A SimpleBlock is a
// keyframe when its key flag is set; a BlockGroup when it has no
// ReferenceBlock. For a laced block only the first lace is key, at the block
// timestamp.
//
// When the file has Cues for the track, the CueTimes are used and no Cluster
// is read. mkvmerge and ffmpeg write a CuePoint for every video keyframe, so
// this is the same list for the files they produce. A muxer that indexes
// fewer keyframes gives a subset, which only makes segments longer. Without
// Cues the Clusters are walked, reading block headers only.

// errNoKeyframes reports a video track with no keyframe at a non-negative time.
var errNoKeyframes = errs.Unsup("keyframe times", "no keyframes")

// keyWalkBuffer is the read-ahead used while walking Cluster block headers,
// so neighbouring small blocks (audio, subtitles) cost one read.
const keyWalkBuffer = 16 << 10

// VideoKeyframeTimes returns keyframe presentation times in seconds of the
// first video track, ascending, without duplicates and without negative
// times. windowSec > 0 keeps only keyframes within windowSec of the first
// one, and stops a Cluster walk there.
func VideoKeyframeTimes(r io.ReaderAt, size int64, windowSec float64) ([]float64, error) {
	head, err := readHead(r, size)
	if err != nil {
		return nil, err
	}
	scale := head.Segment.Info.TimecodeScale
	if scale == 0 {
		scale = 1_000_000
	}
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, err
	}
	track, ok := firstVideoTrack(ix.tracks)
	if !ok {
		return nil, errs.Unsup("keyframe times", "no video track")
	}
	var windowTicks int64 = -1
	if windowSec > 0 {
		windowTicks = int64(windowSec * 1e9 / float64(scale))
	}
	ticks, err := cueKeyframeTicks(r, ix, track)
	if err != nil || len(ticks) == 0 {
		ticks, err = walkKeyframeTicks(r, ix, track, windowTicks)
		if err != nil {
			return nil, err
		}
	}
	out := ticksToSeconds(ticks, scale, windowTicks)
	if len(out) == 0 {
		return nil, errNoKeyframes
	}
	return out, nil
}

// firstVideoTrack returns the track number of the first video track.
func firstVideoTrack(tracks []trackEntry) (uint64, bool) {
	for i, t := range tracks {
		if t.TrackType != 1 {
			continue
		}
		if t.TrackNumber != 0 {
			return t.TrackNumber, true
		}
		return uint64(i + 1), true
	}
	return 0, false
}

// ticksToSeconds sorts, de-duplicates and window-limits timecode ticks.
// Duplicates are dropped after conversion, so ticks that only differ below
// float64 precision (absurd timecodes) still give a strictly ascending list.
func ticksToSeconds(ticks []int64, scale uint64, windowTicks int64) []float64 {
	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	out := make([]float64, 0, len(ticks))
	first := int64(-1)
	for _, t := range ticks {
		if t < 0 {
			continue
		}
		if first < 0 {
			first = t
		}
		if windowTicks >= 0 && t-first > windowTicks {
			break
		}
		v := float64(t) * float64(scale) / 1e9
		if len(out) > 0 && v <= out[len(out)-1] {
			continue
		}
		out = append(out, v)
	}
	return out
}

// cueKeyframeTicks returns the CueTimes that index track. It returns nothing
// (and no error) when the file has no usable Cues.
func cueKeyframeTicks(r io.ReaderAt, ix *segmentIndex, track uint64) ([]int64, error) {
	if ix.cuesOff < 0 {
		return nil, nil
	}
	id, hdrLen, dataSize, err := readElementHeader(r, ix.cuesOff, ix.segEnd)
	if err != nil || id != idCues || dataSize <= 0 || dataSize > maxCuesElement {
		return nil, nil
	}
	buf, err := readBody(r, ix.cuesOff+hdrLen, dataSize)
	if err != nil {
		return nil, err
	}
	return parseCueTimes(buf, track), nil
}

// parseCueTimes lists the CueTime of every CuePoint with a
// CueTrackPositions entry for track.
func parseCueTimes(b []byte, track uint64) []int64 {
	var out []int64
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
		var haveTime, forTrack bool
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
				for len(cb) > 0 {
					gid, gh, gn, ok := bufElem(cb)
					if !ok {
						break
					}
					if gid == idCueTrack {
						if v, ok := bufUint(cb[gh : gh+gn]); ok && v == track {
							forTrack = true
						}
					}
					cb = cb[gh+gn:]
				}
			}
		}
		if haveTime && forTrack && t <= 1<<62 {
			out = append(out, int64(t))
		}
	}
	return out
}

// walkKeyframeTicks walks every Cluster from the first one and collects the
// timecodes of track's key blocks. windowTicks >= 0 stops the walk once a
// block of track is past the first keyframe plus the window.
func walkKeyframeTicks(r io.ReaderAt, ix *segmentIndex, track uint64, windowTicks int64) ([]int64, error) {
	if ix.firstCluster < 0 {
		return nil, errs.Unsup("keyframe times", "no clusters")
	}
	br := &bufferedReaderAt{r: r, buf: make([]byte, 0, keyWalkBuffer)}
	w := clusterWalker{r: br, segEnd: ix.segEnd, off: ix.firstCluster}
	var out []int64
	first := int64(-1)
	for {
		bi, err := w.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if len(out) > 0 {
				// A corrupt block after usable keyframes ends the index there,
				// as a truncated tail does.
				break
			}
			return nil, err
		}
		if br.err != nil {
			return nil, br.err
		}
		if bi.track != track {
			continue
		}
		if windowTicks >= 0 && first >= 0 && bi.timecode-first > windowTicks {
			break
		}
		if bi.keyframe && bi.timecode >= 0 {
			if first < 0 {
				first = bi.timecode
			}
			out = append(out, bi.timecode)
		}
	}
	if br.err != nil {
		// The walker reads a failed read as the end of the file; a reader
		// error (for example a cancelled context) must not pass for one.
		return nil, br.err
	}
	return out, nil
}

// bufferedReaderAt serves small reads from one read-ahead window and records
// the first read error other than io.EOF.
type bufferedReaderAt struct {
	r   io.ReaderAt
	buf []byte
	off int64
	err error
}

// ReadAt implements io.ReaderAt.
func (b *bufferedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= b.off && off+int64(len(p)) <= b.off+int64(len(b.buf)) {
		return copy(p, b.buf[off-b.off:]), nil
	}
	if len(p) > cap(b.buf) {
		n, err := b.r.ReadAt(p, off)
		b.note(err)
		return n, err
	}
	n, err := b.r.ReadAt(b.buf[:cap(b.buf)], off)
	b.note(err)
	b.buf, b.off = b.buf[:n], off
	m := copy(p, b.buf)
	if m < len(p) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return m, err
	}
	return m, nil
}

// note records err when it is the first non-EOF read error.
func (b *bufferedReaderAt) note(err error) {
	if err != nil && !errors.Is(err, io.EOF) && b.err == nil {
		b.err = err
	}
}
