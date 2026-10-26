package h264

import "sync/atomic"

// allowGatedProfiles lifts the performance-based profile restrictions in gatedProfiles.
//
// Support for a profile is decided by the ≤20% ffmpeg timing gate, not asserted: a profile
// that has not met the gate is listed in gatedProfiles and rejected with
// ErrUnsupportedProfile in production. The
// comparison harness sets this so it can still decode and time such a profile, and report
// the measured gap (or notice that the gate now passes). Production callers never set it.
var allowGatedProfiles atomic.Bool

// AllowGatedProfiles enables decoding of profiles listed in gatedProfiles (rejected only
// because they have not met the performance gate). It is for measurement tooling and tests.
func AllowGatedProfiles(on bool) { allowGatedProfiles.Store(on) }
