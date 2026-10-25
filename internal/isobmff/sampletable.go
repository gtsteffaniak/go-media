package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// SampleRef is a media sample byte range in the host file.
type SampleRef struct {
	Index  int
	Offset int64
	Size   int64
}

// SyncSampleNumbers returns 1-based sync sample indices from stss when present.
// When stss is absent (all-intra tracks), every sample index is returned.
func SyncSampleNumbers(r io.ReaderAt, size int64, trackIndex int) ([]uint32, error) {
	refs, err := SampleRefs(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	sync, err := readStss(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	if len(sync) > 0 {
		return sync, nil
	}
	out := make([]uint32, len(refs))
	for i := range refs {
		out[i] = uint32(i + 1)
	}
	return out, nil
}

// KeyframeSamples returns file offsets and sizes for sync samples (stss).
func KeyframeSamples(r io.ReaderAt, size int64, trackIndex int) ([]SampleRef, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, err
	}
	return keyframeSamplesFromInfo(r, size, trackIndex, info)
}

func keyframeSamplesFromInfo(r io.ReaderAt, size int64, trackIndex int, info *mp4.ProbeInfo) ([]SampleRef, error) {
	refs, err := sampleRefsFromInfo(r, size, trackIndex, info)
	if err != nil {
		return nil, err
	}
	sync, err := SyncSampleNumbers(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	var out []SampleRef
	for _, n := range sync {
		idx := int(n) - 1
		if idx < 0 || idx >= len(refs) {
			continue
		}
		out = append(out, refs[idx])
	}
	return out, nil
}

// SampleRefs builds absolute offsets for each sample using stco/co64, stsc, and stsz.
func SampleRefs(r io.ReaderAt, size int64, trackIndex int) ([]SampleRef, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, err
	}
	return sampleRefsFromInfo(r, size, trackIndex, info)
}

func sampleRefsFromInfo(r io.ReaderAt, size int64, trackIndex int, info *mp4.ProbeInfo) ([]SampleRef, error) {
	if trackIndex < 0 || trackIndex >= len(info.Tracks) {
		return nil, errs.Unsup("sample table", "track index out of range")
	}
	tr := info.Tracks[trackIndex]
	if len(tr.Samples) == 0 || len(tr.Chunks) == 0 {
		return nil, errs.Unsup("sample table", "missing chunk or sample tables")
	}
	defaultSize, err := stszDefaultSampleSize(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	var out []SampleRef
	si := 0
	for _, chunk := range tr.Chunks {
		off := chunk.DataOffset
		end := si + int(chunk.SamplesPerChunk)
		for ; si < end && si < len(tr.Samples); si++ {
			sz := tr.Samples[si].Size
			if sz == 0 {
				sz = defaultSize
			}
			out = append(out, SampleRef{
				Index:  si,
				Offset: int64(off),
				Size:   int64(sz),
			})
			off += uint64(sz)
		}
	}
	return out, nil
}

func stszDefaultSampleSize(r io.ReaderAt, size int64, trackIndex int) (uint32, error) {
	rs := ioat.New(r, size)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil || trackIndex < 0 || trackIndex >= len(traks) {
		return 0, err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, traks[trackIndex], []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsz()},
	})
	if err != nil || len(bips) == 0 {
		return 0, err
	}
	stsz := bips[0].Payload.(*mp4.Stsz)
	if stsz.SampleSize != 0 {
		return stsz.SampleSize, nil
	}
	return 0, nil
}

func readStss(r io.ReaderAt, size int64, trackIndex int) ([]uint32, error) {
	rs := ioat.New(r, size)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil || trackIndex < 0 || trackIndex >= len(traks) {
		return nil, err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, traks[trackIndex], []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStss()},
	})
	if err != nil {
		return nil, err
	}
	if len(bips) == 0 {
		return nil, nil
	}
	stss := bips[0].Payload.(*mp4.Stss)
	return stss.SampleNumber, nil
}
