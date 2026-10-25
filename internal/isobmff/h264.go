package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// AVCDecoderConfig returns the avcC box for an H.264 video track.
func AVCDecoderConfig(r io.ReaderAt, size int64, trackIndex int) (*mp4.AVCDecoderConfiguration, bool) {
	rs := ioat.New(r, size)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, false
	}
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil || trackIndex < 0 || trackIndex >= len(traks) {
		return nil, false
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, false
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, traks[trackIndex], []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeAvc1(), mp4.BoxTypeAvcC()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeEncv(), mp4.BoxTypeAvcC()},
	})
	if err != nil {
		return nil, false
	}
	for _, bip := range bips {
		if bip == nil {
			continue
		}
		if avcc, ok := bip.Payload.(*mp4.AVCDecoderConfiguration); ok && avcc != nil {
			return avcc, true
		}
	}
	return nil, false
}

// FirstAVCTrackIndex returns the track index of the first H.264 (avc1) stream.
func FirstAVCTrackIndex(r io.ReaderAt, size int64) (int, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return 0, false
	}
	for i, tr := range info.Tracks {
		if tr.Codec == mp4.CodecAVC1 {
			return i, true
		}
	}
	return 0, false
}

// H264SyncSample returns one stss sync sample payload (AVCC length-prefixed NALs).
// seekPercent selects the sync sample whose presentation time is closest to that
// fraction of the track duration (matching ffmpeg -ss seek semantics).
func H264SyncSample(r io.ReaderAt, size int64, trackIndex int, seekPercent float64) ([]byte, bool) {
	keyframes, err := KeyframeSamples(r, size, trackIndex)
	if err != nil || len(keyframes) == 0 {
		return nil, false
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	if seekPercent > 100 {
		seekPercent = 100
	}
	ref, ok := pickKeyframeByTime(r, size, trackIndex, keyframes, seekPercent)
	if !ok {
		return nil, false
	}
	if ref.Size <= 0 {
		return nil, false
	}
	payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
	if err != nil || len(payload) < 4 {
		return nil, false
	}
	return payload, true
}

// H264SyncTimeSec returns presentation time in seconds for the sync sample at seekPercent.
func H264SyncTimeSec(r io.ReaderAt, size int64, trackIndex int, seekPercent float64) (float64, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil || trackIndex < 0 || trackIndex >= len(info.Tracks) {
		return 0, false
	}
	tr := info.Tracks[trackIndex]
	if tr.Timescale == 0 {
		return 0, false
	}
	keyframes, err := KeyframeSamples(r, size, trackIndex)
	if err != nil || len(keyframes) == 0 {
		return 0, false
	}
	ref, ok := pickKeyframeByTime(r, size, trackIndex, keyframes, seekPercent)
	if !ok {
		return 0, false
	}
	pts := samplePTS(tr.Samples, ref.Index)
	return float64(pts) / float64(tr.Timescale), true
}

func pickKeyframeByTime(r io.ReaderAt, size int64, trackIndex int, keyframes []SampleRef, seekPercent float64) (SampleRef, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil || trackIndex < 0 || trackIndex >= len(info.Tracks) {
		return SampleRef{}, false
	}
	tr := info.Tracks[trackIndex]
	if tr.Timescale == 0 || len(tr.Samples) == 0 {
		return keyframes[0], true
	}
	durTicks := tr.Duration
	if durTicks == 0 {
		var sum uint64
		for _, s := range tr.Samples {
			sum += uint64(s.TimeDelta)
		}
		durTicks = sum
	}
	target := uint64(float64(durTicks) * seekPercent / 100.0)

	// Map sync sample index (1-based stss) to presentation timestamp.
	syncNums, err := readStss(r, size, trackIndex)
	if err != nil || len(syncNums) == 0 {
		return keyframes[0], true
	}
	best := keyframes[0]
	bestDelta := uint64(1<<62)
	for i, sn := range syncNums {
		pts := samplePTS(tr.Samples, int(sn)-1)
		var delta uint64
		if pts >= target {
			delta = pts - target
		} else {
			delta = target - pts
		}
		if delta < bestDelta && i < len(keyframes) {
			bestDelta = delta
			best = keyframes[i]
		}
	}
	return best, true
}

func samplePTS(samples mp4.Samples, sampleIndex int) uint64 {
	if sampleIndex < 0 {
		return 0
	}
	var pts uint64
	for i := 0; i < sampleIndex && i < len(samples); i++ {
		pts += uint64(samples[i].TimeDelta)
	}
	return pts
}
