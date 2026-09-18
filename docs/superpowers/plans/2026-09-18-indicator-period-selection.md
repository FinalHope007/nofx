# Indicator Period Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the backend compute and feed the LLM exactly the technical-indicator periods the user selected in the frontend (EMA 9/10/20/50/200, RSI 7/14/21, ATR 7/14/21, BOLL 10/20/50), gated by each indicator's enable toggle.

**Architecture:** Add fixed optional series/scalar fields to `market.TimeframeSeriesData` for every selectable period, carry an `IndicatorPeriods` value through the market fetch path (runtime + test-run only), compute the selected periods alongside the existing hardcoded ones, and have `formatTimeframeSeriesData` print the selected periods. Legacy EMA20/50, RSI7/14, ATR14, BOLL20 fields are retained so the grid engine and old data paths keep working. MACD stays unchanged.

**Tech Stack:** Go (backend), TypeScript/React (frontend, no changes needed).

## Global Constraints

- Do NOT import `store` from package `market` (import cycle). `IndicatorPeriods` lives in `market`.
- Keep legacy fields `EMA20Values`, `EMA50Values`, `RSI7Values`, `RSI14Values`, `ATR14`, `BOLLUpper/Middle/Lower` populated unchanged for backward compatibility.
- MACD remains 12/26 line-only (no signal/histogram).
- Grid/trader callers (`trader/auto_trader_grid.go`, `auto_trader_grid_levels.go`) must compile unchanged.
- Honor `EnableVolume`: the standalone `Volume:` series must be gated by it (raw OHLCV table stays).
- Verification per task: `go build ./... && go vet ./...` and relevant `go test`.
- Commits: concise, match repo style; only commit when the task says so.
- No comments added to code.

---

### Task 1: Add fixed period fields + `IndicatorPeriods` to market types

**Files:**
- Modify: `market/types.go`
- Test: `market/types_periods_test.go` (create)

**Interfaces:**
- Produces: `market.IndicatorPeriods{EMA, RSI, ATR, BOLL []int}`; new fields on `TimeframeSeriesData`: `Periods IndicatorPeriods`, `EMA9Values`, `EMA10Values`, `EMA200Values`, `RSI21Values`, `ATR7`, `ATR21`, `BOLL10Upper/Middle/Lower`, `BOLL50Upper/Middle/Lower` (all `[]float64` series except `ATR7`/`ATR21` which are `float64`).

- [ ] **Step 1: Write failing test** asserting `IndicatorPeriods{}` zero value and that a `TimeframeSeriesData` literal accepting the new fields compiles and holds values.

```go
package market

import "testing"

func TestIndicatorPeriodsFields(t *testing.T) {
    d := TimeframeSeriesData{
        Periods:     IndicatorPeriods{EMA: []int{10}, RSI: []int{21}, ATR: []int{7}, BOLL: []int{50}},
        EMA10Values: []float64{1.5},
        RSI21Values: []float64{55.0},
        ATR7:        2.5,
        BOLL50Upper: []float64{9.9},
    }
    if d.Periods.EMA[0] != 10 || d.EMA10Values[0] != 1.5 || d.ATR7 != 2.5 || d.BOLL50Upper[0] != 9.9 {
        t.Fatalf("new indicator fields not stored: %+v", d)
    }
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./market/ -run TestIndicatorPeriodsFields -v` (compile error: unknown fields).

- [ ] **Step 3: Implement** — add the `IndicatorPeriods` struct and all new fields to `TimeframeSeriesData` in `market/types.go`.

- [ ] **Step 4: Run** `go test ./market/ -run TestIndicatorPeriodsFields -v` and `go build ./...`. Expected PASS.

- [ ] **Step 5: Commit** — `git add market/types.go market/types_periods_test.go && git commit -m "feat(market): add fixed indicator period fields"`

---

### Task 2: Compute selected periods in `calculateTimeframeSeries`

