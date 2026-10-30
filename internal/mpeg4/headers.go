package mpeg4

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Start code values (the byte after 00 00 01), ISO/IEC 14496-2 Table 6-3.
const (
	scVOLFirst  = 0x20
	scVOLLast   = 0x2F
	scVOS       = 0xB0
	scUserData  = 0xB2
	scGOV       = 0xB3
	scVisualObj = 0xB5
	scVOP       = 0xB6
)

// Limits on what a header may ask the decoder to allocate.
const (
	// MaxWidth and MaxHeight bound the coded picture (the 13-bit VOL
	// fields allow 8191).
	MaxWidth  = 8192
	MaxHeight = 8192
	// MaxPixels bounds width*height (8K UHD with headroom).
	MaxPixels = 8192 * 4608
)

// vol holds the video_object_layer fields the I-VOP decoder uses.
type vol struct {
	valid          bool
	width, height  int
	timeIncBits    int
	interlaced     bool
	quantPrecision int
	mpegQuant      bool
	intraMatrix    [64]uint8 // raster order
	resyncMarkers  bool
	// signal type, from the visual object header.
	fullRange    bool
	matrix       uint8
	matrixValid  bool
	userDataTags []string
	packed       bool // DivX "p" (packed B-frames)
	xvidBuild    int  // > 0 when XviD user data was seen
}

// defaultIntraMatrix is §6.3.3 default intra quantiser matrix, raster order.
var defaultIntraMatrix = [64]uint8{
	8, 17, 18, 19, 21, 23, 25, 27,
	17, 18, 19, 21, 23, 25, 27, 28,
	20, 21, 22, 23, 24, 26, 28, 30,
	21, 22, 23, 24, 26, 28, 30, 32,
	22, 23, 24, 26, 28, 30, 32, 35,
	23, 24, 26, 28, 30, 32, 35, 38,
	25, 26, 28, 30, 32, 35, 38, 41,
	27, 28, 30, 32, 35, 38, 41, 45,
}

func unsup(reason string) error { return errs.UnsupProfile("mpeg4 keyframe", reason) }

func corrupt(reason string) error {
	return fmt.Errorf("%w: mpeg4: %s", errs.Corrupt, reason)
}

// unit is one start-code delimited chunk: code is the byte after 00 00 01
// and payload runs to the next start code.
type unit struct {
	code    byte
	payload []byte
}

// nextStartCode returns the index of the next 00 00 01 at or after i, or -1.
func nextStartCode(b []byte, i int) int {
	j := bytes.Index(b[i:], []byte{0, 0, 1})
	if j < 0 {
		return -1
	}
	return i + j
}

// splitUnits calls fn for each start-code unit in b until fn returns false.
func splitUnits(b []byte, fn func(u unit) bool) {
	i := nextStartCode(b, 0)
	for i >= 0 && i+3 < len(b) {
		code := b[i+3]
		start := i + 4
		next := nextStartCode(b, start)
		end := len(b)
		if next >= 0 {
			end = next
		}
		if !fn(unit{code: code, payload: b[start:end]}) {
			return
		}
		i = next
	}
}

// parseVisualObject reads video_signal_type from a visual_object header
// (§6.2.2), as ffmpeg's mpeg4_decode_visual_object does.
func (v *vol) parseVisualObject(p []byte) {
	r := newReader(p)
	if r.bit() { // is_visual_object_identifier
		r.skip(4 + 3)
	}
	if r.bits(4) != 1 { // visual_object_type: 1 = video ID
		return
	}
	if r.bit() { // video_signal_type
		r.skip(3) // video_format
		v.fullRange = r.bit()
		if r.bit() { // colour_description
			r.skip(8 + 8)
			v.matrix = uint8(r.bits(8))
			v.matrixValid = !r.over
		}
	}
}

