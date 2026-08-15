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

func TestThrottlingPercentageZeroRoundTripsToDefault(t *testing.T) {
	// A persisted value of exactly 0 for a bypass/floor gate means "unset" and
	// must round-trip to the enforced default after ClampLimits, not stay 0
	// (store and runtime must agree).
	cfg := GetDefaultStrategyConfig("en")
	th := &cfg.RiskControl.Throttling
	th.EarlyCloseStopLossBypassPct = 0
	th.EarlyCloseTakeProfitBypassPct = 0
	th.NoiseCloseLossFloorPct = 0
	th.NoiseCloseProfitCeilingPct = 0
	cfg.ClampLimits()

	if got := th.EarlyCloseStopLossBypassPct; got != -3.0 {
		t.Fatalf("expected early_close_stop_loss_bypass_pct -3.0, got %f", got)
	}
	if got := th.EarlyCloseTakeProfitBypassPct; got != 8.0 {
		t.Fatalf("expected early_close_take_profit_bypass_pct 8.0, got %f", got)
	}
	if got := th.NoiseCloseLossFloorPct; got != -2.0 {
		t.Fatalf("expected noise_close_loss_floor_pct -2.0, got %f", got)
	}
	if got := th.NoiseCloseProfitCeilingPct; got != 3.0 {
		t.Fatalf("expected noise_close_profit_ceiling_pct 3.0, got %f", got)
	}
}

func TestThrottlingPercentageWrongSignResetsToDefault(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	th := &cfg.RiskControl.Throttling
	th.EarlyCloseStopLossBypassPct = 5    // wrong sign (loss gate)
	th.NoiseCloseLossFloorPct = 10        // wrong sign (loss gate)
	th.EarlyCloseTakeProfitBypassPct = -1 // wrong sign (profit gate)
	th.NoiseCloseProfitCeilingPct = -9    // wrong sign (profit gate)
	cfg.ClampLimits()

	if got := th.EarlyCloseStopLossBypassPct; got != -3.0 {
		t.Fatalf("expected early_close_stop_loss_bypass_pct -3.0, got %f", got)
	}
	if got := th.NoiseCloseLossFloorPct; got != -2.0 {
		t.Fatalf("expected noise_close_loss_floor_pct -2.0, got %f", got)
	}
	if got := th.EarlyCloseTakeProfitBypassPct; got != 8.0 {
		t.Fatalf("expected early_close_take_profit_bypass_pct 8.0, got %f", got)
	}
	if got := th.NoiseCloseProfitCeilingPct; got != 3.0 {
		t.Fatalf("expected noise_close_profit_ceiling_pct 3.0, got %f", got)
	}
}
