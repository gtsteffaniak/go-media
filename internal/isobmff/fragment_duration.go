package isobmff

import (
	"encoding/binary"
	"io"
)

// Fragmented MP4 (DASH, camera and screen recorders) often leaves mvhd duration
// at 0 and stores the length in mvex/mehd or in the moof/traf/trun sample
// durations. ffprobe reports those, so go-media must too.
//
// The probe must not grow with recording length. It reads mehd when present;
// otherwise it finds the first and last moof of each track, from the mfra
// index (located through the mfro box in the last 16 bytes) or by hopping
// top-level box headers, and takes each track's length from
// tfdt(last) + trun durations(last) - tfdt(first). Only when a moof has no
// tfdt does it fall back to summing every trun.

const (
	trunDataOffsetPresent      = 0x000001
	trunFirstFlagsPresent      = 0x000004
	trunSampleDurationPresent  = 0x000100
	trunSampleSizePresent      = 0x000200
	trunSampleFlagsPresent     = 0x000400
	trunSampleCTOPresent       = 0x000800
	tfhdBaseDataOffsetPresent  = 0x000001
	tfhdSampleDescPresent      = 0x000002
	tfhdDefaultDurationPresent = 0x000008

	// maxFragmentBoxes caps top-level boxes hopped looking for moofs.
	maxFragmentBoxes = 1 << 20
	// maxMoofBytes caps one moof payload read.
	maxMoofBytes = 16 << 20
	// maxMoofProbe caps how many moofs are parsed from each end while looking
	// for every track's first and last fragment.
	maxMoofProbe = 64
)

// fragTrack is one track's contribution from a single moof.
type fragTrack struct {
	tfdt    uint64
	hasTfdt bool
	units   uint64 // sum of trun sample durations in this moof
}

// fragmentDurationSeconds returns the length of a fragmented MP4 in seconds.
// movieTimescale is the mvhd timescale; timescales maps track ID to media
// timescale. ok is false when no fragment length can be derived.
func fragmentDurationSeconds(r io.ReaderAt, size int64, movieTimescale uint32, timescales map[uint32]uint32) (float64, bool) {
	moovS, moovE, ok := findTopLevel(r, size, [4]byte{'m', 'o', 'o', 'v'})
	if !ok {
		return 0, false
	}
	mehd, trex := readMvex(r, size, moovS, moovE)
	if mehd > 0 && movieTimescale > 0 {
		return float64(mehd) / float64(movieTimescale), true
	}
	if len(timescales) == 0 {
		return 0, false
	}

	first, last, ok := moofsFromMfra(r, size, timescales)
	var moofs []int64
	if !ok {
		moofs, ok = hopMoofs(r, size, moovE)
		if !ok || len(moofs) == 0 {
			return 0, false
		}
		first, last = moofEnds(r, size, moofs, timescales, trex)
	}

	units := make(map[uint32]uint64)
	exact := true
	for id := range timescales {
		f, fok := first[id]
		l, lok := last[id]
		if !fok || !lok {
			continue
		}
		fa, ok1 := parseMoofAt(r, size, f, trex)
		la, ok2 := parseMoofAt(r, size, l, trex)
		if !ok1 || !ok2 || !fa[id].hasTfdt || !la[id].hasTfdt || la[id].tfdt < fa[id].tfdt {
			exact = false
			break
		}
		units[id] = la[id].tfdt + la[id].units - fa[id].tfdt
	}
	if !exact {
		// No tfdt: the only source of time is every trun, so sum them all.
		if moofs == nil {
			if moofs, ok = hopMoofs(r, size, moovE); !ok {
				return 0, false
			}
		}
		units = sumAllMoofs(r, size, moofs, trex)
	}

	var best float64
	found := false
	for id, u := range units {
		ts := timescales[id]
		if ts == 0 || u == 0 {
			continue
		}
		if sec := float64(u) / float64(ts); !found || sec > best {
			best, found = sec, true
		}
	}
	return best, found
}

// findTopLevel returns the payload bounds of the first top-level box of typ.
func findTopLevel(r io.ReaderAt, size int64, typ [4]byte) (int64, int64, bool) {
	var hdr [16]byte
	var off int64
	for i := 0; off+8 <= size && i < maxTopLevelBoxes; i++ {
		t, hl, bs, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			return 0, 0, false
		}
		if t == typ {
			return off + hl, off + bs, true
		}
		off += bs
	}
	return 0, 0, false
}

