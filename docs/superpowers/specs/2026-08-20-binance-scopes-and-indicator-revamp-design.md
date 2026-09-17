# Binance Opportunity Scopes + Per-Coin Detail + Indicator Revamp — Design Spec

> **⚠️ Discontinued (2026-09-17):** Binance removed the Opportunity web API
> (`/bapi/apex/v1/friendly/apex/web/opportunity/{assets,asset-details}`) that this
> design depends on. The `binance_technical` / `binance_sentiment` scopes and
> `enable_binance_technical_data` / `enable_binance_sentiment_data` toggles are
> retained as reference implementations only and no longer return data. See
> `docs/superpowers/plans/2026-09-17-binance-opportunity-discontinued-notice.md`.

**Date:** 2026-08-20
**Branch:** `dev`
**Session scope:** Folded into ONE session — new Binance free data sources (Part A scope + Part B per-coin detail) AND the indicator-customization revamp (Part C).

## Context

The prior session delivered Tasks 1-5 (single-scope cleanup, decision_context, strategy versions, strategy stats).
This session adds two independent feature sets on top:

1. **New free Binance "Opportunity" data sources** (candidate-pool scopes + per-coin detail enrichment). Both are FREE
   (no API key, no claw402 wallet) — the same pattern as the existing vergex.trade free endpoints.
2. **Indicator-customization revamp** — surface the already-built backend indicator period/ranking config to the UI.

Both fold into this session (user confirmed).

---

## New data source: Binance Opportunity API

All endpoints are free, no auth:
- Pool: `https://www.binance.com/bapi/apex/v1/friendly/apex/web/opportunity/assets?interval=1h|24h&type=technical|sentiment`
- Detail: `https://www.binance.com/bapi/apex/v1/friendly/apex/web/opportunity/asset-details?asset=<SYM>&type=technical|sentiment&interval=1h|24h&quote=USDT`

### Verified shapes (from live fetches this session)

**Pool (`assets`) — technical (471 items), sentiment (403 items), all SPOT/USDT:**
- Top-level: `{code, message, messageDetail, data:{timestamp, items[]}, success}`
- Item fields: `asset` (symbol, e.g. "TREE"), `baseAsset`, `quoteAsset` (USDT), `assetType` (SPOT), `sceneType`
  (technical|sentiment), `intervalType` (technical→"1h"/"1d", sentiment→null), `rank` (int, sparse), `metrics` (object).
- Technical `metrics` keys: `technical_score_1h`/`technical_score_1d` (composite, 0-10 string) + sub-scores
  `technical_score_trend/momentum/volatility/volprice_<tf>` + 13 indicator signals (`technical_ind_*_signal_<tf>`).
- Sentiment `metrics` keys: `sentiment_score` (0-10) + `sentiment_score_kol/volume/news/social` +
  `sentiment_24h_social_volume`. NOTE: `intervalType` is null even for sentiment — sentiment is 24h-only.

**Detail (`asset-details`):** per-symbol. Technical → `technical_summary_<tf>` narrative + indicator signal `valueLabel`s.
Sentiment → `sentiment_summary` + `sentiment_news_summary` narratives + score/subscores/counts.

---

## Part A — New candidate-pool scope cards

### Scope selection model (confirmed with user)

Two new **free** cards, each with **sub-controls revealed on select** (NOT separate cards, per Q1=A):

| Card | `source_type` | Revealed sub-controls |
|------|--------------|----------------------|
| **Binance Technical** | `binance_technical` | interval `1h \| 24h` + direction `Top \| Bottom` |
| **Binance Sentiment** | `binance_sentiment` | direction `Top \| Bottom` (24h-only, no interval) |

- Rank pool by `technical_score_1h`/`technical_score_1d` (technical) or `sentiment_score` (sentiment). Q2=A (composite only).
- Direction: Top = highest score; Bottom = lowest score (contrarian/short). Q3=B confirmed.
- Max pool = `store.MaxCandidateCoins` = **10** (hard backend cap; all limits clamped to 10).

### SPOT → perp mapping (Q4=A4 confirmed)

The pool returns **SPOT/USDT** symbols. Traders trade **perpetual futures**. The getter maps each spot symbol to its
Binance-futures perp (`TREE` → `TREEUSDT` perp) and **drops any symbol with no matching perp ticker** (via the exchange's
futures symbol/ticker availability).

