# Open-Position TP/SL, Entry Timestamp, and Close-Reason Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show each open position's entry time / SL / TP in the LLM prompt, classify each recent close as LLM decision vs TP vs SL, and expand recent trades to 15.

**Architecture:** Persist the AI's validated SL/TP onto the open `TraderPosition` at execution time; carry entry time + SL/TP into `kernel.PositionInfo` and render them. Derive each recent trade's close reason at read time in `GetRecentTrades` — first by a **successful** LLM close decision in the decision log, else by exit price vs recorded SL/TP corroborated by PnL sign. Tag and tally closes in the prompt.

**Tech Stack:** Go, GORM (SQLite/Postgres), existing `store` / `kernel` / `trader` packages.

## Global Constraints

- No code comments may be added to production code (existing trailing comments on edited lines may remain).
- Do NOT change MACD behavior.
- Every opened position already has SL/TP > 0 enforced at `kernel/engine_position.go:81` — rely on this; do not weaken it.
- LLM classification REQUIRES `success == true` on the close decision. A blocked/throttled close (`success:false`) must NOT classify as `llm`; it falls through to SL/TP/PnL.
- SL/TP set or persistence failure must never abort the trade; it logs and leaves SL/TP at 0 (rendered `none`).
- `PositionBuilder.handleOpen` must never zero an existing position's SL/TP when averaging.
- Legacy positions with no recorded SL/TP render `none` and classify by successful-LLM-decision then PnL sign.
- Verification per task: `go build ./... && go vet ./...` plus the task's tests. Final: `go test ./...`.
- Commits: concise conventional commits; only commit the task's files.

---

## File Structure

- `store/position.go` — add `StopLoss`/`TakeProfit` columns; add `UpdatePositionSLTP`; add `DecisionRecord`-range query lives in `store/decision.go`.
- `store/position_builder.go` — preserve SL/TP on averaging.
- `store/position_query.go` — `RecentTrade.CloseReason`; classification in `GetRecentTrades`.
- `store/decision.go` — `GetRecordsInRange`.
- `trader/auto_trader_orders.go` — persist SL/TP after setting orders.
- `trader/auto_trader_loop.go` — populate `PositionInfo` SL/TP; limit 10→15; copy `CloseReason`.
- `kernel/engine.go` — `PositionInfo` + `RecentOrder` fields.
- `kernel/engine_prompt.go` — position line rendering; recent-trades tags + tally; Decision Process line.
- `kernel/formatter.go` — `formatRecentTradesEN` (legacy path) tags + tally.
- `store/strategy.go` — default `DecisionProcess` templates.

---

### Task 1: Persist SL/TP columns on positions

**Files:**
- Modify: `store/position.go:97-126` (struct), add method near `UpdatePositionExchangeInfo` (~:291)
- Test: `store/position_sltp_test.go` (create)

**Interfaces:**
- Produces: `TraderPosition.StopLoss float64` (`json:"stop_loss"`), `TraderPosition.TakeProfit float64` (`json:"take_profit"`); `(*PositionStore).UpdatePositionSLTP(id int64, stopLoss, takeProfit float64) error`.

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"testing"
	"time"
)

func TestUpdatePositionSLTP(t *testing.T) {
	st := newTestStore(t)
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "HBARUSDT", Side: "LONG",
		Quantity: 100, EntryPrice: 0.0752, EntryTime: time.Now().UTC().UnixMilli(),
		Leverage: 5, Status: "OPEN",
	}
	if err := st.Position().CreateOpenPosition(pos); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Position().UpdatePositionSLTP(pos.ID, 0.0714, 0.0827); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.Position().GetOpenPositionBySymbol("t1", "HBARUSDT", "LONG")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StopLoss != 0.0714 || got.TakeProfit != 0.0827 {
		t.Fatalf("SL/TP = %v/%v, want 0.0714/0.0827", got.StopLoss, got.TakeProfit)
	}
}
```

Use the existing test-store constructor pattern from `store/position_test.go`; if no `newTestStore` helper exists, follow that file's setup verbatim (it constructs a `Store` over an in-memory/temp DB) rather than inventing one.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./store/ -run TestUpdatePositionSLTP -v`
Expected: FAIL — unknown fields `StopLoss`/`TakeProfit` and undefined `UpdatePositionSLTP`.

