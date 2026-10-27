package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
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
