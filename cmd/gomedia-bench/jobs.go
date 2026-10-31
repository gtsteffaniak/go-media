package main

import (
	"fmt"
	"sync"
)

// defaultJobs is the default cap on concurrently running benchmark jobs
// (go test processes). Use -jobs 1 for timing comparisons; see bench/README.md.
const defaultJobs = 4

// validateJobs rejects a concurrency cap that would run nothing.
func validateJobs(n int) error {
	if n < 1 {
		return fmt.Errorf("-jobs must be >= 1 (got %d)", n)
	}
	return nil
}

// runJobs runs run(i) for i in [0, n) with at most limit calls in flight at
// once. The semaphore is taken by the dispatching goroutine before each job
// starts, so the number of live job goroutines never exceeds limit. It returns
// each job's output and error, indexed by job number.
func runJobs(n, limit int, run func(i int) ([]byte, error)) ([][]byte, []error) {
	outs := make([][]byte, n)
	errs := make([]error, n)
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			outs[i], errs[i] = run(i)
		}(i)
	}
	wg.Wait()
	return outs, errs
}
