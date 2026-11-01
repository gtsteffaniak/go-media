package heifthumb

import (
	"encoding/binary"
	"io"
	"math"
)

const heicMetaScanLimit = 32 * 1024 * 1024

// PrimaryDisplayRotation reads the primary image item's irot property and returns
// the counter-clockwise rotation in degrees (0, 90, 180, 270) to apply for display,
// matching thumb.rotateImage / ffmpeg autorotate.
func PrimaryDisplayRotation(r io.ReaderAt, size int64) (int, bool) {
	if size <= 0 {
		return 0, false
	}
	scan := size
	if scan > heicMetaScanLimit {
		scan = heicMetaScanLimit
	}
	data := make([]byte, scan)
	n, err := r.ReadAt(data, 0)
	if err != nil && n == 0 {
		return 0, false
	}
	data = data[:n]
	irot, ok := parsePrimaryIrot(data)
	if !ok {
		return 0, false
	}
	return irotDegrees(irot), true
}

// irotDegrees maps HEIF irot (0–3) to counter-clockwise degrees for display.
func irotDegrees(irot uint8) int {
	switch irot & 0x03 {
	case 1:
		return 90
	case 2:
		return 180
	case 3:
		return 270
	default:
		return 0
	}
}

func parsePrimaryIrot(data []byte) (uint8, bool) {
	meta := findBMFFBox(data, "meta")
	if meta == nil {
		return 0, false
	}
	primaryID := parsePITMItemID(findBMFFChildBox(meta, "pitm"))
	if primaryID == 0 {
		return 0, false
	}
	iprp := findBMFFChildBox(meta, "iprp")
	if iprp == nil {
		return 0, false
	}
	ipco := findBMFFChildBox(iprp, "ipco")
	ipma := findBMFFChildBox(iprp, "ipma")
	if ipco == nil || ipma == nil {
		return 0, false
	}
	properties := parseIPCOProperties(ipco)
	indices := parseIPMAPropertyIndices(ipma, primaryID)
	for _, idx := range indices {
		if idx == 0 || int(idx) > len(properties) {
			continue
		}
		prop := properties[idx-1]
		if prop.typ == "irot" {
			return prop.irot & 0x03, true
		}
	}
	return 0, false
}

const maxBMFFRecursionDepth = 32

func findBMFFBox(data []byte, typ string) []byte {
	return findBMFFBoxRange(data, 0, len(data), typ, 0)
}

func findBMFFBoxRange(data []byte, start, end int, typ string, depth int) []byte {
	if depth > maxBMFFRecursionDepth {
		return nil
	}
	pos := start
	for pos+8 <= end {
		size, header, ok := bmffBoxHeader(data, pos, end)
		if !ok || size < header {
			break
		}
		boxEnd := pos + size
		if boxEnd > end || boxEnd < pos {
			break
		}
		if string(data[pos+4:pos+8]) == typ {
			return data[pos:boxEnd]
		}
		if isBMFFContainerType(data[pos+4 : pos+8]) {
			if child := findBMFFBoxRange(data, pos+header, boxEnd, typ, depth+1); child != nil {
				return child
			}
		}
		pos = boxEnd
	}
	return nil
}

func findBMFFChildBox(container []byte, typ string) []byte {
	payload := bmffBoxPayload(container)
	if payload == nil {
		return nil
	}
	return findBMFFBoxRange(payload, 0, len(payload), typ, 0)
}

func isBMFFContainerType(typ []byte) bool {
	switch string(typ) {
	case "meta", "iprp", "ipco", "moov", "trak", "mdia", "minf", "stbl", "dinf":
		return true
	default:
		return false
	}
}

func bmffBoxHeader(data []byte, pos, end int) (size int, header int, ok bool) {
	if pos+8 > end {
		return 0, 0, false
	}
	size32 := binary.BigEndian.Uint32(data[pos : pos+4])
	if size32 == 0 {
		return 0, 0, false
	}
	if size32 == 1 {
		if pos+16 > end {
			return 0, 0, false
		}
		size64 := binary.BigEndian.Uint64(data[pos+8 : pos+16])
		if size64 < 16 || size64 > math.MaxInt32 {
			return 0, 0, false
		}
		return int(size64), 16, true
	}
	if size32 < 8 {
		return 0, 0, false
	}
	return int(size32), 8, true
}

func bmffBoxPayload(box []byte) []byte {
	if len(box) < 8 {
		return nil
	}
	typ := string(box[4:8])
	header := 8
	if binary.BigEndian.Uint32(box[0:4]) == 1 {
		header = 16
	}
	if isBMFFFullBoxType(typ) {
		header += 4
	}
	if header > len(box) {
		return nil
	}
	return box[header:]
}

func isBMFFFullBoxType(typ string) bool {
	switch typ {
	case "meta", "pitm", "ispe", "ipma", "hdlr", "iloc", "iinf", "infe":
		return true
	default:
		return false
	}
}

func parsePITMItemID(pitm []byte) uint32 {
	payload := bmffBoxPayload(pitm)
	if len(pitm) < 12 || payload == nil {
		return 0
	}
	version := pitm[8]
	switch version {
	case 0:
		if len(payload) < 2 {
			return 0
		}
		return uint32(binary.BigEndian.Uint16(payload[0:2]))
	default:
		if len(payload) < 4 {
			return 0
		}
		return binary.BigEndian.Uint32(payload[0:4])
	}
}

type ipcoProperty struct {
	typ  string
	irot uint8
}

func parseIPCOProperties(ipco []byte) []ipcoProperty {
	payload := bmffBoxPayload(ipco)
	if payload == nil {
		return nil
	}
	var props []ipcoProperty
	pos := 0
	for pos+8 <= len(payload) {
		size, header, ok := bmffBoxHeader(payload, pos, len(payload))
		if !ok || size < header || pos+size > len(payload) {
			break
		}
		child := payload[pos : pos+size]
		prop := ipcoProperty{typ: string(child[4:8])}
		if prop.typ == "irot" {
			if p := bmffBoxPayload(child); len(p) >= 1 {
				prop.irot = p[0] & 0x03
			}
		}
		props = append(props, prop)
		pos += size
	}
	return props
}

func parseIPMAPropertyIndices(ipma []byte, itemID uint32) []uint16 {
	payload := bmffBoxPayload(ipma)
	if len(ipma) < 12 || payload == nil || len(payload) < 4 {
		return nil
	}
	version := ipma[8]
	wide := ipma[11]&0x01 != 0
	entrySize := 1
	if wide {
		entrySize = 2
	}
	entryCount := int(binary.BigEndian.Uint32(payload[0:4]))
	pos := 4
	for i := 0; i < entryCount && pos < len(payload); i++ {
		var id uint32
		switch version {
		case 0:
			if pos+2 > len(payload) {
				return nil
			}
			id = uint32(binary.BigEndian.Uint16(payload[pos : pos+2]))
			pos += 2
		default:
			if pos+4 > len(payload) {
				return nil
			}
			id = binary.BigEndian.Uint32(payload[pos : pos+4])
			pos += 4
		}
		if pos >= len(payload) {
			return nil
		}
		count := int(payload[pos])
		pos++
		if pos+count*entrySize > len(payload) {
			return nil
		}
		indices := make([]uint16, count)
		for j := 0; j < count; j++ {
			if wide {
				indices[j] = binary.BigEndian.Uint16(payload[pos+2*j:]) & 0x7fff
			} else {
				indices[j] = uint16(payload[pos+j] & 0x7f)
			}
		}
		pos += count * entrySize
		if id != itemID {
			continue
		}
		return indices
	}
	return nil
}
