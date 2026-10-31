package isobmff

import (
	"encoding/binary"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// trakMeta holds per-track handler and sample-entry metadata from moov.
type trakMeta struct {
	HandlerType   string
	HandlerName   string
	Language      string
	SampleFourCC  string
	Width         int
	Height        int
	AttachedCover bool
	// TrackID and Timescale come from tkhd and mdhd. Timescale is the media
	// timescale used by fragment sample durations.
	TrackID   uint32
	Timescale uint32
	// Rotation is the display rotation in degrees from the tkhd matrix, using
	// ffprobe's "rotation" sign convention: -90 means rotate 90 degrees clockwise.
	Rotation int
	// PCMBits and PCMFlags describe a PCM sample entry (lpcm, sowt, twos):
	// the sample size in bits and, for a version 2 lpcm entry, its
	// formatSpecificFlags. Both are 0 when unread.
	PCMBits  int
	PCMFlags uint32
}

// listTrakMeta returns one entry per trak in moov order.
func listTrakMeta(r io.ReaderAt, size int64) ([]trakMeta, error) {
	rs := ioat.New(r, size)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil {
		return nil, err
	}
	out := make([]trakMeta, len(traks))
	for i, trak := range traks {
		meta, err := readTrakMeta(rs, r, size, trak)
		if err != nil {
			return nil, err
		}
		out[i] = meta
	}
	return out, nil
}

// stsdEntryBoxes returns the sample entry boxes under trak/mdia/minf/stbl/stsd
// in file order, read from box headers only. Any structural error fails the
// whole lookup.
func stsdEntryBoxes(r io.ReaderAt, size int64, trak *mp4.BoxInfo) ([]mp4.BoxInfo, error) {
	var out []mp4.BoxInfo
	err := eachChildBox(r, size, int64(trak.Offset+trak.HeaderSize), int64(trak.Offset+trak.Size), func(typ [4]byte, off, hl, bs int64) error {
		if typ != [4]byte{'m', 'd', 'i', 'a'} {
			return nil
		}
		return eachChildBox(r, size, off+hl, off+bs, func(typ [4]byte, off, hl, bs int64) error {
			if typ != [4]byte{'m', 'i', 'n', 'f'} {
				return nil
			}
			return eachChildBox(r, size, off+hl, off+bs, func(typ [4]byte, off, hl, bs int64) error {
				if typ != [4]byte{'s', 't', 'b', 'l'} {
					return nil
				}
				return eachChildBox(r, size, off+hl, off+bs, func(typ [4]byte, off, hl, bs int64) error {
					if typ != [4]byte{'s', 't', 's', 'd'} {
						return nil
					}
					// stsd is a full box: version/flags and entry_count precede the entries.
					if bs-hl < 8 {
						return errs.Unsup("mp4 index", "short stsd")
					}
					return eachChildBox(r, size, off+hl+8, off+bs, func(typ [4]byte, off, hl, bs int64) error {
						out = append(out, mp4.BoxInfo{
							Type:       mp4.BoxType(typ),
							Offset:     uint64(off),
							Size:       uint64(bs),
							HeaderSize: uint64(hl),
						})
						return nil
					})
				})
			})
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// eachChildBox calls fn for each box header in [start,end), with the box's
// file offset, header length and total size.
func eachChildBox(r io.ReaderAt, size, start, end int64, fn func(typ [4]byte, off, hdrLen, boxSize int64) error) error {
	var hdr [16]byte
	for off := start; off+8 <= end; {
		typ, hl, bs, err := readBoxHeader(r, size, off, end, hdr[:])
		if err != nil {
			return err
		}
		if err := fn(typ, off, hl, bs); err != nil {
			return err
		}
		off += bs
	}
	return nil
}

func readTrakMeta(rs io.ReadSeeker, r io.ReaderAt, size int64, trak *mp4.BoxInfo) (trakMeta, error) {
	var meta trakMeta
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return meta, err
	}
	stsdPaths := []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeMp4v()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeAvc1()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeMp4a()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeWvtt()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeStpp()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeSbtt()},
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, trak, append([]mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeHdlr()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()},
		{mp4.BoxTypeTkhd()},
	}, stsdPaths...))
	if err != nil {
		return meta, err
	}
	for _, bip := range bips {
		switch bip.Info.Type {
		case mp4.BoxTypeHdlr():
			h := bip.Payload.(*mp4.Hdlr)
			meta.HandlerType = string(h.HandlerType[:])
			meta.HandlerName = strings.TrimSpace(h.Name)
		case mp4.BoxTypeMdhd():
			m := bip.Payload.(*mp4.Mdhd)
			meta.Language = iso639ToString(m.Language)
			meta.Timescale = m.Timescale
		case mp4.BoxTypeTkhd():
			tk := bip.Payload.(*mp4.Tkhd)
			meta.TrackID = tk.TrackID
			meta.Rotation = rotationFromMatrix(tk.Matrix)
			if meta.Width == 0 {
				meta.Width = int(tk.Width >> 16)
			}
			if meta.Height == 0 {
				meta.Height = int(tk.Height >> 16)
			}
		default:
			if meta.SampleFourCC == "" {
				meta.SampleFourCC = bip.Info.Type.String()
				if vse, ok := bip.Payload.(*mp4.VisualSampleEntry); ok {
					meta.Width = int(vse.Width)
					meta.Height = int(vse.Height)
				}
			}
		}
	}
	if meta.SampleFourCC == "" {
		// Sample entries without a payload decoder (AV1, ProRes, PCM, ...) are
		// found by a direct scan of the stsd entry headers, which does not
		// decode their contents. Entries come back in stsd order: tx3g wins
		// if present, otherwise the first candidate entry in stsd order.
		// Any walk error leaves SampleFourCC unset, as the abema lookups did.
		if entries, err := stsdEntryBoxes(r, size, trak); err == nil {
			for _, e := range entries {
				if e.Type == mp4.StrToBoxType("tx3g") {
					meta.SampleFourCC = "tx3g"
					break
				}
			}
			if meta.SampleFourCC == "" {
				candidates := sampleEntryCandidates(meta.HandlerType)
				for i := range entries {
					if !slices.ContainsFunc(candidates, func(fc string) bool {
						return mp4.StrToBoxType(fc) == entries[i].Type
					}) {
						continue
					}
					meta.SampleFourCC = entries[i].Type.String()
					if detect.IsMP4PCMFourCC(meta.SampleFourCC) {
						meta.PCMBits, meta.PCMFlags = readPCMSampleEntry(rs, &entries[i])
					}
					break
				}
			}
		}
	}
	meta.AttachedCover = isAttachedCoverSampleEntry(meta.SampleFourCC, meta.HandlerType)
	return meta, nil
}

// readPCMSampleEntry reads the sample size and LPCM flags from a sound sample
// entry box. Version 0/1 entries give the 16-bit samplesize field; version 2
// entries give constBitsPerChannel and formatSpecificFlags. It returns zeros
// when the entry is too short or unreadable.
func readPCMSampleEntry(r io.ReadSeeker, box *mp4.BoxInfo) (int, uint32) {
	const v2Len = 56 // SampleEntry + v0 sound fields + v2 extension
	payload := box.Size - box.HeaderSize
	if payload < 28 {
		return 0, 0
	}
	n := int64(v2Len)
	if uint64(n) > payload {
		n = int64(payload)
	}
	var buf [v2Len]byte
	if _, err := r.Seek(int64(box.Offset+box.HeaderSize), io.SeekStart); err != nil {
		return 0, 0
	}
	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		return 0, 0
	}
	version := binary.BigEndian.Uint16(buf[8:])
	if version == 2 {
		if n < v2Len {
			return 0, 0
		}
		return int(binary.BigEndian.Uint32(buf[48:])), binary.BigEndian.Uint32(buf[52:])
	}
	return int(binary.BigEndian.Uint16(buf[18:])), 0
}

