package matroska

import (
	"io"

	"github.com/gen2brain/h265/hevc"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/hevcdecode"
)

const CodecHEVC = "V_MPEGH/ISO/HEVC"

// HEVCPictureAtSeek decodes the HEVC keyframe at or before seekPercent.
func HEVCPictureAtSeek(r io.ReaderAt, size int64, h *Header, seekPercent float64) (*hevc.Picture, error) {
	if h == nil || !h.HasDuration() {
		return nil, errs.Unsup("thumbnail", "matroska: no duration, seek impossible")
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, hevcThumbErr(err)
	}
	track, _, ok := videoTrackNumber(ix.tracks, CodecHEVC)
	if !ok {
		return nil, nil
	}
	var hvcC []byte
	for i, t := range ix.tracks {
		n := t.TrackNumber
		if n == 0 {
			n = uint64(i + 1)
		}
		if n == track {
			hvcC = t.CodecPrivate
			break
		}
	}
	if len(hvcC) == 0 {
		return nil, errs.Unsup("thumbnail", "matroska: HEVC track missing CodecPrivate (hvcC)")
	}
	scale := h.timecodeScale()
	start, err := seekStartCluster(r, ix, track, msToTicks(targetMs, scale), true)
	if err != nil {
		return nil, hevcThumbErr(err)
	}
	w := clusterWalker{r: r, segEnd: ix.segEnd, off: start}
	var pick []byte
	for {
		bi, err := w.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, hevcThumbErr(err)
		}
		if bi.track != track || bi.payloadLen <= 0 || !bi.keyframe {
			continue
		}
		if timecodeTicksToMs(bi.timecode, scale) > targetMs && pick != nil {
			break
		}
		if bi.payloadLen > maxCollectBytes {
			return nil, errs.Unsup("matroska", "HEVC keyframe too large")
		}
		frames, err := readFrames(r, bi)
		if err != nil {
			return nil, hevcThumbErr(err)
		}
		if len(frames) > 0 && len(frames[0]) > 0 {
			pick = frames[0]
		}
		if timecodeTicksToMs(bi.timecode, scale) > targetMs {
			break
		}
	}
	if pick == nil {
		return nil, errs.Unsup("thumbnail", "matroska: no HEVC keyframe found")
	}
	pic, err := hevcdecode.DecodeHVCCSample(hvcC, pick)
	if err != nil {
		return nil, hevcThumbErr(err)
	}
	return pic, nil
}

func hevcThumbErr(err error) error {
	return mpeg4ThumbErr(err)
}