- [ ] **Step 3: Implement**

Add to `TraderPosition` after `Leverage`:
```go
	StopLoss           float64 `gorm:"column:stop_loss;default:0" json:"stop_loss"`
	TakeProfit         float64 `gorm:"column:take_profit;default:0" json:"take_profit"`
```

Add method:
```go
func (s *PositionStore) UpdatePositionSLTP(id int64, stopLoss, takeProfit float64) error {
	nowMs := time.Now().UTC().UnixMilli()
	return s.db.Model(&TraderPosition{}).Where("id = ?", id).Updates(map[string]interface{}{
		"stop_loss":   stopLoss,
		"take_profit": takeProfit,
		"updated_at":  nowMs,
	}).Error
}
```

Confirm `AutoMigrate(&TraderPosition{})` in `InitTables` picks up the columns (it is already called).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./store/ -run TestUpdatePositionSLTP -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add store/position.go store/position_sltp_test.go
git commit -m "feat(store): add stop_loss/take_profit columns to positions"
```

---

### Task 2: Preserve SL/TP when averaging in PositionBuilder

**Files:**
- Modify: `store/position_builder.go:45-93`
- Test: `store/position_builder_sltp_test.go` (create)

**Interfaces:**
- Consumes: `TraderPosition.StopLoss`/`TakeProfit` (Task 1).

- [ ] **Step 1: Write the failing test**

Create an OPEN position with SL/TP set, then call `ProcessTrade` with an `open_long` for the same symbol/side at a different price, and assert SL/TP are unchanged and quantity increased.

```go
package store

import (
	"testing"
	"time"
)

func TestProcessTradeAveragingPreservesSLTP(t *testing.T) {
	st := newTestStore(t)
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 10, EntryPrice: 0.60, EntryTime: time.Now().UTC().UnixMilli(),
		Leverage: 5, Status: "OPEN", StopLoss: 0.50, TakeProfit: 0.80,
	}
	if err := st.Position().CreateOpenPosition(pos); err != nil {
		t.Fatalf("create: %v", err)
	}
	pb := NewPositionBuilder(st.Position())
	if err := pb.ProcessTrade("t1", "ex", "binance", "BRUSDT", "LONG", "open_long", 5, 0.70, 0, 0, time.Now().UTC().UnixMilli(), "o2"); err != nil {
		t.Fatalf("process: %v", err)
	}
	got, _ := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "LONG")
	if got.StopLoss != 0.50 || got.TakeProfit != 0.80 {
		t.Fatalf("SL/TP mutated to %v/%v, want 0.50/0.80", got.StopLoss, got.TakeProfit)
	}
	if got.Quantity <= 10 {
		t.Fatalf("quantity not averaged: %v", got.Quantity)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./store/ -run TestProcessTradeAveragingPreservesSLTP -v`
Expected: PASS on SL/TP if `UpdatePositionQuantityAndPrice` never touches those columns — if it already passes, add an explicit assertion path only if it fails. (This test documents the invariant; if green immediately, keep it as a regression guard and proceed.)

- [ ] **Step 3: Implement (only if the test fails)**

`UpdatePositionQuantityAndPrice` (`store/position.go:213`) updates only quantity/entry_price/fee/updated_at, so no change should be needed. If `handleOpen` creates a merged row that bypasses that method, change it to call `UpdatePositionQuantityAndPrice` so SL/TP are untouched. Do not add SL/TP to the sync-create path (new sync rows legitimately start at 0).

- [ ] **Step 4: Run**

Run: `go test ./store/ -run TestProcessTrade -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add store/position_builder.go store/position_builder_sltp_test.go
git commit -m "test(store): guard SL/TP preservation on position averaging"
```

---

### Task 3: Persist SL/TP at open execution

**Files:**
- Modify: `trader/auto_trader_orders.go:152-158` and `:270-276`
- Test: not unit-testable without an exchange; covered by Task 8 prompt/classification tests. No test in this task.

**Interfaces:**
- Consumes: `(*PositionStore).UpdatePositionSLTP` (Task 1), `(*PositionStore).GetOpenPositionBySymbol`.

- [ ] **Step 1: Implement**

In `executeOpenLongWithRecord`, replace the SL/TP block so that after a successful set it persists onto the position:
```go
	if err := at.trader.SetStopLoss(exchangeSymbol, "LONG", quantity, decision.StopLoss); err != nil {
		logger.Infof("  ⚠ Failed to set stop loss: %v", err)
	} else if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, market.Normalize(exchangeSymbol), "LONG"); err == nil && openPos != nil {
			if err := at.store.Position().UpdatePositionSLTP(openPos.ID, decision.StopLoss, decision.TakeProfit); err != nil {
				logger.Infof("  ⚠ Failed to persist SL/TP: %v", err)
			}
		}
	}
	if err := at.trader.SetTakeProfit(exchangeSymbol, "LONG", quantity, decision.TakeProfit); err != nil {
		logger.Infof("  ⚠ Failed to set take profit: %v", err)
	}
