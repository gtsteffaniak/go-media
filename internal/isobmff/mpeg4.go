package isobmff

import (
	"fmt"
	"image"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/go-media/internal/mpeg4"
)

// maxDescriptorDepth bounds nested esds descriptor parsing.
const maxDescriptorDepth = 4

// readDescriptor reads one MPEG-4 Systems descriptor header (ISO/IEC
// 14496-1 §8.3.3: tag byte, then up to four 7-bit size bytes) and returns
// its tag, payload and the bytes after it.
func readDescriptor(b []byte) (tag byte, body, rest []byte, ok bool) {
	if len(b) < 2 {
		return 0, nil, nil, false
	}
	tag = b[0]
	n, i := 0, 1
	for ; i < len(b) && i <= 4; i++ {
		n = n<<7 | int(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			i++
			break
		}
	}
	if n > len(b)-i {
		return 0, nil, nil, false
	}
	return tag, b[i : i+n], b[i+n:], true
}

// esdsConfig returns the objectTypeIndication and DecoderSpecificInfo of an
// esds box payload (version/flags followed by an ES_Descriptor).
func esdsConfig(esds []byte) (oti byte, dsi []byte, err error) {
	bad := func(why string) (byte, []byte, error) {
		return 0, nil, errs.Unsup("thumbnail", "mp4: esds "+why)
	}
	if len(esds) < 4 {
		return bad("box truncated")
	}
	tag, es, _, ok := readDescriptor(esds[4:])
	if !ok || tag != 0x03 || len(es) < 3 {
		return bad("has no ES_Descriptor")
	}
	flags := es[2]
	es = es[3:]
	if flags&0x80 != 0 { // streamDependenceFlag
		if len(es) < 2 {
			return bad("ES_Descriptor truncated")
		}
		es = es[2:]
	}
	if flags&0x40 != 0 { // URL_Flag
		if len(es) < 1 || len(es) < 1+int(es[0]) {
			return bad("ES_Descriptor truncated")
		}
		es = es[1+int(es[0]):]
	}
	if flags&0x20 != 0 { // OCRstreamFlag
		if len(es) < 2 {
			return bad("ES_Descriptor truncated")
		}
		es = es[2:]
	}
	for depth := 0; len(es) > 0 && depth < maxDescriptorDepth; depth++ {
		tag, body, rest, ok := readDescriptor(es)
		if !ok {
			return bad("descriptor truncated")
		}
		es = rest
		if tag != 0x04 {
			continue
		}
		if len(body) < 13 {
			return bad("DecoderConfigDescriptor truncated")
		}
		oti = body[0]
		for d := body[13:]; len(d) > 0; {
			t, b, r, ok := readDescriptor(d)
			if !ok {
				break
			}
			if t == 0x05 {
				return oti, b, nil
			}
			d = r
		}
		return oti, nil, nil
	}
	return bad("has no DecoderConfigDescriptor")
}

// otiName names the objectTypeIndication values ffmpeg maps to mp4v tracks
// (libavformat/isom.c ff_mp4_obj_type) for unsupported messages.
func otiName(oti byte) string {
	switch oti {
	case 0x20:
		return "MPEG-4 Part 2"
	case 0x21:
		return "H.264"
	case 0x60, 0x61, 0x62, 0x63, 0x64, 0x65:
		return "MPEG-2 video"
	case 0x6A:
		return "MPEG-1 video"
	case 0x6C:
		return "JPEG"
	case 0x6E:
		return "JPEG 2000"
	case 0xA3:
		return "VC-1"
	case 0xA4:
		return "Dirac"
	}
	return fmt.Sprintf("unknown objectTypeIndication 0x%02x", oti)
}

// firstMPEG4 returns the first motion-video track with an mp4v sample entry.
func (m *movieIndex) firstMPEG4() (int, bool) {
	for i := range m.tracks {
		tr := &m.tracks[i]
		if string(tr.entry[:]) == "mp4v" && string(tr.handler[:]) == "vide" {
			return i, true
		}
	}
	return 0, false
}

// MPEG4ThumbnailYCbCr decodes the MPEG-4 Part 2 (mp4v, objectTypeIndication
// 0x20) I-VOP nearest seekPercent. Keyframes come from stss (or traf sample
// flags in fragmented files), as for H.264. It returns (nil, nil) when the
// file has no mp4v track, and a descriptive unsupported error when the track
// exists but cannot be decoded, naming the codec the esds declares or the
// MPEG-4 coding tool the decoder lacks.
func (s ThumbSource) MPEG4ThumbnailYCbCr(seekPercent float64) (*h264.ThumbYCbCr, error) {
	track, ok := s.idx.firstMPEG4()
	if !ok {
		return nil, nil
	}
	tr, err := s.idx.track(track)
	if err != nil {
		return nil, thumbErr(err)
	}
	if len(tr.esds) == 0 {
		return nil, errs.Unsup("thumbnail", "mp4: esds box missing from mp4v sample entry")
	}
	oti, dsi, err := esdsConfig(tr.esds)
	if err != nil {
		return nil, err
	}
	if oti != 0x20 {
		return nil, errs.Unsup("thumbnail", "mp4: mp4v track carries "+otiName(oti)+" (go-media has no decoder for this codec)")
	}
	sample, err := s.idx.h264SyncSampleErr(track, seekPercent)
	if err != nil {
		return nil, thumbErr(err)
	}
	pic, err := mpeg4.DecodeKeyframe(dsi, sample)
	if err != nil {
		return nil, err
	}
	return PictureYCbCr(pic), nil
}

// PictureYCbCr wraps a decoded MPEG-4 picture for the thumbnail pipeline.
// Untagged streams are BT.601 limited range, as ffmpeg's mpeg4 decoder
// reports them.
func PictureYCbCr(pic *mpeg4.Picture) *h264.ThumbYCbCr {
	ycc := &image.YCbCr{
		Y: pic.Y, Cb: pic.Cb, Cr: pic.Cr,
		YStride: pic.YStride, CStride: pic.CStride,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, pic.Width, pic.Height),
	}
	cs, rng := yuv.BT601, yuv.LimitedRange
	if pic.ColorDescriptionValid && pic.MatrixCoefficients != 0 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(uint(pic.MatrixCoefficients))
	}
	if pic.FullRange {
		rng = yuv.FullRange
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: cs, RNG: rng}
}
