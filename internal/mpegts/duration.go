package ts

import (
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
)

// Duration returns media length from PTS in a bounded scan.
func Duration(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (detect.DurationResult, error) {
	sec, err := DurationSec(r, size)
	if err != nil {
		return detect.DurationResult{}, err
	}
	if sec <= 0 {
		return detect.DurationResult{Source: "unknown"}, nil
	}
	return detect.DurationResult{
		Duration: time.Duration(sec * float64(time.Second)),
		Source:   "mpegts",
	}, nil
}
