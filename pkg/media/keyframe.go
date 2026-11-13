package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

// KeyframeSample is a sync sample byte range inside an MP4/MOV file.
type KeyframeSample struct {
	Index  int
	Offset int64
	Size   int64
}

// MP4KeyframeSamples returns stss sync sample locations for a track (offset/size via stco/co64, stsc, stsz).
func MP4KeyframeSamples(r io.ReaderAt, size int64, trackIndex int) ([]KeyframeSample, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return nil, mapErr(err)
	}
	if id != format.MP4 {
		return nil, Unsupported("keyframe lookup", string(id))
	}
	refs, err := isobmff.KeyframeSamples(r, size, trackIndex)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]KeyframeSample, len(refs))
	for i, ref := range refs {
		out[i] = KeyframeSample{Index: ref.Index, Offset: ref.Offset, Size: ref.Size}
	}
	return out, nil
}

// KeyframeTimes returns the presentation times, in seconds, of the keyframes of
// the first video stream in an MP4/MOV, Matroska or WebM file. It is the
// pure-Go counterpart of
//
//	ffprobe -select_streams v:0 -show_entries packet=pts_time,flags
//
// filtered to key packets: times are on the container timeline (ffprobe's
// pts_time, so a non-zero start_time is kept, not subtracted), ascending,
// without duplicates, and without negative times (keyframes an MP4 edit list
// starts before).
//
// The list comes from the container index only. ffprobe can also flag
// keyframes its codec parsers find in the bitstream (H.264 recovery points,
// for example) that the index does not mark, and Matroska Cues may index
// only some keyframes, so the result can be a subset of ffprobe's; it never
// adds a keyframe the container does not mark.
//
// MP4/MOV reads only the moov sample tables (stss/stps, 'rap ' sbgp, stts, ctts, elst) and
// applies a single edit list entry, with or without leading empty edits, the
// way ffmpeg's mov demuxer does. Matroska/WebM uses Cues when they index the
// video track and otherwise walks the Clusters, reading block headers only.
//
// Fragmented MP4, MP4 edit lists with several media edits, other containers,
// and files without a video track return an error that unwraps to
// ErrUnsupported, so callers can fall back to ffprobe. A read error from r is
// returned as is.
func KeyframeTimes(r io.ReaderAt, size int64) ([]float64, error) {
	return keyframeTimes(r, size, 0)
}

// KeyframeTimesWithin is KeyframeTimes limited to the keyframes at most
// windowSec seconds after the first one. A Matroska Cluster walk stops at the
// window, so this is the cheap pre-flight read for a file without Cues; the
// result is a prefix of KeyframeTimes. windowSec <= 0 means no limit.
func KeyframeTimesWithin(r io.ReaderAt, size int64, windowSec float64) ([]float64, error) {
	return keyframeTimes(r, size, windowSec)
}

// keyframeTimes dispatches by container and maps internal errors.
func keyframeTimes(r io.ReaderAt, size int64, windowSec float64) (out []float64, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = nil, panicErr(p)
		}
	}()
	if size <= 0 {
		return nil, ErrCorrupt
	}
	id, err := sniff.Detect(r, size)
	if err != nil {
		return nil, mapErr(err)
	}
	switch id {
	case format.MP4:
		out, err = isobmff.VideoKeyframeTimes(r, size, windowSec)
	case format.MKV, format.WebM:
		out, err = matroska.VideoKeyframeTimes(r, size, windowSec)
	default:
		return nil, Unsupported("keyframe times", string(id))
	}
	if err != nil {
		return nil, mapErr(err)
	}
	if len(out) == 0 {
		return nil, Unsupported("keyframe times", "no keyframes")
	}
	return out, nil
}