```
Note: the position row may be created asynchronously by OrderSync, so `GetOpenPositionBySymbol` can return nil momentarily. That is acceptable — SL/TP is best-effort; do not block. Mirror the same pattern for `executeOpenShortWithRecord` with `"SHORT"`.

- [ ] **Step 2: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add trader/auto_trader_orders.go
git commit -m "feat(trader): persist AI stop-loss/take-profit on open"
```

---

### Task 4: Carry entry time + SL/TP into PositionInfo

**Files:**
- Modify: `kernel/engine.go:29-42`
- Modify: `trader/auto_trader_loop.go:565-578`

**Interfaces:**
- Produces: `PositionInfo.EntryTime int64` (`json:"entry_time"`), `PositionInfo.StopLoss float64`, `PositionInfo.TakeProfit float64`.

- [ ] **Step 1: Implement struct fields**

Add to `PositionInfo`:
```go
	EntryTime        int64   `json:"entry_time"`
	StopLoss         float64 `json:"stop_loss"`
	TakeProfit       float64 `json:"take_profit"`
```

- [ ] **Step 2: Populate from the DB row**

In `trader/auto_trader_loop.go`, the loop already fetches `dbPos` at ~:540 for `updateTime`. Extend it to capture SL/TP:
```go
		var sl, tp float64
		if at.store != nil {
			if dbPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, side); err == nil && dbPos != nil {
				if dbPos.EntryTime > 0 {
					updateTime = dbPos.EntryTime
				}
				sl = dbPos.StopLoss
				tp = dbPos.TakeProfit
			}
		}
```
Then add to the `kernel.PositionInfo{...}` literal:
```go
			EntryTime:        updateTime,
			StopLoss:         sl,
			TakeProfit:       tp,
```

- [ ] **Step 3: Build and vet**

Run: `go build ./... && go vet ./... && go test ./trader/... ./kernel/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add kernel/engine.go trader/auto_trader_loop.go
git commit -m "feat(kernel,trader): carry entry time and SL/TP into position info"
```

---

### Task 5: Render SL/TP + Opened in the position line

**Files:**
- Modify: `kernel/engine_prompt.go:962-1007` (`formatPositionInfo`)
- Test: `kernel/engine_prompt_test.go`

**Interfaces:**
- Consumes: `PositionInfo.EntryTime`, `.StopLoss`, `.TakeProfit` (Task 4).

- [ ] **Step 1: Write the failing test**

