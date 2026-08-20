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

func TestGetCandidateCoinsBinanceSentiment(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_sentiment"
	cfg.CoinSource.BinanceSentimentDirection = "bottom"
	cfg.CoinSource.BinanceSentimentLimit = 5

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClient{}

	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 2 {
		t.Fatalf("expected 2 coins, got %d", len(coins))
	}
	// bottom sort: BTC (7.85) first, TREE (9.45) second
	if coins[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT first for bottom sort, got %s", coins[0].Symbol)
	}
	if coins[1].Symbol != "TREEUSDT" {
		t.Fatalf("expected TREEUSDT second for bottom sort, got %s", coins[1].Symbol)
	}
}

func TestAttachPerCoinSignalsBinanceSentiment(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceSentimentData = true

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClient{}

	// Pre-populate cache with fake sentiment detail for BTCUSDT
	engine.cacheBinanceDetail("sentiment|BTCUSDT", map[string]string{
		"sentiment_score":  "Positive",
		"sentiment_summary": "In the past 24h BTC was bullish.",
	})

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}},
		Ctx:            context.Background(),
	}

	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sig, ok := engine.PerCoinSignalFor("BTCUSDT")
	if !ok {
		t.Fatal("expected PerCoinSignal for BTCUSDT")
	}
	if sig.BinanceSentiment == nil {
		t.Fatal("expected BinanceSentiment to be non-nil")
	}
	if sig.BinanceSentiment["sentiment_score"] != "Positive" {
		t.Fatalf("expected sentiment_score=Positive, got %q", sig.BinanceSentiment["sentiment_score"])
	}
	if sig.BinanceSentiment["sentiment_summary"] != "In the past 24h BTC was bullish." {
		t.Fatalf("expected sentiment_summary to match, got %q", sig.BinanceSentiment["sentiment_summary"])
	}
}

func TestAttachPerCoinSignalsBinanceTechnical(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h"}

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClient{}

	// Pre-populate cache with fake technical detail for BTCUSDT
	engine.cacheBinanceDetail("technical|BTCUSDT|1h", map[string]string{
		"technical_score_1h":  "Positive",
		"technical_summary_1h": "Bullish overall for BTC.",
	})

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}},
		Ctx:            context.Background(),
	}

	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sig, ok := engine.PerCoinSignalFor("BTCUSDT")
	if !ok {
		t.Fatal("expected PerCoinSignal for BTCUSDT")
	}
	if sig.BinanceTechnical == nil {
		t.Fatal("expected BinanceTechnical to be non-nil")
	}
	if sig.BinanceTechnical["1h|technical_score_1h"] != "Positive" {
		t.Fatalf("expected technical_score_1h=Positive, got %q", sig.BinanceTechnical["1h|technical_score_1h"])
	}
	if sig.BinanceTechnical["1h|technical_summary_1h"] != "Bullish overall for BTC." {
		t.Fatalf("expected technical_summary_1h to match, got %q", sig.BinanceTechnical["1h|technical_summary_1h"])
	}
}
