package kernel

import (
	"testing"
	"time"
)

func TestBinanceDetailCacheTTL(t *testing.T) {
	e := NewStrategyEngine(nil)
	key := "technical|BTCUSDT|1h"
	if _, ok := e.binanceDetail(key); ok {
		t.Fatal("expected empty cache")
	}
	e.cacheBinanceDetail(key, map[string]string{"technical_score_1h": "Positive"})
	got, ok := e.binanceDetail(key)
	if !ok || got["technical_score_1h"] != "Positive" {
		t.Fatalf("expected cached value, got %v ok=%v", got, ok)
	}
	// TTL is 10 minutes — verify it hasn't expired after a short wait
	// (we don't wait 10 minutes in the test, just check the constant)
	if binanceDetailCacheTTL != 10*time.Minute {
		t.Fatalf("expected TTL of 10 minutes, got %v", binanceDetailCacheTTL)
	}
}
