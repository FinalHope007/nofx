package store

import "testing"

func TestCoinSourceBinanceNormalizeAndClamp(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 50
	cfg.NormalizeProductSchema()
	if cfg.CoinSource.SourceType != "binance_technical" {
		t.Fatalf("source_type lost: %q", cfg.CoinSource.SourceType)
	}
	if cfg.CoinSource.BinanceTechnicalInterval != "1h" {
		t.Fatalf("interval not preserved: %q", cfg.CoinSource.BinanceTechnicalInterval)
	}

	cfg.ClampLimits()
	if cfg.CoinSource.BinanceTechnicalLimit != MaxCandidateCoins {
		t.Fatalf("expected limit clamped to %d, got %d", MaxCandidateCoins, cfg.CoinSource.BinanceTechnicalLimit)
	}
}