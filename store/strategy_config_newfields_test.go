package store

import "testing"

func TestThrottlingConfigDefaultsPreserveBehavior(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	// Zero-value throttle must fall back to the historical hardcoded values via
	// ClampLimits filling in defaults (see step 3). Assert the effective values.
	cfg.ClampLimits()
	if cfg.RiskControl.Throttling.MaxOpensPerHour != 3 {
		t.Fatalf("expected default max_opens_per_hour 3, got %d", cfg.RiskControl.Throttling.MaxOpensPerHour)
	}
	if cfg.RiskControl.Throttling.MinHoldDurationMin != 90 {
		t.Fatalf("expected default min_hold_duration_min 90, got %d", cfg.RiskControl.Throttling.MinHoldDurationMin)
	}
	if cfg.RiskControl.Throttling.NoiseCloseHoldDurationMin != 180 {
		t.Fatalf("expected default noise_close_hold_duration_min 180, got %d", cfg.RiskControl.Throttling.NoiseCloseHoldDurationMin)
	}
	if cfg.RiskControl.Throttling.ReentryCooldownMin != 240 {
		t.Fatalf("expected default reentry_cooldown_min 240, got %d", cfg.RiskControl.Throttling.ReentryCooldownMin)
	}
	if cfg.RiskControl.OILiquidityFilterMinUSDT != 15000000 {
		t.Fatalf("expected default oi_liquidity_filter_min_usdt 15000000, got %.0f", cfg.RiskControl.OILiquidityFilterMinUSDT)
	}
	if !cfg.RiskControl.EnableOILiquidityFilter {
		t.Fatal("expected default enable_oi_liquidity_filter true")
	}
}

func TestThrottlingConfigClamping(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	cfg.RiskControl.Throttling.MaxOpensPerHour = 0
	cfg.RiskControl.Throttling.MinHoldDurationMin = -5
	cfg.RiskControl.Throttling.EarlyCloseStopLossBypassPct = 100
	cfg.ClampLimits()
	if cfg.RiskControl.Throttling.MaxOpensPerHour < 1 {
		t.Fatalf("expected max_opens_per_hour clamped to >=1, got %d", cfg.RiskControl.Throttling.MaxOpensPerHour)
	}
	if cfg.RiskControl.Throttling.MinHoldDurationMin < 1 {
		t.Fatalf("expected min_hold_duration_min clamped to >=1, got %d", cfg.RiskControl.Throttling.MinHoldDurationMin)
	}
	if cfg.RiskControl.Throttling.EarlyCloseStopLossBypassPct > 0 {
		t.Fatalf("expected early_close_stop_loss_bypass_pct clamped to <=0, got %f", cfg.RiskControl.Throttling.EarlyCloseStopLossBypassPct)
	}
}
