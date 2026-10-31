package heifthumb

import (
	"encoding/binary"
)

func synthBox(typ string, parts ...[]byte) []byte {
	var body []byte
	for _, p := range parts {
		body = append(body, p...)
	}
	out := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(out[0:4], uint32(8+len(body)))
	copy(out[4:8], typ)
	copy(out[8:], body)
	return out
}

// SynthJPEGHEIF builds a minimal HEIF-like file with a JPEG item and thmb reference.
func SynthJPEGHEIF(jpeg []byte, thumbID, primaryID uint16) []byte {
	ftyp := synthBox("ftyp", []byte("heic"), u32(0), []byte("heicheic"))
	infePrimary := synthInfe(primaryID, "jpeg")
	infeThumb := synthInfe(thumbID, "jpeg")
	iinfBody := append([]byte{0, 0, 0, 0}, u16(2)...)
	iinfBody = append(iinfBody, infePrimary...)
	iinfBody = append(iinfBody, infeThumb...)
	iinf := synthBox("iinf", iinfBody)
	iref := synthThmbRef(primaryID, thumbID)
	placeholder := synthIloc(thumbID, 0, int64(len(jpeg)))
	meta := synthBox("meta", append([]byte{0, 0, 0, 0}, append(iinf, append(iref, placeholder...)...)...))
	mdat := synthBox("mdat", jpeg)
	jpegOff := int64(len(ftyp) + len(meta) + 8)
	iloc := synthIloc(thumbID, jpegOff, int64(len(jpeg)))
	meta = synthBox("meta", append([]byte{0, 0, 0, 0}, append(iinf, append(iref, iloc...)...)...))
	return append(append(ftyp, meta...), mdat...)
}

func synthInfe(id uint16, itemType string) []byte {
	body := append([]byte{2, 0, 0, 0}, u16(id)...)
	body = append(body, []byte(itemType)...)
	return synthBox("infe", body)
}

func synthThmbRef(from, to uint16) []byte {
	body := append(u16(from), make([]byte, 2)...)
	body = append(body, u16(1)...)
	body = append(body, u16(to)...)
	return synthBox("thmb", body)
}

func synthIloc(itemID uint16, offset, length int64) []byte {
	// version 0: offset_size=4 length_size=4, base_offset_size=4
	body := []byte{0, 0, 0, 0, 0x44, 0, 0, 1}
	body = append(body, u16(itemID)...)
	body = append(body, u16(0)...) // data_reference_index
	body = append(body, u32(0)...) // base_offset
	body = append(body, u16(1)...) // one extent
	body = append(body, u32(uint32(offset))...)
	body = append(body, u32(uint32(length))...)
	return synthBox("iloc", body)
}

func u16(v uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return b[:]
}

func u32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}
