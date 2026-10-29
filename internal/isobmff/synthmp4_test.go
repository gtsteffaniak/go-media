package isobmff

import (
	"encoding/binary"
)

// synthBox serializes one ISO BMFF box with a 32-bit size header.
func synthBox(typ string, payload ...[]byte) []byte {
	n := 8
	for _, p := range payload {
		n += len(p)
	}
	out := make([]byte, 8, n)
	binary.BigEndian.PutUint32(out, uint32(n))
	copy(out[4:], typ)
	for _, p := range payload {
		out = append(out, p...)
	}
	return out
}

func u32s(vs ...uint32) []byte {
	b := make([]byte, 4*len(vs))
	for i, v := range vs {
		binary.BigEndian.PutUint32(b[4*i:], v)
	}
	return b
}

// synthMP4Opts controls the generated H.264-in-MP4 layout.
type synthMP4Opts struct {
	Samples         int  // total video samples
	GOP             int  // sync sample every GOP samples (0 = no stss box, all intra)
	SamplesPerChunk int  // stsc samples per chunk
	Co64            bool // use co64 instead of stco
	MoovAfterMdat   bool // place moov after mdat (non-faststart)
	FixedSize       bool // stsz default sample size instead of a table
}

const synthSampleSize = 12
const synthTimescale = 60000
const synthDelta = 1000 // 60 fps

