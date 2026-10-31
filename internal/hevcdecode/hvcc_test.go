package hevcdecode_test

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/hevcdecode"
)

func TestParseHVCCInvalid(t *testing.T) {
	if _, err := hevcdecode.ParseHVCCRecord(nil); err == nil {
		t.Fatal("want error")
	}
}
