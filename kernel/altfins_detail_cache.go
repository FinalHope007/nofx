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
