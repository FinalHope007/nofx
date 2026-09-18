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

func TestAltFinsClampAndDefaults(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15", "HOURS1", "DAILY", "MINUTES15", "", "HOURS4"}
	cfg.ClampLimits()
	want := []string{"MINUTES15", "DAILY", "HOURS4"}
	if len(cfg.Indicators.AltFinsIntervals) != len(want) {
		t.Fatalf("got %v want %v", cfg.Indicators.AltFinsIntervals, want)
	}
	for i := range want {
		if cfg.Indicators.AltFinsIntervals[i] != want[i] {
			t.Fatalf("got %v want %v", cfg.Indicators.AltFinsIntervals, want)
		}
	}

	// Toggle on with empty list -> default [MINUTES15, DAILY].
	cfg2 := GetDefaultStrategyConfig("en")
	cfg2.Indicators.EnableAltFinsData = true
	cfg2.Indicators.AltFinsIntervals = nil
	cfg2.ClampLimits()
	if len(cfg2.Indicators.AltFinsIntervals) != 2 ||
		cfg2.Indicators.AltFinsIntervals[0] != "MINUTES15" ||
		cfg2.Indicators.AltFinsIntervals[1] != "DAILY" {
		t.Fatalf("expected default [MINUTES15 DAILY], got %v", cfg2.Indicators.AltFinsIntervals)
	}
}

func TestAltFinsEstimateTokensIncreases(t *testing.T) {
	base := GetDefaultStrategyConfig("en")
	withData := GetDefaultStrategyConfig("en")
	withData.Indicators.EnableAltFinsData = true
	withData.Indicators.AltFinsIntervals = []string{"MINUTES15", "DAILY"}
	if withData.EstimateTokens().Total <= base.EstimateTokens().Total {
		t.Fatal("expected AltFins to increase token estimate")
	}
}
