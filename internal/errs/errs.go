// Package errs supports go-media parsers.

package errs

import (
	"errors"
	"fmt"
)

var (
	Unsupported        = errors.New("media: unsupported")
	UnsupportedProfile = errors.New("media: unsupported profile")
	Corrupt            = errors.New("media: corrupt or truncated")
	NotMedia           = errors.New("media: not a recognized media file")
)

// UnsupportedError is a data type used by this package.
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
	return target == Unsupported
}

// Unsup supports media inspection.
func Unsup(op, reason string) error {
	return &UnsupportedError{Op: op, Reason: reason}
}

// ProfileError marks a codec feature gap that should fall back to ffmpeg.
type ProfileError struct {
	Op     string
	Reason string
}

// Error implements error.
func (e *ProfileError) Error() string {
	return fmt.Sprintf("media: unsupported profile %s: %s", e.Op, e.Reason)
}

// Is supports errors.Is for ProfileError.
func (e *ProfileError) Is(target error) bool {
	return target == UnsupportedProfile
}

// UnsupProfile returns an error that unwraps to UnsupportedProfile.
func UnsupProfile(op, reason string) error {
	return &ProfileError{Op: op, Reason: reason}
}
