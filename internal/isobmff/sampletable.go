package isobmff

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
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
	st, err := openSampleTable(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	return st.syncNumbers()
}

// KeyframeSamples returns file offsets and sizes for sync samples (stss).
// Only sync samples are located; per-sample state is never expanded.
func KeyframeSamples(r io.ReaderAt, size int64, trackIndex int) ([]SampleRef, error) {
	st, err := openSampleTable(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	if !st.hasStss {
		return st.allRefs()
	}
	sync, err := st.syncNumbers()
	if err != nil {
		return nil, err
	}
	out := make([]SampleRef, 0, len(sync))
	for _, n := range sync {
		i := int(n) - 1
		if i < 0 || uint32(i) >= st.sampleCount {
			continue
		}
		off, sz, err := st.locate(i)
		if err != nil {
			return nil, err
		}
		out = append(out, SampleRef{Index: i, Offset: off, Size: sz})
	}
	return out, nil
}

// SampleRefs builds absolute offsets for each sample using stco/co64, stsc, and stsz.
func SampleRefs(r io.ReaderAt, size int64, trackIndex int) ([]SampleRef, error) {
	st, err := openSampleTable(r, size, trackIndex)
	if err != nil {
		return nil, err
	}
	return st.allRefs()
}

// openSampleTable indexes moov and returns a usable table for trackIndex.
func openSampleTable(r io.ReaderAt, size int64, trackIndex int) (*sampleTable, error) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return nil, err
	}
	tr, err := idx.track(trackIndex)
	if err != nil {
		return nil, err
	}
	if !tr.st.usable() {
		return nil, errs.Unsup("sample table", "missing chunk or sample tables")
	}
	return &tr.st, nil
}

// syncNumbers reads stss, or returns 1..N when stss is absent.
func (st *sampleTable) syncNumbers() ([]uint32, error) {
	if !st.hasStss {
		out := make([]uint32, st.sampleCount)
		for i := range out {
			out[i] = uint32(i + 1)
		}
		return out, nil
	}
	b, err := readPayload(st.r, st.stssOff, st.stssOff+4*int64(st.stssCount), 4*int64(st.sampleCount)+4)
	if err != nil {
		return nil, err
	}
	out := make([]uint32, st.stssCount)
	for i := range out {
		out[i] = uint32(b[4*i])<<24 | uint32(b[4*i+1])<<16 | uint32(b[4*i+2])<<8 | uint32(b[4*i+3])
	}
	return out, nil
}

// allRefs walks every chunk once and returns one SampleRef per sample.
func (st *sampleTable) allRefs() ([]SampleRef, error) {
	sizes, err := st.sampleSizes()
	if err != nil {
		return nil, err
	}
	out := make([]SampleRef, 0, len(sizes))
	si := 0
	for k := 0; k+2 < len(st.stsc) && si < len(sizes); k += 3 {
		fc, spc := st.stsc[k], int(st.stsc[k+1])
		next := st.chunkCount + 1
		if k+5 < len(st.stsc) && st.stsc[k+3] > fc && st.stsc[k+3] <= st.chunkCount+1 {
			next = st.stsc[k+3]
		}
		for c := fc; c < next && si < len(sizes); c++ {
			first, err := st.chunkStart(c - 1)
			if err != nil {
				return nil, err
			}
			off := first
			for j := 0; j < spc && si < len(sizes); j, si = j+1, si+1 {
				out = append(out, SampleRef{Index: si, Offset: int64(off), Size: int64(sizes[si])})
				off += uint64(sizes[si])
			}
		}
	}
	return out, nil
}

// chunkStart reads the file offset of 0-based chunk c.
func (st *sampleTable) chunkStart(c uint32) (uint64, error) {
	if c >= st.chunkCount {
		return 0, errs.Unsup("sample table", "chunk out of range")
	}
	if st.co64 {
		var b [8]byte
		if !readFullAt(st.r, b[:], st.chunkOff+8*int64(c)) {
			return 0, errs.Unsup("sample table", "short co64 read")
		}
		return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
			uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7]), nil
	}
	v, err := st.u32At(st.chunkOff + 4*int64(c))
	return uint64(v), err
}
