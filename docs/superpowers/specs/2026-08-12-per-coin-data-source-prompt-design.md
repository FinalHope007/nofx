# Per-coin Data Source Selection for LLM Prompt

**Date:** 2026-08-12

## Problem

The AI500 strategy LLM log shows the user prompt only contains OHLCV klines plus a
source tag like `(AI500)`. The system prompt's data dictionary ("OIChange", "OI",
"Volume") and its decision process ("AI500 / OI_Top filter tags (if available)")
reference per-coin enrichment data that is never delivered.

Two families of data need to reach the LLM, both behind user-selectable toggles in
strategy creation:

1. **Free per-coin data sources** (AI500 score, OI change, net flow, price change)
   sourced from public `vergex.trade` `trending-crypto` / `trending-category`
   endpoints. The backend already fetches these to build candidate pools but throws
   away everything except the symbol.
2. **Basic indicators** already fully wired in the backend prompt builder
   (EMA20, MACD, RSI7, OI, funding rate) but disabled by the current strategy
   wizard (`strategyFactory.buildStrategyConfig` hardcodes them all to `false`).

These are independent from the existing paid `enable_*_ranking` / quant configs and
must NOT be tangled with them.

## Goals

- Let the user pick which per-coin data sources appear in the LLM prompt for a
  strategy: AI500, OI, Netflow, Price (free) and EMA, MACD, RSI, OI, Funding rate
  (already-wired indicators).
- Selecting a scope card auto-enables its matching data source by default
  (AI500 scope → AI500 data; OI scope → OI data; netflow scope → netflow data;
  price scope → price data). **Exception:** Bias Radar scope (vergex) does NOT
  auto-enable any free source, because it uses the vergex prompt which already
  carries bias data.
- For a shared duration multiselect `[15m, 30m, 1h, 4h, 8h, 12h, 24h]` that applies
  to OI, Netflow, and Price (AI500 has no duration).
- Per-coin embedding: each candidate coin's section — and each **open position's**
  section — shows the enabled sources that data exists for; sources/durations with
  no matching row for a coin are silently omitted (no error/warning text). The
  same data-source enrichment applies identically to positions as to candidates
  (e.g. a held BTC position gets its AI500/OI/netflow/price lines when enabled and
  available).

## Non-Goals

- Do NOT modify the existing paid `enable_oi_ranking` / `enable_netflow_ranking` /
  `enable_price_ranking` configs (Q3 answer A).
- Do NOT change kline behavior (kline is always on, configured under Candles).
- Do NOT add new scope cards.
- Do NOT change the vergex claw402 prompt/path for `vergex_signal` strategies.

## Data Sources (fields + endpoint)

All free, no auth required.

### AI500 — `GET https://vergex.trade/trending-category?lang=en&key=ai500`
No duration. Parsed by `nofxos.FreeTrendingClient.GetAI500()` → `CoinData`:
`Pair`, `Score`, `StartTime`, `StartPrice`, `ChangePctValue` (populated as
`IncreasePercent`). Current price is added from the coin's `market.Data.CurrentPrice`
at render time.

### OI — `GET https://vergex.trade/trending-crypto?tab=oi&duration=<d>&limit=50`
Parsed by `FreeTrendingClient.getOIArray` into `nofxos.OIPosition`:
`Rank`, `Symbol`, `Price`, `CurrentOI`, `OIDelta`, `OIDeltaPercent`,
`OIDeltaValue`, `PriceDeltaPercent`, `NetLong`, `NetShort`.
Top array key `top`, low array key `low`.

### Netflow — `GET https://vergex.trade/trending-crypto?tab=net_flow&duration=<d>&limit=50`
Parsed by `FreeTrendingClient.getNetFlowArray` into `nofxos.NetFlowPosition`:
`Rank`, `Symbol`, `Amount` (signed, +inflow / −outflow), `Price`.
Top = inflow, low = outflow.