// readMvex returns the mehd fragment duration (0 when absent) and the trex
// default sample duration per track.
func readMvex(r io.ReaderAt, size, s, e int64) (uint64, map[uint32]uint32) {
	trex := make(map[uint32]uint32)
	var mehd uint64
	_ = walkBoxes(r, size, s, e, func(typ [4]byte, ps, pe int64) error {
		if typ != [4]byte{'m', 'v', 'e', 'x'} {
			return nil
		}
		return walkBoxes(r, size, ps, pe, func(typ [4]byte, cs, ce int64) error {
			switch string(typ[:]) {
			case "mehd":
				b, err := readPayload(r, cs, min(ce, cs+12), 12)
				if err != nil || len(b) < 8 {
					return nil
				}
				if b[0] == 1 && len(b) >= 12 {
					mehd = binary.BigEndian.Uint64(b[4:12])
				} else {
					mehd = uint64(binary.BigEndian.Uint32(b[4:8]))
				}
			case "trex":
				b, err := readPayload(r, cs, min(ce, cs+16), 16)
				if err != nil || len(b) < 16 {
					return nil
				}
				trex[binary.BigEndian.Uint32(b[4:8])] = binary.BigEndian.Uint32(b[12:16])
			}
			return nil
		})
	})
	return mehd, trex
}

// moofsFromMfra reads the first and last moof offset of each track from the
// mfra/tfra index. Only the tfra headers and two entries per track are read.
func moofsFromMfra(r io.ReaderAt, size int64, timescales map[uint32]uint32) (first, last map[uint32]int64, ok bool) {
	if size < 16 {
		return nil, nil, false
	}
	var tail [16]byte
	if !readFullAt(r, tail[:], size-16) || string(tail[4:8]) != "mfro" || binary.BigEndian.Uint32(tail[:4]) != 16 {
		return nil, nil, false
	}
	mfraSize := int64(binary.BigEndian.Uint32(tail[12:16]))
	start := size - mfraSize
	if mfraSize < 16 || start < 0 {
		return nil, nil, false
	}
	var hdr [16]byte
	typ, hl, bs, err := readBoxHeader(r, size, start, size, hdr[:])
	if err != nil || typ != [4]byte{'m', 'f', 'r', 'a'} {
		return nil, nil, false
	}
	first = make(map[uint32]int64)
	last = make(map[uint32]int64)
	_ = walkBoxes(r, size, start+hl, start+bs, func(typ [4]byte, s, e int64) error {
		if typ != [4]byte{'t', 'f', 'r', 'a'} {
			return nil
		}
		var h [16]byte
		if e-s < 16 || !readFullAt(r, h[:], s) {
			return nil
		}
		id := binary.BigEndian.Uint32(h[4:8])
		lens := binary.BigEndian.Uint32(h[8:12])
		n := int64(binary.BigEndian.Uint32(h[12:16]))
		tw := int64(4)
		if h[0] == 1 {
			tw = 8
		}
		entry := 2*tw + int64((lens>>4)&3+1) + int64((lens>>2)&3+1) + int64(lens&3+1)
		if n == 0 || s+16+n*entry > e {
			return nil
		}
		at := func(i int64) (int64, bool) {
			b := make([]byte, tw)
			if !readFullAt(r, b, s+16+i*entry+tw) {
				return 0, false
			}
			if tw == 8 {
				return int64(binary.BigEndian.Uint64(b)), true
			}
			return int64(binary.BigEndian.Uint32(b)), true
		}
		f, ok1 := at(0)
		l, ok2 := at(n - 1)
		if ok1 && ok2 {
			first[id], last[id] = f, l
		}
		return nil
	})
	for id := range timescales {
		if _, ok := last[id]; ok {
			return first, last, true
		}
	}
	return nil, nil, false
}

// hopMoofs lists moof offsets by reading top-level box headers only.
func hopMoofs(r io.ReaderAt, size, from int64) ([]int64, bool) {
	var hdr [16]byte
	var out []int64
	off := from
	for i := 0; off+8 <= size; i++ {
		if i >= maxFragmentBoxes {
			return nil, false
		}
		t, _, bs, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			// A truncated tail (interrupted recording) keeps what was found.
			break
		}
		if t == [4]byte{'m', 'o', 'o', 'f'} {
			out = append(out, off)
		}
		off += bs
	}
	return out, true
}