// buildSynthMP4 generates a minimal, structurally valid H.264 MP4 whose
// sample tables scale with opts.Samples. Each sample is a 12-byte AVCC
// payload (4-byte length + 8-byte NAL) that encodes its own sample index,
// so tests can verify which sample was read.
func buildSynthMP4(o synthMP4Opts) []byte {
	if o.SamplesPerChunk <= 0 {
		o.SamplesPerChunk = 1
	}
	n := o.Samples
	ftyp := synthBox("ftyp", []byte("isom"), u32s(0x200), []byte("isomavc1"))
	mdatPayload := make([]byte, n*synthSampleSize)
	for i := 0; i < n; i++ {
		s := mdatPayload[i*synthSampleSize:]
		binary.BigEndian.PutUint32(s, 8)
		s[4] = 0x65 // IDR NAL header
		binary.BigEndian.PutUint32(s[8:], uint32(i))
	}
	mdat := synthBox("mdat", mdatPayload)
	build := func(mdatStart int) []byte {
		dataStart := uint64(mdatStart + 8)
		chunks := (n + o.SamplesPerChunk - 1) / o.SamplesPerChunk
		var chunkBox []byte
		if o.Co64 {
			p := make([]byte, 8+8*chunks)
			binary.BigEndian.PutUint32(p[4:], uint32(chunks))
			for c := 0; c < chunks; c++ {
				binary.BigEndian.PutUint64(p[8+8*c:], dataStart+uint64(c*o.SamplesPerChunk*synthSampleSize))
			}
			chunkBox = synthBox("co64", p)
		} else {
			p := make([]byte, 8+4*chunks)
			binary.BigEndian.PutUint32(p[4:], uint32(chunks))
			for c := 0; c < chunks; c++ {
				binary.BigEndian.PutUint32(p[8+4*c:], uint32(dataStart)+uint32(c*o.SamplesPerChunk*synthSampleSize))
			}
			chunkBox = synthBox("stco", p)
		}
		stscEntries := []uint32{1, uint32(o.SamplesPerChunk), 1}
		cnt := uint32(1)
		if last := n % o.SamplesPerChunk; last != 0 && chunks > 1 {
			stscEntries = append(stscEntries, uint32(chunks), uint32(last), 1)
			cnt = 2
		} else if last != 0 {
			stscEntries[1] = uint32(last)
		}
		stsc := synthBox("stsc", u32s(0, cnt), u32s(stscEntries...))
		var stsz []byte
		if o.FixedSize {
			stsz = synthBox("stsz", u32s(0, synthSampleSize, uint32(n)))
		} else {
			sz := make([]uint32, n)
			for i := range sz {
				sz[i] = synthSampleSize
			}
			stsz = synthBox("stsz", u32s(0, 0, uint32(n)), u32s(sz...))
		}
		stts := synthBox("stts", u32s(0, 1, uint32(n), synthDelta))
		parts := [][]byte{stts}
		if o.GOP > 0 {
			var sync []uint32
			for i := 0; i < n; i += o.GOP {
				sync = append(sync, uint32(i+1))
			}
			parts = append(parts, synthBox("stss", u32s(0, uint32(len(sync))), u32s(sync...)))
		}
		avcC := synthBox("avcC", []byte{1, 66, 0, 30, 0xFF, 0xE1, 0, 4, 0x67, 66, 0, 30, 1, 0, 2, 0x68, 0xCE})
		avc1Payload := make([]byte, 78)
		binary.BigEndian.PutUint16(avc1Payload[6:], 1)   // data reference index
		binary.BigEndian.PutUint16(avc1Payload[24:], 64) // width
		binary.BigEndian.PutUint16(avc1Payload[26:], 48) // height
		binary.BigEndian.PutUint32(avc1Payload[28:], 0x00480000)
		binary.BigEndian.PutUint32(avc1Payload[32:], 0x00480000)
		binary.BigEndian.PutUint16(avc1Payload[40:], 1)
		binary.BigEndian.PutUint16(avc1Payload[74:], 0x18)
		binary.BigEndian.PutUint16(avc1Payload[76:], 0xFFFF)
		stsd := synthBox("stsd", u32s(0, 1), synthBox("avc1", avc1Payload, avcC))
		parts = append([][]byte{stsd}, parts...)
		parts = append(parts, stsc, stsz, chunkBox)
		stbl := synthBox("stbl", parts...)
		minf := synthBox("minf", synthBox("vmhd", u32s(1, 0, 0)), stbl)
		dur := uint32(n * synthDelta)
		mdhd := synthBox("mdhd", u32s(0, 0, 0, synthTimescale, dur), []byte{0x55, 0xC4, 0, 0})
		hdlr := synthBox("hdlr", u32s(0, 0), []byte("vide"), u32s(0, 0, 0), []byte("Video\x00"))
		mdia := synthBox("mdia", mdhd, hdlr, minf)
		tkhdP := make([]byte, 84)
		tkhdP[3] = 3
		binary.BigEndian.PutUint32(tkhdP[12:], 1)
		binary.BigEndian.PutUint32(tkhdP[20:], dur)
		binary.BigEndian.PutUint32(tkhdP[76:], 64<<16)
		binary.BigEndian.PutUint32(tkhdP[80:], 48<<16)
		tkhd := synthBox("tkhd", tkhdP)
		trak := synthBox("trak", tkhd, mdia)
		mvhdP := make([]byte, 100)
		binary.BigEndian.PutUint32(mvhdP[12:], synthTimescale)
		binary.BigEndian.PutUint32(mvhdP[16:], dur)
		binary.BigEndian.PutUint32(mvhdP[20:], 0x00010000)
		binary.BigEndian.PutUint16(mvhdP[24:], 0x0100)
		binary.BigEndian.PutUint32(mvhdP[96:], 2)
		return synthBox("moov", synthBox("mvhd", mvhdP), trak)
	}
	if o.MoovAfterMdat {
		moov := build(len(ftyp))
		return append(append(ftyp, mdat...), moov...)
	}
	moovLen := len(build(0))
	moov := build(len(ftyp) + moovLen)
	return append(append(ftyp, moov...), mdat...)
}

// synthSampleIndex returns the sample index encoded in a synthetic payload.
func synthSampleIndex(p []byte) int {
	if len(p) != synthSampleSize {
		return -1
	}
	return int(binary.BigEndian.Uint32(p[8:]))
}