**Files:**
- Modify: `market/data_klines.go:176-259`
- Test: `market/data_klines_periods_test.go` (create)

**Interfaces:**
- Consumes: `market.IndicatorPeriods` (Task 1).
- Produces: `calculateTimeframeSeries(klines []Kline, timeframe string, count int, periods IndicatorPeriods) *TimeframeSeriesData` — new 4th param. Populates legacy fields exactly as before AND the newly selected period fields.

- [ ] **Step 1: Write failing test** — with `generateTestKlines(120)` and `IndicatorPeriods{EMA:[]int{10}, RSI:[]int{21}, ATR:[]int{7}, BOLL:[]int{50}}`, assert:
  - `data.EMA10Values` non-empty and last value ≈ `ExportCalculateEMA(klines, 10)`.
  - `data.RSI21Values` last ≈ `ExportCalculateRSI(klines, 21)`.
  - `data.ATR7` ≈ `ExportCalculateATR(klines, 7)`.
  - `data.BOLL50Upper` last ≈ upper from `ExportCalculateBOLL(klines, 50, 2.0)`.
  - legacy `data.EMA20Values` still non-empty.

- [ ] **Step 2: Run** `go test ./market/ -run TestCalculateTimeframeSeriesPeriods -v` — expect FAIL (signature/fields missing).

- [ ] **Step 3: Implement**:
  - Change signature to accept `periods IndicatorPeriods`; set `data.Periods = periods`.
  - Inside the existing per-bar loop, add guarded blocks computing each selected period (EMA≥ period-1, RSI≥ period, BOLL≥ period-1), mirroring the legacy appends.
  - After the loop compute scalar ATRs for selected periods (`ATR7`, `ATR21`) via `calculateATR`, keeping `data.ATR14` as-is.
  - Keep legacy EMA20/50, RSI7/14, ATR14, BOLL20 blocks unchanged.
  - Update the single caller `calculateTimeframeSeries(klines, tf, count)` in `market/data.go:237` to pass an `IndicatorPeriods` param threaded from Task 3 (for now, pass zero value to keep compiling).

- [ ] **Step 4: Run** `go test ./market/... -v`. Expected PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(market): compute user-selected indicator periods"`

---

### Task 3: Thread `IndicatorPeriods` through `data.go`

**Files:**
- Modify: `market/data.go:148-283` (`GetWithTimeframes`, `GetWithTimeframesWithExchange`)

**Interfaces:**
- Consumes: `IndicatorPeriods` (Task 1), `calculateTimeframeSeries(..., periods)` (Task 2).
- Produces: `GetWithTimeframesWithExchange(symbol string, timeframes []string, primaryTimeframe string, count int, exchange string, periods IndicatorPeriods) (*Data, error)`; `GetWithTimeframes(symbol string, timeframes []string, primaryTimeframe string, count int, periods IndicatorPeriods) (*Data, error)`.

- [ ] **Step 1: Implement** — add `periods IndicatorPeriods` param to both; pass it to `calculateTimeframeSeries`.
- [ ] **Step 2: Update callers to compile**: `kernel/engine_analysis.go:433,462` and `api/strategy.go:627` pass a zero value for now (filled in Tasks 4–5); grid/trader callers use `GetWithTimeframes(... , market.IndicatorPeriods{})`.
- [ ] **Step 3: Run** `go build ./... && go test ./market/... ./trader/...`. Expected PASS.
- [ ] **Step 4: Commit** — `git commit -m "feat(market): thread indicator periods through timeframe fetch"`

---

### Task 4: Feed runtime config periods in `engine_analysis.go`

**Files:**
- Modify: `kernel/engine_analysis.go:433,462`

**Interfaces:**
- Consumes: `store.IndicatorConfig` fields `EMAPeriods/RSIPeriods/ATRPeriods/BOLLPeriods`; `market.IndicatorPeriods`.

