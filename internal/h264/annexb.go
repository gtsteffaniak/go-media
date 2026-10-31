package h264

import (
	"bytes"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
)

// DecodeAnnexBFrame decodes the first IDR (or open-GOP intra) picture in an
// Annex-B elementary stream, as carried in MPEG-TS/PS PES payloads.
func DecodeAnnexBFrame(es []byte, fast bool) (fr *frame.Frame, err error) {
	defer recoverDecode(&err)
	raw := nal.SplitAnnexB(es)
	if len(raw) == 0 {
		return nil, mapDecodeErr(annexBErr("empty annex-B stream"))
	}
	var avc bytes.Buffer
	for _, unit := range raw {
		n := len(unit)
		avc.WriteByte(byte(n >> 24))
		avc.WriteByte(byte(n >> 16))
		avc.WriteByte(byte(n >> 8))
		avc.WriteByte(byte(n))
		avc.Write(unit)
	}
	return decodeFrame(nil, avc.Bytes(), fast)
}

type annexBErr string

func (e annexBErr) Error() string { return string(e) }
