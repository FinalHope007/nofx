# Configurable Trading Gates + Trading Style Presets — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a curated subset of the backend trading gates (Sections A/B/C/D) editable per-strategy in the frontend, reconcile the decision validator to read those config fields, add a canned Trading Style preset (scalp/intraday/swing/default), and fix the open-rate counting bug.

**Architecture:** Extend the per-strategy `RiskControlConfig` in `store/strategy.go` with a new nested `ThrottlingConfig` struct plus two OI-liquidity fields, add a `TradingStyle` enum to `StrategyConfig`, thread `min_position_size` + `min_risk_reward_ratio` into the decision validator, wire the runtime throttle to read `ThrottlingConfig` live, and surface everything in the frontend strategy editor (`EditorStepPage.tsx`) under Basic Rules / Advanced Settings / Throttling Settings (Risky) / Trading Style.

**Tech Stack:** Go 1.25 (Gin, GORM), React 18 + TypeScript + Vite. Testing: `testify`, `gomonkey` (test suite only), Vitest.

## Global Constraints

- English-only UI strings.
- Backend uses `SafeError`/`SafeInternalError`/`SanitizeError`; never leak internals.
- `store.*` is the only DB access layer; timestamps UTC.
- HIGH-RISK: live order/position changes need explicit confirmation. Config saves take effect via trader remove → reload-from-store → restart (`api/handler_trader.go:707-726`); no separate hot-reload.
- Margin math (`1.01`/`0.001`/`0.98`) and `max_margin_usage` enforcement are **untouched** — never wire `max_margin_usage` as a hard limit.
- `gofmt` is the formatting standard; `go vet ./...` must be clean.
- Preset values are clamped to Section A hard caps by `StrategyConfig.ClampLimits()`.
- Defaults must preserve current behavior for strategies that never set the new fields.

---

## Task 1: Backend — Add `ThrottlingConfig` + OI fields + `TradingStyle` to strategy config

**Files:**
- Modify: `store/strategy.go` (RiskControlConfig struct at ~line 994; StrategyConfig struct at ~line 683; `ClampLimits()` at ~line 36)

**Interfaces:**
- Consumes: existing `RiskControlConfig` struct (lines 994-1018), `StrategyConfig` struct (lines 683-705), `ClampLimits()` method (line 36), hard-cap consts (lines 13-33).
- Produces: a new nested `store.ThrottlingConfig` struct; extended `store.RiskControlConfig` (adds `ThrottlingConfig` + OI fields); a new `TradingStyle` field on `store.StrategyConfig`; updated `ClampLimits()` clamping the new fields. These exact names/JSON tags are consumed by Tasks 2-7.

- [ ] **Step 1: Add the `ThrottlingConfig` struct and extend `RiskControlConfig`**

Add this struct near `RiskControlConfig` (before line 994), and add fields to `RiskControlConfig`:

```go
// ThrottlingConfig anti-churn throttle gates (Section B). All durations are
// integer minutes. Percentages are PRICE-move, leverage-independent.
type ThrottlingConfig struct {
	MaxOpensPerHour                 int     `json:"max_opens_per_hour"`
	MaxOpensPerCycle                int     `json:"max_opens_per_cycle"`
	MinHoldDurationMin              int     `json:"min_hold_duration_min"`
	NoiseCloseHoldDurationMin       int     `json:"noise_close_hold_duration_min"`
	ReentryCooldownMin              int     `json:"reentry_cooldown_min"`
	EarlyCloseStopLossBypassPct     float64 `json:"early_close_stop_loss_bypass_pct"`
	EarlyCloseTakeProfitBypassPct   float64 `json:"early_close_take_profit_bypass_pct"`
	NoiseCloseLossFloorPct          float64 `json:"noise_close_loss_floor_pct"`
	NoiseCloseProfitCeilingPct      float64 `json:"noise_close_profit_ceiling_pct"`
}
```

Inside `RiskControlConfig` (after `MinConfidence int` at line 1017), add:

```go
	// OI-liquidity candidate filter (Section D). When EnableOILiquidityFilter is
	// true, candidates with OI value below OILiquidityFilterMinUSDT are dropped.
	EnableOILiquidityFilter bool    `json:"enable_oi_liquidity_filter"`
	OILiquidityFilterMinUSDT float64 `json:"oi_liquidity_filter_min_usdt"`
	// Anti-churn throttle gates (Section B).
	Throttling ThrottlingConfig `json:"throttling"`
```

Add `TradingStyle` to `StrategyConfig` (after `Language` at line 689):

```go
	// Trading style preset: "scalp" | "intraday" | "swing" | "default". Saved for
	// record and UI highlight; does NOT drive runtime logic.
	TradingStyle string `json:"trading_style,omitempty"`
```

- [ ] **Step 2: Write the failing test for defaults + clamping**

Create `store/strategy_config_newfields_test.go`:

```go
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
```

- [ ] **Step 3: Implement `ClampLimits()` additions + defaults**

At the end of `ClampLimits()` (after line 156), add:

```go
	// Clamp OI-liquidity filter.
	if cfg.RiskControl.EnableOILiquidityFilter && cfg.RiskControl.OILiquidityFilterMinUSDT <= 0 {
		cfg.RiskControl.OILiquidityFilterMinUSDT = 15000000
	}

	// Clamp throttle gates. Zero/negative durations and opens caps fall back to
	// the historical hardcoded values (Section B defaults).
	t := &cfg.RiskControl.Throttling
	if t.MaxOpensPerHour < 1 || t.MaxOpensPerHour > 100 { t.MaxOpensPerHour = 3 }
	if t.MaxOpensPerCycle < 1 || t.MaxOpensPerCycle > t.MaxOpensPerHour { t.MaxOpensPerCycle = 2 }
	if t.MinHoldDurationMin < 1 || t.MinHoldDurationMin > 10080 { t.MinHoldDurationMin = 90 }
	if t.NoiseCloseHoldDurationMin < t.MinHoldDurationMin || t.NoiseCloseHoldDurationMin > 10080 { t.NoiseCloseHoldDurationMin = 180 }
	if t.ReentryCooldownMin < 1 || t.ReentryCooldownMin > 10080 { t.ReentryCooldownMin = 240 }
	if t.EarlyCloseStopLossBypassPct > 0 { t.EarlyCloseStopLossBypassPct = -3.0 }
	if t.EarlyCloseTakeProfitBypassPct < 0 { t.EarlyCloseTakeProfitBypassPct = 8.0 }
	if t.NoiseCloseLossFloorPct > 0 { t.NoiseCloseLossFloorPct = -2.0 }
	if t.NoiseCloseProfitCeilingPct < 0 { t.NoiseCloseProfitCeilingPct = 3.0 }
```