// parseUserData records encoder tags ffmpeg keys workarounds on.
func (v *vol) parseUserData(p []byte) {
	if len(p) > 256 {
		p = p[:256]
	}
	s := string(p)
	if i := bytes.Index(p, []byte("DivX")); i >= 0 {
		// DivX%dBuild%d%c or DivX%db%d%c; a trailing 'p' marks packed B-frames.
		t := s[i:]
		if j := indexAny(t, "Build", "b"); j > 0 {
			k := j
			for k < len(t) && t[k] >= '0' && t[k] <= '9' {
				k++
			}
			if k > j && k < len(t) && t[k] == 'p' {
				v.packed = true
			}
		}
		v.userDataTags = append(v.userDataTags, "DivX")
	}
	if i := bytes.Index(p, []byte("XviD")); i >= 0 {
		t := s[i+4:]
		k := 0
		for k < len(t) && t[k] >= '0' && t[k] <= '9' {
			k++
		}
		if n, err := strconv.Atoi(t[:k]); err == nil {
			v.xvidBuild = n
		}
		v.userDataTags = append(v.userDataTags, "XviD")
	}
}

// indexAny returns the index just past the first of the markers found in s.
func indexAny(s string, markers ...string) int {
	for _, m := range markers {
		if i := bytes.Index([]byte(s), []byte(m)); i >= 0 {
			return i + len(m)
		}
	}
	return -1
}

// parseVOL reads video_object_layer() (§6.2.3). Every feature the I-VOP
// path cannot decode returns an error naming that feature.
func (v *vol) parseVOL(p []byte) error {
	r := newReader(p)
	r.skip(1) // random_accessible_vol
	voType := r.bits(8)
	if voType == 0x12 {
		return unsup("fine granularity scalable object type not supported")
	}
	verID := uint32(1)
	if r.bit() { // is_object_layer_identifier
		verID = r.bits(4)
		r.skip(3)
	}
	if r.bits(4) == 15 { // aspect_ratio_info: extended PAR
		r.skip(16)
	}
	if r.bit() { // vol_control_parameters
		if cf := r.bits(2); cf != 1 {
			return unsup(fmt.Sprintf("chroma_format %d (only 4:2:0 is defined)", cf))
		}
		r.skip(1)    // low_delay
		if r.bit() { // vbv_parameters
			r.skip(15 + 1 + 15 + 1 + 15 + 1 + 3 + 11 + 1 + 15 + 1)
		}
	}
	shape := r.bits(2)
	switch shape {
	case 1:
		return unsup("binary shape coding (video_object_layer_shape=binary) not supported")
	case 2:
		return unsup("binary-only shape coding not supported")
	case 3:
		return unsup("grayscale shape coding not supported")
	}
	r.marker()
	res := r.bits(16)
	if res == 0 {
		return corrupt("VOL vop_time_increment_resolution is 0")
	}
	v.timeIncBits = 1
	for (1 << v.timeIncBits) < res {
		v.timeIncBits++
	}
	r.marker()
	if r.bit() { // fixed_vop_rate
		r.skip(v.timeIncBits)
	}
	r.marker()
	v.width = int(r.bits(13))
	r.marker()
	v.height = int(r.bits(13))
	r.marker()
	v.interlaced = r.bit()
	r.skip(1) // obmc_disable (inter only)
	var sprite uint32
	if verID == 1 {
		sprite = r.bits(1)
	} else {
		sprite = r.bits(2)
	}
	switch sprite {
	case 1:
		return unsup("static sprite coding (sprite_enable=static) not supported")
	case 2:
		return unsup("global motion compensation (sprite_enable=GMC) not supported")
	case 3:
		return unsup("reserved sprite_enable value 3")
	}
	v.quantPrecision = 5
	if r.bit() { // not_8_bit
		v.quantPrecision = int(r.bits(4))
		if bpp := r.bits(4); bpp != 8 {
			return unsup(fmt.Sprintf("%d-bit MPEG-4 video (not_8_bit, bits_per_pixel=%d) not supported", bpp, bpp))
		}
		if v.quantPrecision < 3 || v.quantPrecision > 9 {
			return corrupt(fmt.Sprintf("VOL quant_precision %d", v.quantPrecision))
		}
	}
	v.mpegQuant = r.bit()
	v.intraMatrix = defaultIntraMatrix
	if v.mpegQuant {
		if r.bit() { // load_intra_quant_mat
			loadMatrix(&r, &v.intraMatrix)
		}
		if r.bit() { // load_nonintra_quant_mat (unused by I-VOPs)
			var m [64]uint8
			loadMatrix(&r, &m)
		}
	}
	if verID != 1 {
		r.skip(1) // quarter_sample (motion only; I-VOPs are unaffected)
	}
	if !r.bit() { // complexity_estimation_disable
		return unsup("complexity estimation header (complexity_estimation_disable=0) not supported")
	}
	v.resyncMarkers = !r.bit()
	if r.bit() { // data_partitioned
		if r.bit() {
			return unsup("data partitioning with reversible VLC (data_partitioned=1, reversible_vlc=1) not supported")
		}
		return unsup("data partitioning (data_partitioned=1) not supported")
	}
	if verID != 1 {
		if r.bit() {
			return unsup("NEWPRED (newpred_enable=1) not supported")
		}
		if r.bit() {
			return unsup("reduced resolution VOP (reduced_resolution_vop_enable=1) not supported")
		}
	}
	if r.bit() {
		return unsup("scalability (scalability=1) not supported")
	}
	if r.over {
		return corrupt("VOL header truncated")
	}
	if v.width <= 0 || v.height <= 0 {
		return corrupt(fmt.Sprintf("VOL size %dx%d", v.width, v.height))
	}
	if v.width > MaxWidth || v.height > MaxHeight || v.width*v.height > MaxPixels {
		return unsup(fmt.Sprintf("picture %dx%d exceeds the %d-pixel decode limit", v.width, v.height, MaxPixels))
	}
	v.valid = true
	return nil
}

