package vc1

import (
	"github.com/gtsteffaniak/go-media/internal/errs"
)

func decodeIntra(config, sample []byte, width, height int) (*Picture, error) {
	if width <= 0 || height <= 0 {
		return nil, errs.Corrupt
	}
	d, err := newIntraDecoder(config, sample, width, height)
	if err != nil {
		return nil, err
	}
	if err := d.parseFrameHeader(); err != nil {
		return nil, err
	}
	if d.seq.profile == 3 {
		if err := d.decodeIBlocksAdvanced(); err != nil {
			return nil, err
		}
	} else if err := d.decodeIBlocks(); err != nil {
		return nil, err
	}
	d.copyPictureFromBuf()
	return d.pic, nil
}

func errUnsupportedProfile(profile int) error {
	switch profile {
	case 2:
		return errs.UnsupProfile("wmv3 keyframe", "VC-1 Complex Profile")
	default:
		return errs.UnsupProfile("wmv3 keyframe", "VC-1 profile")
	}
}
