package kernel

import (
	"encoding/json"
	"testing"
	"time"
)

func TestVergexDetailCacheTTL(t *testing.T) {
	c := newVergexDetailCache()
	key := "ZECUSDT|signal-lab"
	if _, ok := c.get(key); ok {
		t.Fatal("expected empty cache")
	}
	c.set(key, json.RawMessage(`{"symbol":"ETH"}`))
	got, ok := c.get(key)
	if !ok || len(got) == 0 {
		t.Fatalf("expected cached value, got %s ok=%v", got, ok)
	}
	if vergexDetailCacheTTL != 10*time.Minute {
		t.Fatalf("expected TTL of 10 minutes, got %v", vergexDetailCacheTTL)
	}
}

func TestVergexDetailCacheDoesNotStoreEmpty(t *testing.T) {
	c := newVergexDetailCache()
	c.set("ZECUSDT|signal-lab", nil)
	if _, ok := c.get("ZECUSDT|signal-lab"); ok {
		t.Fatal("expected empty body not to be cached")
	}
}
