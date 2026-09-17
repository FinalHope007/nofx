# OI-Aware Candidate Pool + Pre-Cycle Binance Prefetch Design

> **⚠️ Discontinued (2026-09-17):** Binance removed the Opportunity web API
> (`/bapi/apex/v1/friendly/apex/web/opportunity/{assets,asset-details}`) that this
> design depends on. The `binance_technical` / `binance_sentiment` scopes and
> `enable_binance_technical_data` / `enable_binance_sentiment_data` toggles are
> retained as reference implementations only and no longer return data. See
> `docs/superpowers/plans/2026-09-17-binance-opportunity-discontinued-notice.md`.

## Goal

Two related fixes for the Binance Opportunity candidate pipeline:

1. **OI-Aware Candidate Pool** — Stop truncating the Binance candidate list before the OI liquidity filter runs. Fetch all candidates, let the OI filter evaluate them sequentially, and stop once `MaxCandidateCoins` (10) survivors are found. This fixes the issue where enabling the OI filter reduced the pool from 10 to 3.

2. **Pre-Cycle Binance Prefetch** — Move Binance per-coin detail fetching out of the decision cycle. A timer-based prefetch fires ~145s before the next cycle trigger, warming the TTL cache for the top 30 candidates. By cycle time, the cache is warm and the prompt builder finds cache hits. This fixes the 429 Too Many Requests errors caused by bursting requests during the cycle.

## Architecture

### Feature 1: OI-Aware Candidate Pool

**Current broken flow:**
```
Binance API (470 assets)
  → sort by score, truncate to limit (10)    ← PROBLEM
  → mapSpotToPerp (10 perp symbols)
  → fetchMarketDataWithStrategy (OI filter removes 7)
  → 3 survivors
```

**Fixed flow:**
```
Binance API (470 assets)
  → mapSpotToPerp (all 470, sorted by score)
  → fetchMarketDataWithStrategy iterates sequentially:
      for each candidate (in score order):
        fetch market data (price/OHLCV/OI)
        if OI filter passes: add to survivors
        if len(survivors) >= MaxCandidateCoins (10): STOP immediately
  → prune candidates without market data
  → final pool: up to 10 OI-filtered survivors
```

**Files changed:**

- `kernel/engine.go` — `getBinanceOpportunityCoins`: remove the `limit` truncation on the final candidates. Return all mapped candidates sorted by score. The `limit` parameter is still accepted as a safety soft cap on the return value (default 50) to avoid passing an absurdly large list to the prompt builder — but this is NOT the OI filter cap. The OI filter runs on the full sorted list and stops at `MaxCandidateCoins` survivors.
- `kernel/engine_analysis.go` — `fetchMarketDataWithStrategy`: add early exit once `MaxCandidateCoins` survivors are collected. Stop fetching market data for remaining candidates.
- `kernel/engine_analysis.go` — `pruneCandidateCoinsWithoutMarketData`: no change needed (already prunes correctly).

**Key invariant:** `MaxCandidateCoins = 10` remains the hard cap for the LLM prompt. It is applied AFTER the OI filter, not before.

**Non-Binance sources:** Unchanged. AI500, OI, Netflow, etc. already return exchange-native symbols with inherent liquidity. The early-exit logic applies to all sources but has no effect on them since they typically return ≤10 candidates.

### Feature 2: Pre-Cycle Binance Prefetch

**Current broken flow:**
```
ticker.C fires
  → buildTradingContext()
    → GetCandidateCoins()
    → runPrefetchJobs(3 workers, burst)      ← 429 errors
    → fetchMarketDataWithStrategy (price/OHLCV)
    → attachPerCoinSignals (sync fetch on cache miss)  ← 429 errors
  → AI call
```

**Fixed flow:**
```
[145s before ticker.C]
  → timer fires (time.AfterFunc)
  → GetCandidateCoins() (fresh call)
  → take top 30 by score
  → prefetch Binance detail: 1 worker, 1.5s delay between requests
  → cache warmed (10 min TTL)

[ticker.C fires]
  → buildTradingContext()
    → GetCandidateCoins() (reused or fresh)
    → fetchMarketDataWithStrategy (price/OHLCV — freshness-critical)
    → attachPerCoinSignals:
        for each candidate:
          if cache hit (from prefetch): use cached data
          if cache miss (not in top 30): sync fetch with 429 retry
  → AI call
```

**Files changed:**

- `kernel/binance_detail_cache.go` — Change `binanceDetailCacheTTL` from 60s to 10 minutes.
- `provider/binance/opportunity.go` — `GetAssetDetails`: add HTTP 429 detection. On 429, sleep 1.5s and retry once. Return a clear error if retry also fails.
- `trader/auto_trader_prefetch.go` — Replace `runPrefetchJobs` (3 concurrent workers) with `runRateLimitedPrefetch` (1 worker, 1.5s delay between requests). The old `runPrefetchJobs` is kept for the concurrency test but the trader loop uses the new function.
- `trader/auto_trader_loop.go` — Remove the inline prefetch goroutine. Add a `scheduleBinancePrefetch` method that uses `time.AfterFunc` to fire the prefetch 145s before the next cycle. The timer is scheduled after each cycle completes and cancelled if the trader stops.
- `trader/auto_trader.go` — Add a `prefetchTimer *time.Timer` field to `AutoTrader` for lifecycle management.

**Prefetch lead time calculation:**
```
requestsPerCoin = len(BinanceTechnicalIntervals) + (EnableBinanceSentimentData ? 1 : 0)
leadTime = requestsPerCoin × 30 × 1.5s + 10s
```

Example: 1h + 24h technical + sentiment = 3 requests/coin → `3 × 30 × 1.5 + 10 = 145s`

