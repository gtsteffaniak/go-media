package asf

import (
	"bytes"
	"os"
	"testing"
)

func TestKeyframeMatchesVC1Fixture(t *testing.T) {
	b, err := os.ReadFile("testdata/wmv3_360x288.wmv")
	if err != nil {
		t.Fatal(err)
	}
	r := bytesReaderAt{b: b}
	_, sample, err := videoKeyframe(r, int64(len(b)), 0)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := os.ReadFile("../vc1/testdata/frame1_360x288.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sample, ref) {
		t.Fatalf("payload mismatch len sample=%d ref=%d", len(sample), len(ref))
	}
}
