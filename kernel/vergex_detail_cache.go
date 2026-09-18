package kernel

import (
	"encoding/json"
	"sync"
	"time"
)

// vergexDetailCacheTTL bounds how long a cached vergex per-coin detail body is
// kept. It mirrors the AltFins / Binance detail cache TTL so all three per-coin
// sources share the same staleness envelope.
const vergexDetailCacheTTL = 10 * time.Minute

type vergexDetailEntry struct {
	value   json.RawMessage
	expires time.Time
}

// vergexDetailCache is a concurrency-safe TTL cache for vergex free per-coin
// detail feed bodies, keyed by "<symbol>|<feed>" where feed is "signal-lab" or
// "heatmap". Failures are never cached (callers simply skip).
type vergexDetailCache struct {
	mu   sync.Mutex
	data map[string]vergexDetailEntry
}

func newVergexDetailCache() *vergexDetailCache {
	return &vergexDetailCache{data: make(map[string]vergexDetailEntry)}
}

func (c *vergexDetailCache) get(key string) (json.RawMessage, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *vergexDetailCache) set(key string, v json.RawMessage) {
	if c == nil || len(v) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = vergexDetailEntry{value: v, expires: time.Now().Add(vergexDetailCacheTTL)}
}
