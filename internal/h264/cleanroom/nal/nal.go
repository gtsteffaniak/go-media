// Package nal — parses the H.264 NAL (Network Abstraction Layer):
// splitting an Annex-B bytestream into NAL units, removing emulation-prevention
// (EBSP→RBSP) and parsing the NAL header. Pure Go, no dependencies.
package nal

// Type — the NAL unit type (nal_unit_type, ISO/IEC 14496-10).
type Type uint8

const (
	TypeNonIDR Type = 1 // slice of a coded non-IDR frame
	TypePartA  Type = 2 // partition A
	TypePartB  Type = 3
	TypePartC  Type = 4
	TypeIDR    Type = 5 // slice of an IDR frame (key frame)
	TypeSEI    Type = 6 // supplemental enhancement information
	TypeSPS    Type = 7 // sequence parameter set
	TypePPS    Type = 8 // picture parameter set
	TypeAUD    Type = 9 // access unit delimiter
)

// String gives a short NAL type name for logs.
func (t Type) String() string {
	switch t {
	case TypeNonIDR:
		return "non-IDR slice"
	case TypeIDR:
		return "IDR slice"
	case TypeSEI:
		return "SEI"
	case TypeSPS:
		return "SPS"
	case TypePPS:
		return "PPS"
	case TypeAUD:
		return "AUD"
	default:
		return "NAL"
	}
}

// Unit — a parsed NAL unit.
type Unit struct {
	RefIDC uint8  // nal_ref_idc — reference priority (0 = non-reference)
	Type   Type   // nal_unit_type
	RBSP   []byte // payload without the header byte, emulation-prevention removed
}

// Parse parses one "raw" NAL unit (EBSP with header byte, without start code).
// ok=false on empty input or forbidden_zero_bit != 0.
func Parse(ebsp []byte) (Unit, bool) {
	if len(ebsp) < 1 {
		return Unit{}, false
	}
	hdr := ebsp[0]
	if hdr&0x80 != 0 { // forbidden_zero_bit must be 0
		return Unit{}, false
	}
	return Unit{
		RefIDC: (hdr >> 5) & 0x03,
		Type:   Type(hdr & 0x1f),
		RBSP:   EBSPtoRBSP(ebsp[1:]),
	}, true
}

// EBSPtoRBSP removes emulation_prevention_three_byte: inside a NAL the sequence
// 00 00 03 encodes 00 00, so the body contains no accidental start codes. The
// byte 0x03 is dropped if it follows two zeros and precedes a byte <= 0x03.
func EBSPtoRBSP(ebsp []byte) []byte {
	rbsp := make([]byte, 0, len(ebsp))
	zeros := 0
	for i := 0; i < len(ebsp); i++ {
		b := ebsp[i]
		if zeros >= 2 && b == 0x03 && (i+1 >= len(ebsp) || ebsp[i+1] <= 0x03) {
			zeros = 0 // emulation prevention byte — skip it
			continue
		}
		rbsp = append(rbsp, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return rbsp
}
