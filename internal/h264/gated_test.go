package h264

import (
	"errors"
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// TestGatedProfileMechanism exercises the measure-before-support path with a temporary
// gated profile, since no profile is gated in production right now.
func TestGatedProfileMechanism(t *testing.T) {
	f, err := os.Open("../../testdata/baseline_cavlc_128x128.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	avcc, _, err := firstSyncSample(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	nal := avcc.SequenceParameterSets[0].NALUnit

	if err := validateSPSNAL(nal); err != nil {
		t.Fatalf("Baseline must be enabled by default: %v", err)
	}

	gatedProfiles[66] = "test: not enabled"
	defer delete(gatedProfiles, 66)
	if err := validateSPSNAL(nal); !errors.Is(err, errs.UnsupportedProfile) {
		t.Fatalf("gated profile: err = %v, want UnsupportedProfile", err)
	}
	AllowGatedProfiles(true)
	defer AllowGatedProfiles(false)
	if err := validateSPSNAL(nal); err != nil {
		t.Fatalf("gated profile with AllowGatedProfiles: %v", err)
	}
}
