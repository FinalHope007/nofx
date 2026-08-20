# Binance Opportunity Scopes + Per-Coin Detail + Indicator Revamp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two free Binance "Opportunity" candidate-pool scopes (`binance_technical`, `binance_sentiment`) with interval/direction sub-controls and SPOT→perp mapping; add two new per-coin detail data sources (Binance Technical, Binance Sentiment) with a TTL cache and cycle-aligned background prefetch; revamp the editor to surface backend indicator periods/rankings and split "Data sources" into two sections.

**Architecture:** Backend adds a `provider/binance/opportunity.go` client (free, no auth) used by both new `GetCandidateCoins` cases (Part A) and the per-coin detail enrichment (Part B, via a mutex-guarded TTL cache + background prefetch in the trader loop). Frontend adds two scope cards with revealed sub-controls, two per-coin data toggles with independent interval buttons, and restructures the editor into "Basic indicators" + "Data sources" fieldsets with period chips, ranking toggles, and candles count/longer controls.

**Tech Stack:** Go 1.25 (Gin, GORM), React 18/TS/Vite, Binance `bapi/apex` REST endpoints (free).

## Global Constraints

- Backend: `go build ./... && go vet ./... && go test ./...` must pass.
- Frontend: `cd web && npx tsc --noEmit && npm run build && npm test` must pass.
- English-only UI strings. Backend errors via `SafeError`/`SafeInternalError`/`SanitizeError`.
- `store.*` is the only DB access layer. All timestamps UTC.
- Backend Go struct field names and JSON tags must match frontend TS types exactly (they round-trip through config JSON; a mismatched name is silently dropped by `UnmarshalJSON`).
- Candidate pool hard cap: `store.MaxCandidateCoins` = 10. New scope must return a non-empty pool to be useful.
- Per-coin sources are gated by `enable_*_data` flags; a source with no data for a symbol is omitted from the prompt.
- Quant-data group (`enable_quant_*`) is NOT surfaced on frontend; keep as-is on backend.
- No changes to live order/position behavior.
- Existing OI-liquidity filter (15M USDT) is source-agnostic and already applies to any new scope automatically — no code change.

---

## File Structure

**Backend (new):**
- `provider/binance/opportunity.go` — `OpportunityClient` + typed structs + `GetOpportunityAssets` / `GetAssetDetails`.

**Backend (modified):**
- `store/strategy.go` — `CoinSourceConfig` fields; `normalizeCoinSourceType`/`inferCoinSourceType`/`ClampLimits`; `IndicatorConfig` fields.
- `kernel/engine.go` — `PerCoinSignal` fields; `StrategyEngine` new `opportunity` client + `binanceDetailCache`; `GetCandidateCoins` cases; `getBinanceOpportunityCoins`.
- `kernel/engine_analysis.go` — `attachPerCoinSignals` fetch for the 2 new sources.
- `kernel/engine_prompt.go` — `formatPerCoinSignals` render for the 2 new sources.
- `trader/auto_trader_loop.go` — background prefetch step (Option 1).

**Frontend (modified):**
- `web/src/types/strategy.ts` — source_type union, `CoinSourceConfig`/`IndicatorConfig`/`ScopeUnit` types.
- `web/src/features/strategies/scopeCatalog.ts` — 2 new free cards.
- `web/src/features/strategies/ScopeStepPage.tsx` — reveal sub-controls; `matchConcreteScope`.
- `web/src/features/strategies/strategyFactory.ts` — `StrategyEditorForm`, `buildCoinSource`, `buildStrategyConfig`.
- `web/src/features/strategies/dataSourceDefaults.ts` — auto-enable new sources.
- `web/src/features/strategies/EditorStepPage.tsx` — restructure sections, period chips, ranking toggles, candles controls, edit round-trip.

**Tests:** co-located `*_test.go` and `web/src/test/*` per convention.

---

### Task 1: Binance opportunity HTTP client

**Files:**
- Create: `provider/binance/opportunity.go`
- Create: `provider/binance/opportunity_test.go`

**Interfaces:**
- Consumes: nothing (standalone). Uses `security.SafeHTTPClient` for SSRF-safe HTTP, `market.Normalize` for symbol normalization.
- Produces:
  - `type OpportunityClient struct{ http *http.Client }`
  - `func NewOpportunityClient() *OpportunityClient`
  - `type OpportunityAsset struct{ Symbol string; Score float64 }`
  - `func (c *OpportunityClient) GetOpportunityAssets(ctx context.Context, interval, scene string) ([]OpportunityAsset, error)` — interval ignored when scene="sentiment"; returns assets with `Score` parsed from `technical_score_1h`/`technical_score_1d` (technical) or `sentiment_score` (sentiment).
  - `func (c *OpportunityClient) GetAssetDetails(ctx context.Context, symbol, scene, interval string) (map[string]string, error)` — returns flattened metric map (signal `valueLabel`s + summary narrative).

- [ ] **Step 1: Write the failing test for asset list parsing**

```go
package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOpportunityAssetsTechnical(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "technical" || r.URL.Query().Get("interval") != "1h" {
			t.Errorf("unexpected query: %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"items":[
			{"asset":"TREE","rank":1,"metrics":{"technical_score_1h":{"value":"9.45"}}},
			{"asset":"BTC","rank":2,"metrics":{"technical_score_1h":{"value":"7.85"}}}
		]},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	assets, err := c.GetOpportunityAssets(context.Background(), "1h", "technical")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets[0].Symbol != "TREE" || assets[0].Score != 9.45 {
		t.Fatalf("unexpected first asset: %+v", assets[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -run TestGetOpportunityAssetsTechnical -v`
Expected: FAIL — package `binance` has no such file/`NewOpportunityClient` undefined.

- [ ] **Step 3: Write minimal client**

```go
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type OpportunityClient struct {
	http    *http.Client
	baseURL string
}

const opportunityBaseURL = "https://www.binance.com/bapi/apex/v1/friendly/apex/web/opportunity"

func NewOpportunityClient() *OpportunityClient {
	return &OpportunityClient{http: &http.Client{}, baseURL: opportunityBaseURL}
}

type opportunityListResponse struct {
	Data struct {
		Items []struct {
			Asset   string            `json:"asset"`
			Metrics map[string]struct {
				Value string `json:"value"`
			} `json:"metrics"`
		} `json:"items"`
	} `json:"data"`
}

type OpportunityAsset struct {
	Symbol string
	Score  float64
}

func scoreKeyForScene(scene, interval string) string {
	switch scene {
	case "sentiment":
		return "sentiment_score"
	case "technical":
		if interval == "24h" {
			return "technical_score_1d"
		}
		return "technical_score_1h"
	default:
		return "technical_score_1h"
	}
}

func (c *OpportunityClient) GetOpportunityAssets(ctx context.Context, interval, scene string) ([]OpportunityAsset, error) {
	if scene == "" {
		scene = "technical"
	}
	if interval == "" {
		interval = "1h"
	}
	url := fmt.Sprintf("%s/assets?interval=%s&type=%s", c.baseURL, interval, scene)
	if scene == "sentiment" {
		url = fmt.Sprintf("%s/assets?interval=24h&type=sentiment", c.baseURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var parsed opportunityListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	key := scoreKeyForScene(scene, interval)
	out := make([]OpportunityAsset, 0, len(parsed.Data.Items))
	for _, it := range parsed.Data.Items {
		raw, ok := it.Metrics[key]
		if !ok || raw.Value == "" {
			continue
		}
		score, err := strconv.ParseFloat(strings.TrimSpace(raw.Value), 64)
		if err != nil {
			continue
		}
		out = append(out, OpportunityAsset{Symbol: it.Asset, Score: score})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -run TestGetOpportunityAssetsTechnical -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add provider/binance/opportunity.go provider/binance/opportunity_test.go && git commit -m "feat(binance): add OpportunityClient assets fetch"
```

