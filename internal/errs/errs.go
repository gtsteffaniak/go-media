// Package errs supports go-media parsers.

package errs

import (
	"errors"
	"fmt"
)

var (
	Unsupported = errors.New("media: unsupported")
	Corrupt     = errors.New("media: corrupt or truncated")
	NotMedia    = errors.New("media: not a recognized media file")
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
