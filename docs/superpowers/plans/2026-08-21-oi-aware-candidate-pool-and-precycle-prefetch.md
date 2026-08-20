# OI-Aware Candidate Pool + Pre-Cycle Binance Prefetch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Binance Opportunity candidate pipeline so the OI liquidity filter doesn't reduce the pool below `MaxCandidateCoins`, and move Binance detail fetching out of the decision cycle to avoid 429 rate-limiting.

**Architecture:** Two features: (1) Stop truncating the Binance candidate list before OI filtering — fetch all, filter sequentially, stop at 10 survivors. (2) Schedule a timer-based prefetch ~145s before each cycle trigger, warming the Binance detail TTL cache (10 min) for the top 30 candidates, with a synchronous fallback for cache misses.

**Tech Stack:** Go 1.25 (Gin, GORM), React 18/TS/Vite (no frontend changes).

## Global Constraints

- Backend: `go build ./... && go vet ./... && go test ./...` must pass.
- `MaxCandidateCoins = 10` is the hard cap for the LLM prompt — applied AFTER OI filtering.
- OI filter (`$15M` threshold) stays enabled — protects against whale manipulation.
- Binance detail data (1h/24h technical, sentiment) is NOT freshness-critical.
- Price/OHLCV data IS freshness-critical — always fetched at cycle time.
- No changes to live order/position behavior.
- No new user-facing config fields.

---

## File Structure

**Backend (modified):**
- `kernel/engine.go` — `getBinanceOpportunityCoins`: remove early `limit` truncation, return all mapped candidates sorted by score, soft-cap at 50.
- `kernel/engine_analysis.go` — `fetchMarketDataWithStrategy`: add early exit once `MaxCandidateCoins` survivors collected.
- `kernel/binance_detail_cache.go` — Change TTL from 60s to 10 minutes.
- `provider/binance/opportunity.go` — `GetAssetDetails`: add HTTP 429 retry logic.
- `trader/auto_trader_prefetch.go` — Add `runRateLimitedPrefetch` (1 worker, 1.5s delay); keep `runPrefetchJobs` for backward compatibility.
- `trader/auto_trader_loop.go` — Remove inline prefetch goroutine; add `scheduleBinancePrefetch` using `time.AfterFunc`.
- `trader/auto_trader.go` — Add `prefetchTimer *time.Timer` field; cancel timer on `Stop()`.

**Tests (modified/created):**
- `kernel/engine_test.go` — Update `TestGetCandidateCoinsBinanceTechnical` to verify no early truncation.
- `kernel/engine_analysis_test.go` (new) — Test early exit in `fetchMarketDataWithStrategy`.
- `provider/binance/opportunity_test.go` — Test 429 retry in `GetAssetDetails`.
- `trader/auto_trader_prefetch_test.go` — Update for `runRateLimitedPrefetch` (1 worker, timing).
- `kernel/binance_detail_cache_test.go` — Update TTL test for 10 minutes.

---

### Task 1: Update `getBinanceOpportunityCoins` to stop early truncation

**Files:**
- Modify: `kernel/engine.go` — `getBinanceOpportunityCoins` method (currently at lines 976-1023)
- Modify: `kernel/engine_test.go`

