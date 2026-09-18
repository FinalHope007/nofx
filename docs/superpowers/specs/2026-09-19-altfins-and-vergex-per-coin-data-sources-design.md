# AltFins + Vergex Per-Coin Data Sources

**Date:** 2026-09-19

## Problem

Trading strategies can already enrich the LLM prompt with a small set of per-coin
data sources (AI500 score, OI, net flow, price change, and the Binance Opportunity
technical/sentiment feeds). Two gaps remain:

1. **AltFins is not available at all.** AltFins exposes an unofficial, scraped
   analytics feed (trend grades, trend changes, MACD signal + histogram) per coin
   and per interval. This is exactly the kind of multi-timeframe trend context the
   LLM needs, and it is free / keyless.

2. **The vergex.trade free per-coin detail feeds are locked to `vergex_signal`
   strategies.** `provider/vergex/client.go` already implements the free
   SignalLab (`/signals`) and Cost/Liquidation Heatmap (`/riskbins`) endpoints, but
   they are only fetched by `FetchVergexDataBatch`, which renders only when
   `source_type == "vergex_signal"` (`kernel/engine_prompt.go` `formatVergexData`
   is called with `omit == source != vergex_signal`). A strategy using any other
   coin source cannot surface this per-coin data, even though the data is per-coin
   and coin-source independent.

A successful existing template for per-coin enrichment exists: the Binance
Opportunity technical/sentiment feature (`d2775e93`, `64344754`, `fd8ddfc7`,
`e4b77cd8`, `e16ab72c`). It adds provider → engine cache → `PerCoinSignal` →
`attachPerCoinSignals` → prompt renderer → trader prefetch → store config → web
toggles. The new sources should follow the same shape.

> The Binance Opportunity feed is itself discontinued (`e96018bb`); its code is
> retained as a reference implementation only. This spec does not touch it.

## Goals

- Add **AltFins** as a free per-coin data source: 7 rendered fields across 5
  selectable intervals, available to **any** coin source.
- Surface **vergex SignalLab + Heatmap** as independent per-coin data sources,
  decoupled from `vergex_signal`, available to **any** coin source.
- All new sources follow the existing `PerCoinSignal` enrichment + prompt-render
  + prefetch pattern.
- Data is fetched once per symbol per cycle (TTL-cached), never blocking a cycle
  on failure; missing data is silently omitted from the prompt.

## Non-Goals

- No new **candidate-pool** (step 1) coin sources. This work is strictly the
  **per-coin detail** (step 2) enrichment.
- No change to the `vergex_signal` candidate-pool path or its paid claw402 calls.
- No change to the Binance Opportunity code (leave the deprecated reference impl).
- No change to kline fetching.
- No attempt to solve Cloudflare server-side. vergex toggles ship with graceful
  degradation (see Error Handling).

## AltFins Data Source

### Endpoints (unofficial, scraped; verified live 2026-09-18/19)

**Step 1 — resolve `securityIdentifierId`:**
```
POST https://altfins.com/vaadinRest/v1/nonauth/signal-feed?size=1
Content-Type: application/json
{"coinFilter":"ZEC","signal":"","marketCap":""}
```
Response `.content[0].securityIdentifierId` (e.g. `1021300`). Filter is
case-insensitive. **No match → `content: []` (HTTP 200)**, treated as "no data".

**Step 2 — analytics:**
```
GET https://altfins.com/api/v1/nonauth/marketData/analytics
  ?id=<securityIdentifierId>
  &valueIds=COIN_SYMBOL,SHORT_TERM_TREND,MEDIUM_TERM_TREND,LONG_TERM_TREND,
              SHORT_TERM_TREND_CHANGE,MEDIUM_TERM_TREND_CHANGE,LONG_TERM_TREND_CHANGE,
              MACD_SIGNAL,AGE,MACD_HISTOGRAM_H2
  &timeInterval=<interval>
  &level=LEVEL_1
```
Response:
```json
{"values":[...raw...],"formattedValues":[...display...]}
```

### Verified constraints

- **Valid `timeInterval` values (only these 5):** `MINUTES15`, `HOURLY`, `HOURS4`,
  `HOURS12`, `DAILY`. (`MINUTES5/30`, `HOURS1`, `HOURS24`, `DAILY1` are rejected;
  `HOURS1` is **not** a valid alias — the enum is `HOURLY`.)
- **Valid `level`:** only `LEVEL_1` (`LEVEL_2/3` → 404).
- **Field order is not guaranteed.** The API reorders `values`/`formattedValues`
  to its own canonical order regardless of the requested `valueIds` order
  (verified: requesting `MACD_SIGNAL,AGE` and `AGE,MACD_SIGNAL` returned the same
  order). Therefore parsing **must not** assume the request order. Implementation
  sends a fixed canonical `valueIds` order and maps the returned arrays by index
  against that **known canonical order**, with a strict length guard
  (`len(values) == len(formattedValues) == expectedCount`) → error on mismatch.
