package isobmff

import (
	"bytes"
	"fmt"
	"image"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/mpeg2video"
)

// mpeg2OTIs are esds objectTypeIndication values ffmpeg maps to mpeg2video.
var mpeg2OTIs = map[byte]bool{
	0x60: true, 0x61: true, 0x62: true, 0x63: true, 0x64: true, 0x65: true,
}

// firstMPEG2 returns the first motion-video track with an mp4v sample entry
// whose esds declares MPEG-2 video.
func (m *movieIndex) firstMPEG2() (int, bool) {
	for i := range m.tracks {
		tr := &m.tracks[i]
		if string(tr.entry[:]) != "mp4v" || string(tr.handler[:]) != "vide" {
			continue
		}
		if len(tr.esds) == 0 {
			continue
		}
		oti, _, err := esdsConfig(tr.esds)
		if err != nil || !mpeg2OTIs[oti] {
			continue
		}
		return i, true
	}
	return 0, false
}

// MPEG2ThumbnailYCbCr decodes the MPEG-2 I-picture nearest seekPercent on an
// mp4v track whose esds names MPEG-2 video. It returns (nil, nil) when the
// file has no such track.
func (s ThumbSource) MPEG2ThumbnailYCbCr(seekPercent float64) (*h264.ThumbYCbCr, error) {
	track, ok := s.idx.firstMPEG2()
	if !ok {
		return nil, nil
	}
	sample, err := s.idx.mpeg2AccessUnitErr(track, seekPercent)
	if err != nil {
		return nil, thumbErr(err)
	}
	pic, err := mpeg2video.DecodeKeyframe(sample)
	if err != nil {
		return nil, thumbErr(err)
	}
	if pic == nil {
		return nil, errs.UnsupProfile("mpeg2 keyframe", "decoder returned no picture")
	}
	return mpeg2ThumbYCbCr(pic), nil
}

func mpeg2ThumbYCbCr(pic *mpeg2video.Picture) *h264.ThumbYCbCr {
	ycc := &image.YCbCr{
		Y: pic.Y, Cb: pic.Cb, Cr: pic.Cr,
		YStride: pic.YStride, CStride: pic.CStride,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, pic.Width, pic.Height),
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.LimitedRange}
}

// mpeg2AccessUnitErr reads the sync sample at seekPercent and any following
// non-sync samples until the next sync sample. Some MP4 muxers split one
// MPEG-2 access unit across multiple sample entries.
func (m *movieIndex) mpeg2AccessUnitErr(trackIndex int, seekPercent float64) ([]byte, error) {
	tr, err := m.track(trackIndex)
	if err != nil {
		return nil, err
	}
	k, err := tr.keyframeAt(seekPercent)
	if err != nil {
		return nil, err
	}
	end := int(tr.st.sampleCount)
	if tr.st.hasStss {
		pos, err := tr.st.syncPosAtOrBefore(k)
		if err != nil {
			return nil, err
		}
		if pos+1 < int(tr.st.stssCount) {
			next, err := tr.st.syncSampleAt(pos + 1)
			if err != nil {
				return nil, err
			}
			end = next
		}
	}
	var buf bytes.Buffer
	for si := k; si < end; si++ {
		off, n, err := tr.st.locate(si)
		if err != nil {
			return nil, err
		}
		if n <= 0 {
			continue
		}
		payload, err := ioat.ReadAll(m.r, m.size, off, n)
		if err != nil {
			return nil, errs.Unsup("mp4 seek", fmt.Sprintf("mpeg2 sample %d unreadable: %v", si+1, err))
		}
		buf.Write(payload)
	}
	if buf.Len() == 0 {
		return nil, errs.Unsup("mp4 seek", "mpeg2 sync sample empty")
	}
	return buf.Bytes(), nil
}