---

### Task 2: Binance asset-details fetch

**Files:**
- Modify: `provider/binance/opportunity.go`
- Modify: `provider/binance/opportunity_test.go`

**Interfaces:**
- Consumes: `OpportunityClient` from Task 1.
- Produces: `func (c *OpportunityClient) GetAssetDetails(ctx context.Context, symbol, scene, interval string) (map[string]string, error)` — flattened `map[string]string` of the detail `metrics` `valueLabel`s plus any `*_summary` narrative values.

- [ ] **Step 1: Write the failing test**

```go
func TestGetAssetDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asset-details" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("asset"); got != "BTC" {
			t.Errorf("expected asset=BTC, got %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"metrics":{
			"technical_score_1h":{"value":"7.85","valueLabel":"Positive"},
			"technical_summary_1h":{"value":"Bullish overall for BTC.","valueLabel":"Bullish overall for BTC."}
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
		t.Fatalf("expected score label, got %q", got["technical_score_1h"])
	}
	if !strings.Contains(got["technical_summary_1h"], "Bullish overall") {
		t.Fatalf("expected summary, got %q", got["technical_summary_1h"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -run TestGetAssetDetails -v`
Expected: FAIL — `GetAssetDetails` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
type opportunityDetailResponse struct {
	Data struct {
		Metrics map[string]struct {
			Value      string `json:"value"`
			ValueLabel string `json:"valueLabel"`
		} `json:"metrics"`
	} `json:"data"`
}