```go
func TestFormatPositionInfoRendersSLTPAndOpened(t *testing.T) {
	cfg := &store.StrategyConfig{}
	e := NewStrategyEngine(cfg)
	ctx := &Context{}
	pos := PositionInfo{
		Symbol: "HBARUSDT", Side: "long", EntryPrice: 0.0752, MarkPrice: 0.0752,
		Quantity: 160, Leverage: 5, MarginUsed: 2, LiquidationPrice: 0.0608,
		UnrealizedPnLPct: -0.42, UnrealizedPnL: -0.01, PeakPnLPct: 0.52,
		EntryTime: 1758112993000, StopLoss: 0.0714, TakeProfit: 0.0827,
	}
	out := e.formatPositionInfo(1, pos, ctx)
	for _, want := range []string{"SL 0.0714", "TP 0.0827", "Opened"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
```

Also add a fallback test with `StopLoss:0, TakeProfit:0` asserting the output contains `SL none` and `TP none`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./kernel/ -run TestFormatPositionInfoRendersSLTPAndOpened -v`
Expected: FAIL — missing labels.

- [ ] **Step 3: Implement**

In `formatPositionInfo`, compute distance strings and the opened timestamp, then extend the `fmt.Sprintf` line. Add after `positionValue`:
```go
	opened := ""
	if pos.EntryTime > 0 {
		opened = " | Opened " + time.Unix(pos.EntryTime/1000, 0).UTC().Format("01-02 15:04 UTC")
	}

	slTp := " | SL none TP none"
	if pos.StopLoss > 0 || pos.TakeProfit > 0 {
		slStr, tpStr := "none", "none"
		if pos.StopLoss > 0 {
			slStr = market.FormatPriceSigFigs(pos.StopLoss)
		}
		if pos.TakeProfit > 0 {
			tpStr = market.FormatPriceSigFigs(pos.TakeProfit)
		}
		slTp = fmt.Sprintf(" | SL %s TP %s", slStr, tpStr)
		if pos.EntryPrice > 0 {
			dist := ""
			if pos.StopLoss > 0 {
				if strings.EqualFold(pos.Side, "short") {
					dist += fmt.Sprintf(" | SL Dist %+.2f%%", (pos.EntryPrice-pos.StopLoss)/pos.EntryPrice*100)
				} else {
					dist += fmt.Sprintf(" | SL Dist %+.2f%%", (pos.StopLoss-pos.EntryPrice)/pos.EntryPrice*100)
				}
			}
			if pos.TakeProfit > 0 {
				if strings.EqualFold(pos.Side, "short") {
					dist += fmt.Sprintf(" TP Dist %+.2f%%", (pos.EntryPrice-pos.TakeProfit)/pos.EntryPrice*100)
				} else {
					dist += fmt.Sprintf(" TP Dist %+.2f%%", (pos.TakeProfit-pos.EntryPrice)/pos.EntryPrice*100)
				}
			}
			if dist != "" {
				slTp += dist
			}
		}
	}
```
Then change the final format to append `slTp` before the holding duration, and place `opened` immediately before it. Concretely, precede `holdingDuration` by constructing:
```go
	openedHold := opened
	if holdingDuration != "" {
		trimmed := strings.TrimPrefix(holdingDuration, " | ")
		openedHold += " (" + strings.TrimPrefix(trimmed, "Holding Duration ") + ")"
	}
```
and append `slTp + openedHold` to the `Sprintf` argument list, adjusting the format string accordingly. Ensure the existing `Holding Duration` text is not duplicated. Keep the result: `... | Liq Price X | SL a TP b | SL Dist c% TP Dist d% | Opened MM-DD HH:MM UTC (Holding 13 min)`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./kernel/ -run TestFormatPositionInfo -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add kernel/engine_prompt.go kernel/engine_prompt_test.go
git commit -m "feat(kernel): render open-position SL/TP, distances, and entry time"
```

---

### Task 6: Close-reason classification in GetRecentTrades

**Files:**
- Modify: `store/decision.go` (add `GetRecordsInRange`)
- Modify: `store/position_query.go:117-170`
- Test: `store/recent_trades_classify_test.go` (create)

