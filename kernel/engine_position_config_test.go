package kernel

import "testing"

func TestValidateDecisionUsesConfiguredMinPositionSize(t *testing.T) {
	// BTC/ETH path: old hardcoded floor was 60; a 30 USDT BTC position must
	// PASS when minPositionSize is 12, and FAIL when minPositionSize is 40.
	base := Decision{
		Action: "open_long", Symbol: "BTCUSDT", Leverage: 3,
		PositionSizeUSD: 30, StopLoss: 90000, TakeProfit: 100000, Confidence: 85,
	}
	if err := validateDecision(&base, 1000, 10, 5, 5, 1, 12, 3.0); err != nil {
		t.Fatalf("expected PASS with minPositionSize 12, got: %v", err)
	}
	if err := validateDecision(&base, 1000, 10, 5, 5, 1, 40, 3.0); err == nil {
		t.Fatal("expected FAIL with minPositionSize 40")
	}
}

func TestValidateDecisionUsesConfiguredRiskReward(t *testing.T) {
	// The validator places entry at 20% between SL and TP, so the computed
	// risk/reward is structurally 0.8/0.2 = 4.0 for any SL/TP geometry. The
	// correct behavioral test is that minRiskRewardRatio is honored: a value
	// above 4.0 rejects, a value below 4.0 passes.
	d := Decision{
		Action: "open_long", Symbol: "ALGOUSDT", Leverage: 3,
		PositionSizeUSD: 200, StopLoss: 95, TakeProfit: 105, Confidence: 85,
	}
	// minRR 5.0 (> 4.0) -> reject (config honored, not hardcoded 3.0)
	if err := validateDecision(&d, 1000, 10, 5, 5, 1, 12, 5.0); err == nil {
		t.Fatal("expected FAIL with minRiskRewardRatio 5.0 (> structural 4.0)")
	}
	// minRR 1.0 (< 4.0) -> pass
	if err := validateDecision(&d, 1000, 10, 5, 5, 1, 12, 1.0); err != nil {
		t.Fatalf("expected PASS with minRiskRewardRatio 1.0, got: %v", err)
	}
	// minRR 3.0 (the historical hardcoded value) -> pass (4.0 >= 3.0)
	if err := validateDecision(&d, 1000, 10, 5, 5, 1, 12, 3.0); err != nil {
		t.Fatalf("expected PASS with minRiskRewardRatio 3.0, got: %v", err)
	}
}
