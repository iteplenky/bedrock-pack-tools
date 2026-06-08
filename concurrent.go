package main

import (
	"runtime"
	"sync"
)

// mapConcurrent runs fn over every job across a bounded worker pool (one
// worker per CPU, capped at len(jobs)) and returns the results in completion
// order. It is the shared fan-out for the per-file decrypt and encrypt
// pipelines; callers build the job slice and tally the results themselves.
func mapConcurrent[J, R any](jobs []J, fn func(J) R) []R {
	if len(jobs) == 0 {
		return nil
	}
	workers := min(runtime.NumCPU(), len(jobs))
	jobCh := make(chan J)
	resultCh := make(chan R, len(jobs))

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for j := range jobCh {
				resultCh <- fn(j)
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
	}()
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]R, 0, len(jobs))
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}