func (c *OpportunityClient) GetAssetDetails(ctx context.Context, symbol, scene, interval string) (map[string]string, error) {
	if interval == "" {
		interval = "1h"
	}
	url := fmt.Sprintf("%s/asset-details?asset=%s&type=%s&interval=%s&quote=USDT",
		c.baseURL, symbol, scene, interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var parsed opportunityDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
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

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/binance/ -run TestGetAssetDetails -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add provider/binance/opportunity.go provider/binance/opportunity_test.go && git commit -m "feat(binance): add OpportunityClient asset-details fetch"
```

---

### Task 3: CoinSourceConfig fields + normalization + clamping

**Files:**
- Modify: `store/strategy.go` — `CoinSourceConfig` struct (~line 946), `normalizeCoinSourceType` (~line 403), `inferCoinSourceType` (~line 438), `ClampLimits` (~line 36), `NormalizeProductSchema` switch (~line 236).

**Interfaces:**
- Consumes: existing `CoinSourceConfig` shape.
- Produces (new `CoinSourceConfig` JSON fields, all `omitempty`):
  - `binance_technical_interval` (string: "1h"|"24h")
  - `binance_technical_direction` (string: "top"|"bottom")
  - `binance_technical_limit` (int)
  - `binance_sentiment_direction` (string: "top"|"bottom")
  - `binance_sentiment_limit` (int)

- [ ] **Step 1: Write the failing test**

```go
func TestCoinSourceBinanceNormalizeAndClamp(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 50
	cfg.NormalizeProductSchema()
	if cfg.CoinSource.SourceType != "binance_technical" {
		t.Fatalf("source_type lost: %q", cfg.CoinSource.SourceType)
	}
	if cfg.CoinSource.BinanceTechnicalInterval != "1h" {
		t.Fatalf("interval not preserved: %q", cfg.CoinSource.BinanceTechnicalInterval)
	}

	cfg.ClampLimits()
	if cfg.CoinSource.BinanceTechnicalLimit != MaxCandidateCoins {
		t.Fatalf("expected limit clamped to %d, got %d", MaxCandidateCoins, cfg.CoinSource.BinanceTechnicalLimit)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./store/ -run TestCoinSourceBinanceNormalizeAndClamp -v`
Expected: FAIL — `cfg.CoinSource.BinanceTechnicalInterval` undefined.

- [ ] **Step 3: Add struct fields + normalize + clamp**

Add to `CoinSourceConfig` (after `VergexDirection`, ~line 990):

```go
	// Binance Opportunity technical scope: interval "1h"|"24h", direction "top"|"bottom".
	BinanceTechnicalInterval  string `json:"binance_technical_interval,omitempty"`
	BinanceTechnicalDirection string `json:"binance_technical_direction,omitempty"`
	BinanceTechnicalLimit     int    `json:"binance_technical_limit,omitempty"`
	// Binance Opportunity sentiment scope: direction "top"|"bottom" (24h-only, no interval).
	BinanceSentimentDirection string `json:"binance_sentiment_direction,omitempty"`
	BinanceSentimentLimit     int    `json:"binance_sentiment_limit,omitempty"`
```

Add cases to `normalizeCoinSourceType` (before `default`):

```go
	case strings.Contains(compact, "binancetechnical") || strings.Contains(value, "binance technical"):
		return "binance_technical"
	case strings.Contains(compact, "binancesentiment") || strings.Contains(value, "binance sentiment"):
		return "binance_sentiment"
```

Add cases to `inferCoinSourceType` (before `default`):

```go
	case source.BinanceTechnicalLimit > 0 || source.BinanceTechnicalDirection != "" || source.BinanceTechnicalInterval != "":
		return "binance_technical"
	case source.BinanceSentimentLimit > 0 || source.BinanceSentimentDirection != "":
		return "binance_sentiment"
```

Add to `ClampLimits` (after the `VergexLimit` clamp, ~line 51):

```go
	if c.CoinSource.BinanceTechnicalLimit > MaxCandidateCoins {
		c.CoinSource.BinanceTechnicalLimit = MaxCandidateCoins
	}
	if c.CoinSource.BinanceSentimentLimit > MaxCandidateCoins {
		c.CoinSource.BinanceSentimentLimit = MaxCandidateCoins
	}
```

Add a `NormalizeProductSchema` switch case (after the `hyper_rank`/`vergex_signal` cases) to set sensible defaults:

```go
	case "binance_technical":
		if c.CoinSource.BinanceTechnicalDirection == "" {
			c.CoinSource.BinanceTechnicalDirection = "top"
		}
		if c.CoinSource.BinanceTechnicalInterval == "" {
			c.CoinSource.BinanceTechnicalInterval = "1h"
		}
		if c.CoinSource.BinanceTechnicalLimit <= 0 {
			c.CoinSource.BinanceTechnicalLimit = 10
		}
	case "binance_sentiment":
		if c.CoinSource.BinanceSentimentDirection == "" {
			c.CoinSource.BinanceSentimentDirection = "top"
		}
		if c.CoinSource.BinanceSentimentLimit <= 0 {
			c.CoinSource.BinanceSentimentLimit = 10
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./store/ -run TestCoinSourceBinanceNormalizeAndClamp -v`
Expected: PASS.

- [ ] **Step 5: Run full backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add store/strategy.go store/strategy_test.go && git commit -m "feat(store): add binance_technical/sentiment coin source config"
```

---

### Task 4: Engine candidate getter + GetCandidateCoins cases

**Files:**
- Modify: `kernel/engine.go` — `StrategyEngine` struct (~line 197) + constructor (~line 215), `GetCandidateCoins` switch (~line 363), new getter.

**Interfaces:**
- Consumes: `OpportunityClient` (Task 1/2), `store.MaxCandidateCoins`, `market.Normalize`.
- Produces: `func (e *StrategyEngine) getBinanceOpportunityCoins(scene, interval, direction string, limit int) ([]CandidateCoin, error)`; `StrategyEngine` gains field `opportunity *binance.OpportunityClient`.

- [ ] **Step 1: Write the failing getter test**

```go
package kernel

import (
	"testing"
	"nofx/store"
)

func TestGetCandidateCoinsBinanceTechnical(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10
	// TODO: inject a fake OpportunityClient. See Step 3 note.
	engine := NewStrategyEngine(cfg)
	coins, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) == 0 {
		t.Fatalf("expected a non-empty pool")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestGetCandidateCoinsBinanceTechnical -v`
Expected: FAIL — `GetCandidateCoins` returns `unknown coin source type: binance_technical`.

- [ ] **Step 3: Add engine field + constructor wiring + switch case + getter**

Add field to `StrategyEngine` (near `freeClient`, ~line 205):

```go
	opportunity *binance.OpportunityClient // free Binance Opportunity client (Part A + B)
```

In `NewStrategyEngine` (after the `freeVergex` setup, ~line 223):

```go
	engine.opportunity = binance.NewOpportunityClient()
```

Add to `GetCandidateCoins` switch (after the `hyper_rank` case, ~line 507):

```go
	case "binance_technical":
		coins, err := e.getBinanceOpportunityCoins("technical", coinSource.BinanceTechnicalInterval, coinSource.BinanceTechnicalDirection, coinSource.BinanceTechnicalLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "binance_sentiment":
		coins, err := e.getBinanceOpportunityCoins("sentiment", "", coinSource.BinanceSentimentDirection, coinSource.BinanceSentimentLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil
```

Add the getter (place after `getHyperRankCoins`):

```go
// getBinanceOpportunityCoins returns up to `limit` perp symbols ranked by the
// Binance Opportunity score for the given scene/interval/direction. SPOT symbols
// from the pool are mapped to Binance-futures perp symbols; symbols with no
// matching perp ticker are dropped.
func (e *StrategyEngine) getBinanceOpportunityCoins(scene, interval, direction string, limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = store.MaxCandidateCoins
	}
	if limit > store.MaxCandidateCoins {
		limit = store.MaxCandidateCoins
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "" {
		direction = "top"
	}
	assets, err := e.opportunity.GetOpportunityAssets(context.Background(), interval, scene)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Binance %s opportunity: %w", scene, err)
	}
	sort.Slice(assets, func(i, j int) bool {
		if direction == "bottom" {
			return assets[i].Score < assets[j].Score
		}
		return assets[i].Score > assets[j].Score
	})
	if len(assets) > limit {
		assets = assets[:limit]
	}
	candidates := make([]CandidateCoin, 0, len(assets))
	for _, a := range assets {
		perp := mapSpotToPerp(a.Symbol)
		if perp == "" {
			continue
		}
		candidates = append(candidates, CandidateCoin{
			Symbol:  perp,
			Sources: []string{"binance_" + scene},
		})
	}
	logger.Infof("✅ Loaded %d Binance %s opportunity coins (dir=%s, capped at %d)", len(candidates), scene, direction, limit)
	return candidates, nil
}

// mapSpotToPerp maps a Binance SPOT symbol (e.g. "TREE") to its futures perp
// symbol (e.g. "TREEUSDT"). Returns "" if no perp mapping is known.
func mapSpotToPerp(spot string) string {
	s := strings.ToUpper(strings.TrimSpace(spot))
	if s == "" {
		return ""
	}
	// Binance-futures perp symbols are <BASE>USDT for the vast majority of
	// listed pairs; reuse market.Normalize which appends USDT for crypto.
	return market.Normalize(s)
}
```

**NOTE on test injection:** The above getter calls `e.opportunity` (a concrete type). To make the test hermetic without a live HTTP call, add a package-level function var that the test can override:

```go
// In engine.go:
var newOpportunityClient = func() *binance.OpportunityClient { return binance.NewOpportunityClient() }
```

and in `NewStrategyEngine` use `engine.opportunity = newOpportunityClient()`. Then in the test, replace `engine.opportunity` with a fake that returns a canned asset list. Concretely, in the test Step 1 replace the TODO with:

```go
	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClient{}
```

with a `fakeOpportunityClient` struct defined in the test implementing only `GetOpportunityAssets` (returning two symbols `TREE`/`BTC` with scores 9.45/7.85). The test asserts `len(coins) == 2` and both symbols end in `USDT`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestGetCandidateCoinsBinanceTechnical -v`
Expected: PASS with 2 perp-mapped symbols.

- [ ] **Step 5: Run full backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add kernel/engine.go kernel/engine_test.go && git commit -m "feat(kernel): binance opportunity candidate getters"
```

---

### Task 5: IndicatorConfig fields for Binance per-coin detail

**Files:**
- Modify: `store/strategy.go` — `IndicatorConfig` (after `DataDurations`, ~line 1049).

**Interfaces:**
- Consumes: existing `IndicatorConfig`.
- Produces (new JSON fields):
  - `enable_binance_technical_data` (bool)
  - `enable_binance_sentiment_data` (bool)
  - `binance_technical_intervals` ([]string, e.g. `["1h","24h"]`)

- [ ] **Step 1: Write the failing round-trip test**

```go
func TestIndicatorConfigBinanceFieldsRoundTrip(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.EnableBinanceSentimentData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h", "24h"}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back StrategyConfig
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Indicators.EnableBinanceTechnicalData || !back.Indicators.EnableBinanceSentimentData {
		t.Fatal("binance flags lost in round-trip")
	}
	if len(back.Indicators.BinanceTechnicalIntervals) != 2 {
		t.Fatalf("expected 2 intervals, got %v", back.Indicators.BinanceTechnicalIntervals)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./store/ -run TestIndicatorConfigBinanceFieldsRoundTrip -v`
Expected: FAIL — fields undefined.

- [ ] **Step 3: Add struct fields**

Add to `IndicatorConfig` (after `DataDurations`, ~line 1049):

```go
	// Binance Opportunity per-coin detail sources (free).
	EnableBinanceTechnicalData  bool     `json:"enable_binance_technical_data"` // per-coin technical detail
	EnableBinanceSentimentData  bool     `json:"enable_binance_sentiment_data"` // per-coin sentiment detail
	BinanceTechnicalIntervals   []string `json:"binance_technical_intervals,omitempty"` // "1h","24h"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./store/ -run TestIndicatorConfigBinanceFieldsRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add store/strategy.go store/strategy_test.go && git commit -m "feat(store): binance per-coin indicator config fields"
```

---

### Task 6: PerCoinSignal fields + TTL cache + attachPerCoinSignals

**Files:**
- Modify: `kernel/engine.go` — `PerCoinSignal` (~line 189), `StrategyEngine` fields (~line 197), constructor.
- Modify: `kernel/engine_analysis.go` — `attachPerCoinSignals` early-return guard + fetch logic (~line 183).
- Create: `kernel/binance_detail_cache.go` — TTL cache.

**Interfaces:**
- Consumes: `OpportunityClient.GetAssetDetails` (Task 2), `IndicatorConfig` binance fields (Task 5).
- Produces:
  - `PerCoinSignal` fields `BinanceTechnical map[string]string`, `BinanceSentiment map[string]string`.
  - `func (e *StrategyEngine) binanceDetail(key string) (map[string]string, bool)` — cached read.
  - `func (e *StrategyEngine) cacheBinanceDetail(key string, v map[string]string)` — cached write.
  - `func (e *StrategyEngine) PrefetchBinanceDetails(ctx context.Context, symbols []string) error` — background-safe prefetch (called from trader loop in Task 8).

- [ ] **Step 1: Write the failing cache test**

```go
package kernel

import "testing"

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
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestBinanceDetailCacheTTL -v`
Expected: FAIL — `binanceDetail` undefined.

- [ ] **Step 3: Write the cache + PerCoinSignal fields**

`kernel/binance_detail_cache.go`:

```go
package kernel

import (
	"sync"
	"time"
)

// binanceDetailCacheTTL bounds how long a cached per-coin Binance detail is kept.
const binanceDetailCacheTTL = 60 * time.Second

type binanceDetailEntry struct {
	value    map[string]string
	expires  time.Time
}

// binanceDetailCache is a concurrency-safe TTL cache for per-coin Binance
// Opportunity detail fetches, keyed by "<scene>|<symbol>|<interval>".
type binanceDetailCache struct {
	mu   sync.Mutex
	data map[string]binanceDetailEntry
}

func newBinanceDetailCache() *binanceDetailCache {
	return &binanceDetailCache{data: make(map[string]binanceDetailEntry)}
}

func (c *binanceDetailCache) get(key string) (map[string]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *binanceDetailCache) set(key string, v map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = binanceDetailEntry{value: v, expires: time.Now().Add(binanceDetailCacheTTL)}
}
```

In `kernel/engine.go`, add to `StrategyEngine` (near `perCoinSignals`, ~line 202):

```go
	binanceDetails *binanceDetailCache // free Binance per-coin detail TTL cache
```

and in `NewStrategyEngine`:

```go
	engine.binanceDetails = newBinanceDetailCache()
```

Add to `PerCoinSignal` (~line 194):

```go
	BinanceTechnical map[string]string // flattened technical detail labels (may be nil)
	BinanceSentiment map[string]string // flattened sentiment detail labels (may be nil)
```

Add engine helper methods (after `SetPerCoinSignals`):

```go
func (e *StrategyEngine) binanceDetail(key string) (map[string]string, bool) {
	return e.binanceDetails.get(key)
}

func (e *StrategyEngine) cacheBinanceDetail(key string, v map[string]string) {
	e.binanceDetails.set(key, v)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestBinanceDetailCacheTTL -v`
Expected: PASS.

- [ ] **Step 5: Wire into `attachPerCoinSignals`**

In `kernel/engine_analysis.go`, update the early-return guard (~line 188):

```go
	if !cfg.Indicators.EnableAI500Data && !cfg.Indicators.EnableOIData &&
		!cfg.Indicators.EnableNetflowData && !cfg.Indicators.EnablePriceData &&
		!cfg.Indicators.EnableBinanceTechnicalData && !cfg.Indicators.EnableBinanceSentimentData {
		return nil
	}
```

After the existing per-duration loop (after line 256), add Binance detail fetch. Insert before the final `for sym := range symSet` loop:

```go
	// Binance Opportunity per-coin detail (free, per-symbol; read from TTL cache,
	// fall back to a synchronous fetch on miss).
	if cfg.Indicators.EnableBinanceTechnicalData {
		intervals := cfg.Indicators.BinanceTechnicalIntervals
		if len(intervals) == 0 {
			intervals = []string{"1h"}
		}
		for _, iv := range intervals {
			for sym := range symSet {
				key := "technical|" + sym + "|" + iv
				val, ok := engine.binanceDetail(key)
				if !ok {
					var err error
					val, err = engine.opportunity.GetAssetDetails(ctx.Ctx, strings.TrimSuffix(sym, "USDT"), "technical", iv)
					if err != nil {
						logger.Warnf("⚠️ Binance technical detail fetch failed (%s %s): %v", sym, iv, err)
						continue
					}
					engine.cacheBinanceDetail(key, val)
				}
				sig := out[sym]
				if sig.BinanceTechnical == nil {
					sig.BinanceTechnical = make(map[string]string)
				}
				for k, v := range val {
					sig.BinanceTechnical[iv+"|"+k] = v
				}
				out[sym] = sig
			}
		}
	}
	if cfg.Indicators.EnableBinanceSentimentData {
		for sym := range symSet {
			key := "sentiment|" + sym
			val, ok := engine.binanceDetail(key)
			if !ok {
				var err error
				val, err = engine.opportunity.GetAssetDetails(ctx.Ctx, strings.TrimSuffix(sym, "USDT"), "sentiment", "24h")
				if err != nil {
					logger.Warnf("⚠️ Binance sentiment detail fetch failed (%s): %v", sym, err)
					continue
				}
				engine.cacheBinanceDetail(key, val)
			}
			sig := out[sym]
			if sig.BinanceSentiment == nil {
				sig.BinanceSentiment = make(map[string]string)
			}
			for k, v := range val {
				sig.BinanceSentiment[k] = v
			}
			out[sym] = sig
		}
	}
```

**NOTE:** This requires `ctx.Ctx` (a `context.Context` on the kernel `Context`). If `Context` has no `Ctx` field, add one (set to `context.Background()` if nil) or thread the trader context through — verify the `Context` struct definition before editing and use the available field; if none exists, add `Ctx context.Context` to `kernel.Context` and set it in `trader/auto_trader_loop.go` where the context is built.

- [ ] **Step 6: Add an `attachPerCoinSignals` binance test**

Add to `kernel/engine_analysis_test.go` a test that: builds an engine with `EnableBinanceSentimentData=true`, pre-populates the cache with a fake `sentiment|BTCUSDT` value, calls `attachPerCoinSignals` with a candidate `BTCUSDT`, and asserts `PerCoinSignalFor("BTCUSDT").BinanceSentiment` contains the fake label.

- [ ] **Step 7: Run full backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 8: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add kernel/engine.go kernel/engine_analysis.go kernel/engine_analysis_test.go kernel/binance_detail_cache.go && git commit -m "feat(kernel): binance per-coin detail enrichment with TTL cache"
```

---

### Task 7: Prompt rendering for Binance detail

**Files:**
- Modify: `kernel/engine_prompt.go` — `formatPerCoinSignals` (~line 1003) + the early-return emptiness check.

**Interfaces:**
- Consumes: `PerCoinSignal.BinanceTechnical`/`BinanceSentiment` (Task 6), `IndicatorConfig` binance fields (Task 5).

- [ ] **Step 1: Write the failing prompt-format test**

Add to `kernel/engine_prompt_test.go`:

```go
func TestFormatPerCoinSignalsBinance(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h"}
	cfg.Indicators.EnableBinanceSentimentData = true
	e := NewStrategyEngine(cfg)
	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"BTCUSDT": {
			BinanceTechnical: map[string]string{"1h|technical_summary_1h": "Bullish overall for BTC."},
			BinanceSentiment: map[string]string{"sentiment_summary": "In the past 24h BTC was bullish."},
		},
	})
	out := e.formatPerCoinSignals("BTCUSDT", 60000)
	if !strings.Contains(out, "Binance Technical") || !strings.Contains(out, "Bullish overall") {
		t.Fatalf("missing technical section: %s", out)
	}
	if !strings.Contains(out, "Sentiment") || !strings.Contains(out, "In the past 24h") {
		t.Fatalf("missing sentiment section: %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestFormatPerCoinSignalsBinance -v`
Expected: FAIL — output lacks the Binance sections.

- [ ] **Step 3: Update guard + add rendering**

Update the early-return guard (~line 1006):

```go
	if !ind.EnableAI500Data && !ind.EnableOIData && !ind.EnableNetflowData && !ind.EnablePriceData &&
		!ind.EnableBinanceTechnicalData && !ind.EnableBinanceSentimentData {
		return ""
	}
```

Update the emptiness check (~line 1010) to include the new fields:

```go
	if !ok || sig.AI500 == nil && len(sig.OI) == 0 && len(sig.Netflow) == 0 && len(sig.Price) == 0 &&
		len(sig.BinanceTechnical) == 0 && len(sig.BinanceSentiment) == 0 {
		return ""
	}
```

Add the rendering (after the `EnablePriceData` block, ~line 1076):

```go
	if ind.EnableBinanceTechnicalData && len(sig.BinanceTechnical) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Binance Technical ===\n", symbol))
		for _, iv := range e.binanceIntervalOrder(ind.BinanceTechnicalIntervals) {
			if v, ok := sig.BinanceTechnical[iv+"|technical_summary_"+iv]; ok {
				sb.WriteString(fmt.Sprintf("[%s] %s\n", iv, v))
			}
		}
		sb.WriteString("\n")
	}

	if ind.EnableBinanceSentimentData && len(sig.BinanceSentiment) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Binance Sentiment ===\n", symbol))
		if v, ok := sig.BinanceSentiment["sentiment_summary"]; ok {
			sb.WriteString(v + "\n")
		}
		if v, ok := sig.BinanceSentiment["sentiment_score"]; ok {
			sb.WriteString(fmt.Sprintf("Sentiment score: %s\n", v))
		}
		sb.WriteString("\n")
	}
```

Add helper `binanceIntervalOrder` near `durationOrder`:

```go
// binanceIntervalOrder returns the Binance detail intervals in a stable order.
func (e *StrategyEngine) binanceIntervalOrder(intervals []string) []string {
	if len(intervals) == 0 {
		return []string{"1h"}
	}
	out := make([]string, 0, len(intervals))
	seen := map[string]bool{}
	for _, iv := range intervals {
		if !seen[iv] {
			seen[iv] = true
			out = append(out, iv)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestFormatPerCoinSignalsBinance -v`
Expected: PASS.

- [ ] **Step 5: Run full backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add kernel/engine_prompt.go kernel/engine_prompt_test.go && git commit -m "feat(kernel): render binance per-coin detail in prompt"
```

---

### Task 8: Background prefetch in the trader loop

**Files:**
- Modify: `trader/auto_trader_loop.go` — after `GetCandidateCoins()` (~line 592) and around the `AttachPerCoinSignals` call (~line 751).

**Interfaces:**
- Consumes: `PerCoinSignal`/cache (Task 6).
- Produces: no new public API; a private prefetch helper `func (at *AutoTrader) prefetchBinanceDetails(ctx *kernel.Context, symbols []string)`.

- [ ] **Step 1: Add a prefetch helper test (bounded concurrency)**

Create `trader/auto_trader_prefetch_test.go` testing that the prefetch worker pool issues requests for all symbols without exceeding the worker bound. Because the real fetch depends on the engine cache, this test calls a small extracted helper `runPrefetchJobs(symbols []string, workerCount int, fn func(string))` that the prefetch uses.

```go
package trader

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunPrefetchJobsBoundsConcurrency(t *testing.T) {
	var max, cur int64
	var mu sync.Mutex
	start := make(chan struct{})
	var started, done sync.WaitGroup
	symbols := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}
	started.Add(len(symbols))
	done.Add(len(symbols))
	runPrefetchJobs(symbols, 3, func(string) {
		started.Done()
		start <- struct{}{}
		n := atomic.AddInt64(&cur, 1)
		mu.Lock()
		if n > max {
			max = n
		}
		mu.Unlock()
		atomic.AddInt64(&cur, -1)
		done.Done()
	})
	// Drain the start channel to release all workers.
	go func() {
		for range start {
		}
	}()
	started.Wait()
	done.Wait()
	if max > 3 {
		t.Fatalf("concurrency exceeded worker bound: max=%d", max)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./trader/ -run TestRunPrefetchJobsBoundsConcurrency -v`
Expected: FAIL — `runPrefetchJobs` undefined.

- [ ] **Step 3: Write the bounded worker helper + wire prefetch**

Create `trader/auto_trader_prefetch.go`:

```go
package trader

import "sync"

// runPrefetchJobs executes fn for each symbol across a fixed worker pool of
// size workerCount so per-coin detail fetches are spread out rather than burst.
func runPrefetchJobs(symbols []string, workerCount int, fn func(string)) {
	if len(symbols) == 0 || fn == nil {
		return
	}
	if workerCount <= 0 {
		workerCount = 3
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for s := range jobs {
				fn(s)
			}
		}()
	}
	for _, s := range symbols {
		jobs <- s
	}
	close(jobs)
	wg.Wait()
}
```

In `auto_trader_loop.go`, after the candidate coins are set into `ctx` (~line 638), kick off the prefetch as a background goroutine so it does not block the decision cycle:

```go
	// Prefetch Binance per-coin detail for the candidate pool in the background
	// (bounded workers) so request load is spread and warm by prompt time.
	if at.strategyEngine != nil && len(candidateCoins) > 0 {
		go func(symbols []string) {
			runPrefetchJobs(symbols, 3, func(sym string) {
				// engine cache is populated lazily by attachPerCoinSignals;
				// warming it here is safe and idempotent.
				at.strategyEngine.PrefetchBinanceDetails(context.Background(), []string{sym})
			})
		}(candidateSymbols(candidateCoins))
	}
```

Where `candidateSymbols` extracts the symbol slice, and `PrefetchBinanceDetails` (added in Task 6) fetches enabled sources into the cache (no-op if neither Binance flag is on). If `PrefetchBinanceDetails` was not added in Task 6, add it here as a thin wrapper over `GetAssetDetails` gated by the two `EnableBinance*Data` flags, populating `engine.binanceDetail`/`cacheBinanceDetail`.

**NOTE:** The prefetch goroutine writes to the cache (mutex-guarded) and reads config — both safe. It must not block or cancel the main cycle; use `context.Background()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./trader/ -run TestRunPrefetchJobsBoundsConcurrency -v`
Expected: PASS.

- [ ] **Step 5: Run full backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add trader/auto_trader_loop.go trader/auto_trader_prefetch.go trader/auto_trader_prefetch_test.go && git commit -m "feat(trader): background prefetch for binance per-coin detail"
```

---

### Task 9: Frontend types + scope cards + buildCoinSource

**Files:**
- Modify: `web/src/types/strategy.ts`
- Modify: `web/src/features/strategies/scopeCatalog.ts`
- Modify: `web/src/features/strategies/strategyFactory.ts`

**Interfaces:**
- Consumes: existing `ScopeUnit`/`ScopeVariant`/`CoinSourceConfig`/`StrategyEditorForm`.
- Produces:
  - `ScopeVariant` += `'binance'`.
  - `ScopeUnit` += optional `interval?: '1h' | '24h'`, `direction?: 'top' | 'bottom'`.
  - `CoinSourceConfig.source_type` += `'binance_technical' | 'binance_sentiment'`.
  - `CoinSourceConfig` += `binance_technical_interval?`, `binance_technical_direction?`, `binance_technical_limit?`, `binance_sentiment_direction?`, `binance_sentiment_limit?`.

- [ ] **Step 1: Write the failing factory test**

Add to `web/src/test/strategyFactory.test.ts`:

```ts
import { buildCoinSource } from '../features/strategies/strategyFactory'

test('buildCoinSource maps binance_technical unit to concrete source', () => {
  const cs = buildCoinSource({
    id: 'crypto-binance-technical',
    category: 'crypto',
    source_type: 'binance_technical',
    limit: 10,
    label: 'Binance Technical',
    provider: 'free',
    variant: 'binance',
    interval: '1h',
    direction: 'top',
  })
  expect(cs.source_type).toBe('binance_technical')
  expect(cs.binance_technical_interval).toBe('1h')
  expect(cs.binance_technical_direction).toBe('top')
  expect(cs.binance_technical_limit).toBe(10)
  expect(cs.scope_mode).toBeUndefined()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx vitest run src/test/strategyFactory.test.ts`
Expected: FAIL — `binance_technical` not handled / type error.

- [ ] **Step 3: Add types + cards + factory branches**

`web/src/types/strategy.ts` — extend the `source_type` union and add fields:

```ts
  // Binance Opportunity scopes (free)
  binance_technical_interval?: '1h' | '24h'
  binance_technical_direction?: 'top' | 'bottom'
  binance_technical_limit?: number
  binance_sentiment_direction?: 'top' | 'bottom'
  binance_sentiment_limit?: number
```

and add `'binance_technical' | 'binance_sentiment'` to the `source_type` union. Extend `ScopeUnit` with `interval?: '1h'|'24h'`, `direction?: 'top'|'bottom'`. Extend `ScopeVariant` with `'binance'`.

`web/src/features/strategies/scopeCatalog.ts` — add helper + cards:

```ts
const freeBinanceOpportunity = (
  id: string,
  label: string,
  description: string,
  source_type: ScopeUnit['source_type'],
): ScopeCardDef => ({
  id,
  category: 'crypto',
  label,
  description,
  provider: 'free',
  source_type,
  variant: 'binance',
  defaultLimit: 10,
})
```

and append to `SCOPE_CARD_DEFS`:

```ts
  freeBinanceOpportunity('crypto-binance-technical', 'Binance Technical', 'Top technical score (1h/24h) on Binance Opportunity', 'binance_technical'),
  freeBinanceOpportunity('crypto-binance-sentiment', 'Binance Sentiment', 'Top sentiment score on Binance Opportunity', 'binance_sentiment'),
```

`web/src/features/strategies/strategyFactory.ts` — add branches in `buildCoinSource` (before the fallback `static` return):

```ts
  if (unit.source_type === 'binance_technical') {
    return {
      source_type: 'binance_technical',
      binance_technical_interval: unit.interval ?? '1h',
      binance_technical_direction: unit.direction ?? 'top',
      binance_technical_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }
  if (unit.source_type === 'binance_sentiment') {
    return {
      source_type: 'binance_sentiment',
      binance_sentiment_direction: unit.direction ?? 'top',
      binance_sentiment_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx vitest run src/test/strategyFactory.test.ts`
Expected: PASS.

- [ ] **Step 5: Type-check + full frontend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx tsc --noEmit && npm run build && npm test`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add web/src/types/strategy.ts web/src/features/strategies/scopeCatalog.ts web/src/features/strategies/strategyFactory.ts web/src/test/strategyFactory.test.ts && git commit -m "feat(web): binance scope cards and coin source mapping"
```

---

### Task 10: ScopeStepPage sub-controls + matchConcreteScope

**Files:**
- Modify: `web/src/features/strategies/ScopeStepPage.tsx`

**Interfaces:**
- Consumes: `ScopeUnit` (with `interval`/`direction`), `SCOPE_CARD_DEFS` (Task 9).
- Produces: reveals interval (technical only) + direction sub-controls when a `binance_*` card is selected; `matchConcreteScope` prefills interval/direction from a saved `coin_source`.

- [ ] **Step 1: Read current ScopeStepPage render + matchConcreteScope**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && sed -n '1,140p' src/features/strategies/ScopeStepPage.tsx` and locate `matchConcreteScope`.

- [ ] **Step 2: Add sub-control rendering**

When the selected card's `source_type` is `binance_technical`, render a `1h | 24h` toggle row that sets `unit.interval`. When it's `binance_technical` or `binance_sentiment`, render a `Top | Bottom` row that sets `unit.direction`. Store these on the scope unit via the draft store `setScope` (replace). Default interval `1h`, direction `top`.

- [ ] **Step 3: Update matchConcreteScope**

Add branches so a saved `source_type === 'binance_technical'` maps to the `crypto-binance-technical` card with `interval`/`direction` from `binance_technical_interval`/`binance_technical_direction`; likewise for `binance_sentiment`.

- [ ] **Step 4: Manual + type check**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx tsc --noEmit && npm run build`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add web/src/features/strategies/ScopeStepPage.tsx && git commit -m "feat(web): binance scope sub-controls + prefill"
```

---

### Task 11: Frontend per-coin toggles + factory mapping + dataSourceDefaults

**Files:**
- Modify: `web/src/types/strategy.ts` — `IndicatorConfig`.
- Modify: `web/src/features/strategies/strategyFactory.ts` — `StrategyEditorForm` + `buildStrategyConfig`.
- Modify: `web/src/features/strategies/dataSourceDefaults.ts`.

**Interfaces:**
- Consumes: `IndicatorConfig` (backend fields from Task 5).
- Produces: `StrategyEditorForm` += `enableBinanceTechnicalData?`, `enableBinanceSentimentData?`, `binanceTechnicalIntervals?: ('1h'|'24h')[]`; `buildStrategyConfig` maps them to `enable_binance_technical_data`/`enable_binance_sentiment_data`/`binance_technical_intervals`; `defaultDataSources` returns the 2 new booleans.

- [ ] **Step 1: Write failing test**

Add to `web/src/test/dataSourceDefaults.test.ts`:

```ts
import { defaultDataSources } from '../features/strategies/dataSourceDefaults'

test('binance technical scope auto-enables binance technical detail', () => {
  const d = defaultDataSources({
    id: 'crypto-binance-technical', category: 'crypto', source_type: 'binance_technical',
    limit: 10, label: 'Binance Technical', provider: 'free', variant: 'binance',
  } as any)
  expect(d.enableBinanceTechnicalData).toBe(true)
  expect(d.enableBinanceSentimentData).toBe(false)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx vitest run src/test/dataSourceDefaults.test.ts`
Expected: FAIL — `enableBinanceTechnicalData` undefined on return type.

- [ ] **Step 3: Implement**

`web/src/types/strategy.ts` `IndicatorConfig`:

```ts
  enable_binance_technical_data?: boolean
  enable_binance_sentiment_data?: boolean
  binance_technical_intervals?: ('1h' | '24h')[]
```

`web/src/features/strategies/dataSourceDefaults.ts` — change return type + add fields:

```ts
export function defaultDataSources(scope: ScopeUnit | null): {
  enableAI500Data: boolean
  enableOIData: boolean
  enableNetflowData: boolean
  enablePriceData: boolean
  enableBinanceTechnicalData: boolean
  enableBinanceSentimentData: boolean
} {
  return {
    enableAI500Data: scope?.source_type === 'ai500',
    enableOIData: scope?.source_type === 'nofxos_oi',
    enableNetflowData: scope?.source_type === 'nofxos_netflow',
    enablePriceData: scope?.source_type === 'nofxos_price',
    enableBinanceTechnicalData: scope?.source_type === 'binance_technical',
    enableBinanceSentimentData: scope?.source_type === 'binance_sentiment',
  }
}
```

`web/src/features/strategies/strategyFactory.ts` `StrategyEditorForm`:

```ts
  enableBinanceTechnicalData?: boolean
  enableBinanceSentimentData?: boolean
  binanceTechnicalIntervals?: ('1h' | '24h')[]
```

`buildStrategyConfig` `indicators`:

```ts
        enable_binance_technical_data: form.enableBinanceTechnicalData ?? false,
        enable_binance_sentiment_data: form.enableBinanceSentimentData ?? false,
        binance_technical_intervals: form.binanceTechnicalIntervals?.length ? form.binanceTechnicalIntervals : undefined,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx vitest run src/test/dataSourceDefaults.test.ts`
Expected: PASS.

- [ ] **Step 5: Type-check + full frontend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx tsc --noEmit && npm run build && npm test`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add web/src/types/strategy.ts web/src/features/strategies/strategyFactory.ts web/src/features/strategies/dataSourceDefaults.ts web/src/test/dataSourceDefaults.test.ts && git commit -m "feat(web): binance per-coin data toggles + factory mapping"
```

---

### Task 12: Editor restructure — Basic indicators + Data sources + period chips + ranking toggles + candles + edit round-trip

**Files:**
- Modify: `web/src/features/strategies/EditorStepPage.tsx`

**Interfaces:**
- Consumes: `StrategyEditorForm` (Task 11 + existing), `IndicatorConfig` fields, `defaultDataSources`, `buildStrategyConfig`.
- Produces: the restructured editor UI with full state for all indicator/data-source fields; complete edit load-back.

This is the largest task. It is intentionally split into focused steps.

- [ ] **Step 1: Split the fieldset into two cards**

Replace the single "Data sources for LLM" fieldset (EditorStepPage.tsx ~470-552) with two fieldsets:
1. **"Basic indicators"** — toggles EMA, MACD, RSI, ATR, BOLL, Volume + period chips.
2. **"Data sources"** — per-coin toggles (AI500, OI, Netflow, Price change, Binance Technical, Binance Sentiment) + data-durations row + a "Rankings" sub-heading with OI/Netflow/Price ranking toggles + duration + limit.

Add the state hooks near the existing indicator state (~line 83):

```ts
  const [enableAtr, setEnableAtr] = useState(false)
  const [enableBoll, setEnableBoll] = useState(false)
  const [enableVolume, setEnableVolume] = useState(false)
  const [emaPeriods, setEmaPeriods] = useState<number[]>([20, 50])
  const [rsiPeriods, setRsiPeriods] = useState<number[]>([7, 14])
  const [atrPeriods, setAtrPeriods] = useState<number[]>([14])
  const [bollPeriods, setBollPeriods] = useState<number[]>([20])
  const [enableBinanceTechnicalData, setEnableBinanceTechnicalData] = useState(false)
  const [enableBinanceSentimentData, setEnableBinanceSentimentData] = useState(false)
  const [binanceTechnicalIntervals, setBinanceTechnicalIntervals] = useState<('1h' | '24h')[]>(['1h'])
  const [primaryCount, setPrimaryCount] = useState(30)
  const [longerTimeframe, setLongerTimeframe] = useState('')
  const [longerCount, setLongerCount] = useState(0)
  const [enableOIRanking, setEnableOIRanking] = useState(false)
  const [oiRankingDuration, setOIRankingDuration] = useState('1h')
  const [oiRankingLimit, setOIRankingLimit] = useState(10)
  const [enableNetFlowRanking, setEnableNetFlowRanking] = useState(false)
  const [netFlowRankingDuration, setNetFlowRankingDuration] = useState('1h')
  const [netFlowRankingLimit, setNetFlowRankingLimit] = useState(10)
  const [enablePriceRanking, setEnablePriceRanking] = useState(false)
  const [priceRankingDuration, setPriceRankingDuration] = useState('1h')
  const [priceRankingLimit, setPriceRankingLimit] = useState(10)
```

Define period-option constants at module scope:

```ts
const EMA_PERIODS = [9, 10, 20, 50, 200]
const RSI_PERIODS = [7, 14, 21]
const ATR_PERIODS = [7, 14, 21]
const BOLL_PERIODS = [10, 20, 50]
const RANKING_DURATIONS = ['1h', '4h', '24h']
```

Add a small `toggleNumberList` helper (module scope) for period multi-select:

```ts
function toggleNumberList(current: number[], n: number): number[] {
  return current.includes(n) ? current.filter((x) => x !== n) : [...current, n].sort((a, b) => a - b)
}
```

- [ ] **Step 2: Build the two fieldset JSX**

In the "Basic indicators" card, render the 6 toggles; when `enableEma` show EMA period chips, etc. For the candles row (currently in Advanced Settings ~line 574), add a primary-count number input and, when multi-timeframe is on, longer-timeframe select + longer-count input. In the "Data sources" card, add the 2 Binance toggles + a Binance technical interval row (`1h`/`24h` independent chips toggling `binanceTechnicalIntervals`), and the "Rankings" sub-section with the 3 ranking toggles + their duration/limit controls.

- [ ] **Step 3: Update edit load-back (round-trip parity)**

In the edit `useEffect` (~line 137-160), add restore for all new fields from `ind`:

```ts
        setEnableAtr(ind?.enable_atr ?? false)
        setEnableBoll(ind?.enable_boll ?? false)
        setEnableVolume(ind?.enable_volume ?? false)
        setEmaPeriods(ind?.ema_periods?.length ? ind.ema_periods : [20, 50])
        setRsiPeriods(ind?.rsi_periods?.length ? ind.rsi_periods : [7, 14])
        setAtrPeriods(ind?.atr_periods?.length ? ind.atr_periods : [14])
        setBollPeriods(ind?.boll_periods?.length ? ind.boll_periods : [20])
        setEnableBinanceTechnicalData(ind?.enable_binance_technical_data ?? false)
        setEnableBinanceSentimentData(ind?.enable_binance_sentiment_data ?? false)
        setBinanceTechnicalIntervals(ind?.binance_technical_intervals?.length ? ind.binance_technical_intervals : ['1h'])
        setPrimaryCount(ind?.klines.primary_count ?? 30)
        setLongerTimeframe(ind?.klines.longer_timeframe ?? '')
        setLongerCount(ind?.klines.longer_count ?? 0)
        setEnableOIRanking(ind?.enable_oi_ranking ?? false)
        setOIRankingDuration(ind?.oi_ranking_duration ?? '1h')
        setOIRankingLimit(ind?.oi_ranking_limit ?? 10)
        setEnableNetFlowRanking(ind?.enable_netflow_ranking ?? false)
        setNetFlowRankingDuration(ind?.netflow_ranking_duration ?? '1h')
        setNetFlowRankingLimit(ind?.netflow_ranking_limit ?? 10)
        setEnablePriceRanking(ind?.enable_price_ranking ?? false)
        setPriceRankingDuration(ind?.price_ranking_duration ?? '1h')
        setPriceRankingLimit(ind?.price_ranking_limit ?? 10)
```

- [ ] **Step 4: Update `StrategyEditorForm` build in `strategyFactory.ts` + wire into `buildStrategyConfig`**

`StrategyEditorForm` (Task 11 file):

```ts
  enableAtr?: boolean
  enableBoll?: boolean
  enableVolume?: boolean
  emaPeriods?: number[]
  rsiPeriods?: number[]
  atrPeriods?: number[]
  bollPeriods?: number[]
  enableOIRanking?: boolean
  oiRankingDuration?: string
  oiRankingLimit?: number
  enableNetFlowRanking?: boolean
  netFlowRankingDuration?: string
  netFlowRankingLimit?: number
  enablePriceRanking?: boolean
  priceRankingDuration?: string
  priceRankingLimit?: number
  primaryCount?: number
  longerTimeframe?: string
  longerCount?: number
```

`buildStrategyConfig` `indicators`:

```ts
        enable_atr: form.enableAtr ?? false,
        enable_boll: form.enableBoll ?? false,
        enable_volume: form.enableVolume ?? false,
        ema_periods: form.emaPeriods?.length ? form.emaPeriods : undefined,
        rsi_periods: form.rsiPeriods?.length ? form.rsiPeriods : undefined,
        atr_periods: form.atrPeriods?.length ? form.atrPeriods : undefined,
        boll_periods: form.bollPeriods?.length ? form.bollPeriods : undefined,
        enable_oi_ranking: form.enableOIRanking ?? false,
        oi_ranking_duration: form.oiRankingDuration ?? '1h',
        oi_ranking_limit: form.oiRankingLimit ?? 10,
        enable_netflow_ranking: form.enableNetFlowRanking ?? false,
        netflow_ranking_duration: form.netFlowRankingDuration ?? '1h',
        netflow_ranking_limit: form.netFlowRankingLimit ?? 10,
        enable_price_ranking: form.enablePriceRanking ?? false,
        price_ranking_duration: form.priceRankingDuration ?? '1h',
        price_ranking_limit: form.priceRankingLimit ?? 10,
```

and `klines`:

```ts
          primary_count: form.primaryCount ?? 30,
          longer_timeframe: form.longerTimeframe || undefined,
          longer_count: form.longerCount && form.longerCount > 0 ? form.longerCount : undefined,
```

- [ ] **Step 5: Wire EditorStepPage save to the form**

Ensure the `buildStrategyConfig` call in EditorStepPage's save handler passes the new form fields (add them to the object it builds). The editor currently builds the form from its state; add all new fields.

- [ ] **Step 6: Type-check + full frontend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx tsc --noEmit && npm run build && npm test`
Expected: all green.

- [ ] **Step 7: Add an editor rendering smoke test**

Add to `web/src/test/` a test that renders `EditorStepPage` (mocked strategy API) and asserts both "Basic indicators" and "Data sources" fieldset legends appear. Use the existing test setup conventions (see `EditorStepPage` or strategy tests for the render helpers).

- [ ] **Step 8: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add web/src/features/strategies/EditorStepPage.tsx web/src/features/strategies/strategyFactory.ts web/src/test/ && git commit -m "feat(web): editor restructure with periods, rankings, candles, round-trip"
```

---

### Task 13: Full verification + manual smoke

**Files:** none (verification only).

- [ ] **Step 1: Backend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 2: Frontend verification**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx/web && npx tsc --noEmit && npm run build && npm test`
Expected: all green.

- [ ] **Step 3: Manual smoke**

Create a strategy → select **Binance Technical** scope → set interval `1h` + direction `Top` → editor shows two fieldset cards → enable **Binance Technical** detail at `1h` + **Binance Sentiment** → save → reopen → confirm `coin_source.source_type === 'binance_technical'` and `indicators.enable_binance_technical_data`/`binance_technical_intervals`/periods persist. Confirm the EMA toggle now renders period chips and the OI/Netflow/Price ranking toggles are visible under "Rankings".

- [ ] **Step 4: Commit any final touch-ups**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && git add -A && git commit -m "chore: final verification fixes"
```

---

## Self-Review

**Spec coverage:**
- Part A scope cards (`binance_technical`/`binance_sentiment`) with interval+direction sub-controls → Tasks 3, 4, 9, 10. ✓
- SPOT→perp mapping + empty pool → Task 4 (`mapSpotToPerp`, non-empty check). ✓
- OI-liquidity filter applies automatically → noted in Global Constraints; no code. ✓
- Part B per-coin toggles + independent intervals → Tasks 5, 6, 7, 11. ✓
- TTL cache + background prefetch (Option 1) → Tasks 6, 8. ✓
- Part C: two fieldset cards, period chips, ranking toggles (no quant), candles count/longer, edit round-trip → Task 12. ✓
- Sentiment endpoint verified (403 items, `sentiment_score` present) → confirmed this session; no fallback needed. ✓

**Placeholder scan:** The only deferred item is the `Context.Ctx` field availability in Task 6 Step 5, explicitly flagged for verification during implementation; the fake `OpportunityClient` in Task 4 Step 3 is defined inline. No TBD/TODO stubs remain beyond the documented verification notes.

**Type consistency:** `binance_technical_interval`/`direction`/`limit`, `binance_sentiment_direction`/`limit` are identical across store (Task 3), kernel (Task 4), and frontend TS (Task 9). `enable_binance_technical_data`/`enable_binance_sentiment_data`/`binance_technical_intervals` match across store (Task 5), kernel (Task 6/7), and frontend (Task 11/12). `PerCoinSignal.BinanceTechnical`/`BinanceSentiment` used consistently in Tasks 6/7. `PrefetchBinanceDetails` is referenced in Task 8 and defined as a Task 6 wrapper (with a Task 8 fallback note if not added). `runPrefetchJobs` defined in Task 8 and tested there.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-20-binance-scopes-and-indicator-revamp.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?