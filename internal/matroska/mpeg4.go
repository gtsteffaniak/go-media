package matroska

import (
	"errors"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/mpeg4"
)

// CodecMPEG4ASP is the Matroska CodecID for MPEG-4 Part 2 video.
const CodecMPEG4ASP = "V_MPEG4/ISO/ASP"

// mpeg4CodecIDs are the Matroska MPEG-4 Part 2 CodecIDs ffmpeg maps to its
// mpeg4 decoder (libavformat/matroska.c ff_mkv_codec_tags).
var mpeg4CodecIDs = []string{CodecMPEG4ASP, "V_MPEG4/ISO/SP", "V_MPEG4/ISO/AP"}

// IsMPEG4CodecID reports whether id is an MPEG-4 Part 2 CodecID.
func IsMPEG4CodecID(id string) bool {
	for _, c := range mpeg4CodecIDs {
		if c == id {
			return true
		}
	}
	return false
}

// MPEG4PictureAtSeek decodes the MPEG-4 Part 2 keyframe at or before
// seekPercent of the file duration (the first keyframe after it when none
// precede it). CodecPrivate supplies the VOL headers. It returns (nil, nil)
// when the file has no MPEG-4 Part 2 video track. h must come from
// ProbeHeader on the same reader.
func MPEG4PictureAtSeek(r io.ReaderAt, size int64, h *Header, seekPercent float64) (*mpeg4.Picture, error) {
	if h == nil || !h.HasDuration() {
		return nil, errs.Unsup("thumbnail", "matroska: no duration, seek impossible")
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)
	// Demux errors past ProbeHeader (truncated or corrupt index, clusters or
	// blocks) go through mpeg4ThumbErr like decoder errors, so callers get
	// errs.Unsupported and fall back to ffmpeg, matching the MP4 thumbErr
	// contract. io.EOF from the cluster walk is the normal end of the file.
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, mpeg4ThumbErr(err)
	}
	track, _, ok := videoTrackNumber(ix.tracks, mpeg4CodecIDs...)
	if !ok {
		return nil, nil
	}
	var config []byte
	for i, t := range ix.tracks {
		n := t.TrackNumber
		if n == 0 {
			n = uint64(i + 1)
		}
		if n == track {
			config = t.CodecPrivate
			break
		}
	}
	pick, _, err := keyframeAtSeek(r, ix, track, h.timecodeScale(), targetMs, "MPEG-4")
	if err != nil {
		return nil, mpeg4ThumbErr(err)
	}
	if pick == nil {
		return nil, errs.Unsup("thumbnail", "matroska: no MPEG-4 Part 2 keyframe found")
	}
	pic, err := mpeg4.DecodeKeyframe(config, pick)
	if err != nil {
		return nil, mpeg4ThumbErr(err)
	}
	return pic, nil
}

// keyframeAtSeek returns the first frame of the keyframe block for track at
// or before targetMs (the first keyframe after it when none precede it) and
// its timestamp in milliseconds. It returns a nil frame when the track has no
// keyframe from the seek cluster on.
func keyframeAtSeek(r io.ReaderAt, ix *segmentIndex, track uint64, scale uint64, targetMs int64, codec string) ([]byte, int64, error) {
	start, err := seekStartCluster(r, ix, track, msToTicks(targetMs, scale), true)
	if err != nil {
		return nil, 0, err
	}
	w := clusterWalker{r: r, segEnd: ix.segEnd, off: start}
	var pick []byte
	var pickMs int64
	for {
		bi, err := w.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if bi.track != track || bi.payloadLen <= 0 || !bi.keyframe {
			continue
		}
		tsMs := timecodeTicksToMs(bi.timecode, scale)
		if tsMs > targetMs && pick != nil {
			break
		}
		if bi.payloadLen > maxCollectBytes {
			return nil, 0, errs.Unsup("matroska", codec+" keyframe too large")
		}
		frames, err := readFrames(r, bi)
		if err != nil {
			return nil, 0, err
		}
		if len(frames) > 0 && len(frames[0]) > 0 {
			pick, pickMs = frames[0], tsMs
		}
		if tsMs > targetMs {
			break
		}
	}
	return pick, pickMs, nil
}

// mpeg4ThumbErr maps an MPEG-4 Part 2 demux or decoder error to a thumbnail error:
// a ProfileError keeps its identity, and any other error (corrupt or
// truncated bitstream) becomes errs.Unsupported with the decoder's reason so
// callers fall back to ffmpeg, matching the MP4 path's thumbErr.
func mpeg4ThumbErr(err error) error {
	var pe *errs.ProfileError
	if errors.As(err, &pe) {
		return err
	}
	var ue *errs.UnsupportedError
	if errors.As(err, &ue) {
		return errs.Unsup("thumbnail", "matroska: "+ue.Reason)
	}
	return errs.Unsup("thumbnail", "matroska: "+err.Error())
}
