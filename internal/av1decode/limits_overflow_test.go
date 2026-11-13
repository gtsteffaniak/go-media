package av1decode

import (
	"math"
	"testing"

	"github.com/gen2brain/gav1d/av1"
)

// TestFrameSizeLimitOverflow covers the CodeRabbit overflow finding on
// limits.go: caller-supplied LongSide/ShortSide must not wrap the
// per-frame pixel product.
func TestFrameSizeLimitOverflow(t *testing.T) {
	for _, c := range []struct {
		name string
		lim  FrameLimits
		want int
	}{
		{"normal", FrameLimits{LongSide: 1920, ShortSide: 1080}, 1920 * 1080},
		{"default", FrameLimits{}, DefaultFrameLongSide * DefaultFrameShortSide},
		{"coderabbit 1<<30 fits int64", FrameLimits{LongSide: 1 << 30, ShortSide: 1 << 30}, 1 << 60},
		{"1<<32 saturates", FrameLimits{LongSide: 1 << 32, ShortSide: 1 << 32}, math.MaxInt},
		{"maxint x 2", FrameLimits{LongSide: math.MaxInt, ShortSide: 2}, math.MaxInt},
		{"maxint x maxint", FrameLimits{LongSide: math.MaxInt, ShortSide: math.MaxInt}, math.MaxInt},
		{"disabled", FrameLimits{LongSide: -1, ShortSide: 1080}, 0},
	} {
		if got := c.lim.frameSizeLimit(); got != c.want {
			t.Errorf("%s: frameSizeLimit() = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestDecodePixelBudgetOverflow covers the second multiplication flagged by
// CodeRabbit: budget = per-frame limit × maxShownFramesPerSample must
// saturate, not wrap to zero (which rejected every picture).
func TestDecodePixelBudgetOverflow(t *testing.T) {
	for _, c := range []struct {
		name string
		lim  FrameLimits
		want int64
	}{
		{"default", FrameLimits{}, int64(DefaultFrameLongSide*DefaultFrameShortSide) * maxShownFramesPerSample},
		{"normal", FrameLimits{LongSide: 1920, ShortSide: 1080}, int64(1920*1080) * maxShownFramesPerSample},
		{"coderabbit 1<<30", FrameLimits{LongSide: 1 << 30, ShortSide: 1 << 30}, math.MaxInt64},
		{"maxint x maxint", FrameLimits{LongSide: math.MaxInt, ShortSide: math.MaxInt}, math.MaxInt64},
	} {
		if got := decodePixelBudget(c.lim); got != c.want {
			t.Errorf("%s: decodePixelBudget() = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestFramePixelsOverflow checks the checked pixel-count helper used for
// decoded picture dimensions.
func TestFramePixelsOverflow(t *testing.T) {
	if px, ok := framePixels(1920, 1080); !ok || px != 1920*1080 {
		t.Errorf("framePixels(1920,1080) = %d,%v", px, ok)
	}
	if _, ok := framePixels(math.MaxInt, math.MaxInt); ok {
		t.Error("framePixels(MaxInt,MaxInt) should report overflow")
	}
	if _, ok := framePixels(-1, 10); ok {
		t.Error("framePixels(-1,10) should report invalid")
	}
	if px, ok := framePixels(0, 10); !ok || px != 0 {
		t.Errorf("framePixels(0,10) = %d,%v", px, ok)
	}
}

// TestHugeLimitsSmallFrameDecodes is the end-to-end regression for the
// CodeRabbit scenario: with FrameLimits{1<<30, 1<<30} the aggregate
// budget previously wrapped to zero, so even a small in-limit picture
// failed the aggregate check. It must decode now.
func TestHugeLimitsSmallFrameDecodes(t *testing.T) {
	small := av1.Encode(av1.EncodeConfig{Width: 64, Height: 64, BitDepth: 8, QIndex: 128, Speed: 10, Threads: 1})
	lim := FrameLimits{LongSide: 1 << 30, ShortSide: 1 << 30}
	if err := lim.checkFrameSize(64, 64); err != nil {
		t.Fatalf("checkFrameSize(64,64): %v", err)
	}
	if decodePixelBudget(lim) <= 64*64 {
		t.Fatalf("decodePixelBudget(huge) = %d, must exceed a small frame", decodePixelBudget(lim))
	}
	th, err := DecodeSampleCached(nil, small, lim)
	if err != nil {
		t.Fatalf("small in-limit frame with huge limits: %v", err)
	}
	if th.YCbCr.Bounds().Dy() != 64 {
		t.Fatalf("height = %d, want 64", th.YCbCr.Bounds().Dy())
	}
}
