package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Fragmented MP4 (DASH, camera and screen recorders) often leaves mvhd duration
// at 0 and stores the length in mvex/mehd or in the moof/traf/trun sample
// durations. ffprobe reports those, so go-media must too.

const (
	trunSampleDurationPresent  = 0x000100
	tfhdDefaultDurationPresent = 0x000008
)

// fragmentDurationSeconds returns the length of a fragmented MP4 in seconds.
// movieTimescale is the mvhd timescale. metas supplies the track id and media
// timescale for each trak. ok is false when no fragment length can be derived.
func fragmentDurationSeconds(r io.ReaderAt, size int64, movieTimescale uint32, metas []trakMeta) (float64, bool) {
	rs := ioat.New(r, size)

	// mehd is the authoritative length when present.
	mehds, err := mp4.ExtractBoxesWithPayload(rs, nil, []mp4.BoxPath{
		{mp4.BoxTypeMoov(), mp4.BoxTypeMvex(), mp4.BoxTypeMehd()},
	})
	if err == nil && movieTimescale > 0 {
		for _, bip := range mehds {
			mh, ok := bip.Payload.(*mp4.Mehd)
			if !ok {
				continue
			}
			units := mh.FragmentDurationV0
			if mh.GetVersion() == 1 {
				units = uint32(mh.FragmentDurationV1)
			}
			if units > 0 {
				return float64(units) / float64(movieTimescale), true
			}
		}
	}

	timescales := make(map[uint32]uint32)
	for _, m := range metas {
		if m.TrackID != 0 && m.Timescale != 0 {
			timescales[m.TrackID] = m.Timescale
		}
	}
	if len(timescales) == 0 {
		return 0, false
	}

	trexDefaults := make(map[uint32]uint32)
	trexs, err := mp4.ExtractBoxesWithPayload(rs, nil, []mp4.BoxPath{
		{mp4.BoxTypeMoov(), mp4.BoxTypeMvex(), mp4.BoxTypeTrex()},
	})
	if err != nil {
		return 0, false
	}
	for _, bip := range trexs {
		if tx, ok := bip.Payload.(*mp4.Trex); ok {
			trexDefaults[tx.TrackID] = tx.DefaultSampleDuration
		}
	}

	// tfhd always precedes the trun boxes of its traf, and ExtractBoxesWithPayload
	// returns boxes in file order, so one pass can track the current track.
	boxes, err := mp4.ExtractBoxesWithPayload(rs, nil, []mp4.BoxPath{
		{mp4.BoxTypeMoof(), mp4.BoxTypeTraf(), mp4.BoxTypeTfhd()},
		{mp4.BoxTypeMoof(), mp4.BoxTypeTraf(), mp4.BoxTypeTrun()},
	})
	if err != nil {
		return 0, false
	}

	sums := make(map[uint32]uint64)
	var curTrack, curDefault uint32
	for _, bip := range boxes {
		switch p := bip.Payload.(type) {
		case *mp4.Tfhd:
			curTrack = p.TrackID
			curDefault = trexDefaults[p.TrackID]
			if p.GetFlags()&tfhdDefaultDurationPresent != 0 {
				curDefault = p.DefaultSampleDuration
			}
		case *mp4.Trun:
			if p.GetFlags()&trunSampleDurationPresent != 0 {
				for _, e := range p.Entries {
					sums[curTrack] += uint64(e.SampleDuration)
				}
			} else {
				sums[curTrack] += uint64(p.SampleCount) * uint64(curDefault)
			}
		}
	}

	var best float64
	found := false
	for track, units := range sums {
		ts, ok := timescales[track]
		if !ok || units == 0 {
			continue
		}
		sec := float64(units) / float64(ts)
		if !found || sec > best {
			best = sec
			found = true
		}
	}
	return best, found
}
