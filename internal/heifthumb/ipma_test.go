package heifthumb

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// TestParseIPMAPropertyIndicesWideEntries verifies ipma full-box flags bit 0 selects
// 16-bit association entries (essential + 15-bit property index).
func TestParseIPMAPropertyIndicesWideEntries(t *testing.T) {
	// One entry: item 1, two associations — ispe essential index 1, irot index 130.
	const propIrot = uint16(130)
	payload := make([]byte, 0, 16)
	payload = append(payload, 0, 0, 0, 1) // entry_count
	payload = append(payload, 0, 1)       // item_id
	payload = append(payload, 2)          // association_count
	payload = append(payload, 0x80, 0x01) // essential, property 1
	var irotEntry [2]byte
	binary.BigEndian.PutUint16(irotEntry[:], propIrot)
	payload = append(payload, irotEntry[:]...)

	ipma := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(ipma[0:4], uint32(len(ipma)))
	copy(ipma[4:8], "ipma")
	ipma[8] = 0     // version
	ipma[11] = 0x01 // flags: wide entries
	copy(ipma[12:], payload)

	got, err := parseIPMAPropertyIndices(ipma, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != propIrot {
		t.Fatalf("parseIPMAPropertyIndices() = %v, want [1 %d]", got, propIrot)
	}
}

func TestParseIPMAPropertyIndicesExtendedBoxSize(t *testing.T) {
	payload := make([]byte, 0, 12)
	payload = append(payload, 0, 0, 0, 1)
	payload = append(payload, 0, 1)
	payload = append(payload, 1)
	payload = append(payload, 0x81) // essential, property 1

	ipma := make([]byte, 20+len(payload))
	binary.BigEndian.PutUint32(ipma[0:4], 1)
	binary.BigEndian.PutUint64(ipma[8:16], uint64(len(ipma)))
	copy(ipma[4:8], "ipma")
	ipma[16] = 0    // version (after 16-byte header + full-box)
	ipma[19] = 0x00 // flags: narrow entries
	copy(ipma[20:], payload)

	got, err := parseIPMAPropertyIndices(ipma, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("parseIPMAPropertyIndices() = %v, want [1]", got)
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

	got, err := parseIPMAPropertyIndices(ipma, 49)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("parseIPMAPropertyIndices() = %v, want [3 4]", got)
	}
}

// TestParseIPMAPropertyIndicesMalformed checks that an ipma box cut short
// is an error, while an item without an entry is not.
func TestParseIPMAPropertyIndicesMalformed(t *testing.T) {
	// Header and full-box fields only: no entry count.
	if _, err := parseIPMAPropertyIndices(synthFullBox("ipma", 0, nil), 1); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("no entry count: err = %v, want unsupported", err)
	}
	// Two entries declared, one present, item 7 not among them.
	one := append(u32(2), u16(1)...)
	one = append(one, 1, 0x81)
	if _, err := parseIPMAPropertyIndices(synthFullBox("ipma", 0, one), 7); !errors.Is(err, errs.Unsupported) {
		t.Fatalf("truncated entries: err = %v, want unsupported", err)
	}
	// A complete box without item 7 is a valid "no properties".
	full := append(u32(1), u16(1)...)
	full = append(full, 1, 0x81)
	got, err := parseIPMAPropertyIndices(synthFullBox("ipma", 0, full), 7)
	if err != nil || got != nil {
		t.Fatalf("missing item: got %v, %v; want nil, nil", got, err)
	}
}

// TestPrimaryTransformMalformedIPMA checks that the error reaches the
// transform instead of an identity result.
func TestPrimaryTransformMalformedIPMA(t *testing.T) {
	ipco := synthBox("ipco", synthIrotBox(1))
	iprp := synthBox("iprp", ipco, synthFullBox("ipma", 0, nil))
	if _, found, err := primaryTransformFrom(synthPITM(1), iprp); err == nil || found {
		t.Fatalf("found = %v, err = %v; want an error", found, err)
	}
}

// TestPrimaryTransformAssociationOrder checks irot and imir are composed in
// ipma order and reported in rotate-then-mirror form.
func TestPrimaryTransformAssociationOrder(t *testing.T) {
	imir := func(axis uint8) []byte { return synthBox("imir", []byte{axis}) }
	cases := []struct {
		name  string
		props [][]byte
		want  Transform
	}{
		{"irot1 imir0", [][]byte{synthIrotBox(1), imir(0)}, Transform{Rotation: 90, Mirror: true, MirrorAxis: 0}},
		{"imir0 irot1", [][]byte{imir(0), synthIrotBox(1)}, Transform{Rotation: 90, Mirror: true, MirrorAxis: 1}},
		{"imir1 irot3", [][]byte{imir(1), synthIrotBox(3)}, Transform{Rotation: 270, Mirror: true, MirrorAxis: 0}},
		{"imir1 irot2", [][]byte{imir(1), synthIrotBox(2)}, Transform{Rotation: 180, Mirror: true, MirrorAxis: 1}},
		{"imir0 imir0", [][]byte{imir(0), imir(0)}, Transform{}},
		{"imir0 imir1", [][]byte{imir(0), imir(1)}, Transform{Rotation: 180}},
	}
	for _, c := range cases {
		var assoc []ipmaAssoc
		for i := range c.props {
			assoc = append(assoc, ipmaAssoc{index: uint8(i + 1)})
		}
		ipma := synthIPMA([]struct {
			itemID uint16
			assoc  []ipmaAssoc
		}{{itemID: 1, assoc: assoc}})
		iprp := synthBox("iprp", synthBox("ipco", c.props...), ipma)
		got, found, err := primaryTransformFrom(synthPITM(1), iprp)
		if err != nil || !found || got != c.want {
			t.Errorf("%s: got %+v, %v, %v; want %+v", c.name, got, found, err, c.want)
		}
	}
}
