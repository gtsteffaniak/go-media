package vc1

import "sort"

type vlcElem struct {
	sym int16
	len int16
}

type vlcCode struct {
	bits int
	code uint32
	sym  int16
}

var (
	acVLC2 [8][]vlcElem
	dcVLC2 [2][2][]vlcElem
	vlc2Done bool
)

func ensureVLC2() {
	if vlc2Done {
		return
	}
	for set := 0; set < 8; set++ {
		acVLC2[set] = buildVLC2Table(acSetCodes(set), acVLCBits)
	}
	dcVLC2[0][0] = buildVLC2Table(dcTabCodes(dcLuma0[:]), dcVLCBits)
	dcVLC2[1][0] = buildVLC2Table(dcTabCodes(dcLuma1[:]), dcVLCBits)
	dcVLC2[0][1] = buildVLC2Table(dcTabCodes(dcChroma0[:]), dcVLCBits)
	dcVLC2[1][1] = buildVLC2Table(dcTabCodes(dcChroma1[:]), dcVLCBits)
	vlc2Done = true
}

func acSetCodes(set int) []vlcCode {
	out := make([]vlcCode, 0, len(acVLCTables[set]))
	for _, e := range acVLCTables[set] {
		if e.bits == 0 {
			continue
		}
		out = append(out, vlcCode{
			bits: int(e.bits),
			code: e.code << (32 - e.bits),
			sym:  int16(e.idx),
		})
	}
	return out
}

func dcTabCodes(tab []dcEntry) []vlcCode {
	out := make([]vlcCode, 0, len(tab))
	for _, e := range tab {
		if e.bits == 0 {
			continue
		}
		out = append(out, vlcCode{
			bits: int(e.bits),
			code: e.code << (32 - e.bits),
			sym:  e.val,
		})
	}
	return out
}

func buildVLC2Table(codes []vlcCode, rootBits int) []vlcElem {
	var buf []vlcCode
	for _, c := range codes {
		if c.bits > rootBits {
			buf = append(buf, c)
		}
	}
	sort.Slice(buf, func(i, j int) bool { return (buf[i].code >> 1) < (buf[j].code >> 1) })
	for _, c := range codes {
		if c.bits > 0 && c.bits <= rootBits {
			buf = append(buf, c)
		}
	}
	table := make([]vlcElem, 0, 1<<rootBits)
	appendTable(&table, buf, rootBits)
	return table
}

func appendTable(table *[]vlcElem, codes []vlcCode, tableBits int) int {
	base := len(*table)
	*table = append(*table, make([]vlcElem, 1<<tableBits)...)
	tbl := (*table)[base : base+(1<<tableBits)]
	for i := 0; i < len(codes); {
		n := codes[i].bits
		code := codes[i].code
		sym := codes[i].sym
		if n <= tableBits {
			j := int(code >> (32 - tableBits))
			nb := 1 << (tableBits - n)
			for k := 0; k < nb; k++ {
				tbl[j+k].len = int16(n)
				tbl[j+k].sym = sym
			}
			i++
			continue
		}
		codePrefix := code >> (32 - tableBits)
		j := int(codePrefix)
		k := i + 1
		subMax := n - tableBits
		for k < len(codes) {
			if codes[k].bits <= tableBits {
				break
			}
			if codes[k].code>>(32-tableBits) != codePrefix {
				break
			}
			if codes[k].bits-tableBits > subMax {
				subMax = codes[k].bits - tableBits
			}
			k++
		}
		subBits := subMax
		if subBits > tableBits {
			subBits = tableBits
		}
		group := make([]vlcCode, k-i)
		for g := range group {
			group[g] = codes[i+g]
			group[g].bits -= tableBits
			group[g].code <<= uint(tableBits)
		}
		subOff := appendTable(table, group, subBits)
		tbl = (*table)[base : base+(1<<tableBits)]
		tbl[j].len = -int16(subBits)
		tbl[j].sym = int16(subOff)
		i = k
	}
	for i := range tbl {
		if tbl[i].len == 0 {
			tbl[i].sym = -1
		}
	}
	return base
}

func (r *bitReader) getVLC2(table []vlcElem, rootBits, maxDepth int) (int16, bool) {
	if len(table) == 0 {
		return -1, false
	}
	idx := int(r.peek(rootBits))
	if idx >= len(table) {
		return -1, false
	}
	sym := table[idx].sym
	n := int(table[idx].len)
	if maxDepth > 1 && n < 0 {
		r.skip(rootBits)
		subBits := -n
		subIdx := int(r.peek(subBits)) + int(sym)
		if subIdx >= len(table) {
			return -1, false
		}
		sym = table[subIdx].sym
		n = int(table[subIdx].len)
		if maxDepth > 2 && n < 0 {
			r.skip(subBits)
			subBits = -n
			subIdx = int(r.peek(subBits)) + int(sym)
			if subIdx >= len(table) {
				return -1, false
			}
			sym = table[subIdx].sym
			n = int(table[subIdx].len)
		}
	}
	if n <= 0 {
		return -1, false
	}
	if r.left() < n {
		return -1, false
	}
	r.skip(n)
	return sym, true
}
