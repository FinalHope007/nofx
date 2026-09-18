package store

import (
	"encoding/json"
	"testing"
)

func TestIndicatorPeriodsPersistenceDistinguishesUnsetFromExplicitEmpty(t *testing.T) {
	cfg := StrategyConfig{
		Indicators: IndicatorConfig{
			EMAPeriods:  []int{},
			RSIPeriods:  nil,
			ATRPeriods:  []int{},
			BOLLPeriods: nil,
		},
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	var back StrategyConfig
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if back.Indicators.EMAPeriods == nil || len(back.Indicators.EMAPeriods) != 0 {
		t.Fatalf("EMAPeriods = %#v (nil=%v), want non-nil empty", back.Indicators.EMAPeriods, back.Indicators.EMAPeriods == nil)
	}
	if back.Indicators.ATRPeriods == nil || len(back.Indicators.ATRPeriods) != 0 {
		t.Fatalf("ATRPeriods = %#v (nil=%v), want non-nil empty", back.Indicators.ATRPeriods, back.Indicators.ATRPeriods == nil)
	}
	if back.Indicators.RSIPeriods != nil {
		t.Fatalf("RSIPeriods = %#v, want nil", back.Indicators.RSIPeriods)
	}
	if back.Indicators.BOLLPeriods != nil {
		t.Fatalf("BOLLPeriods = %#v, want nil", back.Indicators.BOLLPeriods)
	}
}

func TestIndicatorPeriodsLegacyAbsentKeyUnmarshalsToNil(t *testing.T) {
	raw := []byte(`{"indicators":{"klines":{"primary_timeframe":"15m"}}}`)

	var cfg StrategyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal legacy config: %v", err)
	}

	if cfg.Indicators.EMAPeriods != nil {
		t.Fatalf("EMAPeriods = %#v, want nil for absent key", cfg.Indicators.EMAPeriods)
	}
	if cfg.Indicators.RSIPeriods != nil {
		t.Fatalf("RSIPeriods = %#v, want nil for absent key", cfg.Indicators.RSIPeriods)
	}
	if cfg.Indicators.ATRPeriods != nil {
		t.Fatalf("ATRPeriods = %#v, want nil for absent key", cfg.Indicators.ATRPeriods)
	}
	if cfg.Indicators.BOLLPeriods != nil {
		t.Fatalf("BOLLPeriods = %#v, want nil for absent key", cfg.Indicators.BOLLPeriods)
	}
}
