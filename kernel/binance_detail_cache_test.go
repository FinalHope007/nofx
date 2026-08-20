package kernel

import "testing"

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
}