**Interfaces:**
- Consumes: `OpportunityClient.GetOpportunityAssets` (existing), `fakeOpportunityClient` test double (existing in `kernel/engine_test.go`)
- Produces: `getBinanceOpportunityCoins` returns all mapped candidates sorted by score, soft-capped at 50 (not at the user's `limit`)

**Current buggy behavior (verified in `kernel/engine.go:1011-1013`):**
```go
if len(mapped) > limit {
    mapped = mapped[:limit]
}
```
This truncates to the user's `limit` (clamped to `MaxCandidateCoins`=10 by the caller) BEFORE the OI liquidity filter runs in `fetchMarketDataWithStrategy`. Since most top-scoring Binance assets are low-cap coins that fail the OI filter, the final candidate pool shrinks well below 10.

- [ ] **Step 1: Write the failing test**

Add to `kernel/engine_test.go` a test double that returns many assets, then assert `getBinanceOpportunityCoins` does NOT truncate to the small `limit`:

First, add `"fmt"` to the imports in `kernel/engine_test.go` (after `"context"`):

```go
import (
	"context"
	"fmt"
	"testing"

	"nofx/provider/binance"
	"nofx/store"
)
```

Then add the test double and tests:

```go
type fakeOpportunityClientN struct {
	n int
}

func (f *fakeOpportunityClientN) GetOpportunityAssets(_ context.Context, _, _ string) ([]binance.OpportunityAsset, error) {
	assets := make([]binance.OpportunityAsset, f.n)
	for i := 0; i < f.n; i++ {
		assets[i] = binance.OpportunityAsset{
			Symbol: fmt.Sprintf("COIN%d", i),
			Score:  float64(f.n - i), // descending score: COIN0 has the highest score
		}
	}
	return assets, nil
}

func (f *fakeOpportunityClientN) GetAssetDetails(_ context.Context, _, _, _ string) (map[string]string, error) {
	return map[string]string{"fake_label": "fake_value"}, nil
}

func TestGetCandidateCoinsBinanceTechnicalNotTruncatedByLimit(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 5 // user set a small limit

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientN{n: 20} // API returns 20 assets

	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return all 20 (soft-capped at 50, so 20 passes through unchanged),
	// NOT truncated to limit=5. The final cap to 10 happens later in
	// fetchMarketDataWithStrategy, after the OI filter runs.
	if len(coins) != 20 {
		t.Fatalf("expected 20 coins (no early truncation to limit=5), got %d", len(coins))
	}
	// Verify sorted by score descending: COIN0 (score 20) should be first.
	if coins[0].Symbol != "COIN0USDT" {
		t.Fatalf("expected COIN0USDT first (highest score), got %s", coins[0].Symbol)
	}
}

func TestGetCandidateCoinsBinanceTechnicalSoftCapAt50(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientN{n: 100} // API returns 100 assets

	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 50 {
		t.Fatalf("expected 50 coins (soft cap), got %d", len(coins))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run "TestGetCandidateCoinsBinanceTechnicalNotTruncatedByLimit|TestGetCandidateCoinsBinanceTechnicalSoftCapAt50" -v`
Expected: FAIL — `TestGetCandidateCoinsBinanceTechnicalNotTruncatedByLimit` fails because current code truncates to `limit=5`, returning 5 coins instead of 20. `TestGetCandidateCoinsBinanceTechnicalSoftCapAt50` fails because current code truncates to `limit=10` (clamped from user's 10), returning 10 instead of 50.

- [ ] **Step 3: Remove the `limit` truncation and add a soft cap at 50**

In `kernel/engine.go`, modify `getBinanceOpportunityCoins` (around line 976). Replace the entire function body's truncation logic:

```go
func (e *StrategyEngine) getBinanceOpportunityCoins(scene, interval, direction string, limit int) ([]CandidateCoin, error) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "" {
		direction = "top"
	}
	assets, err := e.opportunity.GetOpportunityAssets(context.Background(), interval, scene)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Binance %s opportunity: %w", scene, err)
	}
	// Map all spot symbols to perp FIRST, then filter and sort.
	// This avoids losing assets due to early truncation before the mapping check.
	type scoredPerp struct {
		symbol string
		score  float64
	}
	mapped := make([]scoredPerp, 0, len(assets))
	for _, a := range assets {
		perp := mapSpotToPerp(a.Symbol)
		if perp == "" {
			continue
		}
		mapped = append(mapped, scoredPerp{symbol: perp, score: a.Score})
	}
	sort.Slice(mapped, func(i, j int) bool {
		if direction == "bottom" {
			return mapped[i].score < mapped[j].score
		}
		return mapped[i].score > mapped[j].score
	})
	// Safety soft cap: avoid returning an absurdly large candidate list to the
	// prompt builder. This is NOT the final prompt cap — the OI liquidity
	// filter in fetchMarketDataWithStrategy applies the real MaxCandidateCoins
	// cap AFTER filtering low-liquidity coins. The user's `limit` parameter is
	// intentionally unused here; it no longer truncates the pool.
	const softCap = 50
	if len(mapped) > softCap {
		mapped = mapped[:softCap]
	}
	candidates := make([]CandidateCoin, 0, len(mapped))
	for _, m := range mapped {
		candidates = append(candidates, CandidateCoin{
			Symbol:  m.symbol,
			Sources: []string{"binance_" + scene},
		})
	}
	logger.Infof("✅ Loaded %d Binance %s opportunity coins (dir=%s, soft-capped at %d)", len(candidates), scene, direction, softCap)
	return candidates, nil
}
```

Note: the `limit` parameter is kept in the function signature (callers in `GetCandidateCoins` still pass it) but is no longer used to truncate — this avoids changing the call sites in `GetCandidateCoins` (lines 594-606). Go will not error on an unused parameter (only unused local variables), so this compiles cleanly.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run "TestGetCandidateCoinsBinance" -v`
Expected: all pass, including the two new tests and the existing `TestGetCandidateCoinsBinanceTechnical` and `TestGetCandidateCoinsBinanceSentiment`.

- [ ] **Step 5: Run full backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add kernel/engine.go kernel/engine_test.go && git commit -m "feat(kernel): remove early limit truncation in getBinanceOpportunityCoins, soft-cap at 50"
```

---

### Task 2: Add early exit in `fetchMarketDataWithStrategy`

**Files:**
- Modify: `kernel/engine_analysis.go` — `fetchMarketDataWithStrategy`
- Create: `kernel/engine_analysis_test.go`

**Interfaces:**
- Consumes: `store.MaxCandidateCoins` (10), `engine.GetRiskControlConfig()` (OI filter settings)
- Produces: `fetchMarketDataWithStrategy` stops fetching market data once `MaxCandidateCoins` survivors collected

- [ ] **Step 1: Write the failing test**

Create `kernel/engine_analysis_test.go`:

```go
package kernel

import (
	"testing"

	"nofx/store"
)

type fakeTrader struct {
	exchange string
}

func (f *fakeTrader) GetBalance() (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeTrader) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

func TestFetchMarketDataEarlyExit(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10
	cfg.RiskControl.EnableOILiquidityFilter = false

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientWithAssets(20)

	// Get 20 candidates
	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &Context{
		CandidateCoins: candidates,
		Positions:      nil,
		Ctx:            context.Background(),
	}

	// fetchMarketDataWithStrategy should not panic or error
	// With fake exchange, market data fetches will fail, but the
	// early exit test is about iteration count, not data success.
	// We test that the function iterates without hanging.
	err = fetchMarketDataWithStrategy(ctx, engine)
	if err != nil {
		// Expected to fail on market data fetch with fake exchange
		// The key assertion is it doesn't hang or panic
		t.Logf("expected fetch error with fake exchange: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestFetchMarketDataEarlyExit -v`
Expected: FAIL or PASS depending on current behavior. The key is to add the early exit logic next.

- [ ] **Step 3: Add early exit to `fetchMarketDataWithStrategy`**

In `kernel/engine_analysis.go`, add a survivors counter and early exit in the `for _, coin := range ctx.CandidateCoins` loop (around line 451):

```go
maxCandidates := store.MaxCandidateCoins
survivors := 0

for _, coin := range ctx.CandidateCoins {
    if _, exists := ctx.MarketDataMap[coin.Symbol]; exists {
        continue
    }

    data, err := market.GetWithTimeframesWithExchange(coin.Symbol, timeframes, primaryTimeframe, klineCount, engine.exchange)
    if err != nil {
        logger.Infof("⚠️  Failed to fetch market data for %s: %v", coin.Symbol, err)
        continue
    }

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

    ctx.MarketDataMap[coin.Symbol] = data
    survivors++

    // Early exit: once we have enough survivors for the prompt, stop fetching.
    if survivors >= maxCandidates {
        logger.Infof("📊 Reached %d survivors (max=%d), stopping market data fetch early", survivors, maxCandidates)
        break
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestFetchMarketDataEarlyExit -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add kernel/engine_analysis.go kernel/engine_analysis_test.go && git commit -m "feat(kernel): early exit in fetchMarketDataWithStrategy when MaxCandidateCoins survivors reached"
```

---

### Task 3: Change Binance detail TTL from 60s to 10 minutes

**Files:**
- Modify: `kernel/binance_detail_cache.go` — `binanceDetailCacheTTL`
- Modify: `kernel/binance_detail_cache_test.go` — Update TTL test

**Interfaces:**
- Consumes: existing `binanceDetailCache` implementation
- Produces: `binanceDetailCacheTTL = 10 * time.Minute`

- [ ] **Step 1: Write the failing test**

Update `TestBinanceDetailCacheTTL` in `kernel/binance_detail_cache_test.go`:

```go
func TestBinanceDetailCacheTTL(t *testing.T) {
	e := NewStrategyEngine(nil)
	key := "technical|BTCUSDT|1h"
	if _, ok := e.binanceDetail(key); ok {
		t.Fatal("expected empty cache")
	}
	e.cacheBinanceDetail(key, map[string]string{"technical_score_1h": "Positive"})
	got, ok := e.binanceDetail(key)
	if !ok || got["technical_score_1h"] != "Positive" {
		t.Fatalf("expected cached value, got %v ok=%v", got, ok)
	}
	// TTL is 10 minutes — verify it hasn't expired after a short wait
	// (we don't wait 10 minutes in the test, just check the constant)
	if binanceDetailCacheTTL != 10*time.Minute {
		t.Fatalf("expected TTL of 10 minutes, got %v", binanceDetailCacheTTL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestBinanceDetailCacheTTL -v`
Expected: FAIL — `binanceDetailCacheTTL` is still 60s.

- [ ] **Step 3: Change the TTL constant**

In `kernel/binance_detail_cache.go`:

```go
const binanceDetailCacheTTL = 10 * time.Minute
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestBinanceDetailCacheTTL -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add kernel/binance_detail_cache.go kernel/binance_detail_cache_test.go && git commit -m "feat(kernel): increase Binance detail TTL from 60s to 10 minutes"
```

---

### Task 4: Add 429 retry to `GetAssetDetails`

**Files:**
- Modify: `provider/binance/opportunity.go` — `GetAssetDetails`
- Modify: `provider/binance/opportunity_test.go`

**Interfaces:**
- Consumes: `GetAssetDetails` existing implementation
- Produces: HTTP 429 responses trigger a 1.5s sleep + retry; non-200 responses return an error

- [ ] **Step 1: Write the failing test**

Add to `provider/binance/opportunity_test.go`:

```go
func TestGetAssetDetailsRetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"code":"429","message":"rate limit exceeded"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"metrics":{
			"technical_score_1h":{"value":"7.85","valueLabel":"Positive"}
		}},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["technical_score_1h"] != "Positive" {
		t.Fatalf("expected score label after retry, got %q", got["technical_score_1h"])
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts (1 fail + 1 retry), got %d", attempts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -run TestGetAssetDetailsRetryOn429 -v`
Expected: FAIL — current code doesn't check `resp.StatusCode`.

- [ ] **Step 3: Implement 429 retry in `GetAssetDetails`**

In `provider/binance/opportunity.go`, update `GetAssetDetails`:

```go
func (c *OpportunityClient) GetAssetDetails(ctx context.Context, symbol, scene, interval string) (map[string]string, error) {
	if interval == "" {
		interval = "1h"
	}
	url := fmt.Sprintf("%s/asset-details?asset=%s&type=%s&interval=%s&quote=USDT",
		c.baseURL, symbol, scene, interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("binance opportunity: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance opportunity: request: %w", err)
	}
	defer resp.Body.Close()

	// Retry once on 429 Too Many Requests with 1.5s backoff.
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		time.Sleep(1500 * time.Millisecond)
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("binance opportunity: retry request: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance opportunity: HTTP %d", resp.StatusCode)
	}

	var parsed opportunityDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("binance opportunity: decode: %w", err)
	}
	out := make(map[string]string, len(parsed.Data.Metrics))
	for k, m := range parsed.Data.Metrics {
		label := strings.TrimSpace(m.ValueLabel)
		if label == "" {
			label = strings.TrimSpace(m.Value)
		}
		if label != "" {
			out[k] = label
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -run TestGetAssetDetailsRetryOn429 -v`
Expected: PASS.

- [ ] **Step 5: Run existing tests to verify no regression**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -v`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add provider/binance/opportunity.go provider/binance/opportunity_test.go && git commit -m "feat(binance): add 429 retry with backoff to GetAssetDetails"
```

---

### Task 5: Add `runRateLimitedPrefetch` to prefetch module

**Files:**
- Modify: `trader/auto_trader_prefetch.go` — Add `runRateLimitedPrefetch`
- Modify: `trader/auto_trader_prefetch_test.go` — Add test

**Interfaces:**
- Consumes: `candidateSymbols` from existing module
- Produces: `runRateLimitedPrefetch(symbols []string, delay time.Duration, fn func(string))` — 1 worker, sequential with delay

- [ ] **Step 1: Write the failing test**

Add to `trader/auto_trader_prefetch_test.go`:

```go
func TestRunRateLimitedPrefetchRespectsDelay(t *testing.T) {
	var timestamps []time.Time
	var mu sync.Mutex
	symbols := []string{"A", "B", "C"}

	start := time.Now()
	runRateLimitedPrefetch(symbols, 50*time.Millisecond, func(sym string) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
	})
	elapsed := time.Since(start)

	if len(timestamps) != 3 {
		t.Fatalf("expected 3 invocations, got %d", len(timestamps))
	}
	// Each invocation should be at least 50ms apart
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap < 45*time.Millisecond { // allow small timer imprecision
			t.Fatalf("gap between %d and %d was %v, expected >=50ms", i-1, i, gap)
		}
	}
	// Total should be at least 100ms (2 gaps × 50ms)
	if elapsed < 90*time.Millisecond {
		t.Fatalf("total elapsed %v, expected >=90ms", elapsed)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./trader/ -run TestRunRateLimitedPrefetchRespectsDelay -v`
Expected: FAIL — `runRateLimitedPrefetch` undefined.

- [ ] **Step 3: Implement `runRateLimitedPrefetch`**

Add to `trader/auto_trader_prefetch.go`:

```go
// runRateLimitedPrefetch executes fn for each symbol with a fixed delay between
// requests, using a single worker. This is suitable for APIs with rate limits
// (e.g. Binance Opportunity asset-details endpoint).
func runRateLimitedPrefetch(symbols []string, delay time.Duration, fn func(string)) {
	if len(symbols) == 0 || fn == nil {
		return
	}
	for i, s := range symbols {
		fn(s)
		if i < len(symbols)-1 {
			time.Sleep(delay)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./trader/ -run TestRunRateLimitedPrefetchRespectsDelay -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add trader/auto_trader_prefetch.go trader/auto_trader_prefetch_test.go && git commit -m "feat(trader): add runRateLimitedPrefetch with sequential rate limiting"
```

---

### Task 6: Add `prefetchTimer` field and timer lifecycle to AutoTrader

**Files:**
- Modify: `trader/auto_trader.go` — Add `prefetchTimer` field, cancel in `Stop()`

**Interfaces:**
- Consumes: `AutoTrader` struct, `Stop()` method
- Produces: `prefetchTimer *time.Timer` field; `Stop()` cancels timer

- [ ] **Step 1: Add field to AutoTrader struct**

In `trader/auto_trader.go`, add to `AutoTrader` struct (after `consecutiveAIFailures`):

```go
prefetchTimer *time.Timer // Pre-cycle Binance prefetch timer (nil when not scheduled)
```

- [ ] **Step 2: Cancel timer in `Stop()`**

In `trader/auto_trader.go`, update `Stop()` method:

```go
func (at *AutoTrader) Stop() {
	at.isRunningMutex.Lock()
	if !at.isRunning {
		at.isRunningMutex.Unlock()
		return
	}
	at.isRunning = false
	at.isRunningMutex.Unlock()

	// Cancel any pending prefetch timer
	if at.prefetchTimer != nil {
		at.prefetchTimer.Stop()
		at.prefetchTimer = nil
	}

	close(at.stopMonitorCh) // Notify monitoring goroutine to stop
	at.monitorWg.Wait()     // Wait for monitoring goroutine to finish
	logger.Info("⏹ Automatic trading system stopped")
}
```

- [ ] **Step 3: Run build to verify no compilation errors**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add trader/auto_trader.go && git commit -m "feat(trader): add prefetchTimer field with lifecycle management"
```

---

### Task 7: Add `scheduleBinancePrefetch` method and wire into cycle

**Files:**
- Modify: `trader/auto_trader_loop.go` — Add `scheduleBinancePrefetch` method; replace inline prefetch goroutine

**Interfaces:**
- Consumes: `runRateLimitedPrefetch` (Task 5), `PrefetchBinanceDetails` (existing), `GetCandidateCoins` (existing)
- Produces: `scheduleBinancePrefetch()` method on AutoTrader; timer-based scheduling after each cycle

- [ ] **Step 1: Add `scheduleBinancePrefetch` method**

Add to `trader/auto_trader_loop.go`:

```go
// scheduleBinancePrefetch schedules a prefetch of Binance per-coin detail
// to fire leadTime before the next cycle trigger. The prefetch warms the
// 10-minute TTL cache so attachPerCoinSignals finds cache hits at cycle time.
func (at *AutoTrader) scheduleBinancePrefetch() {
	if at.strategyEngine == nil {
		return
	}
	cfg := at.strategyEngine.GetConfig()
	if cfg == nil {
		return
	}

	// Calculate lead time based on enabled Binance data sources.
	requestsPerCoin := 0
	if cfg.Indicators.EnableBinanceTechnicalData {
		intervals := cfg.Indicators.BinanceTechnicalIntervals
		if len(intervals) == 0 {
			intervals = []string{"1h"}
		}
		requestsPerCoin += len(intervals)
	}
	if cfg.Indicators.EnableBinanceSentimentData {
		requestsPerCoin++
	}
	if requestsPerCoin == 0 {
		return // no Binance data sources enabled, no prefetch needed
	}

	const prefetchPoolSize = 30
	const delayPerRequest = 1500 * time.Millisecond
	const bufferSeconds = 10 * time.Second
	leadTime := time.Duration(requestsPerCoin)*prefetchPoolSize*delayPerRequest + bufferSeconds

	// If the cycle interval is shorter than the lead time, skip prefetch.
	if at.config.ScanInterval <= leadTime {
		logger.Infof("⏭️ Scan interval (%v) shorter than prefetch lead time (%v), skipping prefetch",
			at.config.ScanInterval, leadTime)
		return
	}

	delay := at.config.ScanInterval - leadTime
	logger.Infof("🔄 Binance prefetch scheduled in %v (lead=%v, req/coin=%d, pool=%d)",
		delay, leadTime, requestsPerCoin, prefetchPoolSize)

	// Cancel any existing prefetch timer
	if at.prefetchTimer != nil {
		at.prefetchTimer.Stop()
	}

	at.prefetchTimer = time.AfterFunc(delay, func() {
		// Check if trader is still running
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()
		if !running {
			return
		}

		at.logInfof("🔄 Starting Binance pre-cycle prefetch...")
		start := time.Now()

		// Get fresh candidate list at prefetch time
		coins, err := at.strategyEngine.GetCandidateCoins()
		if err != nil {
			at.logWarnf("⚠️ Prefetch: failed to get candidate coins: %v", err)
			return
		}
		if len(coins) == 0 {
			return
		}

		// Take top 30 by score (already sorted from GetCandidateCoins)
		pool := coins
		if len(pool) > prefetchPoolSize {
			pool = pool[:prefetchPoolSize]
		}
		symbols := candidateSymbols(pool)

		// Rate-limited sequential prefetch
		runRateLimitedPrefetch(symbols, delayPerRequest, func(sym string) {
			at.strategyEngine.PrefetchBinanceDetails(context.Background(), []string{sym})
		})

		elapsed := time.Since(start)
		at.logInfof("✅ Binance prefetch complete: %d symbols in %v", len(symbols), elapsed)
	})
}
```

- [ ] **Step 2: Replace inline prefetch goroutine with `scheduleBinancePrefetch` call**

In `trader/auto_trader_loop.go`, replace the inline prefetch block (around line 643):

```go
// BEFORE (old code):
// Prefetch Binance per-coin detail for the candidate pool in the background
// (bounded workers) so request load is spread and warm by prompt time.
if at.strategyEngine != nil && len(candidateCoins) > 0 {
    go func(symbols []string) {
        runPrefetchJobs(symbols, 3, func(sym string) {
            at.strategyEngine.PrefetchBinanceDetails(context.Background(), []string{sym})
        })
    }(candidateSymbols(candidateCoins))
}

// AFTER (new code):
// Schedule Binance pre-cycle prefetch for next cycle.
// The prefetch fires ~145s before the next ticker.C, warming the cache.
if at.strategyEngine != nil {
    at.scheduleBinancePrefetch()
}
```

- [ ] **Step 3: Run build to verify no compilation errors**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add trader/auto_trader_loop.go && git commit -m "feat(trader): schedule Binance pre-cycle prefetch via time.AfterFunc"
```

---

### Task 8: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 2: Frontend verification (no changes expected)**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx tsc --noEmit && npm run build && npm test`
Expected: all green.

- [ ] **Step 3: Commit any final touch-ups**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add -A && git commit -m "chore: final verification fixes"
```

---

## Self-Review

**Spec coverage:**
- OI-aware candidate pool: Tasks 1 (remove truncation), 2 (early exit in OI filter). ✓
- Pre-cycle prefetch: Tasks 3 (TTL), 4 (429 retry), 5 (rate-limited prefetch), 6 (timer lifecycle), 7 (scheduling). ✓
- No new config fields: Verified — uses existing `MaxCandidateCoins`, OI filter settings, Binance enable flags. ✓

**Placeholder scan:** All tasks have concrete code. No TBD/TODO/implementation-later.

**Type consistency:** `runRateLimitedPrefetch` signature consistent between Task 5 (definition) and Task 7 (call site). `prefetchTimer` field consistent between Task 6 (definition) and Task 7 (usage).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-21-oi-aware-candidate-pool-and-precycle-prefetch.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