- [ ] **Step 4: Run tests, verify pass, gofmt, commit**

```bash
go test ./store/ -run 'TestThrottlingConfig' -v
gofmt -l store/strategy.go
go vet ./store/...
git add store/strategy.go store/strategy_config_newfields_test.go
git commit -m "feat(store): add throttling config, OI-liquidity filter, trading style to strategy schema"
```

---

## Task 2: Backend — Reconcile decision validator to read config min-size + RR

**Files:**
- Modify: `kernel/engine_position.go` (validateDecision lines 22-133, validateDecisions lines 13-20)
- Modify: `kernel/engine_analysis.go` (parseFullDecisionResponse lines 420-442, caller lines 123-130)

**Interfaces:**
- Consumes: `store.RiskControlConfig` (Task 1); the `RiskControlConfig` fields `MinPositionSize`, `MinRiskRewardRatio`.
- Produces: `parseFullDecisionResponse` and `validateDecisions`/`validateDecision` gain two params — `minPositionSize float64`, `minRiskRewardRatio float64`. Task 2's caller (`engine_analysis.go:108-130`) passes `riskConfig.MinPositionSize` / `riskConfig.MinRiskRewardRatio`. This exact signature is consumed by any test the implementer writes; there is no other caller.

- [ ] **Step 1: Write the failing test**

Create `kernel/engine_position_config_test.go`:

```go
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

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `go test ./kernel/ -run 'TestValidateDecisionUsesConfigured' -v`
Expected: compile error — `validateDecision` called with too many arguments (signature not yet extended).

- [ ] **Step 3: Extend signatures and thread config**

In `kernel/engine_position.go`:

```go
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio, minPositionSize, minRiskRewardRatio float64) error {
	for i := range decisions {
		if err := validateDecision(&decisions[i], accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio, minPositionSize, minRiskRewardRatio); err != nil {
			return fmt.Errorf("decision #%d validation failed: %w", i+1, err)
		}
	}
	return nil
}