**Interfaces:**
- Consumes: `TraderPosition.StopLoss`/`TakeProfit` (Task 1); `DecisionRecord.Decisions []DecisionAction` with `.Action`, `.Symbol`, `.Success`.
- Produces: `RecentTrade.CloseReason string`; `(*DecisionStore).GetRecordsInRange(traderID string, from, to time.Time) ([]*DecisionRecord, error)`.
- Reason values: `"llm"`, `"tp"`, `"sl"`, `"exchange"`.

- [ ] **Step 1: Write the failing tests**

Cover: successful LLM close → `llm`; blocked close (`success:false`) + loss at SL → `sl`; no decision + loss → `sl`; no decision + profit → `tp`; legacy (no SL/TP, zero PnL) → `exchange`. Seed `trader_positions` (CLOSED) and `decision_records` directly via the test store.

Example (one case):
```go
func TestGetRecentTradesClassifiesLLMClose(t *testing.T) {
	st := newTestStore(t)
	now := time.Now().UTC()
	closeRec := &DecisionRecord{
		TraderID: "t1", CycleNumber: 2, Timestamp: now,
		Success: true,
		Decisions: []DecisionAction{{Action: "close_long", Symbol: "BRUSDT", Success: true}},
	}
	if err := st.Decision().LogDecision(closeRec); err != nil {
		t.Fatalf("log: %v", err)
	}
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 0, EntryPrice: 0.60, ExitPrice: 0.63, Leverage: 5,
		EntryTime: now.Add(-30 * time.Minute).UnixMilli(),
		ExitTime:  now.UnixMilli(), RealizedPnL: 1.0, Status: "CLOSED",
		StopLoss: 0.50, TakeProfit: 0.80,
	}
	if err := st.Position().Create(pos); err != nil {
		t.Fatalf("create: %v", err)
	}
	trades, err := st.Position().GetRecentTrades("t1", 15)
	if err != nil || len(trades) != 1 {
		t.Fatalf("trades=%v err=%v", trades, err)
	}
	if trades[0].CloseReason != "llm" {
		t.Fatalf("reason=%q want llm", trades[0].CloseReason)
	}
}
```
Add a blocked-close variant asserting `trades[0].CloseReason == "sl"` when exit hit SL and the decision had `Success:false`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./store/ -run TestGetRecentTradesClassifies -v`
Expected: FAIL — unknown field `CloseReason`.

- [ ] **Step 3: Implement**

Add to `RecentTrade`: `CloseReason string `json:"close_reason"``.

Add `GetRecordsInRange`:
```go
func (s *DecisionStore) GetRecordsInRange(traderID string, from, to time.Time) ([]*DecisionRecord, error) {
	var dbRecords []*DecisionRecordDB
	err := s.db.Where("trader_id = ? AND timestamp >= ? AND timestamp <= ?", traderID, from.UTC(), to.UTC()).
		Order("timestamp ASC").Find(&dbRecords).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query decision records in range: %w", err)
	}
	records := make([]*DecisionRecord, len(dbRecords))
	for i, db := range dbRecords {
		records[i] = db.toRecord()
	}
	return records, nil
}
```

In `GetRecentTrades`, after loading positions and before/while building trades, fetch decisions once over `[minExit-15m, maxExit+15m]` and build an in-memory set of successful close keys. `PositionStore` has no `DecisionStore` reference; add a lightweight classifier as a method on `PositionStore` that accepts the decision records, or expose a store-level helper `(*Store).GetRecentTradesWithReason`. Prefer adding `func (s *PositionStore) ClassifyRecentlyClosed(traderID string, limit int, decisions []*DecisionRecord) ([]RecentTrade, error)` is over-engineering — instead keep `GetRecentTrades` signature and have it accept decisions is a breaking change to callers. **Chosen approach:** add a new method on `Store` (which owns both sub-stores) in `store/position_query.go`:
```go
func (s *Store) GetRecentTradesWithReason(traderID string, limit int) ([]RecentTrade, error)
```
It loads recent closed positions, fetches `GetRecordsInRange` via `s.Decision()`, classifies, and returns. Keep the old `GetRecentTrades` untouched for other callers. Update `trader/auto_trader_loop.go:652` to call the new method (Task 7).

Classification helper (pure, unit-testable):
```go
func classifyClose(pos TraderPosition, successfulLLMCloses map[string][]int64, tol float64) string {
	side := strings.ToLower(pos.Side)
	for _, et := range successfulLLMCloses[pos.Symbol+"|"+side] {
		if abs64(et-pos.ExitTime) <= 15*60*1000 {
			return "llm"
		}
	}
	exit := pos.ExitPrice
	if pos.StopLoss > 0 {
		if side == "long" && exit <= pos.StopLoss*(1+tol) {
			return "sl"
		}
		if side == "short" && exit >= pos.StopLoss*(1-tol) {
			return "sl"
		}
	}
	if pos.TakeProfit > 0 {
		if side == "long" && exit >= pos.TakeProfit*(1-tol) {
			return "tp"
		}
		if side == "short" && exit <= pos.TakeProfit*(1+tol) {
			return "tp"
		}
	}
	if pos.RealizedPnL < 0 {
		return "sl"
	}
	if pos.RealizedPnL > 0 {
		return "tp"
	}
	return "exchange"
}
```
Build `successfulLLMCloses` only from decisions with `Success == true` and action `close_long`/`close_side` mapping to the position side (`close_long`→`long`, `close_short`→`short`), keyed `symbol|side`, value = decision timestamp millis. Use `tol = 0.001`. Add an `abs64` helper if none exists in the package.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./store/ -run 'TestGetRecentTradesClassifies|TestClassify' -v`
Expected: PASS. Add a table test for the blocked-close case.

