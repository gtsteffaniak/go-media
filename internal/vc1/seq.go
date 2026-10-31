package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

// seq holds VC-1 sequence-layer fields from WMV3 extradata.
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
	pquantUniform bool // from quantMode
}

func parseSequence(config []byte, width, height int) (seq, error) {
	if len(config) < 4 {
		return seq{}, errs.Corrupt
	}
	r := newBitReader(config)
	var s seq
	s.profile = int(r.bits(2))
	if s.profile == 3 {
		return seq{}, errs.UnsupProfile("wmv3 keyframe", "VC-1 Advanced Profile")
	}
	if s.profile == 2 {
		return seq{}, errs.UnsupProfile("wmv3 keyframe", "VC-1 Complex Profile")
	}
	_ = r.bit() // res_y411
	_ = r.bit() // res_sprite
	_ = r.bits(3)
	_ = r.bits(5)
	s.loopFilter = r.bit()
	_ = r.bit() // res_x8
	s.multires = r.bit()
	s.resFastTX = r.bit()
	s.fastUVMC = r.bit()
	s.extendedMV = r.bit()
	s.dquant = int(r.bits(2))
	s.vstrans = r.bit()
	if r.bit() { // res_transtab
		return seq{}, errs.Corrupt
	}
	s.overlap = r.bit()
	_ = r.bit() // resync
	s.ranged = r.bit()
	s.maxB = int(r.bits(3))
	s.quantMode = int(r.bits(2))
	s.finterp = r.bit()
	_ = r.bit() // res_rtm
	if !s.resFastTX {
		r.skip(16)
	}
	_ = width
	_ = height
	return s, nil
}
