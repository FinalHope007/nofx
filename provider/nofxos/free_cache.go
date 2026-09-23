package nofxos

import (
	"strconv"
	"sync"
	"time"
)

const (
	freePoolFreshTTL = 15 * time.Minute
	freePoolMaxStale = 2 * time.Hour
)

type poolResult[T any] struct {
	Value T
	Stale bool
	Age   time.Duration
}

type poolCache[T any] struct {
	mu        sync.Mutex
	value     T
	fetchedAt time.Time
	hasValue  bool
}

func (c *poolCache[T]) get(fetch func() (T, error)) (poolResult[T], error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasValue && time.Since(c.fetchedAt) < freePoolFreshTTL {
		return poolResult[T]{Value: c.value, Stale: false, Age: time.Since(c.fetchedAt)}, nil
	}

	val, err := fetch()
	if err == nil {
		c.value = val
		c.fetchedAt = time.Now()
		c.hasValue = true
		return poolResult[T]{Value: val, Stale: false}, nil
	}

	if c.hasValue {
		age := time.Since(c.fetchedAt)
		if age < freePoolMaxStale {
			return poolResult[T]{Value: c.value, Stale: true, Age: age}, nil
		}
	}
	var zero T
	return poolResult[T]{Value: zero}, err
}

// keyedPoolCache partitions a poolCache per key (duration/limit) so requests
// with different parameters never serve each other's values within the fresh
// TTL. The mutex is only held for entry lookup/creation; each poolCache has
// its own lock, so different keys fetch concurrently.
type keyedPoolCache[T any] struct {
	mu      sync.Mutex
	entries map[string]*poolCache[T]
}

func newKeyedPoolCache[T any]() keyedPoolCache[T] {
	return keyedPoolCache[T]{entries: make(map[string]*poolCache[T])}
}

func (c *keyedPoolCache[T]) get(key string, fetch func() (T, error)) (poolResult[T], error) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &poolCache[T]{}
		c.entries[key] = entry
	}
	c.mu.Unlock()
	return entry.get(fetch)
}

func limitCacheKey(limit int) string {
	return strconv.Itoa(limit)
}

func durationLimitCacheKey(duration string, limit int) string {
	return duration + "\x00" + strconv.Itoa(limit)
}

// ForceStaleForTest backdates the ai500 cache timestamp so tests can
// exercise stale-serve behavior without waiting for the real TTL.
func (c *FreeTrendingClient) ForceStaleForTest(age time.Duration) {
	c.ai500Cache.mu.Lock()
	c.ai500Cache.fetchedAt = time.Now().Add(-age)
	c.ai500Cache.mu.Unlock()
}
