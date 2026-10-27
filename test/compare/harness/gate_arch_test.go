package main

import (
	"runtime"
	"testing"
)

func TestExpectationCaseApplies(t *testing.T) {
	c := ExpectationCase{AssetID: "x", Check: CheckThumbnailLarge, Expect: StatusSupported, Goarch: "arm64"}
	if runtime.GOARCH == "arm64" {
		if !expectationCaseApplies(c) {
			t.Fatal("arm64 case should apply on arm64")
		}
	} else {
		if expectationCaseApplies(c) {
			t.Fatal("arm64-only case should not apply on other arches")
		}
	}
	if !expectationCaseApplies(ExpectationCase{AssetID: "y", Check: CheckProbe, Expect: StatusSupported}) {
		t.Fatal("empty goarch should apply everywhere")
	}
}
