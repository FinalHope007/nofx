# Task 3 Follow-up Fixes — Free `vergex.trade` Per-Coin Detail, Kline Source, and Dashboard Data Design

> Design spec for the follow-up fixes discovered while running the trader against the Task 3 "paid source providers → free vergex.trade" implementation. This is a **plan-first** pass; nothing is implemented until this spec is reviewed and approved.

## Goal

Resolve the runtime issues found when running a trader on the free-mode Task 3 implementation. The trader can now use strategies from paid scope; these fixes make the per-coin detail actually work end-to-end, feed per-coin detail to all strategy source types, source klines from the matching exchange, and make the dashboard funnel reflect the active strategy instead of a hardcoded default.

## Non-goals (explicitly out of scope unless the user says otherwise)

- Risk-radar config surface for `MaxPositions` / `MaxMarginUsage` / `MinPositionSize` (currently no UI; only the two leverage ratios are exposed). Leave as-is.
- `/data` page iframe (`vergex.trade refused to connect`) — a pre-existing frontend iframe that vergex.trade refuses to frame; not fixable by backend endpoint replacement.
- Binance clock-skew `-1021` on positions/balance — pre-existing machine-clock-skew issue, unrelated to this run.
- The multi-card `custom` AND/OR resolver (HANDOFF Task 1) — separate feature.

## Context / current state (verified)

- `vergex.Client` free-mode (`provider/vergex/client.go`) routes `GetSignalLab` → `FreeSignalsPath` and `GetCostLiquidationHeatmap` → `FreeRiskbinsPath`, building the path symbol via `MarketSymbol(marketType, symbol)`, which returns `xyz:SP500` for a stock — the free endpoint instead needs the market-qualified ref (`core_perp:BTC` for crypto, `hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:SP500` for stocks). `riskbins` also requires a valid Bearer token.
- The `0x88806a71d74ad0a510b350545c9ae490912f0888` is a **fixed constant** (the Hyperliquid xyz deployer address), crypto `core_perp` needs **no** address qualification.
- Per-coin detail is gated on `SourceType == "vergex_signal"`:
  - `kernel/engine_analysis.go` `enrichVergexDataWithStrategy` (line ~150) returns early unless `SourceType == "vergex_signal"`.
  - `kernel/engine.go` `FetchVergexDataBatch` (line ~1224) returns early unless `SourceType == "vergex_signal"`.
  - So AI500 / oi / netflow / price strategies get NO per-coin Signal Lab / Heatmap in the user prompt (confirmed in the AI500 LLM log: klines only).
- `BuildSystemPrompt` (`kernel/engine_prompt.go:37`) dispatches on `usesVergexSignalPrompt()`: `vergex_signal` strategies get the Claw402 rules prompt; all other source types get the generic prompt.
- Kline fetch (`market/data.go GetWithTimeframes`, `market/data_klines.go`) sources non-xyz crypto from **CoinAnk** (defaulting to Binance within CoinAnk) and xyz assets from Hyperliquid. There is no per-trader-exchange selection — it always uses CoinAnk for non-xyz.
- Dashboard funnel (`web/src/components/terminal/OrchestrationTopology.tsx` + `TerminalDashboard.tsx`) renders `flow → signal → decision → execute → hold`; the flow/signal layers are currently fed from the default strategy's vergex data rather than the active strategy.
- Backend log shows residual `payment/x402.go … [claw402-data] Payment expired (402)` — an unpaid Claw402 x402 call still firing somewhere. Needs investigation to confirm whether any live path is still on the paid route.

## Design

### Fix A — Market-qualified symbol for free per-coin endpoints

Add a helper in `provider/vergex/client.go`:

```go
// FreeDetailEnv is the fixed Hyperliquid xyz (TradeFi) deployer address used by
// vergex.trade free detail endpoints for stock/index/commodity (hip3_perp) markets.
const freeDetailXYZDeployer = "0x88806a71d74ad0a510b350545c9ae490912f0888"

// FreeDetailSymbol returns the URL path symbol segment that vergex.trade free
// per-coin detail endpoints require, market-qualified:
//   - crypto (core_perp):   "core_perp:BTC"
//   - stock (hip3_perp):    "hip3_perp:0x8880...:xyz:SP500"
func FreeDetailSymbol(marketType, symbol string) string
```

