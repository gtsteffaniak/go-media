package heifthumb

import (
	"bytes"
	"encoding/binary"
	"image/jpeg"
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

func synthFullBox(typ string, version byte, payload []byte) []byte {
	size := 12 + len(payload)
	out := make([]byte, size)
	binary.BigEndian.PutUint32(out[0:4], uint32(size))
	copy(out[4:8], typ)
	out[8] = version
	copy(out[12:], payload)
	return out
}

func synthMetaBox(children ...[]byte) []byte {
	var body []byte
	for _, c := range children {
		body = append(body, c...)
	}
	return synthFullBox("meta", 0, body)
}

type synthIrotMeta struct {
	primaryIrot *uint8
	thumbIrot   *uint8
}

// SynthJPEGHEIF builds a minimal HEIF-like file with a JPEG item and thmb reference.
func SynthJPEGHEIF(jpeg []byte, thumbID, primaryID uint16) []byte {
	return synthJPEGHEIF(jpeg, thumbID, primaryID, synthIrotMeta{})
}

// SynthJPEGHEIFWithPrimaryIrot is like SynthJPEGHEIF but adds pitm/iprp/ipco/ipma with a
// known primary-item irot value (0–3), including essential-bit ipma entries like iPhone HEIC.
func SynthJPEGHEIFWithPrimaryIrot(jpeg []byte, thumbID, primaryID uint16, primaryIrot uint8) []byte {
	r := primaryIrot
	return synthJPEGHEIF(jpeg, thumbID, primaryID, synthIrotMeta{primaryIrot: &r})
}

// SynthJPEGHEIFWithItemIrot sets independent irot on primary and thumbnail items.
func SynthJPEGHEIFWithItemIrot(jpeg []byte, thumbID, primaryID uint16, primaryIrot, thumbIrot uint8) []byte {
	p, th := primaryIrot, thumbIrot
	return synthJPEGHEIF(jpeg, thumbID, primaryID, synthIrotMeta{primaryIrot: &p, thumbIrot: &th})
}

func synthJPEGHEIF(jpegBytes []byte, thumbID, primaryID uint16, rot synthIrotMeta) []byte {
	ftyp := synthBox("ftyp", []byte("heic"), u32(0), []byte("heicheic"))
	infePrimary := synthInfe(primaryID, "jpeg")
	infeThumb := synthInfe(thumbID, "jpeg")
	iinfBody := append([]byte{0, 0, 0, 0}, u16(2)...)
	iinfBody = append(iinfBody, infePrimary...)
	iinfBody = append(iinfBody, infeThumb...)
	iinf := synthBox("iinf", iinfBody)
	iref := synthThmbRef(primaryID, thumbID)

	codedW, codedH := 64, 64
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegBytes)); err == nil {
		codedW, codedH = cfg.Width, cfg.Height
	}

	var metaKids [][]byte
	if rot.primaryIrot != nil {
		metaKids = append(metaKids, synthPITM(primaryID))
	}
	metaKids = append(metaKids, iinf, iref)
	if rot.primaryIrot != nil {
		metaKids = append(metaKids, synthIPRP(primaryID, thumbID, codedW, codedH, rot))
	}
	metaKids = append(metaKids, synthIloc(thumbID, 0, int64(len(jpegBytes))))

	meta := synthMetaBox(metaKids...)
	mdat := synthBox("mdat", jpegBytes)
	jpegOff := int64(len(ftyp) + len(meta) + 8)
	iloc := synthIloc(thumbID, jpegOff, int64(len(jpegBytes)))

	metaKids[len(metaKids)-1] = iloc
	meta = synthMetaBox(metaKids...)
	return append(append(ftyp, meta...), mdat...)
}

func synthPITM(primaryID uint16) []byte {
	return synthFullBox("pitm", 0, u16(primaryID))
}

type ipmaAssoc struct {
	index     uint8
	essential bool
}

func synthIPMA(entries []struct {
	itemID uint16
	assoc  []ipmaAssoc
}) []byte {
	var payload []byte
	payload = append(payload, u32(uint32(len(entries)))...)
	for _, e := range entries {
		payload = append(payload, u16(e.itemID)...)
		payload = append(payload, byte(len(e.assoc)))
		for _, a := range e.assoc {
			v := a.index & 0x7f
			if a.essential {
				v |= 0x80
			}
			payload = append(payload, v)
		}
	}
	return synthFullBox("ipma", 0, payload)
}

func synthIspe(w, h int) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(w))
	binary.BigEndian.PutUint32(payload[4:8], uint32(h))
	return synthFullBox("ispe", 0, payload)
}

func synthIrotBox(angle uint8) []byte {
	return synthBox("irot", []byte{angle & 0x03})
}

func synthIPRP(primaryID, thumbID uint16, codedW, codedH int, rot synthIrotMeta) []byte {
	var props []byte
	propIndex := uint8(1)
	appendProp := func(box []byte) uint8 {
		idx := propIndex
		props = append(props, box...)
		propIndex++
		return idx
	}

	primaryIspe := appendProp(synthIspe(codedW, codedH))
	primaryIrot := appendProp(synthIrotBox(*rot.primaryIrot))
	entries := []struct {
		itemID uint16
		assoc  []ipmaAssoc
	}{
		{
			itemID: primaryID,
			assoc: []ipmaAssoc{
				{index: primaryIspe, essential: true},
				{index: primaryIrot, essential: false},
			},
		},
	}

	if rot.thumbIrot != nil {
		thumbIspe := appendProp(synthIspe(64, 64))
		thumbIrotIdx := appendProp(synthIrotBox(*rot.thumbIrot))
		entries = append(entries, struct {
			itemID uint16
			assoc  []ipmaAssoc
		}{
			itemID: thumbID,
			assoc: []ipmaAssoc{
				{index: thumbIspe, essential: true},
				{index: thumbIrotIdx, essential: false},
			},
		})
	}

	ipco := synthBox("ipco", props)
	ipma := synthIPMA(entries)
	return synthBox("iprp", ipco, ipma)
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
	body := []byte{0, 0, 0, 0, 0x44, 0, 0, 1}
	body = append(body, u16(itemID)...)
	body = append(body, u16(0)...)
	body = append(body, u32(0)...)
	body = append(body, u16(1)...)
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
