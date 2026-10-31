package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

// seq holds VC-1 sequence-layer fields from WMV3/WVC1 extradata.
type seq struct {
	profile       int
	overlap       bool
	loopFilter    bool
	quantMode     int
	maxB          int
	finterp       bool
	ranged        bool
	multires      bool
	resFastTX     bool
	fastUVMC      bool
	extendedMV    bool
	dquant        int
	vstrans       bool
	interlace     bool
	tfcntr        bool
	broadcast     bool
	postproc      bool
	psf           bool
	resX8         bool
}

func parseSequence(config []byte, width, height int) (seq, error) {
	if len(config) < 4 {
		return seq{}, errs.Corrupt
	}
	r := newBitReader(config)
	var s seq
	s.profile = int(r.bits(2))
	if s.profile == 3 {
		return parseSequenceAdvanced(config, width, height)
	}
	if s.profile == 2 {
		return seq{}, errs.UnsupProfile("wmv3 keyframe", "VC-1 Complex Profile")
	}
	_ = r.bit()
	_ = r.bit()
	_ = r.bits(3)
	_ = r.bits(5)
	s.loopFilter = r.bit()
	s.resX8 = r.bit()
	s.multires = r.bit()
	s.resFastTX = r.bit()
	s.fastUVMC = r.bit()
	s.extendedMV = r.bit()
	s.dquant = int(r.bits(2))
	s.vstrans = r.bit()
	if r.bit() {
		return seq{}, errs.Corrupt
	}
	s.overlap = r.bit()
	_ = r.bit()
	s.ranged = r.bit()
	s.maxB = int(r.bits(3))
	s.quantMode = int(r.bits(2))
	s.finterp = r.bit()
	_ = r.bit()
	if !s.resFastTX {
		r.skip(16)
	}
	_ = width
	_ = height
	return s, nil
}

// parseSequenceAdvanced reads WVC1 advanced profile extradata (progressive).
func parseSequenceAdvanced(config []byte, width, height int) (seq, error) {
	if len(config) < 4 {
		return seq{}, errs.Corrupt
	}
	r := newBitReader(config)
	var s seq
	s.profile = 3
	if int(r.bits(2)) != 3 {
		return seq{}, errs.Corrupt
	}
	level := r.bits(3)
	_ = level
	s.interlace = r.bit()
	if s.interlace {
		return seq{}, errs.UnsupProfile("wvc1 keyframe", "interlaced advanced sequence")
	}
	s.psf = r.bit()
	if r.bit() {
		return seq{}, errs.Corrupt
	}
	s.maxB = int(r.bits(3))
	s.quantMode = int(r.bits(2))
	s.vstrans = r.bit()
	if r.bit() {
		return seq{}, errs.Corrupt
	}
	s.loopFilter = r.bit()
	s.resFastTX = true
	s.overlap = r.bit()
	s.fastUVMC = r.bit()
	s.extendedMV = r.bit()
	s.ranged = r.bit()
	s.dquant = int(r.bits(2))
	s.tfcntr = r.bit()
	s.postproc = r.bit()
	s.broadcast = r.bit()
	_ = width
	_ = height
	return s, nil
}
