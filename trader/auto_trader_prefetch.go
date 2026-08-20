package trader

import (
	"nofx/kernel"
	"sync"
	"time"
)

func runPrefetchJobs(symbols []string, workerCount int, fn func(string)) {
	if len(symbols) == 0 || fn == nil {
		return
	}
	if workerCount <= 0 {
		workerCount = 3
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for s := range jobs {
				fn(s)
			}
		}()
	}
	for _, s := range symbols {
		jobs <- s
	}
	close(jobs)
	wg.Wait()
}

// runRateLimitedPrefetch executes fn for each symbol with a fixed delay between
// requests, using a single worker. This is suitable for APIs with rate limits
// (e.g. Binance Opportunity asset-details endpoint).
func runRateLimitedPrefetch(symbols []string, delay time.Duration, fn func(string)) {
	if len(symbols) == 0 || fn == nil {
		return
	}
	for i, s := range symbols {
		fn(s)
		if i < len(symbols)-1 {
			time.Sleep(delay)
		}
	}
}

func candidateSymbols(coins []kernel.CandidateCoin) []string {
	syms := make([]string, len(coins))
	for i, c := range coins {
		syms[i] = c.Symbol
	}
	return syms
}
