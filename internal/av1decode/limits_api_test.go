package av1decode

import (
	"testing"
)

func TestFrameLimitsCallerOverride(t *testing.T) {
	lim := FrameLimits{LongSide: 1920, ShortSide: 1080}
	if err := lim.checkFrameSize(1920, 1080); err != nil {
		t.Fatal(err)
	}
	if err := lim.checkFrameSize(1921, 1080); err == nil {
		t.Fatal("expected over-limit error")
	}
}

func TestFrameLimitsZeroUsesPackageDefault(t *testing.T) {
	oldL, oldS := MaxFrameLongSide, MaxFrameShortSide
	MaxFrameLongSide, MaxFrameShortSide = 320, 240
	t.Cleanup(func() { MaxFrameLongSide, MaxFrameShortSide = oldL, oldS })
	if err := (FrameLimits{}).checkFrameSize(400, 300); err == nil {
		t.Fatal("expected default package limit to apply")
	}
}
