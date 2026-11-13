package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

// ImageTransform is the display transform a HEIF still image stores for its
// primary item (irot and imir properties). Apply Rotation first, then the
// mirror, the order ffmpeg and libheif use.
type ImageTransform struct {
	// Rotation is 0, 90, 180 or 270 degrees counter-clockwise.
	Rotation int
	// Mirror reports whether an imir property is present.
	Mirror bool
	// MirrorHorizontal is the mirror axis when Mirror is set: true flips
	// left-to-right (imir axis 1), false flips top-to-bottom (imir axis 0).
	MirrorHorizontal bool
}

// IsIdentity reports whether the transform leaves the image unchanged.
func (t ImageTransform) IsIdentity() bool {
	return t.Rotation == 0 && !t.Mirror
}

// HEIFTransform returns the primary item's irot/imir display transform for a
// HEIC, HEIF or AVIF file. It reads only the top-level meta box and decodes no
// pixels. A file whose primary item has neither property returns the zero
// ImageTransform and a nil error. Thumbnail already applies this transform;
// HEIFTransform is for callers that decode the coded image some other way
// (for example with ffmpeg) and need to know what still has to be applied.
//
// Other formats return ErrUnsupported. A meta box without readable item
// properties (pitm, iprp, ipco, ipma) also returns ErrUnsupported rather than
// "no transform", so callers do not mistake an unreadable file for an
// unrotated one.
func HEIFTransform(r io.ReaderAt, size int64) (t ImageTransform, err error) {
	defer func() {
		if p := recover(); p != nil {
			t, err = ImageTransform{}, panicErr(p)
		}
	}()
	if size <= 0 {
		return ImageTransform{}, ErrCorrupt
	}
	id, err := sniff.Detect(r, size)
	if err != nil {
		return ImageTransform{}, mapErr(err)
	}
	if !format.IsHEIFImage(id) {
		return ImageTransform{}, Unsupported("heif transform", string(id))
	}
	meta, err := heifthumb.ReadMeta(r, size)
	if err != nil {
		return ImageTransform{}, mapErr(err)
	}
	pt, found, err := meta.PrimaryTransformErr(meta.ReaderAt(r), size)
	if err != nil {
		return ImageTransform{}, mapErr(err)
	}
	if !found {
		return ImageTransform{}, nil
	}
	return ImageTransform{
		Rotation:         pt.Rotation,
		Mirror:           pt.Mirror,
		MirrorHorizontal: pt.Mirror && pt.MirrorAxis == 1,
	}, nil
}
