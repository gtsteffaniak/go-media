package heifthumb

import (
	"encoding/binary"
	"testing"
)

// TestParseIPMAPropertyIndicesWideEntries verifies ipma full-box flags bit 0 selects
// 16-bit association entries (essential + 15-bit property index).
func TestParseIPMAPropertyIndicesWideEntries(t *testing.T) {
	// One entry: item 1, two associations — ispe essential index 1, irot index 130.
	const propIrot = uint16(130)
	payload := make([]byte, 0, 16)
	payload = append(payload, 0, 0, 0, 1) // entry_count
	payload = append(payload, 0, 1)         // item_id
	payload = append(payload, 2)            // association_count
	payload = append(payload, 0x80, 0x01) // essential, property 1
	var irotEntry [2]byte
	binary.BigEndian.PutUint16(irotEntry[:], propIrot)
	payload = append(payload, irotEntry[:]...)

	ipma := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(ipma[0:4], uint32(len(ipma)))
	copy(ipma[4:8], "ipma")
	ipma[8] = 0    // version
	ipma[11] = 0x01 // flags: wide entries
	copy(ipma[12:], payload)

	got := parseIPMAPropertyIndices(ipma, 1)
	if len(got) != 2 || got[0] != 1 || got[1] != propIrot {
		t.Fatalf("parseIPMAPropertyIndices() = %v, want [1 %d]", got, propIrot)
	}
}

func TestParseIPMAPropertyIndicesNarrowEntries(t *testing.T) {
	payload := make([]byte, 0, 12)
	payload = append(payload, 0, 0, 0, 1)
	payload = append(payload, 0, 49)
	payload = append(payload, 2)
	payload = append(payload, 0x83, 0x84) // essential+3, index 4 with essential bit on second

	ipma := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(ipma[0:4], uint32(len(ipma)))
	copy(ipma[4:8], "ipma")
	copy(ipma[12:], payload)

	got := parseIPMAPropertyIndices(ipma, 49)
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("parseIPMAPropertyIndices() = %v, want [3 4]", got)
	}
}