If only 1h technical = 1 request/coin → `1 × 30 × 1.5 + 10 = 55s`

**Timer lifecycle:**
- After each `runCycle()` completes, calculate `nextTick = now + ScanInterval`
- Calculate `prefetchStart = nextTick - leadTime`
- Schedule `time.AfterFunc(prefetchStart - now, at.scheduleBinancePrefetch)`
- If `prefetchStart - now <= 0` (cycle interval shorter than lead time), skip prefetch — fall back to synchronous fetch only
- On `Stop()`: cancel the timer via `at.prefetchTimer.Stop()`

**429 retry in `GetAssetDetails`:**
```go
resp, err := c.http.Do(req)
if err != nil {
    return nil, fmt.Errorf("binance opportunity: request: %w", err)
}
defer resp.Body.Close()
if resp.StatusCode == http.StatusTooManyRequests {
    time.Sleep(1500 * time.Millisecond)
    resp2, err2 := c.http.Do(req)
    if err2 != nil {
        return nil, fmt.Errorf("binance opportunity: retry failed: %w", err2)
    }
    // Replace resp with resp2 for continued processing
    resp.Body.Close()
    resp = resp2
}
if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("binance opportunity: HTTP %d", resp.StatusCode)
}
```

**Fallback behavior:**
At cycle time, `attachPerCoinSignals` checks the cache for each candidate. If a cache hit exists (from prefetch), it uses the cached data. If a cache miss occurs (candidate not in top 30, or prefetch failed), it falls back to synchronous `GetAssetDetails` with the 429 retry logic. The synchronous fallback processes one symbol at a time, naturally rate-limiting itself.

## Data Flow

```
                    ┌─────────────────────────────────────────────────┐
                    │           PRE-CYLE PREFETCH                      │
                    │  (fires ~145s before ticker.C)                   │
                    │                                                   │
                    │  GetCandidateCoins()                              │
                    │     ↓                                             │
                    │  Sort by score, take top 30                       │
                    │     ↓                                             │
                    │  For each of 30 candidates:                       │
                    │    For each enabled source (1h tech, 24h tech,   │
                    │    sentiment):                                    │
                    │      GetAssetDetails → cacheBinanceDetail         │
                    │      sleep 1.5s                                   │
                    │                                                   │
                    │  Cache TTL: 10 minutes                            │
                    └─────────────────────────────────────────────────┘
                                        │
                                        ↓ (cache warm)
                    ┌─────────────────────────────────────────────────┐
                    │           CYCLE TRIGGER (ticker.C)               │
                    │                                                   │
                    │  buildTradingContext()                            │
                    │     ↓                                             │
                    │  GetCandidateCoins()                              │
                    │     ↓ (all candidates, sorted by score)           │
                    │  fetchMarketDataWithStrategy():                   │
                    │     for each candidate (score order):             │
                    │       fetch price/OHLCV from exchange             │
                    │       OI filter check                             │
                    │       if pass: add to survivors                   │
                    │       if survivors >= 10: STOP                    │
                    │     ↓                                             │
                    │  pruneCandidateCoinsWithoutMarketData()           │
                    │     ↓ (up to 10 survivors)                        │
                    │  attachPerCoinSignals():                           │
                    │     for each survivor:                            │
                    │       cache hit? → use cached Binance detail      │
                    │       cache miss? → sync GetAssetDetails (retry)  │
                    │     ↓                                             │
                    │  formatPerCoinSignals() → prompt                  │
                    │     ↓                                             │
                    │  AI call (with fresh price + Binance detail)      │
                    └─────────────────────────────────────────────────┘
```

## Error Handling

| Scenario | Behavior |
|---|---|
| Binance API 429 on prefetch | Log warning, skip that symbol. Cache miss at cycle time → sync fallback |
| Binance API 429 on sync fallback | Retry once after 1.5s. If retry fails, log warning, omit Binance section for that symbol |
| Prefetch timer fires but trader stopped | Early exit check (`at.isRunning`), no-op |
| Cycle interval < lead time | Skip prefetch scheduling, rely on sync fallback only |
| `GetCandidateCoins()` fails at prefetch time | Log warning, no prefetch. Cache empty at cycle time → sync fallback |
| All 30 prefetched candidates fail OI filter | Sync fallback handles any new survivors not in top 30 |
| Exchange API fails for a candidate | Existing behavior: log warning, skip coin, continue to next |

## Testing

- **OI-aware pool test:** Mock Binance API returning 20 assets, mock market data with varying OI values, assert that the final pool contains exactly `MaxCandidateCoins` survivors with sufficient OI, and that early exit occurred (no market data fetched for candidates beyond the 10th survivor).
- **429 retry test:** Mock HTTP server returning 429 on first call, 200 on second. Assert `GetAssetDetails` retries and returns data.
- **Prefetch rate-limiting test:** Assert `runRateLimitedPrefetch` processes symbols sequentially with ≥1.5s between requests.
- **TTL test:** Update existing `TestBinanceDetailCacheTTL` to use 10-minute TTL.

## Constraints

- `MaxCandidateCoins = 10` is a hard cap — no candidate pool exceeds this.
- OI filter (`$15M` threshold) stays enabled by default — user no longer needs to disable it.
- Binance detail data (1h/24h technical, sentiment) is NOT freshness-critical.
- Price/OHLCV data IS freshness-critical — always fetched at cycle time, never prefetched.
- No changes to live order/position behavior.
- No new user-facing config fields.
- Backend Go struct field names and JSON tags match frontend TS types exactly.
- `go build ./... && go vet ./... && go test ./...` must pass.
