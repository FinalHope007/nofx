package store

import (
	"encoding/json"
	"testing"
)

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

func TestIndicatorConfigBinanceFieldsRoundTrip(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.EnableBinanceSentimentData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h", "24h"}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back StrategyConfig
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Indicators.EnableBinanceTechnicalData || !back.Indicators.EnableBinanceSentimentData {
		t.Fatal("binance flags lost in round-trip")
	}
	if len(back.Indicators.BinanceTechnicalIntervals) != 2 {
		t.Fatalf("expected 2 intervals, got %v", back.Indicators.BinanceTechnicalIntervals)
	}
}