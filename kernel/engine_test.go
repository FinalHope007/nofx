package kernel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"nofx/provider/altfins"
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

func (f *fakeOpportunityClient) GetAssetDetails(_ context.Context, _, _, _ string) (*binance.BinanceAssetDetail, error) {
	return &binance.BinanceAssetDetail{
		Metrics: map[string]binance.BinanceMetric{"fake_label": {ValueLabel: "fake_value"}},
	}, nil
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
	engine.cacheBinanceDetail("sentiment|BTCUSDT", &binance.BinanceAssetDetail{
		Metrics: map[string]binance.BinanceMetric{
			"sentiment_score":   {Value: "10.00", ValueLabel: "Positive"},
			"sentiment_summary": {ValueLabel: "In the past 24h BTC was bullish."},
		},
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
	engine.cacheBinanceDetail("technical|BTCUSDT|1h", &binance.BinanceAssetDetail{
		Metrics: map[string]binance.BinanceMetric{
			"technical_score_1h":   {Value: "8.73", ValueLabel: "Positive"},
			"technical_summary_1h": {ValueLabel: "Bullish overall for BTC."},
		},
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
	detail, ok := sig.BinanceTechnical["1h"]
	if !ok {
		t.Fatal("expected BinanceTechnical[1h] to exist")
	}
	if detail.Metrics["technical_score_1h"].ValueLabel != "Positive" {
		t.Fatalf("expected technical_score_1h=Positive, got %q", detail.Metrics["technical_score_1h"].ValueLabel)
	}
	if detail.Metrics["technical_summary_1h"].ValueLabel != "Bullish overall for BTC." {
		t.Fatalf("expected technical_summary_1h to match, got %q", detail.Metrics["technical_summary_1h"].ValueLabel)
	}
}

type fakeOpportunityClientN struct {
	n int
}

func (f *fakeOpportunityClientN) GetOpportunityAssets(_ context.Context, _, _ string) ([]binance.OpportunityAsset, error) {
	assets := make([]binance.OpportunityAsset, f.n)
	for i := 0; i < f.n; i++ {
		assets[i] = binance.OpportunityAsset{
			Symbol: fmt.Sprintf("COIN%d", i),
			Score:  float64(f.n - i),
		}
	}
	return assets, nil
}

func (f *fakeOpportunityClientN) GetAssetDetails(_ context.Context, _, _, _ string) (*binance.BinanceAssetDetail, error) {
	return &binance.BinanceAssetDetail{
		Metrics: map[string]binance.BinanceMetric{"fake_label": {ValueLabel: "fake_value"}},
	}, nil
}

func TestGetCandidateCoinsBinanceTechnicalNotTruncatedByLimit(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 5

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientN{n: 20}

	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 20 {
		t.Fatalf("expected 20 coins (no early truncation to limit=5), got %d", len(coins))
	}
	if coins[0].Symbol != "COIN0USDT" {
		t.Fatalf("expected COIN0USDT first (highest score), got %s", coins[0].Symbol)
	}
}

func TestGetCandidateCoinsBinanceTechnicalSoftCapAt50(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientN{n: 100}

	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 50 {
		t.Fatalf("expected 50 coins (soft cap), got %d", len(coins))
	}
}

type fakeAltfinsClient struct {
	ids map[string]int64
}

func (f *fakeAltfinsClient) ResolveIdentifier(_ context.Context, symbol string) (int64, bool, error) {
	id, ok := f.ids[strings.ToUpper(strings.TrimSuffix(symbol, "USDT"))]
	return id, ok, nil
}

func (f *fakeAltfinsClient) GetAnalytics(_ context.Context, _ int64, interval string) (*altfins.Analytics, error) {
	return &altfins.Analytics{Interval: interval, ShortTermTrend: "Bullish (8/10)"}, nil
}

func TestPrefetchAltFinsDetailsPopulatesCache(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15"}

	engine := NewStrategyEngine(cfg)
	engine.altfinsClient = &fakeAltfinsClient{ids: map[string]int64{"ZEC": 1021300}}

	engine.PrefetchAltFinsDetails(context.Background(), []string{"ZECUSDT"})

	got, ok := engine.altfinsDetail("ZECUSDT|MINUTES15")
	if !ok || got == nil || got.ShortTermTrend != "Bullish (8/10)" {
		t.Fatalf("expected cached altfins detail, got %+v ok=%v", got, ok)
	}
}
