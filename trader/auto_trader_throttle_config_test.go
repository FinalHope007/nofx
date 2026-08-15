package trader

import (
	"testing"
	"time"

	"nofx/store"
)

func TestThrottleConfigReflectsStrategy(t *testing.T) {
	at := &AutoTrader{}
	at.config = AutoTraderConfig{StrategyConfig: &store.StrategyConfig{}}
	at.config.StrategyConfig.RiskControl.Throttling = store.ThrottlingConfig{
		MaxOpensPerHour: 9, MaxOpensPerCycle: 4, MinHoldDurationMin: 10,
		NoiseCloseHoldDurationMin: 30, ReentryCooldownMin: 30,
		EarlyCloseStopLossBypassPct: -1, EarlyCloseTakeProfitBypassPct: 5,
		NoiseCloseLossFloorPct: -0.5, NoiseCloseProfitCeilingPct: 1.5,
	}
	d := at.throttleDurations()
	if d.reentry != 30*time.Minute {
		t.Fatalf("expected reentry 30m, got %s", d.reentry)
	}
	if d.minHold != 10*time.Minute {
		t.Fatalf("expected minHold 10m, got %s", d.minHold)
	}
}
