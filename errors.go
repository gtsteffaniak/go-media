package media

import (
	"errors"
	"fmt"
)

// ErrUnsupported indicates the container, codec, or operation is not handled by go-media.
// Callers should fall back to ffmpeg.
var ErrUnsupported = errors.New("media: unsupported")

// ErrUnsupportedProfile indicates a known codec with an unsupported profile or feature
// (for example H.264 CABAC before the stage-2 decoder lands).
var ErrUnsupportedProfile = errors.New("media: unsupported profile")

// ErrCorrupt indicates the input could not be parsed as valid media.
var ErrCorrupt = errors.New("media: corrupt or truncated")

// ErrNotMedia indicates the input does not look like a supported media file.
var ErrNotMedia = errors.New("media: not a recognized media file")

// UnsupportedError wraps ErrUnsupported with detail for logging.
type UnsupportedError struct {
	Op     string
	Reason string
}

// Error implements error.
func (e *UnsupportedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("media: unsupported %s", e.Op)
	}
	return fmt.Sprintf("media: unsupported %s: %s", e.Op, e.Reason)
}

// Is supports errors.Is for UnsupportedError.
func (e *UnsupportedError) Is(target error) bool {
	return target == ErrUnsupported
}

// Unsupported returns an error that unwraps to ErrUnsupported.
func Unsupported(op, reason string) error {
	return &UnsupportedError{Op: op, Reason: reason}
}

// unsupportedProfileError holds unsupportedProfileError state for this package.
type unsupportedProfileError struct {
	op     string
	reason string
}

// Error implements error.
func (e *unsupportedProfileError) Error() string {
	return fmt.Sprintf("media: unsupported profile %s: %s", e.op, e.reason)
}

// Is supports errors.Is for unsupportedProfileError.
func (e *unsupportedProfileError) Is(target error) bool {
	return target == ErrUnsupportedProfile
}

// UnsupportedProfile returns an error that unwraps to ErrUnsupportedProfile.
func UnsupportedProfile(op, reason string) error {
	return &unsupportedProfileError{op: op, reason: reason}
}
