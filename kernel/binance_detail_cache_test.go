package kernel

import (
	"testing"
	"time"

	"nofx/provider/binance"
)

func TestBinanceDetailCacheTTL(t *testing.T) {
	e := NewStrategyEngine(nil)
	key := "technical|BTCUSDT|1h"
	if _, ok := e.binanceDetail(key); ok {
		t.Fatal("expected empty cache")
	}
	e.cacheBinanceDetail(key, &binance.BinanceAssetDetail{
		Metrics: map[string]binance.BinanceMetric{"technical_score_1h": {ValueLabel: "Positive"}},
	})
	got, ok := e.binanceDetail(key)
	if !ok || got.Metrics["technical_score_1h"].ValueLabel != "Positive" {
		t.Fatalf("expected cached value, got %+v ok=%v", got, ok)
	}
	// TTL is 10 minutes — verify it hasn't expired after a short wait
	// (we don't wait 10 minutes in the test, just check the constant)
	if binanceDetailCacheTTL != 10*time.Minute {
		t.Fatalf("expected TTL of 10 minutes, got %v", binanceDetailCacheTTL)
	}
}
