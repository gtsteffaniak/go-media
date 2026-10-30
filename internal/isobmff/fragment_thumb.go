package isobmff

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Fragmented MP4 keyframe seek (frag_keyframe+empty_moov, CMAF, DASH, ismv).
//
// moov holds empty sample tables and the samples live in moof/traf/trun
// boxes. The seek mirrors ffmpeg's fast -ss on such files: it picks the
// nearest sync sample at or before the target decode time. ffmpeg's mov
// demuxer reads the mfra/tfra random access index when present
// (mov_read_mfra, with use_mfra_for) to jump straight to the fragment; this
// does the same, so cost does not grow with file length when mfra exists:
// the mfro tail, a binary search over tfra entries (O(log n) small reads) and
// one moof payload. Without mfra it hops top-level box headers from the end
// of moov and parses moof payloads only (never mdat) until the fragment past
// the target, which is linear in fragment count but reads headers only.
//
// Sample fields follow ISO/IEC 14496-12 §8.8 and ffmpeg's mov_read_trun:
// tfhd overrides trex defaults, first_sample_flags applies to the first
// sample of a trun, the data offset is relative to base_data_offset, the
// moof start (default-base-is-moof, or the first traf) or the end of the
// previous traf's data, and a sample is a keyframe when neither
// sample_is_non_sync_sample nor sample_depends_on==1 is set (ffmpeg's
// MOV_FRAG_SAMPLE_FLAG_IS_NON_SYNC | MOV_FRAG_SAMPLE_FLAG_DEPENDS_YES).

const (
	tfhdDefaultSizePresent  = 0x000010
	tfhdDefaultFlagsPresent = 0x000020
	tfhdDefaultBaseIsMoof   = 0x020000

	sampleFlagNonSync    = 0x00010000
	sampleFlagDependsYes = 0x01000000

	// maxTfraEntries bounds the tfra entry count trusted from its header.
	maxTfraEntries = 1 << 24
	// maxTrunSamples bounds one trun's sample_count.
	maxTrunSamples = 1 << 20
)

// trexDefaults holds one track's mvex/trex sample defaults.
type trexDefaults struct {
	duration, size, flags uint32
}

// fragSample is one sample located inside a moof.
type fragSample struct {
	time      uint64 // decode time in track timescale
	off, size int64
	sync      bool
}

// readTrex returns the trex defaults for trackID; zero values when absent.
func readTrex(r io.ReaderAt, size, moovS, moovE int64, trackID uint32) trexDefaults {
	var d trexDefaults
	_ = walkBoxes(r, size, moovS, moovE, func(typ [4]byte, ps, pe int64) error {
		if typ != [4]byte{'m', 'v', 'e', 'x'} {
			return nil
		}
		return walkBoxes(r, size, ps, pe, func(typ [4]byte, cs, ce int64) error {
			if typ != [4]byte{'t', 'r', 'e', 'x'} {
				return nil
			}
			b, err := readPayload(r, cs, min(ce, cs+24), 24)
			if err != nil || len(b) < 24 || binary.BigEndian.Uint32(b[4:8]) != trackID {
				return nil
			}
			d = trexDefaults{binary.BigEndian.Uint32(b[12:16]), binary.BigEndian.Uint32(b[16:20]), binary.BigEndian.Uint32(b[20:24])}
			return nil
		})
	})
	return d
}

// fragKeyframe returns the sync sample of a fragmented track at or before
// seekPercent of its duration.
func (m *movieIndex) fragKeyframe(tr *trackIndex, seekPercent float64) (fragSample, error) {
	if tr.trackID == 0 || tr.timescale == 0 {
		return fragSample{}, errs.Unsup("mp4 seek", "fragmented MP4: video trak has no track_ID or timescale")
	}
	mh, err := readMovieHeader(m.r, m.size)
	if err != nil || mh.Timescale == 0 || mh.Duration == 0 {
		return fragSample{}, errs.Unsup("mp4 seek", "fragmented MP4: duration unknown (no mehd, mfra or readable moof), seek impossible")
	}
	ticks := float64(mh.Duration) * float64(tr.timescale) / float64(mh.Timescale)
	target := uint64(ticks * normalizeSeekPercent(seekPercent) / 100.0)

	moovS, moovE, ok := findTopLevel(m.r, m.size, [4]byte{'m', 'o', 'o', 'v'})
	if !ok {
		return fragSample{}, errs.Unsup("mp4 seek", "fragmented MP4: moov not found")
	}
	trex := readTrex(m.r, m.size, moovS, moovE, tr.trackID)

	if moofs, ok := m.tfraLookup(tr.trackID, target); ok {
		if s, ok := m.pickInMoofs(moofs, tr.trackID, trex, target); ok {
			return s, nil
		}
	}
	return m.hopToKeyframe(tr.trackID, trex, target)
}

