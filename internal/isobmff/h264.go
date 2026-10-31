package isobmff

import (
	"fmt"
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// AVCDecoderConfig returns the avcC box for an H.264 video track.
func AVCDecoderConfig(r io.ReaderAt, size int64, trackIndex int) (*mp4.AVCDecoderConfiguration, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return nil, false
	}
	tr, err := idx.track(trackIndex)
	if err != nil {
		return nil, false
	}
	return tr.avcConfig()
}

// FirstAVCTrackIndex returns the track index of the first H.264 (avc1) stream.
func FirstAVCTrackIndex(r io.ReaderAt, size int64) (int, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return 0, false
	}
	return idx.firstAVC()
}

// H264SyncSample returns one stss sync sample payload (AVCC length-prefixed NALs).
// seekPercent (default 10, clamped to 100) selects the sync sample nearest in
// time to that fraction of the video track duration. A keyframe that is tiny
// next to the following few (a black or flat first frame) is skipped in favour
// of the next real one, so a default or 0% request does not return a black
// image. Only the moov tables a seek needs, a few table entries for the size
// check, and the one sample's bytes are read, so cost does not grow with file
// duration.
func H264SyncSample(r io.ReaderAt, size int64, trackIndex int, seekPercent float64) ([]byte, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return nil, false
	}
	return idx.h264SyncSample(trackIndex, seekPercent)
}

// h264SyncSample reads the keyframe payload for seekPercent from an open index.
func (m *movieIndex) h264SyncSample(trackIndex int, seekPercent float64) ([]byte, bool) {
	b, err := m.h264SyncSampleErr(trackIndex, seekPercent)
	return b, err == nil
}

// h264SyncSampleErr is h264SyncSample with the reason a keyframe could not be
// read. Every reason comes from state the seek already computed.
func (m *movieIndex) h264SyncSampleErr(trackIndex int, seekPercent float64) ([]byte, error) {
	tr, err := m.track(trackIndex)
	if err != nil {
		return nil, err
	}
	if tr.fragmented && !tr.st.usable() {
		b, _, err := m.fragSyncSample(tr, seekPercent)
		return b, err
	}
	k, err := tr.keyframeAt(seekPercent)
	if err != nil {
		return nil, err
	}
	off, n, err := tr.st.locate(k)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, errs.Unsup("mp4 seek", fmt.Sprintf("sync sample %d is empty", k+1))
	}
	payload, err := ioat.ReadAll(m.r, m.size, off, n)
	if err != nil {
		return nil, errs.Unsup("mp4 seek", fmt.Sprintf("sync sample %d unreadable (offset %d, %d bytes): %v", k+1, off, n, err))
	}
	if len(payload) < 4 {
		return nil, errs.Unsup("mp4 seek", fmt.Sprintf("sync sample %d shorter than one NAL length prefix", k+1))
	}
	return payload, nil
}

// H264SyncTimeSec returns the decode time in seconds of the sync sample that
// H264SyncSample selects for the same seekPercent.
func H264SyncTimeSec(r io.ReaderAt, size int64, trackIndex int, seekPercent float64) (float64, bool) {
	idx, err := openMovieIndexFor(r, size, trackIndex)
	if err != nil {
		return 0, false
	}
	tr, err := idx.track(trackIndex)
	if err != nil || tr.timescale == 0 {
		return 0, false
	}
	if tr.fragmented && !tr.st.usable() {
		s, err := idx.fragKeyframe(tr, seekPercent)
		if err != nil {
			return 0, false
		}
		return float64(s.time) / float64(tr.timescale), true
	}
	k, err := tr.keyframeAt(seekPercent)
	if err != nil {
		return 0, false
	}
	return float64(tr.st.sampleTime(k)) / float64(tr.timescale), true
}