// isAttachedCoverSampleEntry reports whether a vide or pict trak holds a still
// image (cover art) rather than motion video. FourCCs are compared trimmed.
func isAttachedCoverSampleEntry(fourCC, handler string) bool {
	if handler != "vide" && handler != "pict" {
		return false
	}
	switch strings.TrimSpace(fourCC) {
	case "jpeg", "jpg", "png", "bmp":
		return true
	default:
		return false
	}
}

// iso639ToString decodes the packed ISO 639-2/T language code from mdhd.
func iso639ToString(lang [3]byte) string {
	var b []byte
	for i := 0; i < 3; i++ {
		if lang[i] == 0 {
			break
		}
		b = append(b, lang[i]+0x60)
	}
	return string(b)
}

func isSubtitleHandler(handler string) bool {
	switch handler {
	case "sbtl", "subt", "text":
		return true
	default:
		return false
	}
}

func subtitleCodecFromFourCC(fourCC string) string {
	switch strings.TrimSpace(fourCC) {
	case "wvtt":
		return "wvtt"
	case "stpp":
		return "stpp"
	case "tx3g":
		return "tx3g"
	case "sbtt":
		return "sbtt"
	default:
		return strings.TrimSpace(fourCC)
	}
}

// rotationFromMatrix derives the display rotation from a tkhd transformation
// matrix (16.16 fixed point, a=m[0], b=m[1]). It matches ffmpeg's
// av_display_rotation_get: the result is -atan2(b, a) rounded to a multiple of 90
// and normalized to (-180, 180].
func rotationFromMatrix(m [9]int32) int {
	a := float64(m[0]) / 65536
	b := float64(m[1]) / 65536
	if a == 0 && b == 0 {
		return 0
	}
	deg := -math.Atan2(b, a) * 180 / math.Pi
	r := int(math.Round(deg/90)) * 90
	r = ((r % 360) + 360) % 360
	if r > 180 {
		r -= 360
	}
	return r
}

// sampleEntryCandidates lists the named sample entry types to look for, given the
// track handler.
func sampleEntryCandidates(handler string) []string {
	var out []string
	for _, fc := range detect.MP4SampleEntryFourCCs() {
		if _, ok := detect.MP4VideoCodecName(fc); ok && handler == "vide" {
			out = append(out, fc)
		}
		if _, ok := detect.MP4AudioCodecName(fc); (ok || detect.IsMP4PCMFourCC(fc)) && handler == "soun" {
			out = append(out, fc)
		}
	}
	return out
}
