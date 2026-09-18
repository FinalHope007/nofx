# Open-Position TP/SL, Entry Timestamp, and Close-Reason Classification Design

## Goal

Give the LLM enough context to manage open positions and learn from recent
closes:

1. Show each open position's entry timestamp, recorded stop-loss and
   take-profit (plus derived distances) in the prompt.
2. Persist the AI's SL/TP onto the open-position record at execution time.
3. Expand the recent-completed-trades list from 10 to 15 trades.
4. Classify each recent close as **TP hit**, **SL hit**, or **LLM decision**,
   and render a compact tag plus a tally so the model can judge its own
   strategy effectiveness.

## Architecture

### 1. Persist SL/TP on the open position

`kernel/engine_position.go:81` already rejects any open when
`StopLoss <= 0 || TakeProfit <= 0`, so every opened position has validated
SL/TP. Persist them so they survive reload and can be shown back to the LLM.

- `store/position.go`: add `StopLoss` / `TakeProfit` columns to
  `TraderPosition` (`default:0`); `AutoMigrate` adds them.
- `trader/auto_trader_orders.go`: in `executeOpenLongWithRecord` /
  `executeOpenShortWithRecord`, after `SetStopLoss` / `SetTakeProfit` succeed,
  write `decision.StopLoss` / `decision.TakeProfit` onto the open position for
  `(symbol, side)`. A set-order failure logs and leaves SL/TP at 0 (renders as
  `none`); the write failure must not abort the trade.
- `store/position_builder.go`: `handleOpen` (exchange reconciliation) creates
  rows without SL/TP knowledge — it must never overwrite an existing position's
  SL/TP with zero when averaging; new sync-created rows legitimately start at 0.

### 2. Carry into the prompt

- `kernel/engine.go` `PositionInfo`: add `EntryTime int64`, `StopLoss float64`,
  `TakeProfit float64`.
- `trader/auto_trader_loop.go`: the position loop already fetches the DB row for
  `EntryTime`; also copy `StopLoss` / `TakeProfit` from that row.
- `kernel/engine_prompt.go` `formatPositionInfo`: append, in order:

  ```
  ... | Liq Price <p> | SL <sl> TP <tp> | SL Dist <d>% TP Dist <d>% | Opened <MM-DD HH:MM UTC> (Holding <dur>)
  ```

  - SL/TP via `market.FormatPriceSigFigs`.
  - Distances from entry: long → `(SL-entry)/entry*100` (negative),
    `(TP-entry)/entry*100`; short sign-flipped.
  - If SL or TP is 0, render `SL none` / `TP none` and omit that distance.
  - `Opened` and `Holding <dur>` sit side-by-side; the existing duration logic
    is reused.

### 3. Close-reason classification

Primary signal is the **existence of an LLM close decision**; the residual is
resolved by PnL direction corroborated by the recorded SL/TP.

All closes (LLM and exchange) currently flow through
`PositionBuilder.ProcessTrade` with `close_reason:"sync"`
(`store/position_builder.go:97`), so classification is derived at **read time**
in `store/position_query.go:GetRecentTrades`:

1. **`llm`** — a `close_long` / `close_short` decision for the position's
   `(symbol, side)` exists in the decision log within a window around the exit
   time (e.g. `[exit - cycleWindow, exit + cycleWindow]`).
2. **`sl`** — no LLM decision, and the exit is on the loss side: exit at/beyond
   the recorded SL (long: `exit <= SL*(1+tol)`; short: `exit >= SL*(1-tol)`) or,
   when SL is unrecorded, PnL < 0.
3. **`tp`** — no LLM decision, and the exit is on the profit side: exit at/beyond
   the recorded TP (long: `exit >= TP*(1-tol)`; short: `exit <= TP*(1+tol)`) or,
   when TP is unrecorded, PnL > 0.
4. **`exchange`** — no LLM decision and neither SL nor TP matched and PnL is
   zero (flat) or data is insufficient (legacy row).

Tolerance `tol` is a small relative epsilon (e.g. `0.001`) to absorb fill slip.

`RecentTrade` gains a `CloseReason string` field. `GetRecentTrades` performs the
decision-log lookup in a bounded single query per call. A new store method
`DecisionStore.GetRecordsInRange(traderID string, from, to time.Time)
([]*DecisionRecord, error)` is added (the existing methods are by-count or
by-day only); `GetRecentTrades` calls it once over the span from the oldest
returned trade's exit time to the newest, then matches close decisions in
memory by `(symbol, side)` and timestamp window — no per-trade query.

### 4. Render recent trades

- `trader/auto_trader_loop.go:652`: change `GetRecentTrades(at.id, 10)` → `15`.
- `kernel/engine.go` `RecentOrder`: add `CloseReason string`.
- `trader/auto_trader_loop.go`: copy `trade.CloseReason` into `ctx.RecentOrders`.
- `kernel/engine_prompt.go` recent-trades section: append a tag per row
  (`[TP hit]`, `[SL hit]`, `[LLM close]`, `[exchange]`) and add one tally line
  above the list, e.g.:

  ```
  Recent closes: 2 TP, 4 SL, 4 LLM decisions — frequent TP/SL hits indicate strong directional momentum.
  ```

  The model derives the market read from the tallies; no bullish/bearish verdict
  is hardcoded.

### 5. System-prompt guidance

Extend the `# 📋 Decision Process` section (`kernel/engine_prompt.go:141-150`)
with:

```
4. Your recorded SL/TP are reference levels, not hard constraints — you may take profit or cut the loss early if the thesis is invalidated.
```

Mirror the same line into the default `DecisionProcess` templates in
`store/strategy.go:1275` and `:1295` so custom-prompt strategies receive it.

## Data Flow

```
AI open decision (SL/TP validated >0)
  -> SetStopLoss/SetTakeProfit on exchange
  -> persist SL/TP onto TraderPosition
  -> (later) close via LLM decision or exchange TP/SL
  -> PositionBuilder marks CLOSED, exit_price/exit_time/realized_pnl set
  -> GetRecentTrades derives CloseReason (decision-log match -> llm; else SL/TP/PnL)
  -> prompt renders position line + classified recent-trade tags + tally
```

## Error Handling

- SL/TP set failure: trade proceeds; position SL/TP remains 0 → prompt shows
  `none`.
- SL/TP persistence failure: logged, non-fatal.
- Decision-log lookup failure: classification falls back to price/PnL, then
  `exchange`; must not fail `GetRecentTrades`.
- Legacy positions without recorded SL/TP: `none` on the position line; recent
  closes classify by LLM-decision presence then PnL sign.

## Testing

- `store`: SL/TP persist and round-trip; `handleOpen` averaging preserves
  existing SL/TP; `GetRecentTrades` returns correct `CloseReason` for
  (a) matching LLM close decision, (b) SL hit without decision, (c) TP hit
  without decision, (d) legacy/unknown.
- `kernel`: prompt test asserting the open-position line renders `Opened ...
  (Holding ...)`, `SL`, `TP`, distances, and the `none` fallback; recent-trade
  tags and tally render; Decision Process contains the guidance line.
- `trader`: existing position/throttle tests stay green.
- Full: `go build ./... && go vet ./... && go test ./...`.

## Explicitly Out of Scope

- Reading SL/TP live from exchange trigger orders.
- Backfilling SL/TP for already-open positions.
- Changing order/execution behavior.
- Persisting the derived close reason (it is computed at read time).