### OI-liquidity filter still applies (A5 confirmed)

The 15M USDT OI-liquidity filter is **source-agnostic**: it runs during `fetchMarketData` (kernel/engine_analysis.go:383-411)
for every candidate regardless of `source_type`. It uses the **configured exchange's** OI (Binance-futures OI from the market
client), NOT the opportunity API. So the new scopes automatically inherit the filter. No code change needed for this.

### Files (Part A)

- `web/src/features/strategies/scopeCatalog.ts` — add `freeBinanceOpportunity` helper + 2 cards.
- `web/src/features/strategies/ScopeStepPage.tsx` — reveal sub-controls (interval + direction) for these cards;
  prefill via `matchConcreteScope`.
- `web/src/features/strategies/strategyFactory.ts` — `buildCoinSource` new branches
  (`binance_technical`, `binance_sentiment`).
- `web/src/types/strategy.ts` — add source_type values + `binance_*` fields + direction/interval union types.
- `store/strategy.go` — `CoinSourceConfig`: add `binance_technical_interval`, `binance_technical_direction`,
  `binance_technical_limit`, `binance_sentiment_direction`, `binance_sentiment_limit`; `normalizeCoinSourceType`,
  `inferCoinSourceType`, `ClampLimits`, token-estimate cases.
- `kernel/engine.go` — `GetCandidateCoins` `case "binance_technical"` / `case "binance_sentiment"` →
  `getBinanceOpportunityCoins(...)` → `filterExcludedCoins`.
- New `provider/binance/opportunity.go` — `OpportunityClient` with `GetOpportunityAssets(interval, scene)`,
  `GetAssetDetails(symbol, scene, interval)`.

---

## Part B — New per-coin detail data sources

### Confirmed design (B1, B2)

Two new toggles in the **"Data sources"** section (see Part C restructure):

| Toggle | Detail endpoint | Interval control |
|--------|----------------|------------------|
| **Binance Technical** | `asset-details?type=technical` | user picks `1h` and/or `24h` (independent buttons; no "both" composite) |
| **Binance Sentiment** | `asset-details?type=sentiment` | 24h-only, no interval buttons |

Per-coin detail intervals are **fully independent** of the scope interval (B2). A user may scope technical at 24h but pull
per-coin technical at 1h (or 1h+24h).

### Prefetch + cache design (B3, Option 1 confirmed)

Max candidates = 10. Worst case per-coin detail = 10 symbols × up to 4 sources (1h tech + 24h tech + sentiment + existing)
≈ 30-40 HTTP calls per cycle. To avoid a request burst at decision time:

1. **In-memory TTL cache** keyed by `(source, symbol, interval)`, TTL ~60s. Writes are protected by a mutex (the existing
   engine shared-map race concern in CONCERNS.md — new cache must be concurrency-safe).
2. **Cycle-aligned prefetch (Option 1):** in the trader loop, immediately after `GetCandidateCoins()` returns the ≤10-symbol
   pool, spawn a **background goroutine** (bounded worker pool, ~2-3 workers) to fetch all enabled per-coin sources for the
   candidate symbols, populating the cache. This spreads request load over the scan interval instead of bursting at decision.
3. **Prompt-time fallback:** `attachPerCoinSignals` / `formatPerCoinSignals` reads from the cache; any symbol still missing
   at prompt time (e.g. cold cache on first cycle) fetches synchronously as a single-symbol fallback.

The prefetch step is added to `trader/auto_trader_loop.go` (next to the existing `AttachPerCoinSignals` call).

### Files (Part B)

- `store/strategy.go` `IndicatorConfig` — add `EnableBinanceTechnicalData bool` (`enable_binance_technical_data`),
  `EnableBinanceSentimentData bool` (`enable_binance_sentiment_data`), `BinanceTechnicalIntervals []string`
  (`binance_technical_intervals`).
- `kernel/engine.go` — `PerCoinSignal` add `BinanceTechnical *BinanceTechnicalDetail`, `BinanceSentiment *BinanceSentimentDetail`.
- `kernel/engine_analysis.go` — `attachPerCoinSignals`: add 2 flags to early-return guard; fetch+store per source.
- `kernel/engine_prompt.go` — `formatPerCoinSignals`: render 2 new sections. Tests in `engine_prompt_test.go`.
- `provider/binance/opportunity.go` — `GetAssetDetails` + typed detail structs.
- `trader/auto_trader_loop.go` — background prefetch step (Option 1).
- `web/src/features/strategies/EditorStepPage.tsx` — 2 new ToggleChips + interval buttons.
- `web/src/features/strategies/strategyFactory.ts` — `enableBinanceTechnicalData?`, `enableBinanceSentimentData?`,
  `binanceTechnicalIntervals?` → `IndicatorConfig`.