- **`AGE` parsing:** the raw value is an **epoch-ms timestamp of the last MACD
  crossover**; the `formattedValues` age is stale/incorrect and is ignored. Age is
  computed locally: `bars = floor((now − raw_ms) / intervalDuration)`, plus a
  human-readable duration string.
  - Verified example: ZEC 15m raw `2026-09-18T16:30:00Z`, at `18:13Z` → 103.6 min
    → **6 bars** ≈ 90 min, matching the AltFins website's "age 6".
- **`MACD_HISTOGRAM_H2` can be `null`** (raw `null`, formatted `"-"`) → omit line.

### Translation (provider-owned, shared by prompt + tests)

Raw enums are SCREAMING_SNAKE_CASE. Map to full words; **no abbreviations**:

| Raw | Rendered |
|---|---|
| `STRONG_UP` / `Strong Up (N/10)` | `Strongly Bullish (N/10)` |
| `UP` / `Up (N/10)` | `Bullish (N/10)` |
| `NEUTRAL` / `Neutral (N/10)` | `Neutral (N/10)` |
| `DOWN` / `Down (N/10)` | `Bearish (N/10)` |
| `STRONG_DOWN` / `Strong Down (N/10)` | `Strongly Bearish (N/10)` |
| `X_TO_Y` (trend change) | `<map(X)> to <map(Y)>` |
| `MACD_SIGNAL` `Buy` / `Sell` | `Bullish` / `Bearish` |
| `MACD_HISTOGRAM_H2` `UP` / `DOWN` | `Bullish` / `Bearish` |
| `MACD_HISTOGRAM_H2` `null` / `-` | (omit) |

The `(N/10)` score suffix in the formatted trend values is preserved verbatim.

## Vergex Per-Coin Data Source

### Existing code to reuse

- `provider/vergex/client.go`: `NewFreeClient(baseURL, authToken, logger)`,
  `GetSignalLab(ctx, Query)` (`FreeSignalsPath`), `GetCostLiquidationHeatmap(ctx,
  Query)` (`FreeRiskbinsPath`), `FormatSignalLabMarkdown`, `FormatHeatmapMarkdown`,
  `MarketAnalysis{SignalLab, Heatmap json.RawMessage, *Error}`.
- The free client already resolves market-qualified symbols via
  `FreeDetailSymbol` (`core_perp:<SYM>` crypto, `hip3_perp:<deployer>:xyz:<SYM>`
  for xyz assets) and uses `security.SafeHTTPClient`.
- `StrategyEngine.freeClient` is already constructed once in `NewStrategyEngine`
  from `VERGEX_API_TOKEN`; no new credentials.

### Change

- Split the two feeds into **two independent toggles**:
  `EnableVergexSignalLabData` and `EnableVergexHeatmapData`.
- `attachPerCoinSignals` fetches each enabled feed per symbol (concurrently, small
  worker bound) into the `PerCoinSignal`, **independent of `source_type`**.
- The existing `vergex_signal` path (`FetchVergexDataBatch`) stays as-is. To avoid
  double-fetching the same coin, when `source_type == "vergex_signal"` the new
  toggles are considered already-satisfied by that path (no duplicate fetch); when
  any other source, the new toggles drive the fetch.
- Renderer: `formatPerCoinSignals` gains vergex blocks behind the new toggles,
  using `FormatSignalLabMarkdown` / `FormatHeatmapMarkdown`. Coins with no data
  render nothing.

### Access caveat (verified)

From a server environment with no prior browser session, both vergex.trade
per-coin endpoints return **HTTP 403 Cloudflare JS challenge** (with or without
the Bearer token, with browser headers). The dashboard works because the browser
solves the challenge and holds `cf_clearance`. The deployed server may or may not
be challenged depending on its network path. This is handled by **graceful
degradation**: a 403 is logged and the block is omitted, exactly like "no data".
No challenge-solving code is added in this spec.

## Data Model

Extend `kernel.PerCoinSignal` (`kernel/engine.go`):

```go
type PerCoinSignal struct {
    // ... existing fields ...
    AltFins        map[string]*altfins.Analytics // interval -> parsed analytics (may be nil)
    VergexSignalLab json.RawMessage              // nullable
    VergexHeatmap   json.RawMessage              // nullable
}
```

`altfins.Analytics` (provider type) holds the translated, per-interval values:

