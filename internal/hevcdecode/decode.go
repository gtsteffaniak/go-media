package hevcdecode

import (
	"errors"

	"github.com/gen2brain/h265/hevc"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Decoder wraps hevc.Decoder for one keyframe access unit.
type Decoder struct {
	dec hevc.Decoder
}

// NewDecoder constructs a HEVC decoder for thumbnail use (single-threaded).
func NewDecoder() *Decoder {
	d := &Decoder{}
	d.dec.Threads(1)
	return d
}

// Reset clears parameter sets and DPB state.
func (d *Decoder) Reset() {
	d.dec.Reset()
}

// DecodeHVCCSample decodes one length-prefixed sync sample using hvcC extradata.
func DecodeHVCCSample(hvcC, sample []byte) (*hevc.Picture, error) {
	cfg, err := ParseHVCCRecord(hvcC)
	if err != nil {
		return nil, err
	}
	dec := NewDecoder()
	defer dec.Reset()
	if err := dec.feedParamSets(cfg); err != nil {
		return nil, err
	}
	return dec.decodeLengthPrefixed(sample, cfg.LengthSize)
}

// DecodeAnnexB decodes one Annex B access unit (MKV/TS elementary stream).
func DecodeAnnexB(data []byte) (*hevc.Picture, error) {
	dec := NewDecoder()
	defer dec.Reset()
	var last *hevc.Picture
	for _, nal := range hevc.SplitAnnexB(data) {
		pics, err := dec.dec.DecodeNAL(nal)
		if err != nil {
			return nil, mapErr(err)
		}
		for _, p := range pics {
			if last != nil {
				last.Release()
			}
			last = p
		}
	}
	for _, p := range dec.dec.Flush() {
		if last != nil {
			last.Release()
		}
		last = p
	}
	if last == nil {
		return nil, errs.UnsupProfile("hevc keyframe", "no picture in access unit")
	}
	if last.Corrupt {
		return nil, errs.UnsupProfile("hevc keyframe", "corrupt picture")
	}
	return last, nil
}

func (d *Decoder) feedParamSets(cfg *HVCCRecord) error {
	for _, ps := range cfg.ParamSets {
		nal, ok := hevc.ParseNAL(ps)
		if !ok {
			continue
		}
		if _, err := d.dec.DecodeNAL(nal); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func (d *Decoder) decodeLengthPrefixed(sample []byte, lengthSize int) (*hevc.Picture, error) {
	if len(sample) == 0 {
		return nil, errs.UnsupProfile("hevc keyframe", "empty sample")
	}
	var last *hevc.Picture
	for _, nal := range hevc.SplitHVCC(sample, lengthSize) {
		pics, err := d.dec.DecodeNAL(nal)
		if err != nil {
			return nil, mapErr(err)
		}
		for _, p := range pics {
			if last != nil {
				last.Release()
			}
			last = p
		}
	}
	for _, p := range d.dec.Flush() {
		if last != nil {
			last.Release()
		}
		last = p
	}
	if last == nil {
		return nil, errs.UnsupProfile("hevc keyframe", "no decodable picture in sample")
	}
	if last.Corrupt {
		return nil, errs.UnsupProfile("hevc keyframe", "corrupt picture")
	}
	return last, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, hevc.ErrInvalid) {
		return errs.Corrupt
	}
	if errors.Is(err, hevc.ErrUnsupported) {
		return errs.UnsupProfile("hevc", err.Error())
	}
	var pe *errs.ProfileError
	if errors.As(err, &pe) {
		return err
	}
	return errs.UnsupProfile("hevc keyframe", err.Error())
}
