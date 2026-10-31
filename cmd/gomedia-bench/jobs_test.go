package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunJobsNeverExceedsLimit proves the -jobs cap holds: the number of jobs
// running at the same instant never goes above the limit, and every job runs.
func TestRunJobsNeverExceedsLimit(t *testing.T) {
	for _, limit := range []int{1, 2, 4} {
		const n = 40
		var active, peak, ran int32
		outs, errs := runJobs(n, limit, func(i int) ([]byte, error) {
			cur := atomic.AddInt32(&active, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			atomic.AddInt32(&ran, 1)
			return []byte{byte(i)}, nil
		})
		if got := atomic.LoadInt32(&peak); got > int32(limit) {
			t.Fatalf("limit %d: peak concurrency %d exceeds cap", limit, got)
		}
		if got := atomic.LoadInt32(&ran); got != n {
			t.Fatalf("limit %d: ran %d jobs, want %d", limit, got, n)
		}
		for i := range n {
			if errs[i] != nil || len(outs[i]) != 1 || outs[i][0] != byte(i) {
				t.Fatalf("limit %d: job %d output misplaced: out=%v err=%v", limit, i, outs[i], errs[i])
			}
		}
	}
}

// TestRunJobsLimitOneSerializes checks the timing-safe setting: -jobs 1 never
// overlaps two jobs.
func TestRunJobsLimitOneSerializes(t *testing.T) {
	var active, peak int32
	runJobs(10, 1, func(int) ([]byte, error) {
		cur := atomic.AddInt32(&active, 1)
		if cur > atomic.LoadInt32(&peak) {
			atomic.StoreInt32(&peak, cur)
		}
		time.Sleep(time.Millisecond)
		atomic.AddInt32(&active, -1)
		return nil, nil
	})
	if peak != 1 {
		t.Fatalf("peak concurrency %d with -jobs 1, want 1", peak)
	}
}

// TestRunJobsReportsErrorsPerJob checks that a failing job does not stop the
// others and its error lands in its own slot.
func TestRunJobsReportsErrorsPerJob(t *testing.T) {
	boom := errors.New("boom")
	_, errs := runJobs(5, 4, func(i int) ([]byte, error) {
		if i == 2 {
			return []byte("partial"), boom
		}
		return nil, nil
	})
	for i, err := range errs {
		if (i == 2) != (err != nil) {
			t.Fatalf("job %d: err=%v", i, err)
		}
	}
}

func TestValidateJobs(t *testing.T) {
	if defaultJobs != 4 {
		t.Fatalf("default jobs = %d, want 4", defaultJobs)
	}
	if err := validateJobs(0); err == nil {
		t.Fatal("validateJobs(0): want error")
	}
	if err := validateJobs(-3); err == nil {
		t.Fatal("validateJobs(-3): want error")
	}
	if err := validateJobs(1); err != nil {
		t.Fatalf("validateJobs(1): %v", err)
	}
}