func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio, minPositionSize, minRiskRewardRatio float64) error {
```

Replace the min-size block (lines 66-77):

```go
		if d.PositionSizeUSD < minPositionSize {
			return fmt.Errorf("opening amount too small (%.2f USDT), must be ≥%.2f USDT", d.PositionSizeUSD, minPositionSize)
		}
```

Replace the RR check (lines 126-129):

```go
		if riskRewardRatio < minRiskRewardRatio {
			return fmt.Errorf("risk/reward ratio too low (%.2f:1), must be ≥%.2f:1 [risk: %.2f%% reward: %.2f%%] [stop loss: %.2f take profit: %.2f]",
				riskRewardRatio, minRiskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
```

In `kernel/engine_analysis.go`, update `parseFullDecisionResponse` signature (line 420) and the `validateDecisions` call (line 431) to thread the two new params. Then update the caller (lines 123-130) to pass `riskConfig.MinPositionSize, riskConfig.MinRiskRewardRatio`.

**Update existing callers/tests of the changed signatures:** `kernel/validate_test.go` calls `validateDecision` directly at lines 87 and 113 with the old 6-arg signature. Update those two calls to append `12, 3.0` (preserving the historical min-size and RR floors for those legacy tests). Search the repo for any other direct callers of `validateDecisions` / `parseFullDecisionResponse` and update them to the new signatures.

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./kernel/ -run 'TestValidateDecisionUsesConfigured' -v`
Expected: PASS.

Run full kernel + engine tests: `go test ./kernel/... -count=1` — ensure no other tests broke from the signature change.

- [ ] **Step 5: gofmt, vet, commit**

```bash
gofmt -w kernel/engine_position.go kernel/engine_analysis.go
go vet ./kernel/...
git add kernel/engine_position.go kernel/engine_analysis.go kernel/engine_position_config_test.go
git commit -m "feat(kernel): thread configured min position size and risk/reward into decision validator"
```

---

## Task 3: Backend — Wire runtime throttle to read `ThrottlingConfig`

**Files:**
- Modify: `trader/auto_trader_throttle.go` (consts lines 12-30; openThrottleReason lines 103-134; closeThrottleReason lines 136-198)

**Interfaces:**
- Consumes: `at.config.StrategyConfig.RiskControl.Throttling` (`store.ThrottlingConfig`, Task 1). `at.config` is already populated/refreshed by `RefreshStrategyConfig` (`trader/auto_trader.go:424-452`).
- Produces: helper methods on `AutoTrader` — `throttleConfig() store.ThrottlingConfig` and `throttleDurations()` returning a small value struct — consumed by Tasks 3-4. No external consumers.

- [ ] **Step 1: Write the failing test**

Create `trader/auto_trader_throttle_config_test.go`:

```go
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
```

> Note: `at.config` is type `AutoTraderConfig` (value, not pointer) and `at.config.StrategyConfig` is `*store.StrategyConfig` (pointer, may be nil). The test above matches the real structs (`trader/auto_trader.go:73`, `:162`, `:174`).

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `go test ./trader/ -run TestThrottleConfigReflectsStrategy -v`
Expected: FAIL — `throttleDurations` undefined.

- [ ] **Step 3: Add config readers and replace const usages**

Add helper methods in `auto_trader_throttle.go`:

```go
type throttleDurationConfig struct {
	minHold, noiseClose, reentry time.Duration
}

// throttleConfig returns the effective throttle gates. Zero/unset fields fall
// back to the historical hardcoded defaults (Section B) so existing strategies
// and nil-config tests behave exactly as before.
func (at *AutoTrader) throttleConfig() store.ThrottlingConfig {
	if at == nil || at.config == nil || at.config.StrategyConfig == nil {
		return defaultThrottlingConfig()
	}
	return withThrottlingDefaults(at.config.StrategyConfig.RiskControl.Throttling)
}

// defaultThrottlingConfig returns the historical Section B defaults.
func defaultThrottlingConfig() store.ThrottlingConfig {
	return store.ThrottlingConfig{
		MaxOpensPerHour:               3,
		MaxOpensPerCycle:              2,
		MinHoldDurationMin:            90,
		NoiseCloseHoldDurationMin:     180,
		ReentryCooldownMin:            240,
		EarlyCloseStopLossBypassPct:   -3.0,
		EarlyCloseTakeProfitBypassPct: 8.0,
		NoiseCloseLossFloorPct:        -2.0,
		NoiseCloseProfitCeilingPct:    3.0,
	}
}

// withThrottlingDefaults fills any zero-valued gate with the historical default.
func withThrottlingDefaults(t store.ThrottlingConfig) store.ThrottlingConfig {
	d := defaultThrottlingConfig()
	if t.MaxOpensPerHour <= 0 { t.MaxOpensPerHour = d.MaxOpensPerHour }
	if t.MaxOpensPerCycle <= 0 { t.MaxOpensPerCycle = d.MaxOpensPerCycle }
	if t.MinHoldDurationMin <= 0 { t.MinHoldDurationMin = d.MinHoldDurationMin }
	if t.NoiseCloseHoldDurationMin <= 0 { t.NoiseCloseHoldDurationMin = d.NoiseCloseHoldDurationMin }
	if t.ReentryCooldownMin <= 0 { t.ReentryCooldownMin = d.ReentryCooldownMin }
	if t.EarlyCloseStopLossBypassPct == 0 { t.EarlyCloseStopLossBypassPct = d.EarlyCloseStopLossBypassPct }
	if t.EarlyCloseTakeProfitBypassPct == 0 { t.EarlyCloseTakeProfitBypassPct = d.EarlyCloseTakeProfitBypassPct }
	if t.NoiseCloseLossFloorPct == 0 { t.NoiseCloseLossFloorPct = d.NoiseCloseLossFloorPct }
	if t.NoiseCloseProfitCeilingPct == 0 { t.NoiseCloseProfitCeilingPct = d.NoiseCloseProfitCeilingPct }
	return t
}

func (at *AutoTrader) throttleDurations() throttleDurationConfig {
	tc := at.throttleConfig()
	return throttleDurationConfig{
		minHold:    time.Duration(tc.MinHoldDurationMin) * time.Minute,
		noiseClose: time.Duration(tc.NoiseCloseHoldDurationMin) * time.Minute,
		reentry:    time.Duration(tc.ReentryCooldownMin) * time.Minute,
	}
}
```

Replace usages in `openThrottleReason`:
- `autopilotMaxOpensPerCycle` → `at.throttleConfig().MaxOpensPerCycle`
- `autopilotMaxOpensPerHour` → `at.throttleConfig().MaxOpensPerHour`
- `autopilotReentryCooldown` → `d := at.throttleDurations(); d.reentry`

Replace usages in `closeThrottleReason`:
- `autopilotNoiseCloseHoldDuration` → `d := at.throttleDurations(); d.noiseClose`
- `autopilotMinHoldDuration` → `d.minHold`
- `noiseCloseLossFloorPct` / `noiseCloseProfitCeilingPct` → `at.throttleConfig().NoiseCloseLossFloorPct` / `.NoiseCloseProfitCeilingPct`
- `earlyCloseStopLossBypassPct` / `earlyCloseTakeProfitBypassPct` → `at.throttleConfig().EarlyCloseStopLossBypassPct` / `.EarlyCloseTakeProfitBypassPct`

After all usages are replaced, the package-level consts (lines 12-30) become unused. Package-level unused consts are not a Go compile error, but remove them for cleanliness. The existing throttle tests (e.g. `TestTradeThrottleBlocksEarlyNoiseClose`) construct `at := &AutoTrader{}` with nil config and rely on the historical defaults — these are preserved by `throttleConfig()`'s fallback, so those tests keep passing unchanged.

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./trader/ -run TestThrottleConfigReflectsStrategy -v`
Expected: PASS.

Run: `go test ./trader/ -run 'Test.*Throttle' -v`
Expected: existing throttle tests still pass (behavior unchanged with default config).

- [ ] **Step 5: gofmt, vet, commit**

```bash
gofmt -w trader/auto_trader_throttle.go
go vet ./trader/...
git add trader/auto_trader_throttle.go trader/auto_trader_throttle_config_test.go
git commit -m "feat(trader): read anti-churn throttle gates from per-strategy throttling config"
```

---

## Task 4: Backend — Fix Issue 1: distinct open counting + single-close re-entry

**Files:**
- Modify: `trader/auto_trader_throttle.go` (countRecentOpenOrders lines 233-249; findRecentCloseOrder lines 251-267)

**Interfaces:**
- Consumes: `store.TraderOrder` fields `Symbol`, `PositionSide`, `Side`, `OrderAction`, `Status`, `CreatedAt`; `openActionSide` (line 73); `isCanceledOrder` (line 288).
- Produces: a pure helper `countDistinctOpenEvents(orders []*store.TraderOrder, sinceMs int64) int` counting **distinct (symbol, positionSide|side) opens**, and a pure helper `orderPositionKey(o *store.TraderOrder) string`. `countRecentOpenOrders` and `findRecentCloseOrder` delegate to these. The pure helpers are testable without a store and consumed by Tasks 4's tests and production code.

- [ ] **Step 1: Write the failing test**

Create `trader/auto_trader_throttle_open_test.go` (pure-function test — no store needed):

```go
package trader

import (
	"testing"
	"time"

	"nofx/store"
)

func TestCountDistinctOpenEventsGroupsMultiFillOpens(t *testing.T) {
	now := time.Now().UTC().UnixMilli()
	// A single BTWUSDT long opened as 5 fills (5 order rows, same symbol+side).
	orders := []*store.TraderOrder{}
	for i := 0; i < 5; i++ {
		orders = append(orders, &store.TraderOrder{
			Symbol: "BTWUSDT", PositionSide: "LONG", Side: "BUY",
			OrderAction: "open_long", Status: "FILLED", CreatedAt: now,
		})
	}
	// A genuinely separate ETHUSDT open.
	orders = append(orders, &store.TraderOrder{
		Symbol: "ETHUSDT", PositionSide: "LONG", Side: "BUY",
		OrderAction: "open_long", Status: "FILLED", CreatedAt: now,
	})

	count := countDistinctOpenEvents(orders, time.Unix(0, 0).UnixMilli())
	if count != 2 {
		t.Fatalf("expected 2 distinct opens, got %d (5 fills must count as 1 open)", count)
	}
}

func TestCountDistinctOpenEventsSkipsCanceled(t *testing.T) {
	now := time.Now().UTC().UnixMilli()
	orders := []*store.TraderOrder{
		{Symbol: "BTCUSDT", PositionSide: "LONG", OrderAction: "open_long", Status: "CANCELED", CreatedAt: now},
		{Symbol: "ETHUSDT", PositionSide: "LONG", OrderAction: "open_long", Status: "FILLED", CreatedAt: now},
	}
	if got := countDistinctOpenEvents(orders, time.Unix(0, 0).UnixMilli()); got != 1 {
		t.Fatalf("expected 1 (canceled skipped), got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `go test ./trader/ -run TestCountDistinctOpenEvents -v`
Expected: FAIL — `countDistinctOpenEvents` undefined.

- [ ] **Step 3: Implement the pure helpers**

Add to `auto_trader_throttle.go`:

```go
// orderPositionKey returns a stable dedup key for a position-side regardless of
// how the exchange labels it (positionSide may be empty on some adapters).
func orderPositionKey(o *store.TraderOrder) string {
	key := o.Symbol
	if ps := strings.TrimSpace(o.PositionSide); ps != "" {
		return key + "|" + strings.ToUpper(ps)
	}
	return key + "|" + strings.ToUpper(strings.TrimSpace(o.Side))
}

// countDistinctOpenEvents counts distinct (symbol, side) position-open events,
// so a single position opened as multiple fills counts once.
func countDistinctOpenEvents(orders []*store.TraderOrder, sinceMs int64) int {
	seen := map[string]bool{}
	count := 0
	for _, order := range orders {
		if order == nil || order.CreatedAt < sinceMs || isCanceledOrder(order) {
			continue
		}
		if !isOpenAction(order.OrderAction) {
			continue
		}
		key := orderPositionKey(order)
		if !seen[key] {
			seen[key] = true
			count++
		}
	}
	return count
}
```

Replace `countRecentOpenOrders` to delegate:

```go
func (at *AutoTrader) countRecentOpenOrders(since time.Time) (int, error) {
	orders, err := at.recentOrders(100)
	if err != nil {
		return 0, err
	}
	return countDistinctOpenEvents(orders, since.UTC().UnixMilli()), nil
}
```

- [ ] **Step 4: Make re-entry cooldown treat multi-fill closes as one**

Replace `findRecentCloseOrder`:

```go
func (at *AutoTrader) findRecentCloseOrder(symbol string, since time.Time) *store.TraderOrder {
	orders, err := at.recentOrders(100)
	if err != nil {
		at.logWarnf("⚠️ Trade throttle could not read recent close orders: %v", err)
		return nil
	}
	sinceMs := since.UTC().UnixMilli()
	// Latest close for the symbol (deduped by position-side), so a multi-fill
	// close is treated as a single close event.
	var latest *store.TraderOrder
	seen := map[string]bool{}
	for _, order := range orders {
		if order == nil || order.CreatedAt < sinceMs || isCanceledOrder(order) {
			continue
		}
		if !(isCloseAction(order.OrderAction) && normalizedDecisionSymbol(order.Symbol) == symbol) {
			continue
		}
		key := orderPositionKey(order)
		if seen[key] {
			continue
		}
		seen[key] = true
		if latest == nil || order.CreatedAt > latest.CreatedAt {
			latest = order
		}
	}
	return latest
}
```

- [ ] **Step 5: Run tests, verify pass**

Run: `go test ./trader/ -run TestCountDistinctOpenEvents -v`
Expected: PASS.

Run: `go test ./trader/ -run 'Test.*Throttle' -v`
Expected: all throttle tests pass.

- [ ] **Step 6: gofmt, vet, commit**

```bash
gofmt -w trader/auto_trader_throttle.go
go vet ./trader/...
git add trader/auto_trader_throttle.go trader/auto_trader_throttle_open_test.go
git commit -m "fix(trader): count distinct position opens and dedupe multi-fill closes in throttle"
```

---

## Task 5: Backend — Wire OI-liquidity filter to config toggle + threshold

**Files:**
- Modify: `kernel/engine_analysis.go` (fetchMarketDataWithStrategy lines 320-399; OI filter at lines 368-392)

**Interfaces:**
- Consumes: `store.RiskControlConfig.EnableOILiquidityFilter`, `OILiquidityFilterMinUSDT` (Task 1); the engine accessor `e.GetRiskControlConfig()` (kernel/engine.go:292-294); `market.IsXyzDexAsset`.
- Produces: a pure helper `keepCoinByOILiquidity(isExistingPosition, isXyzAsset bool, oiValue float64, enable bool, minThresholdUSDT float64) bool` returning true if the coin passes the filter. `fetchMarketDataWithStrategy` calls it. The helper is consumed by Task 5's test and production code.

- [ ] **Step 1: Write the failing test**

Create `kernel/engine_oi_filter_test.go` (pure-function test):

```go
package kernel

import "testing"

func TestKeepCoinByOILiquidity(t *testing.T) {
	// enable=true, threshold 15M, low OI -> drop (false)
	if keepCoinByOILiquidity(false, false, 1_000_000, true, 15_000_000) {
		t.Fatal("expected low-OI coin dropped when filter enabled")
	}
	// enable=false -> keep regardless of OI
	if !keepCoinByOILiquidity(false, false, 1_000_000, false, 15_000_000) {
		t.Fatal("expected low-OI coin kept when filter disabled")
	}
	// threshold lowered to 1M, OI == 1M -> keep (equal passes)
	if !keepCoinByOILiquidity(false, false, 1_000_000, true, 1_000_000) {
		t.Fatal("expected OI equal to threshold to pass")
	}
	// existing position or XYZ asset always kept
	if !keepCoinByOILiquidity(true, false, 1_000_000, true, 15_000_000) {
		t.Fatal("expected existing position always kept")
	}
	if !keepCoinByOILiquidity(false, true, 1_000_000, true, 15_000_000) {
		t.Fatal("expected XYZ asset always kept")
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `go test ./kernel/ -run TestKeepCoinByOILiquidity -v`
Expected: FAIL — `keepCoinByOILiquidity` undefined.

- [ ] **Step 3: Implement the pure helper and wire it in**

Add to `kernel/engine_analysis.go`:

```go
// keepCoinByOILiquidity decides whether a candidate coin passes the OI-liquidity
// filter. Existing positions and XYZ (non-perp) assets are always kept, matching
// prior behavior. When the filter is disabled the coin is kept.
func keepCoinByOILiquidity(isExistingPosition, isXyzAsset bool, oiValue float64, enable bool, minThresholdUSDT float64) bool {
	if isExistingPosition || isXyzAsset || !enable {
		return true
	}
	if oiValue <= 0 {
		return false
	}
	return oiValue >= minThresholdUSDT
}
```

In `fetchMarketDataWithStrategy`, replace the const and the inline filter block (lines 368-392):

```go
	riskCfg := engine.GetRiskControlConfig()
	enableOIFilter := riskCfg.EnableOILiquidityFilter
	minOITHRESHOLDUSDT := riskCfg.OILiquidityFilterMinUSDT
	if minOITHRESHOLDUSDT <= 0 {
		minOITHRESHOLDUSDT = 15_000_000 // preserve historical default
	}
```

and replace the `if oiValueInMillions < minOIThresholdMillions { continue }` block with a call:

```go
		// Liquidity filter (skip for xyz dex assets - they don't have OI data from Binance)
		isExistingPosition := positionSymbols[coin.Symbol]
		isXyzAsset := market.IsXyzDexAsset(coin.Symbol)
		if data.OpenInterest != nil && data.CurrentPrice > 0 {
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			if !keepCoinByOILiquidity(isExistingPosition, isXyzAsset, oiValue, enableOIFilter, minOITHRESHOLDUSDT) {
				logger.Infof("⚠️  %s OI value too low (%.2fM USD), skipping coin",
					coin.Symbol, oiValue/1_000_000)
				continue
			}
		}
```

> Note: preserve the existing guard ordering — the original only applied the filter when `!isExistingPosition && !isXyzAsset && data.OpenInterest != nil && data.CurrentPrice > 0`. The helper now encapsulates the position/XYZ exemption, so the call site can apply the same conditions. Double-check the `continue` only fires when the coin should be dropped (not when OI data is simply absent — an absent `OpenInterest`/zero price should keep the coin, as before).

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./kernel/ -run TestKeepCoinByOILiquidity -v`
Expected: PASS.

Run: `go test ./kernel/... -count=1`
Expected: no regressions (existing market-data/OI tests still pass).

- [ ] **Step 5: gofmt, vet, commit**

```bash
gofmt -w kernel/engine_analysis.go kernel/engine_oi_filter_test.go
go vet ./kernel/...
git add kernel/engine_analysis.go kernel/engine_oi_filter_test.go
git commit -m "feat(kernel): make OI-liquidity candidate filter configurable (toggle + threshold)"
```

---

## Task 6: Frontend — Extend types, factory, and preset logic

**Files:**
- Modify: `web/src/types/strategy.ts` (RiskControlConfig at line 222, StrategyConfig at line 41, AIStrategyConfig at line 60)
- Modify: `web/src/features/strategies/strategyFactory.ts` (StrategyEditorForm line 9, defaultRiskControl line 177, buildStrategyConfig line 204)

**Interfaces:**
- Consumes: existing `RiskControlConfig` (line 222) and `StrategyConfig` (line 41) interfaces; `StrategyEditorForm` (line 9).
- Produces: a `ThrottlingConfig` TS interface; extended `RiskControlConfig` (adds `throttling`, `enable_oi_liquidity_filter`, `oi_liquidity_filter_min_usdt`); `trading_style` on `StrategyConfig`; extended `StrategyEditorForm` fields; a `TRADING_STYLE_PRESETS` map + `applyTradingStyle(style, form)` helper; updated `defaultRiskControl` + `buildStrategyConfig`. Consumed by Task 7.

- [ ] **Step 1: Extend `web/src/types/strategy.ts`**

Add after `RiskControlConfig` (line 240):

```ts
export interface ThrottlingConfig {
  max_opens_per_hour: number;
  max_opens_per_cycle: number;
  min_hold_duration_min: number;
  noise_close_hold_duration_min: number;
  reentry_cooldown_min: number;
  early_close_stop_loss_bypass_pct: number;
  early_close_take_profit_bypass_pct: number;
  noise_close_loss_floor_pct: number;
  noise_close_profit_ceiling_pct: number;
}
```

Add to `RiskControlConfig`:

```ts
  enable_oi_liquidity_filter: boolean;
  oi_liquidity_filter_min_usdt: number;
  throttling: ThrottlingConfig;
```

Add to `StrategyConfig` (after `language`, line 46):

```ts
  trading_style?: 'scalp' | 'intraday' | 'swing' | 'default';
```

Add to `AIStrategyConfig` (after `risk_control`, line 64) if needed for nesting (not required since `risk_control` already nested):

```ts
  // no change needed; trading_style lives at StrategyConfig level
```

- [ ] **Step 2: Write the failing frontend test**

Create `web/src/features/strategies/strategyFactoryPresets.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { TRADING_STYLE_PRESETS, applyTradingStyle } from './strategyFactory'
import type { StrategyEditorForm } from './strategyFactory'

const baseForm: StrategyEditorForm = {
  name: 'x', custom_prompt: '', scan_interval_minutes: 5,
  btcEthMaxLeverage: 5, altcoinMaxLeverage: 5,
  btcEthPositionRatio: 5, altcoinPositionRatio: 1,
  isCrossMargin: true, selectedTimeframes: ['15m'], excludedCoins: [],
  decisionContext: { mode: 'structured', count: 10 },
  scopeUnits: [], scopeMode: 'union',
  // Section A / throttle form fields (all new required fields):
  maxPositions: 3, minPositionSize: 12, minRiskRewardRatio: 3.0,
  maxMarginUsage: 1.0, minConfidence: 78,
  enableOILiquidityFilter: true, oiLiquidityFilterMinUSDT: 15000000,
  maxOpensPerHour: 3, maxOpensPerCycle: 2,
  minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240,
  earlyCloseStopLossBypassPct: -3.0, earlyCloseTakeProfitBypassPct: 8.0,
  noiseCloseLossFloorPct: -2.0, noiseCloseProfitCeilingPct: 3.0,
}

describe('applyTradingStyle', () => {
  it('applies scalp values and clamps to section A caps', () => {
    const out = applyTradingStyle('scalp', { ...baseForm })
    expect(out.maxPositions).toBe(5)
    expect(out.minHoldDurationMin).toBe(10)
    expect(out.reentryCooldownMin).toBe(30)
    expect(out.minRiskRewardRatio).toBeGreaterThanOrEqual(1.0)
  })

  it('default style resets to baseline', () => {
    const out = applyTradingStyle('default', { ...baseForm })
    expect(out.maxPositions).toBe(3)
    expect(out.minHoldDurationMin).toBe(90)
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/strategyFactoryPresets.test.ts`
Expected: FAIL — `TRADING_STYLE_PRESETS` / `applyTradingStyle` not exported.

- [ ] **Step 4: Extend `strategyFactory.ts`**

Add to `StrategyEditorForm`:

```ts
  tradingStyle?: 'scalp' | 'intraday' | 'swing' | 'default'
  maxPositions: number
  minPositionSize: number
  minRiskRewardRatio: number
  maxMarginUsage: number
  minConfidence: number
  enableOILiquidityFilter: boolean
  oiLiquidityFilterMinUSDT: number
  maxOpensPerHour: number
  maxOpensPerCycle: number
  minHoldDurationMin: number
  noiseCloseHoldDurationMin: number
  reentryCooldownMin: number
  earlyCloseStopLossBypassPct: number
  earlyCloseTakeProfitBypassPct: number
  noiseCloseLossFloorPct: number
  noiseCloseProfitCeilingPct: number
```

> Note: `tradingStyle` is optional on the form (the `trading_style` field on the saved config is optional too), so existing form fixtures only need the required numeric/boolean fields added. `applyTradingStyle` returns a form with the selected `tradingStyle` set (see the helper below).

Add the preset map + helper:

```ts
export type TradingStyle = 'scalp' | 'intraday' | 'swing' | 'default'

export const TRADING_STYLE_PRESETS: Record<TradingStyle, Partial<StrategyEditorForm>> = {
  scalp: { maxPositions: 5, maxOpensPerHour: 8, maxOpensPerCycle: 4, minHoldDurationMin: 10, noiseCloseHoldDurationMin: 30, reentryCooldownMin: 30, minRiskRewardRatio: 1.5, minPositionSize: 12 },
  intraday: { maxPositions: 4, maxOpensPerHour: 5, maxOpensPerCycle: 3, minHoldDurationMin: 45, noiseCloseHoldDurationMin: 90, reentryCooldownMin: 120, minRiskRewardRatio: 2.0, minPositionSize: 12 },
  swing: { maxPositions: 2, maxOpensPerHour: 2, maxOpensPerCycle: 1, minHoldDurationMin: 360, noiseCloseHoldDurationMin: 720, reentryCooldownMin: 480, minRiskRewardRatio: 3.0, minPositionSize: 12 },
  default: { maxPositions: 3, maxOpensPerHour: 3, maxOpensPerCycle: 2, minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240, minRiskRewardRatio: 3.0, minPositionSize: 12 },
}

export function applyTradingStyle(style: TradingStyle, form: StrategyEditorForm): StrategyEditorForm {
  const patch = TRADING_STYLE_PRESETS[style]
  return { ...form, tradingStyle: style, ...patch }
}
```

Update `buildStrategyConfig` to set `trading_style` from `form.tradingStyle` and to map the new `risk_control` fields (max_positions, min_position_size, min_risk_reward_ratio, max_margin_usage, min_confidence, enable_oi_liquidity_filter, oi_liquidity_filter_min_usdt, throttling) from the form. Because the form now carries all fields, thread them through `defaultRiskControl`/the `risk_control` object explicitly.

Update `defaultRiskControl` to include the new fields (throttle defaults + OI):

```ts
    enable_oi_liquidity_filter: true,
    oi_liquidity_filter_min_usdt: 15000000,
    throttling: {
      max_opens_per_hour: 3,
      max_opens_per_cycle: 2,
      min_hold_duration_min: 90,
      noise_close_hold_duration_min: 180,
      reentry_cooldown_min: 240,
      early_close_stop_loss_bypass_pct: -3.0,
      early_close_take_profit_bypass_pct: 8.0,
      noise_close_loss_floor_pct: -2.0,
      noise_close_profit_ceiling_pct: 3.0,
    },
```

Update `buildStrategyConfig` to map the new form fields into the `risk_control` object (throttling + OI). Because `StrategyEditorForm` now has all the fields, thread them through explicitly.

**Update existing factory tests:** `web/src/features/strategies/strategyFactory.test.ts` constructs `StrategyEditorForm` literals at lines 68-82, 103-113, and 126-140 without the new required fields, so they will no longer type-check. Add the new fields to each fixture (reuse the same block as `baseForm` in Step 2's test — `maxPositions`, `minPositionSize`, `minRiskRewardRatio`, `maxMarginUsage`, `minConfidence`, `enableOILiquidityFilter`, `oiLiquidityFilterMinUSDT`, and all nine throttle fields). Verify with `cd web && npx tsc --noEmit`.

- [ ] **Step 5: Run test, verify pass**

Run: `cd web && npx vitest run src/features/strategies/strategyFactoryPresets.test.ts`
Expected: PASS.

Run: `cd web && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add web/src/types/strategy.ts web/src/features/strategies/strategyFactory.ts web/src/features/strategies/strategyFactoryPresets.test.ts
git commit -m "feat(web): add trading style presets and throttling/oi fields to strategy types and factory"
```

---

## Task 7: Frontend — Surface fields in the strategy editor UI

**Files:**
- Modify: `web/src/features/strategies/EditorStepPage.tsx` (Basic Rules fieldset ~line 307; Advanced Settings fieldset ~line 435; add Trading Style fieldset above Basic Rules; add Throttling Settings (Risky) fieldset after Advanced Settings; wire form state + save handler)

**Interfaces:**
- Consumes: `StrategyEditorForm` extended fields (Task 6), `TRADING_STYLE_PRESETS` + `applyTradingStyle` + `TradingStyle` type (Task 6), existing `NumberField`/`ToggleChip`/`Toggle` components (lines 543-638), existing form state + `handleSave`.
- Produces: UI controls bound to the new form fields; `trading_style` selected state; the save handler writes all new fields. No external consumers.

- [ ] **Step 1: Write the failing frontend test**

Create `web/src/features/strategies/EditorStepPage.test.tsx` (or extend existing) asserting that the Trading Style fieldset renders 4 chips, and the Throttling Settings (Risky) fieldset renders the min-hold input. `EditorStepPage` uses `useParams`, `useAuth`, and the zustand `useStrategyDraft` store, so wrap it in `MemoryRouter` + `AuthProvider`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import EditorStepPage from './EditorStepPage'
import { AuthProvider } from '../../contexts/AuthContext'

// vi.mock any auth-side effect (e.g. token fetch) that would otherwise hit the
// network; provide a minimal auth value via the provider's test seam if needed.

describe('EditorStepPage', () => {
  it('renders Trading Style chips and a Risky throttle fieldset', () => {
    render(
      <MemoryRouter>
        <AuthProvider>
          <EditorStepPage />
        </AuthProvider>
      </MemoryRouter>
    )
    expect(screen.getByText('Trading Style')).toBeTruthy()
    expect(screen.getByText('Scalp')).toBeTruthy()
    expect(screen.getByText('Throttling Settings (Risky)')).toBeTruthy()
    expect(screen.getByLabelText(/min hold/i)).toBeTruthy()
  })
})
```

> Note: `AuthProvider` may trigger a token-fetch effect on mount (check `contexts/AuthContext.tsx:281-293`). Mock it (e.g. `vi.mock('../../lib/api')` or set the token via the provider's state) so the test renders without network calls. If `AuthProvider` requires props, wrap the assertions to provide them. Verify the component's render contract by reading its source first; adjust the wrapper accordingly.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/EditorStepPage.test.tsx`
Expected: FAIL — labels not rendered yet.

- [ ] **Step 3: Add form state + handlers**

In `EditorStepPage.tsx`, add state for the new fields (or extend the existing single config state object): `maxPositions`, `minPositionSize`, `minRiskRewardRatio`, `maxMarginUsage`, `minConfidence`, `enableOILiquidityFilter`, `oiLiquidityFilterMinUSDT`, `maxOpensPerHour`, `maxOpensPerCycle`, `minHoldDurationMin`, `noiseCloseHoldDurationMin`, `reentryCooldownMin`, `earlyCloseStopLossBypassPct`, `earlyCloseTakeProfitBypassPct`, `noiseCloseLossFloorPct`, `noiseCloseProfitCeilingPct`, and `tradingStyle: TradingStyle`. Initialize from the loaded strategy's `RiskControlConfig` (defaulting to `defaultRiskControl`-style values when absent).

Add a handler:

```tsx
const applyStyle = (style: TradingStyle) => {
  setTradingStyle(style)
  const patch = TRADING_STYLE_PRESETS[style]
  if (patch.maxPositions !== undefined) setMaxPositions(patch.maxPositions)
  if (patch.minPositionSize !== undefined) setMinPositionSize(patch.minPositionSize)
  if (patch.minRiskRewardRatio !== undefined) setMinRiskRewardRatio(patch.minRiskRewardRatio)
  if (patch.maxOpensPerHour !== undefined) setMaxOpensPerHour(patch.maxOpensPerHour)
  if (patch.maxOpensPerCycle !== undefined) setMaxOpensPerCycle(patch.maxOpensPerCycle)
  if (patch.minHoldDurationMin !== undefined) setMinHoldDurationMin(patch.minHoldDurationMin)
  if (patch.noiseCloseHoldDurationMin !== undefined) setNoiseCloseHoldDurationMin(patch.noiseCloseHoldDurationMin)
  if (patch.reentryCooldownMin !== undefined) setReentryCooldownMin(patch.reentryCooldownMin)
}
```

- [ ] **Step 4: Add the Trading Style fieldset (top, above Basic Rules)**

```tsx
<fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
  <legend className="px-2 text-sm font-semibold text-nofx-text">Trading Style</legend>
  <div className="flex flex-wrap gap-2">
    {(['scalp', 'intraday', 'swing', 'default'] as TradingStyle[]).map((s) => (
      <ToggleChip key={s} label={s[0].toUpperCase() + s.slice(1)} active={tradingStyle === s} onClick={() => applyStyle(s)} />
    ))}
  </div>
</fieldset>
```

- [ ] **Step 5: Add Basic Rules inputs (max_positions, min_position_size)**

Inside the Basic Rules fieldset grid, after the existing Altcoin ratio input (line ~347), add:

```tsx
<NumberField label="Max concurrent positions" value={maxPositions} onChange={setMaxPositions} min={1} max={8} />
<NumberField label="Min position size (USDT)" value={minPositionSize} onChange={setMinPositionSize} min={10} max={1000} />
```

- [ ] **Step 6: Add Advanced Settings inputs (RR, margin, confidence, OI filter)**

Inside the Advanced Settings fieldset, add:

```tsx
<NumberField label="Min risk/reward ratio" value={minRiskRewardRatio} onChange={setMinRiskRewardRatio} min={1.0} max={10.0} step={0.5} />
<NumberField label="Max margin usage (%) — AI guidance" value={maxMarginUsage} onChange={setMaxMarginUsage} min={0.1} max={1.0} step={0.1} />
<NumberField label="Min AI confidence" value={minConfidence} onChange={setMinConfidence} min={50} max={100} />
<div className="mb-4">
  <span className="text-sm text-nofx-text-muted">OI-liquidity filter</span>
  <Toggle checked={enableOILiquidityFilter} onChange={() => setEnableOILiquidityFilter(!enableOILiquidityFilter)} label={enableOILiquidityFilter ? 'On' : 'Off'} />
  <input type="number" value={oiLiquidityFilterMinUSDT} onChange={(e) => setOILiquidityFilterMinUSDT(Number(e.target.value))} disabled={!enableOILiquidityFilter} className="mt-1 w-full rounded-lg border ... disabled:opacity-40" min={0} />
</div>
```

- [ ] **Step 7: Add Throttling Settings (Risky) fieldset**

After the Advanced Settings fieldset (after line 521), add a fieldset with `nofx-danger` styling (mirror the "Recent decisions context" panel pattern) containing:

```tsx
<NumberField label="Max opens per hour" value={maxOpensPerHour} onChange={setMaxOpensPerHour} min={1} max={100} />
<NumberField label="Max opens per cycle" value={maxOpensPerCycle} onChange={setMaxOpensPerCycle} min={1} max={100} />
<NumberField label="Min hold before close (min)" value={minHoldDurationMin} onChange={setMinHoldDurationMin} min={1} max={10080} />
<NumberField label="Noise-band close window (min)" value={noiseCloseHoldDurationMin} onChange={setNoiseCloseHoldDurationMin} min={1} max={10080} />
<NumberField label="Re-entry cooldown (min)" value={reentryCooldownMin} onChange={setReentryCooldownMin} min={1} max={10080} />
<NumberField label="Early-close stop-loss bypass (%)" value={earlyCloseStopLossBypassPct} onChange={setEarlyCloseStopLossBypassPct} min={-100} max={0} step={0.5} />
<NumberField label="Early-close take-profit bypass (%)" value={earlyCloseTakeProfitBypassPct} onChange={setEarlyCloseTakeProfitBypassPct} min={0} max={100} step={0.5} />
<NumberField label="Noise-band loss floor (%)" value={noiseCloseLossFloorPct} onChange={setNoiseCloseLossFloorPct} min={-100} max={0} step={0.5} />
<NumberField label="Noise-band profit ceiling (%)" value={noiseCloseProfitCeilingPct} onChange={setNoiseCloseProfitCeilingPct} min={0} max={100} step={0.5} />
```

- [ ] **Step 8: Update the save handler to include new fields**

Update `handleSave` to write `trading_style`, the new `risk_control` fields (throttling + OI), `min_position_size`, `min_risk_reward_ratio`, `max_margin_usage`, `min_confidence`, `max_positions` into the strategy config payload (extend `buildStrategyConfig` call / the payload object so all new state is persisted).

- [ ] **Step 9: Run tests, verify pass**

Run: `cd web && npx vitest run src/features/strategies/`
Expected: PASS.

Run: `cd web && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 10: Run full frontend checks**

Run: `cd web && npm run build && npm test`
Expected: build succeeds, tests pass.

- [ ] **Step 11: gofmt/vet backend (no backend change here, but verify tree still clean), commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && go vet ./... && gofmt -l .
git add web/src/features/strategies/EditorStepPage.tsx web/src/features/strategies/EditorStepPage.test.tsx
git commit -m "feat(web): surface trading style, throttling, and risk-control fields in strategy editor"
```

---

## Task 8: Full verification pass

**Files:** none (verification only).

- [ ] **Step 1: Run backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && gofmt -l .`
Expected: clean build, vet clean, no gofmt output.

- [ ] **Step 2: Run backend tests**

Run: `go test ./... -count=1`
Expected: all pass.

- [ ] **Step 3: Run frontend verification**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: clean, build succeeds, tests pass.

- [ ] **Step 4: Verify new-field round-trip**

Run the store tests specifically: `go test ./store/ -run 'TestThrottlingConfig|TestStrategy' -v` and confirm `TestThrottlingConfigDefaultsPreserveBehavior` + `TestThrottlingConfigClamping` pass, proving JSON round-trip of the new fields through `GetDefaultStrategyConfig` + `ClampLimits`.

- [ ] **Step 5: Final review of `git status` and commit any stragglers**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git status
```
Expected: clean working tree (all changes committed across Tasks 1-7).
