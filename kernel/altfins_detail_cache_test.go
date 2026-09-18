package kernel

import (
	"testing"
	"time"

	"nofx/provider/altfins"
)

func TestAltfinsDetailCacheTTL(t *testing.T) {
	c := newAltfinsDetailCache()
	key := "ZEC|MINUTES15"
	if _, ok := c.get(key); ok {
		t.Fatal("expected empty cache")
	}
	c.set(key, &altfins.Analytics{Interval: altfins.IntervalMinutes15, ShortTermTrend: "Bearish (2/10)"})
	got, ok := c.get(key)
	if !ok || got.ShortTermTrend != "Bearish (2/10)" {
		t.Fatalf("expected cached value, got %+v ok=%v", got, ok)
	}
	if altfinsDetailCacheTTL != 10*time.Minute {
		t.Fatalf("expected TTL of 10 minutes, got %v", altfinsDetailCacheTTL)
	}
}