// loadMatrix reads a quantiser matrix in zigzag order; a 0 entry ends the
// list and repeats the last value (§6.3.3), as ffmpeg does.
func loadMatrix(r *reader, m *[64]uint8) {
	var last uint8
	i := 0
	for ; i < 64; i++ {
		v := uint8(r.bits(8))
		if v == 0 {
			break
		}
		last = v
		m[scanZigzag[i]] = v
	}
	for ; i < 64; i++ {
		m[scanZigzag[i]] = last
	}
}

// vop holds the VOP header fields of an I-VOP.
type vop struct {
	codingType    uint32 // 0 I, 1 P, 2 B, 3 S
	coded         bool
	dcThr         int
	topFieldFirst bool
	altScan       bool
	quant         int
}

// intraDCThreshold is ffmpeg's ff_mpeg4_dc_threshold: the intra DC VLC is
// used while qscale is below the value (§6.3.6 Table 6-21).
var intraDCThreshold = [8]int{99, 13, 15, 17, 19, 21, 23, 0}

var vopTypeNames = [4]string{"I", "P", "B", "S(GMC)"}

// parseVOPHeader reads vop() up to vop_quant and returns the reader
// positioned at the first macroblock.
func (v *vol) parseVOPHeader(p []byte) (vop, reader, error) {
	r := newReader(p)
	var h vop
	h.codingType = r.bits(2)
	for r.bit() { // modulo_time_base
		if r.over {
			return h, r, corrupt("VOP header truncated")
		}
	}
	r.marker()
	r.skip(v.timeIncBits)
	r.marker()
	h.coded = r.bit()
	if !h.coded || h.codingType != 0 {
		return h, r, nil
	}
	h.dcThr = intraDCThreshold[r.bits(3)]
	if v.interlaced {
		h.topFieldFirst = r.bit()
		h.altScan = r.bit()
	}
	h.quant = int(r.bits(v.quantPrecision))
	if r.over {
		return h, r, corrupt("VOP header truncated")
	}
	if h.quant == 0 {
		return h, r, corrupt("vop_quant is 0")
	}
	return h, r, nil
}
