package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// writeFileAtomic writes data to path through a temporary file in the same
// directory and then renames it into place. Readers see either the old file or
// the complete new file, never a partial write.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// acquireFileLock claims lockPath for one writer. It fails if another live
// process holds it. A lock left behind by a process that no longer exists (or
// whose contents are unreadable) is removed and taken over. The returned func
// releases the lock.
func acquireFileLock(lockPath string) (func(), error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, werr := fmt.Fprintf(f, "%d\n", os.Getpid())
			cerr := f.Close()
			if werr != nil || cerr != nil {
				os.Remove(lockPath)
				return nil, fmt.Errorf("write lock %s: %v %v", lockPath, werr, cerr)
			}
			return func() { os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		owner, readErr := os.ReadFile(lockPath)
		pid, convErr := strconv.Atoi(strings.TrimSpace(string(owner)))
		if readErr == nil && convErr == nil && processAlive(pid) {
			return nil, fmt.Errorf("%s is locked by running process %d", filepath.Base(lockPath), pid)
		}
		// Stale lock: remove it and retry once.
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("could not acquire lock %s", lockPath)
}

// acquireFileLockWait is acquireFileLock, retrying for up to wait while the
// lock is held by a live process. Used for short critical sections such as
// rebuilding a merged report, where another run is expected to finish soon.
func acquireFileLockWait(lockPath string, wait time.Duration) (func(), error) {
	deadline := time.Now().Add(wait)
	for {
		release, err := acquireFileLock(lockPath)
		if err == nil {
			return release, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// processAlive reports whether a process with the given id exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
