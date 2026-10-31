package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicReplacesWholeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e2e.txt")
	if err := writeFileAtomic(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("second\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second\n" {
		t.Fatalf("got %q, want the second write only", got)
	}
	// No temp files may be left behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only the target file, found %s", strings.Join(names, ", "))
	}
}

func TestFileLockRejectsSecondWriter(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "x.lock")
	release, err := acquireFileLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireFileLock(lock); err == nil {
		t.Fatal("second acquire succeeded while the first lock was held")
	} else if !strings.Contains(err.Error(), "locked by running process") {
		t.Fatalf("unexpected error: %v", err)
	}
	release()
	again, err := acquireFileLock(lock)
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	again()
}

func TestFileLockRecoversStaleLock(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "x.lock")
	// PID 0 is never a live process for this check.
	if err := os.WriteFile(lock, []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := acquireFileLock(lock)
	if err != nil {
		t.Fatalf("stale lock was not recovered: %v", err)
	}
	release()
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("lock file not removed on release: %v", err)
	}
}

func TestFileLockGarbageContentIsStale(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "x.lock")
	if err := os.WriteFile(lock, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := acquireFileLock(lock)
	if err != nil {
		t.Fatalf("unreadable lock was not recovered: %v", err)
	}
	release()
}