### Price change — `GET https://vergex.trade/trending-crypto?tab=price&duration=<d>&limit=50`
Parsed by `FreeTrendingClient.getPriceArray` into `nofxos.PriceRankingItem`:
`Symbol`, `PriceDelta` (decimal, already ×100 for display), `Price`,
`FutureFlow`, `SpotFlow`, `OI`, `OIDelta`, `OIDeltaValue`.
Top = gainers, low = losers.

> Field-mapping verification required during implementation: the parsed structs'
> JSON tags assume the paid nfxos shape (`current_oi`, `oi_delta`, `amount`, etc.).
> The free `trending-crypto` JSON may use different keys. This must be verified
> against live responses and the parse adjusted so all fields above are populated.

### Basic indicators (already wired)
From `market.Data` on the free market path: `CurrentEMA20`, `CurrentMACD`,
`CurrentRSI7`, `OpenInterest` (`OIData{Latest, Average}`), `FundingRate`.
Renderer already exists in `kernel/engine_prompt.go#formatMarketData`, gated by
`Indicators.EnableEMA / EnableMACD / EnableRSI / EnableOI / EnableFundingRate`.

## Frontend

### Config types (`web/src/types/strategy.ts`)
Add to `IndicatorConfig`:
```ts
// Free per-coin data sources (independent toggles)
enable_ai500_data?: boolean   // AI500 score
enable_oi_data?: boolean      // OI change
enable_netflow_data?: boolean // net flow
enable_price_data?: boolean   // price change
// Shared duration multiselect for the above three (15m..24h)
data_durations?: string[]
```
Reuse the existing `enable_ema / enable_macd / enable_rsi / enable_oi /
enable_funding_rate` indicator toggles (already defined in the type).

### EditorStepPage (`web/src/features/strategies/EditorStepPage.tsx`)
- Add a **"Data sources for LLM"** section with toggle chips per source:
  AI500, OI, Netflow, Price (+ the basic indicators: EMA20, MACD, RSI7, OI,
  Funding rate). Kline is not a toggle here (configured under Candles).
- Add a **duration multiselect** (15m, 30m, 1h, 4h, 8h, 12h, 24h) shown/enabled
  when any of OI/Netflow/Price is on.
- Persist and restore these in edit mode.
- Auto-default: on entering the editor with a scope selection, if a scope unit maps
  to a source (ai500→AI500, nofxos_oi→OI, nofxos_netflow→Netflow,
  nofxos_price→Price) toggle that source on. Bias Radar (`vergex`) does not.
  Multi-scope default = OR of the mapped sources of all units.

### strategyFactory (`web/src/features/strategies/strategyFactory.ts`)
`buildStrategyConfig` should map the new form fields into `indicators` instead of
hardcoding everything to `false`. Preserve the current behavior for any toggle the
user hasn't exposed.

### scopeCatalog (`web/src/features/strategies/scopeCatalog.ts`)
No new cards. The auto-default mapping lives in the editor (not the catalog).

## Backend

### Config schema (`store/strategy.go`)
- `IndicatorConfig`: add
  - `EnableAI500Data bool json:"enable_ai500_data"`
  - `EnableOIData bool json:"enable_oi_data"`
  - `EnableNetflowData bool json:"enable_netflow_data"`
  - `EnablePriceData bool json:"enable_price_data"`
  - `DataDurations []string json:"data_durations,omitempty"`
- The existing `enable_ema/macd/rsi/oi/funding_rate` are already present and now
  reachable from the wizard.
- Add these to `EstimateTokens` (per-coin char estimate when each toggle is on) and
  apply sensible defaults / clamping of `DataDurations` in `ClampLimits`.
- The `AIStrategyConfig` JSON marshal path already serializes `Indicators`, so the
  new fields persist automatically.

