package kernel

import (
	"context"
	"testing"

	"nofx/provider/binance"
	"nofx/store"
)

type fakeOpportunityClient struct{}

func (f *fakeOpportunityClient) GetOpportunityAssets(_ context.Context, _, _ string) ([]binance.OpportunityAsset, error) {
	return []binance.OpportunityAsset{
		{Symbol: "TREE", Score: 9.45},
		{Symbol: "BTC", Score: 7.85},
	}, nil
}

func (f *fakeOpportunityClient) GetAssetDetails(_ context.Context, _, _, _ string) (map[string]string, error) {
	return map[string]string{"fake_label": "fake_value"}, nil
}

func TestGetCandidateCoinsBinanceTechnical(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClient{}

	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 2 {
		t.Fatalf("expected 2 coins, got %d", len(coins))
	}
	for _, c := range coins {
		if len(c.Symbol) < 4 || c.Symbol[len(c.Symbol)-4:] != "USDT" {
			t.Fatalf("expected symbol ending in USDT, got %s", c.Symbol)
		}
	}
}
