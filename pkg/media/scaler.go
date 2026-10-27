package media

// AllThumbnailFilters returns scaler modes benchmarked against ffmpeg in the comparison harness.
func AllThumbnailFilters() []ThumbnailFilter {
	return []ThumbnailFilter{
		ThumbnailFilterArea,
		ThumbnailFilterFast,
		ThumbnailFilterBalanced,
		ThumbnailFilterHigh,
	}
}

// ThumbnailFilterLabel returns a stable report-site label.
func ThumbnailFilterLabel(f ThumbnailFilter) string {
	switch f {
	case ThumbnailFilterFast:
		return "fast"
	case ThumbnailFilterArea:
		return "area"
	case ThumbnailFilterBalanced:
		return "balanced"
	case ThumbnailFilterHigh:
		return "high"
	default:
		return string(f)
	}
}