- `web/src/features/strategies/dataSourceDefaults.ts` — auto-enable when scope is the matching `binance_*`.
- `web/src/types/strategy.ts` — `IndicatorConfig` new fields.

---

## Part C — Indicator-customization revamp

### Problem

Backend `IndicatorConfig` already supports period arrays + many toggles, but the frontend only surfaces hardcoded toggles
("EMA20", "RSI7", MACD, OI, Funding rate) with no period customization, and omits ATR/BOLL/Volume entirely. Several fields
are silently dropped on edit (lossy round-trip).

### Confirmed restructure (C1, C2, C3, C4, C5, C6)

Split the crowded "Data sources for LLM" fieldset into **two separate `<fieldset>` cards**:

**1. "Basic indicators"** — technical indicator toggles + period chips:
- Toggles: EMA, MACD, RSI, ATR, BOLL, Volume.
- When a toggle is on, show period multi-select chips (defaults from backend):
  - EMA → `9, 10, 20, 50, 200`, default `[20, 50]` → `ema_periods`
  - RSI → `7, 14, 21`, default `[7, 14]` → `rsi_periods`
  - ATR → `7, 14, 21`, default `[14]` → `atr_periods`
  - BOLL → `10, 20, 50`, default `[20]` → `boll_periods`
  - Volume → no period array.
- Generic labels (EMA, RSI, ATR, BOLL) — remove "EMA20"/"RSI7" hardcoding. (C3)

**2. "Data sources"** — LLM enrichment sources:
- Existing per-coin toggles: AI500, OI, Netflow, Price change + data-durations row.
- New per-coin toggles (Part B): Binance Technical, Binance Sentiment.
- **Ranking toggles** under their own sub-heading (C2): OI ranking (+duration+limit), Netflow ranking (+duration+limit),
  Price ranking (+duration+limit).
- **Quant group skipped** on frontend (C6): `enable_quant_*` NOT surfaced, left as-is on backend.

### Additional surfaces

- **Candles row** (Advanced Settings): add primary kline count (default 30) + longer timeframe/count when multi-timeframe on. (C5)
- **Edit round-trip parity** (C4): load back ALL indicator toggles, period arrays, ranking flags, and per-coin toggles on edit,
  so nothing is silently dropped. Works on both create & edit pages.
- **`buildStrategyConfig`** now emits `ema_periods`, `rsi_periods`, `atr_periods`, `boll_periods`, `enable_atr`,
  `enable_boll`, `enable_volume`, and the ranking fields.
- **primary_timeframe**: keep existing mapping (first selected timeframe); add an explicit label so it's not conflated with the
  selected-timeframes multi-select.

### Files (Part C)

- `web/src/features/strategies/EditorStepPage.tsx` — restructure into two fieldset cards; add period chips, ranking toggles,
  candles count/longer controls; full edit load-back.
- `web/src/features/strategies/strategyFactory.ts` — `StrategyEditorForm` + `buildStrategyConfig` for all new fields.
- `web/src/types/strategy.ts` — `IndicatorConfig` (already has period/ranking fields; add any missing).
- Tests: `strategyFactory.test.ts`, `dataSourceDefaults.test.ts`, `EditorStepPage` rendering.

---

## Constraint notes

- Backend: `go build ./... && go vet ./... && go test ./...` clean. Frontend: `cd web && npx tsc --noEmit && npm run build && npm test`.
- English-only UI strings. Backend errors via `SafeError`/`SafeInternalError`/`SanitizeError`.
- `store.*` is the only DB access layer. All timestamps UTC.
- Field-name parity between backend Go structs and frontend TS types (JSON round-trip; mismatch silently drops config).
- New scope must return a non-empty pool (empty → "No candidate coins available, cycle skipped").
- Per-coin sources gated by `enable_*_data` flags; a source with no data for a symbol is omitted from the prompt.
- No changes to live order/position behavior (HIGH-RISK gating not triggered).