```go
type Analytics struct {
    Interval             string // "MINUTES15" etc. (canonical)
    ShortTermTrend       string // "Strongly Bullish (9/10)"
    MediumTermTrend      string
    LongTermTrend        string
    ShortTermTrendChange string // "Neutral to Bullish"
    MediumTermTrendChange string
    LongTermTrendChange  string
    MACDSignal           string // "Bullish" / "Bearish"
    MACDSignalBarsAgo    int    // computed from raw AGE timestamp
    MACDSignalAgeText    string // "~90 min ago" / "~8 hours ago" / "~2 days ago"
    MACDHistogram        string // "Bullish" / "Bearish" / "" (omitted)
}
```

## Store Config

`store/strategy.go` — `IndicatorConfig` additions:

```go
// AltFins per-coin analytics (free, keyless).
EnableAltFinsData   bool     `json:"enable_altfins_data"`
AltFinsIntervals    []string `json:"altfins_intervals,omitempty"` // MINUTES15, HOURLY, HOURS4, HOURS12, DAILY

// Vergex free per-coin detail feeds (independent of vergex_signal).
EnableVergexSignalLabData bool `json:"enable_vergex_signal_lab_data"`
EnableVergexHeatmapData   bool `json:"enable_vergex_heatmap_data"`
```

- `ClampLimits`: de-dupe + filter `AltFinsIntervals` to the 5 valid enum values;
  default `["MINUTES15","DAILY"]` when the toggle is on but the list is empty.
- `EstimateTokens`: add per-coin char overhead when each toggle is on
  (AltFins ≈ `len(intervals) * 300` chars; vergex SignalLab ≈ 800 chars; vergex
  Heatmap ≈ 600 chars) so the context-limit guard stays accurate.
- Persistence is automatic (the `Indicators` struct is serialized).

## Engine

### Provider — `provider/altfins/analytics.go` (new)

```go
type AnalyticsClient struct { http *http.Client; baseURL string }
func NewAnalyticsClient() *AnalyticsClient
func (c *AnalyticsClient) ResolveIdentifier(ctx, symbol string) (int64, bool, error) // ok=false → no match
func (c *AnalyticsClient) GetAnalytics(ctx, id int64, interval string) (*Analytics, error)
```
- Uses `security.SafeHTTPClient(30 * time.Second)`.
- `symbol` is normalized to the **base** symbol (strip `USDT`).
- Interval constants + validity check; unknown interval → error.
- HTTP 429 → single retry with backoff (mirrors `OpportunityClient`).

### Cache — `kernel/altfins_detail_cache.go` (new)

Mirror `binance_detail_cache.go`: concurrency-safe TTL cache keyed
`"<symbol>|<interval>"`, TTL **10 minutes** (aligns with `binanceDetailCacheTTL`).

### Fetch — `kernel/engine_analysis.go`

- Extend the early-return guard in `attachPerCoinSignals` to include the three new
  toggles.
- **AltFins:** for each enabled interval, for each symbol in `symSet`: cache hit →
  use; miss → `ResolveIdentifier` then `GetAnalytics`; cache on success. Skip and
  log on failure / no-match. Populate `sig.AltFins[interval]`.
- **Vergex:** when `source_type != "vergex_signal"` and either toggle is on, fetch
  the enabled feeds per symbol (bounded concurrency) and set
  `sig.VergexSignalLab` / `sig.VergexHeatmap`. When `source_type == "vergex_signal"`,
  skip (already handled by `FetchVergexDataBatch`).
- Add a `PrefetchAltFinsDetails(ctx, symbols)` method on `StrategyEngine` for
  background warming; vergex uses the existing free-client calls.

### Trader prefetch — `trader/auto_trader_loop.go`

`scheduleBinancePrefetch` is generalized to a per-cycle prefetch scheduler that
warms AltFins (and vergex when not `vergex_signal`) in addition to Binance.
AltFins has 2 requests/coin/interval (resolve + analytics); resolve results should
also be cached so repeat cycles only issue the analytics call. Rate limiting and
the lead-time calculation include the AltFins request count.

## Prompt Rendering

`kernel/engine_prompt.go` — extend `formatPerCoinSignals` after the Binance
blocks. Intervals render in ascending order
`MINUTES15 → HOURLY → HOURS4 → HOURS12 → DAILY`.

### AltFins block (final, approved format)

Only the 7 fields; **no price/performance**. Trend change merged inline as
"changed from X to Y". Full words only (no ST/MT/LT).

```
=== ZEC AltFins ===
[15m] Short Term Trend: Bearish (2/10), changed from Strongly Bearish to Bearish
      Medium Term Trend: Bearish (3/10), changed from Neutral to Bearish
      Long Term Trend: Neutral (5/10), changed from Bullish to Neutral
      MACD Signal: Bearish crossover, 6 bars ago (~90 min ago)
      MACD Histogram: Bullish

[4h]  Short Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      Medium Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      Long Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      MACD Signal: Bullish crossover, 2 bars ago (~8 hours ago)
      MACD Histogram: Bearish

[1d]  Short Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      Medium Term Trend: Strongly Bullish (10/10), changed from Neutral to Strongly Bullish
      Long Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      MACD Signal: Bullish crossover, 2 bars ago (~2 days ago)
      MACD Histogram: Bullish
```

