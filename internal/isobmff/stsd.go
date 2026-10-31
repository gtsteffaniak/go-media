package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// visualMJPEGSampleEntry returns the mp4v (MJPEG) sample entry for trackIndex, if any.
func visualMJPEGSampleEntry(r io.ReaderAt, size int64, trackIndex int) (*mp4.VisualSampleEntry, bool) {
	rs := ioat.New(r, size)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, false
	}
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil || trackIndex < 0 || trackIndex >= len(traks) {
		return nil, false
	}
	parent := traks[trackIndex]
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, false
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, parent, []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeMp4v()},
	})
	if err != nil || len(bips) == 0 {
		return nil, false
	}
	vse, ok := bips[0].Payload.(*mp4.VisualSampleEntry)
	if !ok || vse == nil {
		return nil, false
	}
	return vse, true
}
