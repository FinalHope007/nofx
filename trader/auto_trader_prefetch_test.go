package trader

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunPrefetchJobsBoundsConcurrency(t *testing.T) {
	var max, cur int64
	var mu sync.Mutex
	start := make(chan struct{}, 20)
	var started, done sync.WaitGroup
	symbols := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}
	started.Add(len(symbols))
	done.Add(len(symbols))

	doneCh := make(chan struct{})
	go func() {
		runPrefetchJobs(symbols, 3, func(string) {
			started.Done()
			start <- struct{}{}
			n := atomic.AddInt64(&cur, 1)
			mu.Lock()
			if n > max {
				max = n
			}
			mu.Unlock()
			atomic.AddInt64(&cur, -1)
			done.Done()
		})
		close(doneCh)
	}()

	// Drain the start channel in parallel so workers can progress.
	go func() {
		for range start {
		}
	}()

	started.Wait()
	done.Wait()
	<-doneCh
	if max > 3 {
		t.Fatalf("concurrency exceeded worker bound: max=%d", max)
	}
}