- Interval labels: `MINUTES15→[15m]`, `HOURLY→[1h]`, `HOURS4→[4h]`,
  `HOURS12→[12h]`, `DAILY→[1d]`.
- `MACD Histogram` line omitted when empty/null.
- An interval with no data renders nothing; a symbol with no data renders nothing.

### Vergex blocks

The new `PerCoinSignal.VergexSignalLab` / `VergexHeatmap` fields are populated
**only when `source_type != "vergex_signal"`**. For `vergex_signal` strategies the
existing `ctx.VergexDataMap` + `formatVergexData` path renders the same feeds.
The renderer must therefore avoid double-rendering: the new vergex blocks are
emitted only when `source_type != "vergex_signal"` and the corresponding toggle is
on.

```
=== BTC Vergex Signal Lab ===
<FormatSignalLabMarkdown output>

=== BTC Vergex Liquidation Heatmap ===
<FormatHeatmapMarkdown output>
```
Rendered only when the corresponding toggle is on and data exists.

## Frontend

### Types — `web/src/types/strategy.ts`

Add to `IndicatorConfig`:
```ts
enable_altfins_data?: boolean
altfins_intervals?: ('MINUTES15' | 'HOURLY' | 'HOURS4' | 'HOURS12' | 'DAILY')[]
enable_vergex_signal_lab_data?: boolean
enable_vergex_heatmap_data?: boolean
```

### Data sources UI — `web/src/features/strategies/EditorStepPage.tsx`

Add toggles to the existing "Data sources for LLM" section, matching the Binance
toggle style:
- **AltFins** toggle + an interval multi-select (5 options) shown when enabled.
- **Vergex Signal Lab** toggle.
- **Vergex Liquidation Heatmap** toggle.

Persist + restore in edit mode (mirror `enableAI500Data` handling).

### `dataSourceDefaults.ts` / `strategyFactory.ts`

- No scope auto-default for AltFins or vergex per-coin toggles (they are
  source-independent and opt-in), so `defaultDataSources` is unchanged except the
  new keys default to `false`.
- `strategyFactory.buildStrategyConfig` maps the new form fields into
  `indicators`.

## Error Handling

- All new fetches are best-effort: failure/no-match → log server-side, render
  nothing, never surface an error into the prompt.
- AltFins no-match (`content: []`) is a normal "no data", not an error.
- AltFins malformed response (array length mismatch) → error logged, interval
  skipped.
- vergex 403/Cloudflare/401 → warn once per cycle, omit block (graceful
  degradation). No retry storm; reuse the TTL cache's negative behavior by simply
  not caching failures.
- API key / token never logged.

## Testing

- **Provider:** table-driven parse + translation tests for `provider/altfins`
  using recorded fixtures (canonical order, reordered order, null histogram,
  empty content, invalid interval).
- **Engine:** `attachPerCoinSignals` tests with a fake AltFins client asserting
  `PerCoinSignal.AltFins` population and gating; vergex toggle tests asserting
  fetch only when `source_type != vergex_signal`.
- **Cache:** TTL get/set/expiry test mirroring `binance_detail_cache_test.go`.
- **Prompt:** renderer tests asserting the exact AltFins block (all intervals,
  null histogram omission, translation), and vergex blocks per toggle.
- **Store:** `ClampLimits` interval filtering/defaults + `EstimateTokens` delta;
  schema round-trip for the new fields.
- **Frontend:** Vitest for the new `strategyFactory` mappings and editor
  persist/restore.
- **Verification commands:** `go build ./...`, `go vet ./...`, `gofmt -l .`;
  `cd web && npx tsc --noEmit && npm run build && npm test`.

## Open Risks / Follow-ups

- **AltFins is unofficial and unversioned.** Scraped enum names/paths can change.
  Mitigation: isolate all shape knowledge in `provider/altfins`, keep parsing
  defensive, and add fixtures so drift is caught by tests.
- **Vergex Cloudflare.** If the deployed server is challenged, the vergex toggles
  will silently omit data. Follow-up (out of scope): support an optional
  `cf_clearance` cookie / proxy header env var in `NewFreeClient`.
- **`AGE` semantics.** The provider-owned interpretation is "bars since the last
  MACD crossover, computed from the raw timestamp". The API's own formatted age is
  ignored. Confidence is high (website cross-check on ZEC), but the field remains
  undocumented upstream.
