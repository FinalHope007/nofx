package kernel

import (
	"sync"
	"time"

	"nofx/provider/altfins"
)

// altfinsDetailCacheTTL bounds how long a cached per-coin AltFins analytics
// result is kept.
const altfinsDetailCacheTTL = 10 * time.Minute

type altfinsDetailEntry struct {
	value   *altfins.Analytics
	expires time.Time
}

// altfinsDetailCache is a concurrency-safe TTL cache for per-coin AltFins
// analytics, keyed by "<symbol>|<interval>".
type altfinsDetailCache struct {
	mu   sync.Mutex
	data map[string]altfinsDetailEntry
}

func newAltfinsDetailCache() *altfinsDetailCache {
	return &altfinsDetailCache{data: make(map[string]altfinsDetailEntry)}
}

func (c *altfinsDetailCache) get(key string) (*altfins.Analytics, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *altfinsDetailCache) set(key string, v *altfins.Analytics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = altfinsDetailEntry{value: v, expires: time.Now().Add(altfinsDetailCacheTTL)}
}

type altfinsResolveEntry struct {
	id      int64
	expires time.Time
}

// altfinsResolveCache is a concurrency-safe TTL cache for AltFins
// symbol→securityIdentifierId resolutions, so repeat cycles only issue the
// analytics call. Negative results (no match) are not cached.
type altfinsResolveCache struct {
	mu   sync.Mutex
	data map[string]altfinsResolveEntry
}

func newAltfinsResolveCache() *altfinsResolveCache {
	return &altfinsResolveCache{data: make(map[string]altfinsResolveEntry)}
}

func (c *altfinsResolveCache) get(symbol string) (int64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[symbol]
	if !ok || time.Now().After(e.expires) {
		return 0, false
	}
	return e.id, true
}

func (c *altfinsResolveCache) set(symbol string, id int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[symbol] = altfinsResolveEntry{id: id, expires: time.Now().Add(altfinsDetailCacheTTL)}
}