- [ ] **Step 1: Write failing test** (`kernel/engine_analysis_test.go`) — build a minimal `store.StrategyConfig` with `Indicators.EMAPeriods=[]int{10}` and assert the constructed `market.IndicatorPeriods` has `EMA==[]int{10}` via a small extracted helper `indicatorPeriodsFor(store.IndicatorConfig) market.IndicatorPeriods`.
- [ ] **Step 2: Run** to fail.
- [ ] **Step 3: Implement** — add helper `indicatorPeriodsFor` in `engine_analysis.go`, use it in both fetch calls.
- [ ] **Step 4: Run** `go test ./kernel/ -run TestIndicatorPeriodsFor -v`. PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(kernel): pass strategy indicator periods to market fetch"`

---

### Task 5: Feed test-run config periods in `api/strategy.go`

**Files:**
- Modify: `api/strategy.go:627`

- [ ] **Step 1: Implement** — build `market.IndicatorPeriods` from `req.Config.Indicators` and pass it to `GetWithTimeframes`.
- [ ] **Step 2: Run** `go build ./... && go test ./api/...`. PASS.
- [ ] **Step 3: Commit** — `git commit -m "feat(api): pass indicator periods in strategy test-run"`

---

### Task 6: Format selected periods for the LLM prompt

**Files:**
- Modify: `kernel/engine_prompt.go:1568-1627` (`formatTimeframeSeriesData`)
- Test: `kernel/engine_prompt_test.go`

**Interfaces:**
- Consumes: `data.Periods`, new period fields (Task 1).

- [ ] **Step 1: Write failing test** — `TestFormatTimeframeSeriesDataHonorsSelectedPeriods`: config `EnableEMA/RSI/ATR/BOLL=true`, periods EMA[10] RSI[21] ATR[7] BOLL[50]; data has corresponding fields; assert output contains `EMA10`, `RSI21`, `ATR7`, `BOLL` with 50-period values, and does NOT contain `EMA20`, `EMA50`, `RSI7`, `RSI14`, `ATR14`.
- [ ] **Step 2: Run** `go test ./kernel/ -run TestFormatTimeframeSeriesDataHonorsSelectedPeriods -v` — FAIL.
- [ ] **Step 3: Implement**:
  - In `formatTimeframeSeriesData`, when `data.Periods` is non-empty, iterate `Periods.EMA`/`RSI`/`BOLL` mapping period→field (switch on 9/10/20/50/200 etc.) and print labeled blocks; print `ATR{period}` for each `Periods.ATR`.
  - When `Periods` is empty, fall back to the existing hardcoded EMA20/EMA50/RSI7/RSI14/ATR14/BOLL blocks (preserves `BuildDataFromKlines`/old data).
  - Keep `Enable*` gates.
- [ ] **Step 4: Add volume-gating test** — `enable_volume:false` data yields no standalone `Volume:` line in the timeframes path; `enable_volume:true` yields it.
- [ ] **Step 5: Run** `go test ./kernel/... ./market/...`. PASS.
- [ ] **Step 6: Commit** — `git commit -m "feat(kernel): format user-selected indicator periods in prompt"`

---

### Task 7: Full verification

- [ ] **Step 1:** `go build ./... && go vet ./... && go test ./market/... ./kernel/... ./api/... ./trader/...`
- [ ] **Step 2:** `cd web && npx tsc --noEmit && npm test`
- [ ] **Step 3:** Confirm no regressions in grid engine tests (`go test ./trader/... ./kernel/... -run Grid`).

---

## Self-Review
- **Spec coverage:** EMA/RSI/ATR/BOLL selected periods ✓ (T1–T2,T6), runtime + test-run plumbing ✓ (T4–T5), volume gating ✓ (T6), MACD unchanged ✓, grid compat ✓ (legacy fields retained T2), formula correctness verified in investigation ✓.
- **Type consistency:** `IndicatorPeriods{EMA,RSI,ATR,BOLL []int}` used consistently; `calculateTimeframeSeries` 4th param; `GetWithTimeframes` gains `periods`.
- **Placeholders:** none.
