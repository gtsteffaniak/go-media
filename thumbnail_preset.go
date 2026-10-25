package media

// ThumbnailPreset names FileBrowser-style thumbnail profiles.
type ThumbnailPreset string

const (
	// ThumbnailPresetSmall is a fast listing thumbnail (256px, lower quality).
	ThumbnailPresetSmall ThumbnailPreset = "small"
	// ThumbnailPresetLarge is a high-quality preview (1280px, sharp scaling).
	ThumbnailPresetLarge ThumbnailPreset = "large"
)

// ThumbnailFilter selects the downscale algorithm.
type ThumbnailFilter string

const (
	// ThumbnailFilterFast uses bilinear scaling (lower CPU).
	ThumbnailFilterFast ThumbnailFilter = "fast"
	// ThumbnailFilterHigh uses Catmull-Rom scaling (sharper).
	ThumbnailFilterHigh ThumbnailFilter = "high"
)

// ApplyPreset mutates opt in place when preset is non-empty.
func ApplyPreset(opt *ThumbnailOptions, preset ThumbnailPreset) {
	switch preset {
	case ThumbnailPresetSmall:
		opt.MaxWidth = 256
		opt.MaxHeight = 256
		opt.Quality = 76
		opt.Filter = ThumbnailFilterFast
		opt.Fast = true
	case ThumbnailPresetLarge:
		opt.MaxWidth = 1280
		opt.MaxHeight = 720
		opt.Quality = 90
		opt.Filter = ThumbnailFilterHigh
		opt.Fast = false
	}
	if preset != "" {
		opt.Preset = preset
	}
}

// OptionsForPreset returns thumbnail options for a preset and seek position (0–100).
func OptionsForPreset(preset ThumbnailPreset, seekPercent float64) ThumbnailOptions {
	opt := ThumbnailOptions{SeekPercent: seekPercent, Format: "jpeg"}
	ApplyPreset(&opt, preset)
	return opt
}