- [ ] **Step 5: Commit**

```bash
git add store/decision.go store/position_query.go store/recent_trades_classify_test.go
git commit -m "feat(store): classify recent trade close reason"
```

---

### Task 7: Recent trades 10 → 15 + carry reason into prompt

**Files:**
- Modify: `trader/auto_trader_loop.go:652` (limit + call) and the `ctx.RecentOrders` append (~:668)
- Modify: `kernel/engine.go:83-93` (`RecentOrder`)
- Modify: `kernel/engine_prompt.go:804-819` (recent trades section)
- Modify: `kernel/formatter.go:463-491` (`formatRecentTradesEN`)

**Interfaces:**
- Consumes: `RecentTrade.CloseReason` and `Store.GetRecentTradesWithReason` (Task 6).
- Produces: `RecentOrder.CloseReason string`.

- [ ] **Step 1: Write the failing test**

```go
func TestRecentTradesPromptShowsCloseReasonTagsAndTally(t *testing.T) {
	cfg := &store.StrategyConfig{}
	e := NewStrategyEngine(cfg)
	ctx := &Context{
		RecentOrders: []RecentOrder{
			{Symbol: "BRUSDT", Side: "long", RealizedPnL: -1, PnLPct: -5, CloseReason: "sl", EntryTime: "09-17 10:00 UTC", ExitTime: "09-17 10:30 UTC", HoldDuration: "30m"},
			{Symbol: "ZECUSDT", Side: "long", RealizedPnL: 2, PnLPct: 5, CloseReason: "tp", EntryTime: "09-17 11:00 UTC", ExitTime: "09-17 11:20 UTC", HoldDuration: "20m"},
			{Symbol: "HBARUSDT", Side: "long", RealizedPnL: 0.1, PnLPct: 1, CloseReason: "llm", EntryTime: "09-17 11:00 UTC", ExitTime: "09-17 11:30 UTC", HoldDuration: "30m"},
		},
	}
	out := e.BuildUserPrompt(ctx)
	for _, want := range []string{"[SL hit]", "[TP hit]", "[LLM close]", "Recent closes:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
```
Note: verify where recent trades are rendered in the user prompt; if they render via `formatRecentTradesEN` (legacy) rather than inline, target that function instead and assert on its output. Use the actual rendering path you find; do not assert against a path that isn't used.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./kernel/ -run TestRecentTradesPromptShowsCloseReasonTagsAndTally -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `RecentOrder`: add `CloseReason string `json:"close_reason"``.
- `auto_trader_loop.go`: change `GetRecentTrades(at.id, 10)` → `GetRecentTradesWithReason(at.id, 15)`; copy `CloseReason: trade.CloseReason`.
- In the recent-trades renderer add a per-row tag:
```go
	tag := map[string]string{"llm": "[LLM close]", "tp": "[TP hit]", "sl": "[SL hit]", "exchange": "[exchange]"}[order.CloseReason]
```
appended to the row, and a tally line before the list:
```go
	counts := map[string]int{}
	for _, o := range orders { counts[o.CloseReason]++ }
	sb.WriteString(fmt.Sprintf("Recent closes: %d TP, %d SL, %d LLM decisions\n\n", counts["tp"], counts["sl"], counts["llm"]))
```
Apply to whichever renderer the active prompt path uses; also update `formatRecentTradesEN` for the legacy path.