// tfraLookup returns the moof offsets of the last tfra random access point at
// or before target (or the first one when target precedes them all) and of
// the entry after it. tfra times are presentation times (ffmpeg's muxer
// writes the keyframe pts), which lead decode times by the composition
// offset, so the caller re-checks both fragments against decode time.
func (m *movieIndex) tfraLookup(trackID uint32, target uint64) ([]int64, bool) {
	r, size := m.r, m.size
	if size < 16 {
		return nil, false
	}
	var tail [16]byte
	if !readFullAt(r, tail[:], size-16) || string(tail[4:8]) != "mfro" || binary.BigEndian.Uint32(tail[:4]) != 16 {
		return nil, false
	}
	mfraSize := int64(binary.BigEndian.Uint32(tail[12:16]))
	start := size - mfraSize
	if mfraSize < 16 || start < 0 {
		return nil, false
	}
	var hdr [16]byte
	typ, hl, bs, err := readBoxHeader(r, size, start, size, hdr[:])
	if err != nil || typ != [4]byte{'m', 'f', 'r', 'a'} {
		return nil, false
	}
	var moofs []int64
	found := false
	_ = walkBoxes(r, size, start+hl, start+bs, func(typ [4]byte, s, e int64) error {
		if found || typ != [4]byte{'t', 'f', 'r', 'a'} {
			return nil
		}
		var h [16]byte
		if e-s < 16 || !readFullAt(r, h[:], s) || binary.BigEndian.Uint32(h[4:8]) != trackID {
			return nil
		}
		lens := binary.BigEndian.Uint32(h[8:12])
		n := int64(binary.BigEndian.Uint32(h[12:16]))
		tw := int64(4)
		if h[0] == 1 {
			tw = 8
		}
		entry := 2*tw + int64((lens>>4)&3+1) + int64((lens>>2)&3+1) + int64(lens&3+1)
		if n == 0 || n > maxTfraEntries || s+16+n*entry > e {
			return nil
		}
		var buf [16]byte
		read := func(i int64) (t uint64, off int64, ok bool) {
			b := buf[:2*tw]
			if !readFullAt(r, b, s+16+i*entry) {
				return 0, 0, false
			}
			if tw == 8 {
				return binary.BigEndian.Uint64(b), int64(binary.BigEndian.Uint64(b[8:])), true
			}
			return uint64(binary.BigEndian.Uint32(b)), int64(binary.BigEndian.Uint32(b[4:])), true
		}
		// Binary search for the last entry with time <= target.
		lo, hi := int64(0), n-1
		best := int64(0)
		for lo <= hi {
			mid := (lo + hi) / 2
			t, _, ok := read(mid)
			if !ok {
				return nil
			}
			if t <= target {
				best, lo = mid, mid+1
			} else {
				hi = mid - 1
			}
		}
		for i := best; i <= best+1 && i < n; i++ {
			if _, off, ok := read(i); ok && off >= 0 && off < size && (len(moofs) == 0 || moofs[len(moofs)-1] != off) {
				moofs = append(moofs, off)
			}
		}
		found = len(moofs) > 0
		return nil
	})
	return moofs, found
}

// pickInMoofs returns the last sync sample at or before target across the
// given moofs, else the first sync sample after it. ok is false when a moof
// has no tfdt (ismv uses tfxd instead): its decode time is then only known by
// summing earlier fragments, so the caller falls back to the header hop.
func (m *movieIndex) pickInMoofs(moofs []int64, trackID uint32, trex trexDefaults, target uint64) (fragSample, bool) {
	var best, after fragSample
	haveBest, haveAfter := false, false
	for _, off := range moofs {
		if !m.moofHasTfdt(off, trackID) {
			return fragSample{}, false
		}
		_, _ = m.moofSamples(off, trackID, trex, 0, func(s fragSample) bool {
			if !s.sync {
				return true
			}
			if s.time <= target {
				best, haveBest = s, true
				return true
			}
			if !haveAfter {
				after, haveAfter = s, true
			}
			return false
		})
	}
	if haveBest {
		return best, true
	}
	return after, haveAfter
}

