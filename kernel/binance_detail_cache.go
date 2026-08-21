package kernel

import (
	"sync"
	"time"

	"nofx/provider/binance"
)

// binanceDetailCacheTTL bounds how long a cached per-coin Binance detail is kept.
const binanceDetailCacheTTL = 10 * time.Minute

type binanceDetailEntry struct {
	value   *binance.BinanceAssetDetail
	expires time.Time
}

// binanceDetailCache is a concurrency-safe TTL cache for per-coin Binance
// Opportunity detail fetches, keyed by "<scene>|<symbol>|<interval>".
type binanceDetailCache struct {
	mu   sync.Mutex
	data map[string]binanceDetailEntry
}

func newBinanceDetailCache() *binanceDetailCache {
	return &binanceDetailCache{data: make(map[string]binanceDetailEntry)}
}

func (c *binanceDetailCache) get(key string) (*binance.BinanceAssetDetail, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *binanceDetailCache) set(key string, v *binance.BinanceAssetDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = binanceDetailEntry{value: v, expires: time.Now().Add(binanceDetailCacheTTL)}
}
