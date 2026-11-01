package avi

import (
	"encoding/binary"
	"errors"
	"image"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/mpeg4"
)

// ThumbSource indexes an AVI once for thumbnail generation.
type ThumbSource struct {
	idx *fileIndex
}

// OpenThumbSourceErr parses headers and the sample index for thumbnail decode.
func OpenThumbSourceErr(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (ThumbSource, error) {
	idx, err := openIndex(r, size)
	if err != nil {
		return ThumbSource{}, thumbErr(err)
	}
	return ThumbSource{idx: idx}, nil
}

func thumbErr(err error) error {
	var ue *errs.UnsupportedError
	if errors.As(err, &ue) {
		return errs.Unsup("thumbnail", "avi: "+ue.Reason)
	}
	var pe *errs.ProfileError
	if errors.As(err, &pe) {
		return err
	}
	return errs.Unsup("thumbnail", "avi: "+err.Error())
}

// MJPEGFrame returns JPEG bitstream bytes for the seek target.
func (s ThumbSource) MJPEGFrame(seekPercent float64) ([]byte, bool) {
	vs := s.idx.videoStream()
	if vs == nil || vs.codec != "mjpeg" {
		return nil, false
	}
	ent, ok := s.idx.pickKeyframeBySeek(seekPercent)
	if !ok {
		return nil, false
	}
	data, err := s.idx.readChunkPayload(ent)
	if err != nil {
		return nil, false
	}
	return data, true
}

// H264ThumbnailYCbCr decodes the indexed H.264 keyframe nearest seekPercent.
func (s ThumbSource) H264ThumbnailYCbCr(seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	vs := s.idx.videoStream()
	if vs == nil || vs.codec != "h264" {
		return nil, nil
	}
	sample, avcc, err := s.h264Sample(seekPercent)
	if err != nil || sample == nil {
		return nil, err
	}
	fr, err := h264.DecodeAVCCFrame(avcc, sample, fast)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, errs.UnsupProfile("h264 keyframe", "decoder returned no picture")
	}
	return h264.NewThumbYCbCr(fr), nil
}

// H264ThumbnailRGBA decodes the indexed H.264 keyframe to RGBA.
func (s ThumbSource) H264ThumbnailRGBA(seekPercent float64, fast bool) (*image.RGBA, error) {
	vs := s.idx.videoStream()
	if vs == nil || vs.codec != "h264" {
		return nil, nil
	}
	sample, avcc, err := s.h264Sample(seekPercent)
	if err != nil || sample == nil {
		return nil, err
	}
	img, err := h264.DecodeAVCC(avcc, sample, fast)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, errs.UnsupProfile("h264 keyframe", "decoder returned no picture")
	}
	return img, nil
}

// MPEG4ThumbnailYCbCr decodes an MPEG-4 Part 2 I-VOP nearest seekPercent.
func (s ThumbSource) MPEG4ThumbnailYCbCr(seekPercent float64) (*h264.ThumbYCbCr, error) {
	vs := s.idx.videoStream()
	if vs == nil || vs.codec != "mpeg4" {
		return nil, nil
	}
	ent, ok := s.idx.pickKeyframeBySeek(seekPercent)
	if !ok {
		return nil, errs.Unsup("thumbnail", "avi: no MPEG-4 keyframe in index")
	}
	sample, err := s.idx.readChunkPayload(ent)
	if err != nil {
		return nil, thumbErr(err)
	}
	config := mpeg4ConfigExtra(vs.strf)
	pic, err := mpeg4.DecodeKeyframe(config, sample)
	if err != nil {
		return nil, thumbErr(err)
	}
	return isobmff.PictureYCbCr(pic), nil
}

// UnsupportedReason names the video codec when no decoder matched.
func (s ThumbSource) UnsupportedReason() error {
	vs := s.idx.videoStream()
	if vs == nil {
		return errs.Unsup("thumbnail", "avi: no video stream")
	}
	return errs.Unsup("thumbnail", "avi: video codec "+vs.codec+" not supported (go-media has no decoder for this codec)")
}

func (s ThumbSource) h264Sample(seekPercent float64) (sample []byte, avcc *mp4.AVCDecoderConfiguration, err error) {
	ent, ok := s.idx.pickKeyframeBySeek(seekPercent)
	if !ok {
		return nil, nil, errs.Unsup("thumbnail", "avi: no H.264 keyframe in index")
	}
	sample, err = s.idx.readChunkPayload(ent)
	if err != nil {
		return nil, nil, thumbErr(err)
	}
	vs := s.idx.videoStream()
	avcc = parseAVCCExtra(vs.strf)
	if looksAnnexB(sample) {
		sample = annexBToAVCC(sample)
	}
	return sample, avcc, nil
}

func (f *fileIndex) pickKeyframeBySeek(seekPercent float64) (sampleIndex, bool) {
	if seekPercent <= 0 {
		seekPercent = 10
	}
	var target uint32
	if f.avih.totalFrames > 0 {
		// Frame index at the seek time (frame n starts at n/fps), matching
		// where ffmpeg -ss lands; pickKeyframe then takes the keyframe at or
		// before it.
		target = uint32(float64(f.avih.totalFrames) * seekPercent / 100.0)
		if target >= f.avih.totalFrames {
			target = f.avih.totalFrames - 1
		}
	}
	return f.pickKeyframe(target)
}

func mpeg4ConfigExtra(strf []byte) []byte {
	if len(strf) <= 40 {
		return nil
	}
	biSize := binary.LittleEndian.Uint32(strf[0:4])
	if int(biSize) >= len(strf) {
		return nil
	}
	return strf[biSize:]
}

func parseAVCCExtra(strf []byte) *mp4.AVCDecoderConfiguration {
	if len(strf) <= 40 {
		return nil
	}
	biSize := binary.LittleEndian.Uint32(strf[0:4])
	if int(biSize) >= len(strf) {
		return nil
	}
	// mp4.Unmarshal needs box context that raw extradata lacks, so the
	// record is parsed directly.
	return h264.ParseAVCConfigRecord(strf[biSize:])
}

func looksAnnexB(b []byte) bool {
	return len(b) >= 4 && b[0] == 0 && b[1] == 0 && (b[2] == 1 || b[2] == 0 && b[3] == 1)
}

func annexBToAVCC(b []byte) []byte {
	units := nal.SplitAnnexB(b)
	if len(units) == 0 {
		return b
	}
	var out []byte
	for _, u := range units {
		out = binary.BigEndian.AppendUint32(out, uint32(len(u)))
		out = append(out, u...)
	}
	return out
}
