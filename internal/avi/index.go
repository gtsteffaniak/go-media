package avi

import (
	"encoding/binary"
)

// parseSuperIndex reads an OpenDML 'indx' super index and merges standard indexes.
func (f *fileIndex) parseSuperIndex(body []byte, chunkOff int64) error {
	if len(body) < 24 || f.moviOff == 0 {
		return nil
	}
	// OpenDML index header
	_ = binary.LittleEndian.Uint16(body[0:2]) // wLongsPerEntry
	subType := body[2]
	indexType := body[3]
	nEntries := binary.LittleEndian.Uint32(body[4:8])
	// dwChunkId := body[8:12]
	if indexType != 1 || subType != 0 {
		return nil
	}
	const entrySize = 16
	headerSize := 32
	if len(body) < headerSize+int(nEntries)*entrySize {
		return nil
	}
	for i := uint32(0); i < nEntries; i++ {
		base := headerSize + int(i)*entrySize
		qwOffset := binary.LittleEndian.Uint64(body[base : base+8])
		dwSize := binary.LittleEndian.Uint32(body[base+8 : base+12])
		_ = binary.LittleEndian.Uint32(body[base+12 : base+16]) // dwDuration
		if qwOffset == 0 || dwSize < 32 {
			continue
		}
		ixBody, err := readAt(f.r, int64(qwOffset)+8, int(dwSize))
		if err != nil {
			continue
		}
		_ = chunkOff
		f.parseStandardIndex(ixBody)
	}
	return nil
}

// parseStandardIndex reads an OpenDML standard index (ix## or indx with type 0).
func (f *fileIndex) parseStandardIndex(body []byte) {
	if len(body) < 32 {
		return
	}
	indexType := body[3]
	if indexType != 0 {
		return
	}
	nEntries := binary.LittleEndian.Uint32(body[4:8])
	var chunkID [4]byte
	copy(chunkID[:], body[8:12])
	st, ok := streamIndexFromChunkID(chunkID)
	if !ok {
		// Some indexes store 00dc while chunk id is in each entry.
		st = 0
	}
	const entrySize = 12
	headerSize := 32
	for i := uint32(0); i < nEntries; i++ {
		base := headerSize + int(i)*entrySize
		if base+entrySize > len(body) {
			break
		}
		dwOffset := binary.LittleEndian.Uint32(body[base : base+4])
		dwSize := binary.LittleEndian.Uint32(body[base+4 : base+8])
		// dwDuration := binary.LittleEndian.Uint32(body[base+8 : base+12])
		keyframe := true
		if dwSize&0x80000000 != 0 {
			keyframe = false
		}
		sz := dwSize & 0x7fffffff
		entStream := st
		if !ok {
			entStream = f.video
		}
		f.index = append(f.index, sampleIndex{
			stream:    entStream,
			keyframe:  keyframe,
			chunkOff:  f.moviOff + int64(dwOffset),
			chunkSize: sz,
		})
	}
}
