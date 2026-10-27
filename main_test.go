package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectOutputOverwritesInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(in, []byte("not-empty"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectOutputOverwritesInput(in, in, st); err == nil {
		t.Fatal("expected error for identical paths")
	}
	out := filepath.Join(dir, "out.jpg")
	if err := rejectOutputOverwritesInput(in, out, st); err != nil {
		t.Fatal(err)
	}
}