// hopToKeyframe walks top-level boxes from the end of moov, parsing moof
// payloads only, until a fragment starts past target.
func (m *movieIndex) hopToKeyframe(trackID uint32, trex trexDefaults, target uint64) (fragSample, error) {
	var hdr [16]byte
	var best, after fragSample
	haveBest, haveAfter := false, false
	var t uint64
	moofs := 0
	off := m.moovEnd
	for i := 0; off+8 <= m.size; i++ {
		if i >= maxFragmentBoxes {
			return fragSample{}, errs.Unsup("mp4 seek", "fragmented MP4: more than 1M top-level boxes")
		}
		typ, _, bs, err := readBoxHeader(m.r, m.size, off, m.size, hdr[:])
		if err != nil {
			break // truncated tail: keep what was found, like ffmpeg
		}
		if typ == [4]byte{'m', 'o', 'o', 'f'} {
			moofs++
			var started bool
			t, started = m.moofSamples(off, trackID, trex, t, func(s fragSample) bool {
				if !s.sync {
					return true
				}
				if s.time <= target {
					best, haveBest = s, true
					return true
				}
				if !haveAfter {
					after, haveAfter = s, true
				}
				return false
			})
			if started && haveAfter {
				break
			}
		}
		off += bs
	}
	if haveBest {
		return best, nil
	}
	if haveAfter {
		return after, nil
	}
	if moofs == 0 {
		return fragSample{}, errs.Unsup("mp4 seek", "fragmented MP4: no moof boxes after moov")
	}
	return fragSample{}, errs.Unsup("mp4 seek", fmt.Sprintf("fragmented MP4: no sync sample for track %d in %d moof boxes", trackID, moofs))
}

