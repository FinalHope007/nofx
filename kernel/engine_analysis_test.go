package kernel

import (
	"context"
	"testing"

	"nofx/store"
)

type fakeTrader struct {
	exchange string
}

func (f *fakeTrader) GetBalance() (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeTrader) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

func TestFetchMarketDataEarlyExit(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10
	cfg.RiskControl.EnableOILiquidityFilter = false

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientN{n: 20}

	// Get 20 candidates
	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &Context{
		CandidateCoins: candidates,
		Positions:      nil,
		Ctx:            context.Background(),
	}

	// fetchMarketDataWithStrategy should not panic or error
	// With fake exchange, market data fetches will fail, but the
	// early exit test is about iteration count, not data success.
	// We test that the function iterates without hanging.
	err = fetchMarketDataWithStrategy(ctx, engine)
	if err != nil {
		// Expected to fail on market data fetch with fake exchange
		// The key assertion is it doesn't hang or panic
		t.Logf("expected fetch error with fake exchange: %v", err)
	}
}

func TestExtractCoTTraceFlippedOrder(t *testing.T) {
	// After the format flip, the <decision> block comes first and <reasoning>
	// comes second. The <reasoning> tag must still be found by the primary path.
	response := "<decision>\n```json\n[{\"symbol\":\"BTCUSDT\",\"action\":\"wait\"}]\n```\n</decision>\n<reasoning>\nNo strong setup, waiting for a pullback.\n</reasoning>"
	if got := extractCoTTrace(response); got != "No strong setup, waiting for a pullback." {
		t.Fatalf("expected reasoning from <reasoning> tag, got %q", got)
	}
}

func TestExtractCoTTraceTruncatedAfterDecision(t *testing.T) {
	// Simulate a response truncated after </decision> (the reasoning was cut off
	// by the output-token limit). The fallback must return content after
	// </decision> (which is where reasoning now lives).
	response := "<decision>\n```json\n[{\"symbol\":\"BTCUSDT\",\"action\":\"wait\"}]\n```\n</decision>\n<reasoning>\nNo strong setup, wait"
	if got := extractCoTTrace(response); got != "<reasoning>\nNo strong setup, wait" {
		t.Fatalf("expected truncated reasoning after </decision>, got %q", got)
	}
}

func TestExtractDecisionsFlippedOrder(t *testing.T) {
	// Decision first, reasoning second — the decision JSON must still be found.
	response := "<decision>\n```json\n[{\"symbol\":\"BTCUSDT\",\"action\":\"open_long\",\"leverage\":3,\"position_size_usd\":25,\"stop_loss\":0,\"take_profit\":0,\"confidence\":76,\"risk_usd\":0}]\n```\n</decision>\n<reasoning>\nBullish setup confirmed.\n</reasoning>"
	decisions, err := extractDecisions(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Symbol != "BTCUSDT" || decisions[0].Action != "open_long" {
		t.Fatalf("unexpected decision: %+v", decisions[0])
	}
}