### Engine: capture per-coin source data (`kernel/engine.go`)
- Extend `StrategyEngine` to retain the per-coin enrichment it currently discards:
  - `getAI500Coins`: keep a `map[string]nofxos.CoinData` (AI500 pool) rather than
    only symbols.
  - `getOITopCoins` / `getOILowCoins`: keep `map[string]map[list]nofxos.OIPosition`
    per duration.
  - `getNetFlowTop/LowCoins`, `getPriceTop/LowCoins`: same pattern.
- Provide engine methods that, given the candidate symbol set + enabled sources +
  selected durations, return per-symbol enrichment data to render.

### Enrichment fetch (per cycle)
In `buildTradingContext` / `engine_analysis`, when any free source toggle is on,
fetch each enabled source for the shared durations via the `FreeTrendingClient`
(limit 50), filter rows to candidate coins, and attach to the `Context` for the
prompt builder. Reuse existing `Context` fields or add the minimal per-coin maps.

### Prompt renderer (`kernel/engine_prompt.go`)
In `BuildUserPrompt`, both the candidate coin loop and the **open position loop**
emit per-coin sections for each enabled source with data (positions use
`formatPositionInfo`, which must receive the same source enrichment). Exact format
in "Prompt format" below. Missing rows are silently skipped.

> Basic-indicator rendering needs no change — `formatMarketData` already handles it;
> the only fix is that the wizard can now turn those toggles on.

## Prompt format (final, approved)

Rendered inside each `### N. SYMBOL (...)` candidate section, after market data +
quant + vergex blocks:

```
=== CYSUSDT AI500 Signal ===
AI score 78.3/100 | peak score 87 | current price 1.3821 | start price 0.5117 | change +180.9% since starting alert

=== CYSUSDT Open Interest ===
[15m · Increase] rank #4 | OI change +1.2% (+$0.3M) | price +0.8% | OI $38.5M | long net +$12.0M / short net $9.0M
[1h · Decrease] rank #5 | OI change -2.1% (-$0.5M) | price -1.4% | OI $38.5M | long net +$10.0M / short net $11.0M

=== CYSUSDT Net Flow ===
[15m · inflow] rank #9 | net flow +$0.8M | price $1.3821

=== CYSUSDT Price Change ===
[15m] price change +1.2% | spot +$0.2M / future +$0.1M | OI delta +1.2%
[1h] price change +3.1% | spot +$0.9M / future +$0.8M | OI delta +5.3%
```

- **AI500**: no rank, no duration. `current price` from market data.
- **OI**: has rank; `Increase` / `Decrease` (top/low). One line per duration the
  coin appears in.
- **Netflow**: has rank; `inflow` / `outflow` (top/low). One line per duration.
- **Price change**: no rank, no top/low word. `price change <x>` prefix. One line
  per duration.
- **Vergex signal/heatmap** (if enabled for a vergex-source strategy): reuse the
  existing `FormatAnalysisForAI` / claw402 format under a
  `=== SYMBOL Vergex Signals ===` block. Coins with no data are silently omitted.
- Any source or duration whose row is absent for a coin renders nothing (no
  placeholder, no warning).

## Error Handling

- Free endpoint fetch failures for a source are logged server-side and treated as
  "no data for that source" — never surfaced into the prompt.
- Missing per-coin rows within a successful fetch are skipped silently.
- `data not available` HTTP errors from vergex detail endpoints are swallowed as
  existing `SignalLabError`/`HeatmapError` (unchanged behavior).

## Testing

- Backend: table-driven tests for the prompt renderer asserting each source line
  when data present, and absence when the coin lacks a row for a duration/list.
- Backend: `GetAI500`-style parse tests for the free endpoint field mapping.
- Store: schema round-trip test for the new `IndicatorConfig` fields + defaults.
- Frontend: Vitest for `strategyFactory` mapping and EditorStep auto-default logic.
- Verification commands: `go build ./...`, `go vet ./...`, `gofmt -l .`;
  `cd web && npx tsc --noEmit && npm run build && npm test`.

## Out of Scope / Follow-ups

- Wiring per-coin source data into `decision_context` prompt-builder is a separate
  pending task (HANDOFF task 2).