// moofSamples calls fn for each sample of trackID in the moof at off, in
// decode order, until fn returns false. t is the running decode time used
// when a traf has no tfdt; the updated value is returned. started reports
// whether the moof held any sample of the track.
func (m *movieIndex) moofSamples(off int64, trackID uint32, trex trexDefaults, t uint64, fn func(fragSample) bool) (uint64, bool) {
	var hdr [16]byte
	typ, hl, bs, err := readBoxHeader(m.r, m.size, off, m.size, hdr[:])
	if err != nil || typ != [4]byte{'m', 'o', 'o', 'f'} {
		return t, false
	}
	b, err := readPayload(m.r, off+hl, off+bs, maxMoofBytes)
	if err != nil {
		return t, false
	}
	started := false
	stop := false
	implicit := off // base for a traf without base_data_offset or default-base-is-moof
	eachBox(b, func(typ string, traf []byte) {
		if stop || typ != "traf" {
			return
		}
		var tfhdFlags uint32
		var id uint32
		var have bool
		base := implicit
		dur, sz, fl := trex.duration, trex.size, trex.flags
		var dataEnd int64 = -1
		next := int64(-1)
		eachBox(traf, func(typ string, p []byte) {
			if stop {
				return
			}
			switch typ {
			case "tfhd":
				if len(p) < 8 {
					return
				}
				tfhdFlags = binary.BigEndian.Uint32(p[:4]) & 0xffffff
				id = binary.BigEndian.Uint32(p[4:8])
				have = id == trackID
				o := 8
				if tfhdFlags&tfhdBaseDataOffsetPresent != 0 {
					if len(p) < o+8 {
						have = false
						return
					}
					base = int64(binary.BigEndian.Uint64(p[o:]))
					o += 8
				} else if tfhdFlags&tfhdDefaultBaseIsMoof != 0 {
					base = off
				}
				if tfhdFlags&tfhdSampleDescPresent != 0 {
					o += 4
				}
				for _, f := range []struct {
					bit uint32
					dst *uint32
				}{{tfhdDefaultDurationPresent, &dur}, {tfhdDefaultSizePresent, &sz}, {tfhdDefaultFlagsPresent, &fl}} {
					if tfhdFlags&f.bit != 0 {
						if len(p) < o+4 {
							have = false
							return
						}
						*f.dst = binary.BigEndian.Uint32(p[o:])
						o += 4
					}
				}
			case "tfdt":
				if !have || len(p) < 8 {
					return
				}
				if p[0] == 1 && len(p) >= 12 {
					t = binary.BigEndian.Uint64(p[4:12])
				} else {
					t = uint64(binary.BigEndian.Uint32(p[4:8]))
				}
			case "trun":
				if len(p) < 8 {
					return
				}
				flags := binary.BigEndian.Uint32(p[:4]) & 0xffffff
				n := binary.BigEndian.Uint32(p[4:8])
				o := 8
				pos := next
				if flags&trunDataOffsetPresent != 0 {
					if len(p) < o+4 {
						return
					}
					pos = base + int64(int32(binary.BigEndian.Uint32(p[o:])))
					o += 4
				} else if pos < 0 {
					pos = base
				}
				first, hasFirst := uint32(0), false
				if flags&trunFirstFlagsPresent != 0 {
					if len(p) < o+4 {
						return
					}
					first, hasFirst = binary.BigEndian.Uint32(p[o:]), true
					o += 4
				}
				stride := 0
				for _, f := range []uint32{trunSampleDurationPresent, trunSampleSizePresent, trunSampleFlagsPresent, trunSampleCTOPresent} {
					if flags&f != 0 {
						stride += 4
					}
				}
				if n > maxTrunSamples || (stride > 0 && uint64(n)*uint64(stride) > uint64(len(p)-o)) {
					return
				}
				for i := uint32(0); i < n; i++ {
					d, s, f := dur, sz, fl
					if flags&trunSampleDurationPresent != 0 {
						d = binary.BigEndian.Uint32(p[o:])
						o += 4
					}
					if flags&trunSampleSizePresent != 0 {
						s = binary.BigEndian.Uint32(p[o:])
						o += 4
					}
					if flags&trunSampleFlagsPresent != 0 {
						f = binary.BigEndian.Uint32(p[o:])
						o += 4
					} else if i == 0 && hasFirst {
						f = first
					}
					if flags&trunSampleCTOPresent != 0 {
						o += 4
					}
					if have && !stop {
						started = true
						smp := fragSample{time: t, off: pos, size: int64(s), sync: f&(sampleFlagNonSync|sampleFlagDependsYes) == 0}
						if !fn(smp) {
							stop = true
						}
					}
					if have {
						t += uint64(d)
					}
					pos += int64(s)
				}
				next = pos
				dataEnd = pos
			}
		})
		if dataEnd >= 0 {
			implicit = dataEnd
		}
	})
	return t, started
}

// fragSyncSample reads the payload of the fragmented keyframe for seekPercent.
func (m *movieIndex) fragSyncSample(tr *trackIndex, seekPercent float64) ([]byte, uint64, error) {
	s, err := m.fragKeyframe(tr, seekPercent)
	if err != nil {
		return nil, 0, err
	}
	if s.size <= 0 || s.size > maxMoofBytes*4 || s.off < 0 || s.off+s.size > m.size {
		return nil, 0, errs.Unsup("mp4 seek", fmt.Sprintf("fragmented MP4: keyframe at offset %d size %d outside file", s.off, s.size))
	}
	b, err := ioat.ReadAll(m.r, m.size, s.off, s.size)
	if err != nil {
		return nil, 0, errs.Unsup("mp4 seek", fmt.Sprintf("fragmented MP4: keyframe unreadable: %v", err))
	}
	return b, s.time, nil
}

// moofHasTfdt reports whether the moof at off has a tfdt in trackID's traf.
func (m *movieIndex) moofHasTfdt(off int64, trackID uint32) bool {
	var hdr [16]byte
	typ, hl, bs, err := readBoxHeader(m.r, m.size, off, m.size, hdr[:])
	if err != nil || typ != [4]byte{'m', 'o', 'o', 'f'} {
		return false
	}
	b, err := readPayload(m.r, off+hl, off+bs, maxMoofBytes)
	if err != nil {
		return false
	}
	found := false
	eachBox(b, func(typ string, traf []byte) {
		if typ != "traf" {
			return
		}
		mine := false
		eachBox(traf, func(typ string, p []byte) {
			if typ == "tfhd" && len(p) >= 8 {
				mine = binary.BigEndian.Uint32(p[4:8]) == trackID
			}
			if typ == "tfdt" && mine {
				found = true
			}
		})
	})
	return found
}
