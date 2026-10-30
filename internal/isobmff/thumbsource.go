package isobmff

import (
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

// ThumbSource indexes moov once for the thumbnail path so the rotation check,
// the MJPEG lookup and the H.264 keyframe seek share one header walk.
type ThumbSource struct {
	idx *movieIndex
}

// OpenThumbSource indexes the MP4 moov box with bounded header reads.
// ok is false when the file has no indexable moov. OpenThumbSourceErr gives
// the reason.
func OpenThumbSource(r io.ReaderAt, size int64) (ThumbSource, bool) {
	s, err := OpenThumbSourceErr(r, size)
	return s, err == nil
}

// OpenThumbSourceErr is OpenThumbSource with a descriptive error. The error
// matches errs.Unsupported and its reason names the exact index failure, for
// example "mp4: no moov box" or "mp4: moov exceeds 64 MiB".
func OpenThumbSourceErr(r io.ReaderAt, size int64) (ThumbSource, error) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return ThumbSource{}, thumbErr(err)
	}
	return ThumbSource{idx: idx}, nil
}

// thumbErr rewrites an index, seek or table error as a thumbnail error whose
// reason starts with "mp4: ". Profile errors keep their errs.UnsupportedProfile
// identity; any other error (I/O, corrupt) becomes errs.Unsupported so the
// caller still falls back to ffmpeg as it did before reasons were reported.
func thumbErr(err error) error {
	var ue *errs.UnsupportedError
	if errors.As(err, &ue) {
		return errs.Unsup("thumbnail", "mp4: "+ue.Reason)
	}
	var pe *errs.ProfileError
	if errors.As(err, &pe) {
		return err
	}
	return errs.Unsup("thumbnail", "mp4: "+err.Error())
}

// Rotation returns the display rotation in degrees of the first motion-video
// track (ffprobe sign convention: -90 means rotate 90 degrees clockwise).
func (s ThumbSource) Rotation() int {
	rot, _ := s.idx.videoRotation()
	return rot
}

// MJPEGFrame returns the JPEG sample nearest seekPercent of an MJPEG track.
func (s ThumbSource) MJPEGFrame(seekPercent float64) ([]byte, bool) {
	return s.idx.mjpegFrame(seekPercent)
}

// h264 returns the cached H.264 handle for the first AVC track. found is false
// when the file has no AVC track; err explains why a found track is unusable.
func (s ThumbSource) h264() (t H264MP4Thumb, found bool, err error) {
	track, ok := s.idx.firstAVC()
	if !ok {
		return H264MP4Thumb{}, false, nil
	}
	tr, err := s.idx.track(track)
	if err != nil {
		return H264MP4Thumb{}, true, thumbErr(err)
	}
	avcc, err := tr.avcConfigErr()
	if err != nil {
		return H264MP4Thumb{}, true, err
	}
	return H264MP4Thumb{R: s.idx.r, Size: s.idx.size, Track: track, AVCC: avcc, index: s.idx}, true, nil
}

// H264ThumbnailYCbCr decodes the IDR frame at seekPercent to YCbCr.
// It returns (nil, nil) when the file has no H.264 track, and a descriptive
// unsupported error when the track exists but cannot be decoded.
func (s ThumbSource) H264ThumbnailYCbCr(seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	t, found, err := s.h264()
	if !found || err != nil {
		return nil, err
	}
	sample, err := s.idx.h264SyncSampleErr(t.Track, seekPercent)
	if err != nil {
		return nil, thumbErr(err)
	}
	fr, err := h264.DecodeAVCCFrame(t.AVCC, sample, fast)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, errs.UnsupProfile("h264 keyframe", "decoder returned no picture")
	}
	return h264.NewThumbYCbCr(fr), nil
}

// H264ThumbnailRGBA decodes the IDR frame at seekPercent to RGBA.
// It returns (nil, nil) when the file has no H.264 track, and a descriptive
// unsupported error when the track exists but cannot be decoded.
func (s ThumbSource) H264ThumbnailRGBA(seekPercent float64, fast bool) (*image.RGBA, error) {
	t, found, err := s.h264()
	if !found || err != nil {
		return nil, err
	}
	sample, err := s.idx.h264SyncSampleErr(t.Track, seekPercent)
	if err != nil {
		return nil, thumbErr(err)
	}
	img, err := h264.DecodeAVCC(t.AVCC, sample, fast)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, errs.UnsupProfile("h264 keyframe", "decoder returned no picture")
	}
	return img, nil
}

// UnsupportedReason explains why no thumbnail decoder applies to this file.
// It is called only after the MJPEG and H.264 paths found nothing, and it
// reads no file bytes: the answer comes from the handler and sample entry
// fourcc the moov index already holds. The error matches errs.Unsupported.
func (s ThumbSource) UnsupportedReason() error {
	var video *trackIndex
	for i := range s.idx.tracks {
		tr := &s.idx.tracks[i]
		if string(tr.handler[:]) != "vide" || isAttachedCoverSampleEntry(string(tr.entry[:]), "vide") {
			continue
		}
		video = tr
		break
	}
	if video == nil {
		if len(s.idx.tracks) == 0 {
			return errs.Unsup("thumbnail", "mp4: moov has no trak boxes")
		}
		return errs.Unsup("thumbnail", "mp4: no video track")
	}
	if video.err != nil && video.entry == ([4]byte{}) {
		return thumbErr(video.err)
	}
	return errs.Unsup("thumbnail", "mp4: video codec "+describeSampleEntry(video.entry)+" not supported")
}

// sampleEntryNames maps video sample entry fourccs to codec names for
// unsupported messages. The names follow ffprobe's codec_long_name wording.
var sampleEntryNames = map[string]string{
	"hvc1": "HEVC", "hev1": "HEVC", "dvh1": "Dolby Vision HEVC", "dvhe": "Dolby Vision HEVC",
	"av01": "AV1", "vp09": "VP9", "vp08": "VP8",
	"mp4v": "MPEG-4 Part 2", "s263": "H.263", "h263": "H.263",
	"avc2": "H.264", "avc4": "H.264 with in-band parameter sets",
	"apch": "ProRes", "apcn": "ProRes", "apcs": "ProRes", "apco": "ProRes", "ap4h": "ProRes",
	"mjp2": "Motion JPEG 2000", "vvc1": "VVC", "vvi1": "VVC",
	"encv": "encrypted video", "resv": "restricted video",
}

// describeSampleEntry formats a fourcc as `hvc1 (HEVC)`, or quoted when
// it is not printable.
func describeSampleEntry(e [4]byte) string {
	for _, c := range e {
		if c < 0x20 || c > 0x7e {
			return fmt.Sprintf("%q", string(e[:]))
		}
	}
	if name, ok := sampleEntryNames[string(e[:])]; ok {
		return string(e[:]) + " (" + name + ")"
	}
	return string(e[:])
}