Behavior:
- Normalize `marketType` via `normalizeMarketType`.
  - If it is a core/crypto market (`coreperp`, `core`, `crypto`, `cryptoperp`) → `marketType + ":" + QuerySymbol(symbol)`.
  - Otherwise (hip3_perp, stock, index, commodity, forex, tradeFi, etc.) → `marketType + ":" + freeDetailXYZDeployer + ":xyz:" + QuerySymbol(symbol)`.
- Use `QuerySymbol` so `xyz:SP500` → `SP500` base, then re-qualify.

In free-mode `GetSignalLab` and `GetCostLiquidationHeatmap`, replace `MarketSymbol(q.MarketType, q.Symbol)` with `FreeDetailSymbol(q.MarketType, q.Symbol)`, and **URL-encode** the `:` → `%3A` in the path segment so the request matches the verified working curl (`.../markets/hip3_perp/hip3_perp%3A0x8880...%3Axyz%3ASP500/riskbins`).

Implementation note: the free path constant already carries `%s/%s`. Build the symbol with the format verb, then encode any `:` in the segment to `%3A` before splicing, or build the full URL and use `url.PathEscape` considerations — verify against the live endpoint which encoding the server accepts (the user's curl used `%3A`).

Verify against live `vergex.trade` with the provided Bearer token:
- crypto `core_perp/core_perp%3ABTC` (signals + riskbins) → 200
- crypto-limited `core_perp/CYS` (signals/riskbins) → "not available" (must be treated as no-data, not an error)
- stock `hip3_perp/hip3_perp%3A0x8880...%3Axyz%3ASP500` (signals + riskbins) → 200

### Fix B — Token wiring for `riskbins`

- `VERGEX_API_TOKEN` is already read via `os.Getenv` in `freeVergexClientForRequest` (`api/handler_vergex.go`) and threaded into `NewFreeClient`.
- For local test: set `VERGEX_API_TOKEN` in `.env` to the provided token so `riskbins` returns 200.
- Keep graceful degradation: a missing/invalid token or an endpoint with no data yields an **omitted** heatmap/signals section (never a leaked 401/500 in the user prompt).
- Double-check the 401 path: if a token is present but rejected, treat as "no data" (omit) rather than surfacing the raw error into the prompt (see Fix C).

### Fix C — Per-coin detail for all source types, omit-on-missing

- Remove the `SourceType == "vergex_signal"` early-return gate in both:
  - `kernel/engine_analysis.go` `enrichVergexDataWithStrategy`
  - `kernel/engine.go` `FetchVergexDataBatch`
  so that AI500 / oi / netflow / price strategies also populate per-coin Signal Lab / Heatmap.
- In `engine_prompt.go`, when rendering a symbol's per-coin detail into the **user** prompt:
  - If Signal Lab / Heatmap is present → render it using the **same `Vergex Claw402 Signals` format** (reuse `vergex.FormatAnalysisForAI`).
  - If absent / endpoint returned "not available" / error → **omit** that section entirely; do NOT print an "unavailable (…)" line into the generic user prompt.
- Do NOT change the generic system prompt for non-`vergex_signal` strategies. The Claw402 rules prompt remains only for `vergex_signal`.

Concretely: `enrichVergexDataWithStrategy` currently runs only when the source is `vergex_signal`; make it run for all source types (build symbols from candidates + positions as it already does). Then ensure the per-symbol render in the user prompt (in `engine_prompt.go`/`formatter.go`) suppresses the error lines for non-`vergex_signal` strategies (omit when empty), while `vergex_signal` strategies may still show the empty/error line per existing prompt rules.

Edge: signal-lab and heatmap only have data for a limited set of markets (e.g. `core_perp:CYS` returns "not available"). The renderer must treat "not available" / 404 / empty as no-data → omit.

### Fix D — Dashboard funnel reads the active strategy

- Change the dashboard data source for the `flow` / `signal` funnel layers to use the **active/current strategy's** per-coin detail (`VergexDataMap`), not the default strategy.
- When a candidate has no per-coin detail, `flow` / `signal` layers remain empty, but `decision` / `execute` / `hold` layers must still render from the active strategy's normal decision data.
- Locate the source of the default strategy data (Likely `TerminalDashboard.tsx` or a dashboard context/hook) and re-wire it to the selected/active strategy id.

### Fix E — Per-trader-exchange kline sourcing

- Extend the kline fetch so the source follows the trader's exchange:
  - trader exchange **binance** → Binance native klines (add `getKlinesFromBinance` using the existing `fapi.binance.com` API client; keep CoinAnk fallback only if Binance fails).
  - trader exchange **hyperliquid** → Hyperliquid klines (existing path).
  - all other / unset → **CoinAnk** (current default behavior; unchanged). NOTE: the user's trader is on Binance, so binance → Binance klines; Hyperliquid exchange → Hyperliquid; other → CoinAnk.
- Thread the exchange into `market.GetWithTimeframes` (or a variant). The engine needs to know the trader's exchange when building market data (`kernel/engine_analysis.go fetchMarketDataWithStrategy`). Determine the cleanest seam: the strategy's linked trader/config already carries the exchange; pass it through.

### Fix F — Residual paid x402 on Claw402 (investigate)

- The backend log shows `payment/x402.go … [claw402-data] Payment expired (402)`. Identify the live call still invoking the paid `claw402-data` x402 path.
- If it is a leftover paid per-coin/fetch call within this Task-3 surface, route it to the free client.
- If it belongs to a different feature surface (e.g. NoFXOS `claw402-data` data provider for `nofxos.ai`), report it and confirm scope with the user before changing.

## Files to touch (indicative)

- `provider/vergex/client.go` — add `FreeDetailSymbol` + `freeDetailXYZDeployer`; use it in free-mode `GetSignalLab`/`GetCostLiquidationHeatmap`; URL-encode path symbol.
- `provider/vergex/client_test.go` or `free_mode_test.go` — unit tests for `FreeDetailSymbol` (crypto + stock).
- `kernel/engine_analysis.go` — remove `vergex_signal` gate in `enrichVergexDataWithStrategy`.
- `kernel/engine.go` — remove `vergex_signal` gate in `FetchVergexDataBatch`.
- `kernel/engine_prompt.go` / `kernel/formatter.go` — per-symbol render: reuse `Vergex Claw402 Signals` format; omit on missing (no error line) for non-`vergex_signal`.
- `market/data.go` / `market/data_klines.go` — exchange-aware kline fetch (`getKlinesFromBinance`; binance→binance, hyperliquid→hyperliquid, other→coinank).
- `kernel/engine_analysis.go` `fetchMarketDataWithStrategy` — pass the trader exchange through.
- `web/src/components/terminal/TerminalDashboard.tsx` — funnel `flow`/`signal` from active strategy.
- `.env` (local only) — set `VERGEX_API_TOKEN` for verification; do not commit secrets.

## Verification / acceptance

- Backend: `go build ./... && go vet ./... && go test ./provider/vergex/ ./kernel/`.
- Frontend: `cd web && npx tsc --noEmit && npm test`.
- Live (manual, with token in `.env`):
  - `core_perp/core_perp%3ABTC` signals + riskbins → 200.
  - `core_perp/CYS` → "not available" → the user prompt shows klines only (no error).
  - `hip3_perp/hip3_perp%3A0x8880...%3Axyz%3ASP500` signals + riskbins → 200.
  - AI500 strategy user prompt includes signal-lab/heatmap sections for symbols that have data; generic system prompt unchanged.
- Dashboard: funnel `flow`/`signal` reflect the active strategy; `decision`/`execute`/`hold` still render.
- Binance trader: user prompt klines come from Binance.

## Open item (post-review)

- Confirm whether the residual paid `claw402-data` x402 call (Fix F) is in-scope for this run or belongs to another feature surface; fix only if in-scope.
