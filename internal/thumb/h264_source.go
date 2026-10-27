package thumb

// useH264RGBPipeline is false for all presets: H.264 MP4 thumbnails decode to ThumbYCbCr and
// downscale via tryScaleYCbCrSource (matrix-aware area) before JPEG encode, same as large presets.
func useH264RGBPipeline(_ Options) bool {
	return false
}
