package store

import (
	"testing"
)

func TestNormalizeProductSchema_AltSourceTypes(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.CoinSource.SourceType = "netflow_top"
	cfg.CoinSource.NetflowLimit = 10
	cfg.NormalizeProductSchema()
	if cfg.CoinSource.SourceType != "netflow_top" {
		t.Fatalf("SourceType = %q, want netflow_top", cfg.CoinSource.SourceType)
	}

	cfg2 := &StrategyConfig{}
	cfg2.CoinSource.SourceType = "price_low"
	cfg2.CoinSource.PriceLimit = 10
	cfg2.NormalizeProductSchema()
	if cfg2.CoinSource.SourceType != "price_low" {
		t.Fatalf("SourceType = %q, want price_low", cfg2.CoinSource.SourceType)
	}

	cfg3 := &StrategyConfig{}
	cfg3.CoinSource.SourceType = "vergex_signal"
	cfg3.CoinSource.VergexDirection = "gainers"
	cfg3.CoinSource.VergexLimit = 10
	cfg3.NormalizeProductSchema()
	if cfg3.CoinSource.VergexMarketType != "all" {
		t.Fatalf("VergexMarketType = %q, want all (default preserved)", cfg3.CoinSource.VergexMarketType)
	}
	if cfg3.CoinSource.VergexDirection != "gainers" {
		t.Fatalf("VergexDirection = %q, want gainers", cfg3.CoinSource.VergexDirection)
	}
}