- [ ] **Step 4: Run**

Run: `go test ./kernel/... ./trader/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add kernel/engine.go kernel/engine_prompt.go kernel/formatter.go trader/auto_trader_loop.go kernel/engine_prompt_test.go
git commit -m "feat(kernel,trader): tag and tally recent close reasons, expand to 15"
```

---

### Task 8: Decision Process guidance on SL/TP as reference levels

**Files:**
- Modify: `kernel/engine_prompt.go:140-150`
- Modify: `store/strategy.go:1275-1280` and `:1295-1300`
- Test: `kernel/engine_prompt_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSystemPromptIncludesSLTPReferenceGuidance(t *testing.T) {
	cfg := &store.StrategyConfig{}
	e := NewStrategyEngine(cfg)
	out := e.BuildSystemPrompt(&Context{}, 1000)
	if !strings.Contains(out, "reference levels") {
		t.Fatalf("missing SL/TP reference guidance:\n%s", out)
	}
}
```
Adjust the constructor/args to the actual `BuildSystemPrompt` signature used elsewhere in the test file.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./kernel/ -run TestSystemPromptIncludesSLTPReferenceGuidance -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Append a 4th step to both branches:
```go
		sb.WriteString("4. Your recorded SL/TP are reference levels, not hard constraints — you may take profit or cut the loss early if the thesis is invalidated.\n\n")
```
Append the same line to the two default `DecisionProcess` templates in `store/strategy.go`.

- [ ] **Step 4: Run**

Run: `go test ./kernel/... ./store/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add kernel/engine_prompt.go store/strategy.go kernel/engine_prompt_test.go
git commit -m "feat(kernel,store): note SL/TP are reference levels in decision process"
```

---

### Task 9: Full verification

- [ ] **Step 1:** `go build ./... && go vet ./... && go test ./...`
- [ ] **Step 2:** `cd web && npx tsc --noEmit && npm test`
- [ ] **Step 3:** Confirm grid tests and existing position/throttle tests pass.

---

## Self-Review

- **Spec coverage:** SL/TP schema + persistence ✓ (T1,T3); sync preserves ✓ (T2); PositionInfo + populate ✓ (T4); prompt line with SL/TP/distances/Opened-holding ✓ (T5); close-reason classification incl. `success==true` requirement ✓ (T6); recent trades 10→15 + tags + tally ✓ (T7); Decision Process guidance incl. default templates ✓ (T8); full verification ✓ (T9).
- **Placeholder scan:** none — each code step shows concrete code; Task 5's format-string integration is described with the exact string layout and trimmed-duplication rule.
- **Type consistency:** `IndicatorPeriods` not involved; `RecentTrade.CloseReason` / `RecentOrder.CloseReason` consistent; `GetRecentTradesWithReason` used in T7 as defined in T6; reason strings `llm`/`tp`/`sl`/`exchange` consistent across T6/T7.
- **Known integration caveat:** T3's best-effort SL/TP persistence can miss if the position row does not yet exist (async OrderSync); documented as acceptable.
