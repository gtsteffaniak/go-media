package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
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
	tr, err := m.track(trackIndex)
	if err != nil {
		return nil, false
	}
	k, err := tr.keyframeAt(seekPercent)
	if err != nil {
		return nil, false
	}
	off, n, err := tr.st.locate(k)
	if err != nil || n <= 0 {
		return nil, false
	}
	payload, err := ioat.ReadAll(m.r, m.size, off, n)
	if err != nil || len(payload) < 4 {
		return nil, false
	}
	return payload, true
}

// H264SyncTimeSec returns the decode time in seconds of the sync sample that
// H264SyncSample selects for the same seekPercent.
func H264SyncTimeSec(r io.ReaderAt, size int64, trackIndex int, seekPercent float64) (float64, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return 0, false
	}
	tr, err := idx.track(trackIndex)
	if err != nil || tr.timescale == 0 {
		return 0, false
	}
	k, err := tr.keyframeAt(seekPercent)
	if err != nil {
		return 0, false
	}
	return float64(tr.st.sampleTime(k)) / float64(tr.timescale), true
}
