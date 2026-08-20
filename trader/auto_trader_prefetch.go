package trader

import (
	"nofx/kernel"
	"sync"
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

func candidateSymbols(coins []kernel.CandidateCoin) []string {
	syms := make([]string, len(coins))
	for i, c := range coins {
		syms[i] = c.Symbol
	}
	return syms
}