// moofEnds parses moofs from each end of the list until every track has a
// first and a last fragment, or maxMoofProbe moofs were read from that end.
func moofEnds(r io.ReaderAt, size int64, moofs []int64, timescales map[uint32]uint32, trex map[uint32]uint32) (first, last map[uint32]int64) {
	first = make(map[uint32]int64)
	last = make(map[uint32]int64)
	scan := func(dst map[uint32]int64, idx func(int) int) {
		for i := 0; i < len(moofs) && i < maxMoofProbe && len(dst) < len(timescales); i++ {
			off := moofs[idx(i)]
			tr, ok := parseMoofAt(r, size, off, trex)
			if !ok {
				continue
			}
			for id := range tr {
				if _, seen := dst[id]; !seen {
					if _, known := timescales[id]; known {
						dst[id] = off
					}
				}
			}
		}
	}
	scan(first, func(i int) int { return i })
	scan(last, func(i int) int { return len(moofs) - 1 - i })
	return first, last
}

// sumAllMoofs sums every trun duration per track (used only without tfdt).
func sumAllMoofs(r io.ReaderAt, size int64, moofs []int64, trex map[uint32]uint32) map[uint32]uint64 {
	sums := make(map[uint32]uint64)
	for _, off := range moofs {
		tr, ok := parseMoofAt(r, size, off, trex)
		if !ok {
			continue
		}
		for id, f := range tr {
			sums[id] += f.units
		}
	}
	return sums
}

// parseMoofAt reads the moof box at off and returns each track's tfdt and
// summed trun durations.
func parseMoofAt(r io.ReaderAt, size, off int64, trex map[uint32]uint32) (map[uint32]fragTrack, bool) {
	var hdr [16]byte
	typ, hl, bs, err := readBoxHeader(r, size, off, size, hdr[:])
	if err != nil || typ != [4]byte{'m', 'o', 'o', 'f'} {
		return nil, false
	}
	b, err := readPayload(r, off+hl, off+bs, maxMoofBytes)
	if err != nil {
		return nil, false
	}
	out := make(map[uint32]fragTrack)
	eachBox(b, func(typ string, traf []byte) {
		if typ != "traf" {
			return
		}
		var id, def uint32
		var have bool
		eachBox(traf, func(typ string, p []byte) {
			switch typ {
			case "tfhd":
				if len(p) < 8 {
					return
				}
				flags := binary.BigEndian.Uint32(p[:4]) & 0xffffff
				id, have = binary.BigEndian.Uint32(p[4:8]), true
				def = trex[id]
				o := 8
				if flags&tfhdBaseDataOffsetPresent != 0 {
					o += 8
				}
				if flags&tfhdSampleDescPresent != 0 {
					o += 4
				}
				if flags&tfhdDefaultDurationPresent != 0 && len(p) >= o+4 {
					def = binary.BigEndian.Uint32(p[o:])
				}
			case "tfdt":
				if !have || len(p) < 8 {
					return
				}
				f := out[id]
				if f.hasTfdt {
					return
				}
				if p[0] == 1 && len(p) >= 12 {
					f.tfdt = binary.BigEndian.Uint64(p[4:12])
				} else {
					f.tfdt = uint64(binary.BigEndian.Uint32(p[4:8]))
				}
				f.hasTfdt = true
				out[id] = f
			case "trun":
				if !have {
					return
				}
				f := out[id]
				f.units += trunUnits(p, def)
				out[id] = f
			}
		})
	})
	return out, true
}

// trunUnits returns the summed sample durations of one trun payload.
func trunUnits(p []byte, def uint32) uint64 {
	if len(p) < 8 {
		return 0
	}
	flags := binary.BigEndian.Uint32(p[:4]) & 0xffffff
	n := uint64(binary.BigEndian.Uint32(p[4:8]))
	if flags&trunSampleDurationPresent == 0 {
		return n * uint64(def)
	}
	o := 8
	if flags&trunDataOffsetPresent != 0 {
		o += 4
	}
	if flags&trunFirstFlagsPresent != 0 {
		o += 4
	}
	stride := 4
	for _, f := range []uint32{trunSampleSizePresent, trunSampleFlagsPresent, trunSampleCTOPresent} {
		if flags&f != 0 {
			stride += 4
		}
	}
	var sum uint64
	for i := uint64(0); i < n && o+4 <= len(p); i++ {
		sum += uint64(binary.BigEndian.Uint32(p[o:]))
		o += stride
	}
	return sum
}

// eachBox calls fn for each well-formed child box in b (32-bit sizes only,
// which is all moof children use).
func eachBox(b []byte, fn func(typ string, payload []byte)) {
	for len(b) >= 8 {
		n := int(binary.BigEndian.Uint32(b[:4]))
		if n < 8 || n > len(b) {
			return
		}
		fn(string(b[4:8]), b[8:n])
		b = b[n:]
	}
}
