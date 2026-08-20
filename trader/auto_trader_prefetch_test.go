package trader

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestRunRateLimitedPrefetchRespectsDelay(t *testing.T) {
	var timestamps []time.Time
	var mu sync.Mutex
	symbols := []string{"A", "B", "C"}

	start := time.Now()
	runRateLimitedPrefetch(symbols, 50*time.Millisecond, func(sym string) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
	})
	elapsed := time.Since(start)

	if len(timestamps) != 3 {
		t.Fatalf("expected 3 invocations, got %d", len(timestamps))
	}
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap < 45*time.Millisecond {
			t.Fatalf("gap between %d and %d was %v, expected >=50ms", i-1, i, gap)
		}
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("total elapsed %v, expected >=90ms", elapsed)
	}
}
