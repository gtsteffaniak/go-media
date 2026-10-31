package h264

import "testing"

// TestParseAVCConfigRecord checks SPS/PPS extraction and truncation handling.
func TestParseAVCConfigRecord(t *testing.T) {
	rec := []byte{1, 100, 0, 40, 0xff, 0xe1, 0, 4, 0x67, 0x64, 0, 40, 1, 0, 4, 0x68, 0xee, 0x3c, 0x80, 0xfd, 0xf8, 0xf8, 0}
	cfg := ParseAVCConfigRecord(rec)
	if cfg == nil {
		t.Fatal("nil config")
	}
	if cfg.LengthSizeMinusOne != 3 || len(cfg.SequenceParameterSets) != 1 || len(cfg.PictureParameterSets) != 1 {
		t.Fatalf("unexpected config %+v", cfg)
	}
	if cfg.SequenceParameterSets[0].NALUnit[0] != 0x67 || cfg.PictureParameterSets[0].NALUnit[0] != 0x68 {
		t.Fatal("wrong NAL payloads")
	}
	for n := range len(rec) - 4 {
		if n < 19 && ParseAVCConfigRecord(rec[:n]) != nil {
			t.Fatalf("truncated record len %d parsed", n)
		}
	}
	if ParseAVCConfigRecord([]byte{0, 1, 2, 3, 4, 5}) != nil {
		t.Fatal("version 0 accepted")
	}
}
