package main

// thumbnailPresetConfig mirrors media thumbnail presets for ffmpeg alignment.
type thumbnailPresetConfig struct {
	Name      string
	MaxWidth  int
	MaxHeight int
	Quality   int
	FFScale   string // ffmpeg scale flags
}

var thumbnailPresets = []thumbnailPresetConfig{
	{
		Name:      "small",
		MaxWidth:  256,
		MaxHeight: 256,
		Quality:   76,
		FFScale:   "flags=area",
	},
	{
		Name:      "large",
		MaxWidth:  1280,
		MaxHeight: 720,
		Quality:   90,
		FFScale:   "flags=area",
	},
}
