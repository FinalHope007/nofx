# Per-coin Data Source Selection for LLM Prompt — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users choose which per-coin data sources (AI500, OI, netflow, price, and basic indicators) are embedded into each candidate coin's and open position's LLM prompt section, with selectable durations, auto-defaulting from the chosen scope card.

**Architecture:** New independent toggles + a shared duration list live on `store.IndicatorConfig`. Backend retains the per-coin data it already fetches from free `vergex.trade` trending endpoints (AI500/OI/netflow/price), filters to candidates+positions, and renders per-coin sections in `kernel/engine_prompt.go`. The wizard (`EditorStepPage` + `strategyFactory`) exposes the toggles/durations and auto-enables a source when its scope card is selected (Bias Radar excluded).

**Tech Stack:** Go 1.25 (kernel/store/provider), React 18 + TS + Vitest (web).

## Global Constraints

- English-only UI strings (frontend).
- Backend errors use `SafeError`/`SafeInternalError`/`SanitizeError` — never leak internals to clients.
- `store.*` is the only DB access layer; all timestamps UTC.
- `go vet ./...` + `gofmt -l .` clean; frontend `tsc --noEmit` + `npm run build` + `npm test` pass.
- Existing paid `enable_oi_ranking`/`enable_netflow_ranking`/`enable_price_ranking` toggles are UNTOUCHED.
- Kline is not a source toggle (always on, set under Candles).
- Free endpoints are public `vergex.trade`, no auth (optional `VERGEX_API_TOKEN` only for vergex detail).

---

### Task 1: Backend schema — new IndicatorConfig fields, defaults, clamp, token estimate

**Files:**
- Modify: `store/strategy.go` (IndicatorConfig struct ~886-935, `ClampLimits` ~36, `getEffectiveCoinCount` area, defaults ~1059)
- Modify: `store/strategy.go` `EstimateTokens` (~1377)
- Test: `store/strategy_token_test.go` (new test func)

**Interfaces:**
- Consumes: existing `IndicatorConfig` struct.
- Produces: new `IndicatorConfig` fields `EnableAI500Data`, `EnableOIData`, `EnableNetflowData`, `EnablePriceData`, `DataDurations []string` with JSON tags `enable_ai500_data`, `enable_oi_data`, `enable_netflow_data`, `enable_price_data`, `data_durations`.

- [ ] **Step 1: Write the failing tests**

In `store/strategy_token_test.go`, add tests asserting JSON round-trip of the new fields, and that `EstimateTokens` accounts for them.

```go
func TestIndicatorConfigDataSourceFieldsRoundTrip(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.StrategyType = "ai_trading"
	cfg.Indicators.EnableAI500Data = true
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.EnableNetflowData = true
	cfg.Indicators.EnablePriceData = true
	cfg.Indicators.DataDurations = []string{"15m", "1h"}

	// Marshal uses the product schema (StrategyConfig.MarshalJSON) nesting
	// these under ai_config.indicators.
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"enable_ai500_data":true`) {
		t.Fatalf("enable_ai500_data missing: %s", string(raw))
	}
	if !strings.Contains(string(raw), `"data_durations":["15m","1h"]`) {
		t.Fatalf("data_durations missing: %s", string(raw))
	}

	// Unmarshal back and confirm the fields survive the round-trip.
	var restored store.StrategyConfig
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !restored.Indicators.EnableAI500Data || !restored.Indicators.EnableOIData ||
		!restored.Indicators.EnableNetflowData || !restored.Indicators.EnablePriceData {
		t.Fatalf("unmarshal lost data-source toggles: %+v", restored.Indicators)
	}
	if len(restored.Indicators.DataDurations) != 2 {
		t.Fatalf("unmarshal lost data_durations: %v", restored.Indicators.DataDurations)
	}
}
```

- [ ] **Step 2: Run to verify compile/fail**

Run: `go test ./store/ -run TestIndicatorConfigDataSourceFieldsRoundTrip -v`
Expected: FAIL — `EnableAI500Data` undefined.

- [ ] **Step 3: Add the struct fields**

In `store/strategy.go`, in `IndicatorConfig` (near `EnablePriceRanking`), add:

```go
	// Free per-coin data sources exposed to the LLM prompt (independent of the
	// market-wide ranking toggles above).
	EnableAI500Data   bool     `json:"enable_ai500_data"`    // AI500 score
	EnableOIData      bool     `json:"enable_oi_data"`       // OI change
	EnableNetflowData bool     `json:"enable_netflow_data"`  // net flow
	EnablePriceData   bool     `json:"enable_price_data"`    // price change
	DataDurations     []string `json:"data_durations,omitempty"` // 15m..24h
```

- [ ] **Step 4: Defaults**

In the default AI strategy indicator block (around line 1059 where `EnableOIRanking: false`, etc. are set), initialize:
```go
			DataDurations:     []string{"1h", "24h"},
```

- [ ] **Step 5: ClampLimits**

In `ClampLimits` (top of `store/strategy.go`), ensure `DataDurations` is de-duped and restricted to the supported set `{"15m","30m","1h","4h","8h","12h","24h"}`; drop any unknown entry; if empty after filtering, reset to `["24h"]`.

```go
supportedDurations := []string{"15m", "30m", "1h", "4h", "8h", "12h", "24h"}
seen := map[string]bool{}
var kept []string
for _, d := range c.Indicators.DataDurations {
	d = strings.TrimSpace(d)
	if seen[d] {
		continue
	}
	for _, s := range supportedDurations {
		if d == s {
			seen[d] = true
			kept = append(kept, d)
			break
		}
	}
}
if len(kept) == 0 {
	kept = []string{"24h"}
}
c.Indicators.DataDurations = kept
```

- [ ] **Step 6: EstimateTokens**

In `EstimateTokens`, after the OI/funding block (~1439-1441), add a per-coin enrich estimate when any free source is on:

```go
	if c.Indicators.EnableAI500Data {
		totalMarketChars += numCoins * 120
	}
	if c.Indicators.EnableOIData {
		totalMarketChars += numCoins * 40 * len(c.Indicators.DataDurations)
	}
	if c.Indicators.EnableNetflowData {
		totalMarketChars += numCoins * 30 * len(c.Indicators.DataDurations)
	}
	if c.Indicators.EnablePriceData {
		totalMarketChars += numCoins * 30 * len(c.Indicators.DataDurations)
	}
```

- [ ] **Step 7: Run tests**

Run: `go test ./store/ -run TestIndicatorConfigDataSourceFieldsRoundTrip -v`
Expected: PASS.

- [ ] **Step 8: gofmt + vet + commit**

Run: `gofmt -l ./store ./kernel ./api` then `go vet ./store/...`
Commit:
```bash
git add store/strategy.go store/strategy_token_test.go
git commit -m "feat(strategy): add per-coin data-source config fields to IndicatorConfig"
```

---

### Task 2: Backend free client — per-duration fetchers + per-list raw access + AI500 peak

**Files:**
- Modify: `provider/nofxos/free.go` (`getTrending` hardcodes duration; add duration-aware + per-array accessors)
- Modify: `provider/nofxos/ai500.go` (add `PeakScore` to `CoinData` if needed for free parse)
- Test: `kernel/engine_free_test.go` (extend)

**Interfaces:**
- Consumes: `FreeTrendingClient` with `SetBaseURL`.
- Produces: methods with exact signatures for Task 3:
  - `(c *FreeTrendingClient) GetOIData(duration string, limit int) (*OIDataEnvelope, error)` where `OIDataEnvelope{ Top, Low []OIPosition }`
  - `(c *FreeTrendingClient) GetNetflowData(duration string, limit int) (*NetflowEnvelope, error)` where `NetflowEnvelope{ Top, Low []NetFlowPosition }`
  - `(c *FreeTrendingClient) GetPriceData(duration string, limit int) (*PriceEnvelope, error)` where `PriceEnvelope{ Top, Low []PriceRankingItem }`
  - `(c *FreeTrendingClient) GetAI500CoinData() ([]CoinData, error)` returning full `CoinData` (already exists as `GetAI500`, but ensure `PeakScore` populated).

- [ ] **Step 1: Write failing tests**

In `kernel/engine_free_test.go` add:

```go
func TestFreeTrending_GetOIData_duration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"current_oi":1,"oi_delta":1,"oi_delta_percent":1,"oi_delta_value":1,"price_delta_percent":1,"net_long":1,"net_short":1}],"low":[]}`))
	}))
	defer srv.Close()
	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	env, err := tr.GetOIData("1h", 50)
	if err != nil {
		t.Fatalf("GetOIData: %v", err)
	}
	if len(env.Top) != 1 || len(env.Low) != 0 {
		t.Fatalf("unexpected %+v", env)
	}
	if env.Top[0].OIDeltaPercent != 1 {
		t.Fatalf("oi_delta_percent not parsed: %+v", env.Top[0])
	}
}
```

Add analogous `TestFreeTrending_GetNetflowData_duration` and `TestFreeTrending_GetPriceData_duration` using the real shapes captured below:

```go
// netflow real shape:
// {"top":[{"amount":29309691.4,"price":64119.7,"price_delta_percent":0.2316,"rank":1,"symbol":"BTCUSDT"}],"low":[]}
// price real shape:
// {"top":[{"pair":"RAREUSDT","symbol":"RARE","price_delta":0.2465,"price":0.0153,"future_flow":2199.0,"spot_flow":139489.6,"oi":113026194,"oi_delta":37235516,"oi_delta_value":915909.5}],"low":[]}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./kernel/ -run TestFreeTrending_ -v`
Expected: FAIL — `GetOIData` undefined.

- [ ] **Step 3: Implement duration-aware fetchers in free.go**

Add an envelope helper and refactor:

```go
type OIDataEnvelope struct {
	Top []OIPosition `json:"top"`
	Low []OIPosition `json:"low"`
}
type NetflowEnvelope struct {
	Top []NetFlowPosition `json:"top"`
	Low []NetFlowPosition `json:"low"`
}
type PriceEnvelope struct {
	Top []PriceRankingItem `json:"top"`
	Low []PriceRankingItem `json:"low"`
}

func (c *FreeTrendingClient) GetOIData(duration string, limit int) (*OIDataEnvelope, error) {
	raw, err := c.getTrending("oi", duration, limit)
	if err != nil {
		return nil, err
	}
	var env OIDataEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending oi: %w", err)
	}
	return &env, nil
}

func (c *FreeTrendingClient) GetNetflowData(duration string, limit int) (*NetflowEnvelope, error) {
	raw, err := c.getTrending("net_flow", duration, limit)
	if err != nil {
		return nil, err
	}
	var env NetflowEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending net_flow: %w", err)
	}
	return &env, nil
}

func (c *FreeTrendingClient) GetPriceData(duration string, limit int) (*PriceEnvelope, error) {
	raw, err := c.getTrending("price", duration, limit)
	if err != nil {
		return nil, err
	}
	var env PriceEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending price: %w", err)
	}
	return &env, nil
}
```

Refactor `getTrending` to accept a duration param (keep backwards-compatible default of "24h" for the existing `getOIArray` helpers):

```go
func (c *FreeTrendingClient) getTrending(tab, duration string, limit int) (json.RawMessage, error) {
	params := url.Values{}
	params.Set("tab", tab)
	if duration == "" {
		duration = "24h"
	}
	params.Set("duration", duration)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.get(c.trendingPath(params))
}
```

Update the existing `getOIArray`/`getNetFlowArray`/`getPriceArray` calls to `c.getTrending("oi", "24h", limit)` etc.

- [ ] **Step 4: AI500 peak score**

In `provider/nofxos/ai500.go`, add `PeakScore float64 \`json:"peak_score"\`` to `CoinData`. In `free.go` `GetAI500`, parse the `signal` display string (e.g. "Peak 87") if present, and set `PeakScore`. Add a small parse helper:

```go
func parsePeakScore(signal string) float64 {
	i := strings.LastIndex(signal, "Peak")
	if i < 0 {
		return 0
	}
	var v float64
	rest := strings.TrimSpace(signal[i+4:])
	fmt.Sscanf(rest, "%f", &v)
	return v
}
```
In the `assets` anonymous struct in `GetAI500`, add `Signal string \`json:"signal"\`` and after building the slice set `coins[len(coins)-1].PeakScore = parsePeakScore(a.Signal)`.

- [ ] **Step 5: Run tests**

Run: `go test ./kernel/ -run TestFreeTrending_ -v`
Expected: PASS.

- [ ] **Step 6: gofmt + vet + commit**

Run: `gofmt -l ./provider/nofxos ./kernel ./store` then `go vet ./provider/nofxos/... ./kernel/...`
Commit:
```bash
git add provider/nofxos/free.go provider/nofxos/ai500.go kernel/engine_free_test.go
git commit -m "feat(nofxos): per-duration trending fetchers and AI500 peak score"
```

---

### Task 3: Backend engine — retain per-coin pools + enrichment map

**Files:**
- Modify: `kernel/engine.go` (`StrategyEngine` struct, `NewStrategyEngine` init)
- Test: `kernel/engine_prompt_test.go` (new test)

**Interfaces:**
- Consumes: new `FreeTrendingClient` fetchers from Task 2; `nofxos` types.
- Produces:
  - `type PerCoinSignal struct { AI500 *nofxos.CoinData; OI map[string]map[string]nofxos.OIPosition; Netflow map[string]map[string]nofxos.NetFlowPosition; Price map[string]map[string]nofxos.PriceRankingItem }` (keyed by duration → list "top"/"low")
  - engine field `perCoinSignals map[string]PerCoinSignal`
  - `SetPerCoinSignals(signals map[string]PerCoinSignal)`
  - `PerCoinSignalFor(symbol string) (PerCoinSignal, bool)`

- [ ] **Step 1: Write failing tests**

Add to `kernel/engine_prompt_test.go`:

```go
func TestEnginePerCoinSignals_StoreAndGet(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	e := NewStrategyEngine(&cfg)
	if s, ok := e.PerCoinSignalFor("BTCUSDT"); ok {
		t.Fatalf("unexpected existing signal for BTCUSDT: %+v", s)
	}
	sig := PerCoinSignal{AI500: &nofxos.CoinData{Symbol: "CYS", Pair: "CYSUSDT", Score: 78.3}}
	e.SetPerCoinSignals(map[string]PerCoinSignal{"CYSUSDT": sig})
	if s, ok := e.PerCoinSignalFor("CYSUSDT"); !ok || s.AI500 == nil || s.AI500.Score != 78.3 {
		t.Fatalf("PerCoinSignalFor: %+v", s)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./kernel/ -run TestEnginePerCoinSignals_ -v`
Expected: FAIL — `PerCoinSignal` undefined.

- [ ] **Step 3: Add PerCoinSignal type + engine field/accessor**

In `kernel/engine.go`:

```go
// PerCoinSignal carries the free per-coin enrichment data for one symbol
// (AI500 score, OI / netflow / price change per selected duration).
type PerCoinSignal struct {
	AI500   *nofxos.CoinData                       // may be nil
	OI      map[string]map[string]nofxos.OIPosition  // duration -> list(top/low) -> data
	Netflow map[string]map[string]nofxos.NetFlowPosition
	Price   map[string]map[string]nofxos.PriceRankingItem
}
```

Add field to `StrategyEngine`:
```go
	perCoinSignals map[string]PerCoinSignal
```

Init in `NewStrategyEngine` both return paths (add `perCoinSignals: make(map[string]PerCoinSignal),`).

Add methods:
```go
func (e *StrategyEngine) SetPerCoinSignals(signals map[string]PerCoinSignal) {
	e.perCoinSignals = signals
}

func (e *StrategyEngine) PerCoinSignalFor(symbol string) (PerCoinSignal, bool) {
	if e == nil || e.perCoinSignals == nil {
		return PerCoinSignal{}, false
	}
	s, ok := e.perCoinSignals[symbol]
	return s, ok
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./kernel/ -run TestEnginePerCoinSignals_ -v`
Expected: PASS.

- [ ] **Step 5: gofmt + vet + commit**

Run: `gofmt -l ./kernel ./store` then `go vet ./kernel/...`
Commit:
```bash
git add kernel/engine.go kernel/engine_prompt_test.go
git commit -m "feat(kernel): per-coin signal store and accessor on StrategyEngine"
```

---

### Task 4: Backend — fetch + attach per-coin data per cycle

**Files:**
- Modify: `kernel/engine_analysis.go` (new `attachPerCoinSignals(ctx, engine)`)
- Modify: `trader/auto_trader_loop.go` (`buildTradingContext` — call attach)
- Test: `kernel/engine_prompt_test.go` (new test for attach filtering to candidates+positions)

**Interfaces:**
- Consumes: Task 2 fetchers, Task 3 engine accessors.
- Produces: `attachPerCoinSignals(ctx *Context, engine *StrategyEngine) error` — filters fetched data to `ctx.CandidateCoins`+`ctx.Positions`, calls `engine.SetPerCoinSignals`.

- [ ] **Step 1: Write failing test**

Add to `kernel/engine_prompt_test.go`:

```go
func TestAttachPerCoinSignals_filtersToCandidates(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAI500Data = true
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.EnableNetflowData = true
	cfg.Indicators.EnablePriceData = true
	cfg.Indicators.DataDurations = []string{"1h"}
	e := NewStrategyEngine(&cfg)
	tr := nofxos.NewFreeTrendingClient()
	fsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/trending-category":
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BTC","pair":"BTCUSDT","score":78.3,"startPrice":0.5,"startTime":1785852000,"changePctValue":37.5,"signal":"Peak 87"}]}}`))
		case r.URL.Query().Get("tab") == "oi":
			w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":1,"oi_delta":1,"oi_delta_percent":5.0,"oi_delta_value":1,"price_delta_percent":3.0,"net_long":1,"net_short":1}],"low":[]}`))
		case r.URL.Query().Get("tab") == "net_flow":
			w.Write([]byte(`{"top":[{"amount":29309691.42,"price":64119.7,"price_delta_percent":0.23,"rank":7,"symbol":"BTCUSDT"}],"low":[]}`))
		case r.URL.Query().Get("tab") == "price":
			w.Write([]byte(`{"top":[{"pair":"BTCUSDT","symbol":"BTC","price_delta":0.031,"price":63910,"future_flow":0.8e6,"spot_flow":0.9e6,"oi":100,"oi_delta":10,"oi_delta_value":5.3e6}],"low":[]}`))
		default:
			t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer fsrv.Close()
	tr.SetBaseURL(fsrv.URL)
	e.trending = tr

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT", Sources: []string{"oi_top"}}},
	}
	if err := attachPerCoinSignals(ctx, e); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	g, ok := e.PerCoinSignalFor("BTCUSDT")
	if !ok {
		t.Fatalf("BTCUSDT not attached")
	}
	if g.AI500 == nil || g.AI500.PeakScore != 87 {
		t.Fatalf("AI500 not attached/parsed: %+v", g.AI500)
	}
	if g.OI == nil || len(g.OI) == 0 {
		t.Fatalf("OI not attached: %+v", g.OI)
	}
	if g.Netflow == nil || len(g.Netflow) == 0 {
		t.Fatalf("netflow not attached: %+v", g.Netflow)
	}
	if g.Price == nil || len(g.Price) == 0 {
		t.Fatalf("price not attached: %+v", g.Price)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./kernel/ -run TestAttachPerCoinSignals_ -v`
Expected: FAIL — `attachPerCoinSignals` undefined.

- [ ] **Step 3: Implement attachPerCoinSignals in engine_analysis.go**

```go
// attachPerCoinSignals fetches the enabled free per-coin data sources for the
// current candidate + position symbols and stores them on the engine for the
// prompt builder. Networks/sources with no data for a symbol are skipped.
func attachPerCoinSignals(ctx *Context, engine *StrategyEngine) error {
	if ctx == nil || engine == nil {
		return nil
	}
	cfg := engine.GetConfig()
	if !cfg.Indicators.EnableAI500Data && !cfg.Indicators.EnableOIData &&
		!cfg.Indicators.EnableNetflowData && !cfg.Indicators.EnablePriceData {
		return nil
	}

	symSet := make(map[string]bool)
	for _, c := range ctx.CandidateCoins {
		symSet[c.Symbol] = true
	}
	for _, p := range ctx.Positions {
		symSet[p.Symbol] = true
	}

	durations := cfg.Indicators.DataDurations
	if len(durations) == 0 {
		durations = []string{"24h"}
	}
	const limit = 50

	out := make(map[string]PerCoinSignal)

	if cfg.Indicators.EnableAI500Data {
		coins, err := engine.trending.GetAI500()
		if err == nil {
			for i := range coins {
				coins[i].Symbol = market.Normalize(coins[i].Pair)
				if !symSet[coins[i].Symbol] {
					continue
				}
				sig := out[coins[i].Symbol]
				c := coins[i]
				sig.AI500 = &c
				out[coins[i].Symbol] = sig
			}
		} else {
			logger.Warnf("⚠️ AI500 prompt data fetch failed: %v", err)
		}
	}

	oiByDur := make(map[string]map[string]nofxos.OIPosition)
	nfByDur := make(map[string]map[string]nofxos.NetFlowPosition)
	pxByDur := make(map[string]map[string]nofxos.PriceRankingItem)

	for _, dur := range durations {
		if cfg.Indicators.EnableOIData {
			env, err := engine.trending.GetOIData(dur, limit)
			if err == nil {
				oiByDur[dur] = oiListsToMap(env.Top, env.Low, symSet)
			} else {
				logger.Warnf("⚠️ OI prompt data fetch failed (%s): %v", dur, err)
			}
		}
		if cfg.Indicators.EnableNetflowData {
			env, err := engine.trending.GetNetflowData(dur, limit)
			if err == nil {
				nfByDur[dur] = netflowListsToMap(env.Top, env.Low, symSet)
			} else {
				logger.Warnf("⚠️ netflow prompt data fetch failed (%s): %v", dur, err)
			}
		}
		if cfg.Indicators.EnablePriceData {
			env, err := engine.trending.GetPriceData(dur, limit)
			if err == nil {
				pxByDur[dur] = priceListsToMap(env.Top, env.Low, symSet)
			} else {
				logger.Warnf("⚠️ price prompt data fetch failed (%s): %v", dur, err)
			}
		}
	}

	for sym := range symSet {
		sig := out[sym]
		sig.OI = oiByDur
		sig.Netflow = nfByDur
		sig.Price = pxByDur
		out[sym] = sig
	}

	engine.SetPerCoinSignals(out)
	return nil
}

func oiListsToMap(top, low []nofxos.OIPosition, symSet map[string]bool) map[string]nofxos.OIPosition {
	m := make(map[string]nofxos.OIPosition)
	for _, p := range top {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["top:"+s] = p
		}
	}
	for _, p := range low {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["low:"+s] = p
		}
	}
	return m
}
```

Add analogous `netflowListsToMap` (`amount` sign, key `"top:SYM"`/`"low:SYM"`) and `priceListsToMap`. Ensure `nofxos` and `market` are imported in `engine_analysis.go`.

- [ ] **Step 4: Wire into buildTradingContext**

In `trader/auto_trader_loop.go`, after step 11 (Price ranking, ~748) and before `return ctx`, add:

```go
	// 12. Attach free per-coin prompt data sources (candidates + positions)
	if strategyConfig.Indicators.EnableAI500Data || strategyConfig.Indicators.EnableOIData ||
		strategyConfig.Indicators.EnableNetflowData || strategyConfig.Indicators.EnablePriceData {
		if err := kernel.AttachPerCoinSignals(ctx, at.strategyEngine); err != nil {
			at.logWarnf("⚠️ Failed to attach per-coin signal data: %v", err)
		}
	}
```

> Expose `AttachPerCoinSignals` (exported wrapper) in `engine_analysis.go` that calls the internal `attachPerCoinSignals`.

- [ ] **Step 5: Run tests**

Run: `go test ./kernel/ -run TestAttachPerCoinSignals_ -v`
Expected: PASS.

- [ ] **Step 6: gofmt + vet + commit**

Run: `gofmt -l ./kernel ./trader ./store` then `go vet ./kernel/... ./trader/...`
Commit:
```bash
git add kernel/engine_analysis.go trader/auto_trader_loop.go kernel/engine_prompt_test.go
git commit -m "feat(kernel): attach and filter per-coin data sources per cycle"
```

---

### Task 5: Backend prompt renderer — per-coin source sections (candidates + positions)

**Files:**
- Modify: `kernel/engine_prompt.go` (candidate loop `formatPositionInfo`; add `formatPerCoinSignals`)
- Test: `kernel/engine_prompt_test.go`

**Interfaces:**
- Consumes: Task 3 `PerCoinSignalFor`, Task 2 nofxos types.
- Produces: `func (e *StrategyEngine) formatPerCoinSignals(symbol string, currentPrice float64) string` — rendered string; empty if coin has no data.

- [ ] **Step 1: Write failing test**

Add to `kernel/engine_prompt_test.go`:

```go
func TestFormatPerCoinSignals_rendersEnabledSources(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAI500Data = true
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.EnableNetflowData = true
	cfg.Indicators.EnablePriceData = true
	cfg.Indicators.DataDurations = []string{"1h"}
	e := NewStrategyEngine(&cfg)

	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"CYSUSDT": {
			AI500: &nofxos.CoinData{Pair: "CYSUSDT", Score: 78.3, PeakScore: 87, StartPrice: 0.5117, IncreasePercent: 180.9},
			OI:    map[string]map[string]nofxos.OIPosition{"1h": {"top:CYSUSDT": {Symbol: "CYSUSDT", Rank: 4, OIDeltaValue: 1.2e6, OIDeltaPercent: 5.3, PriceDeltaPercent: 3.1, CurrentOI: 3.85e7, NetLong: 12e6, NetShort: 9e6}}},
			Netflow: map[string]map[string]nofxos.NetFlowPosition{"1h": {"top:CYSUSDT": {Symbol: "CYSUSDT", Rank: 7, Amount: 2.4e6, Price: 1.3821}}},
			Price:   map[string]map[string]nofxos.PriceRankingItem{"1h": {"top:CYSUSDT": {Symbol: "CYSUSDT", PriceDelta: 0.031, SpotFlow: 0.9e6, FutureFlow: 0.8e6, OIDeltaValue: 5.3e6, Price: 1.3821}}},
		},
	})

	out := e.formatPerCoinSignals("CYSUSDT", 1.3821)
	if out == "" {
		t.Fatalf("expected non-empty render")
	}
	for _, want := range []string{"AI500 Signal", "AI score 78.3", "peak score 87", "since starting alert", "Open Interest", "[1h \u00b7 Increase]", "Net Flow", "inflow", "Price Change", "price change +3.1%"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
```

Also add a test `TestFormatPerCoinSignals_omitsMissing` asserting a symbol with no data yields `""`, and an OI line with a negative delta renders `[1h · Decrease]`.

- [ ] **Step 2: Run to verify fail**

Run: `go test ./kernel/ -run TestFormatPerCoinSignals_ -v`
Expected: FAIL — `formatPerCoinSignals` undefined.

- [ ] **Step 3: Implement formatPerCoinSignals**

In `kernel/engine_prompt.go`, add:

```go
func (e *StrategyEngine) formatPerCoinSignals(symbol string, currentPrice float64) string {
	cfg := e.GetConfig()
	ind := cfg.Indicators
	if !ind.EnableAI500Data && !ind.EnableOIData && !ind.EnableNetflowData && !ind.EnablePriceData {
		return ""
	}
	sig, ok := e.PerCoinSignalFor(symbol)
	if !ok || sig.AI500 == nil && len(sig.OI) == 0 && len(sig.Netflow) == 0 && len(sig.Price) == 0 {
		return ""
	}
	var sb strings.Builder

	if ind.EnableAI500Data && sig.AI500 != nil {
		sb.WriteString(fmt.Sprintf("=== %s AI500 Signal ===\n", symbol))
		sb.WriteString(fmt.Sprintf("AI score %.1f/100 | peak score %.0f | current price %.4f | start price %.4f | change %+.1f%% since starting alert\n\n",
			sig.AI500.Score, sig.AI500.PeakScore, currentPrice, sig.AI500.StartPrice, sig.AI500.IncreasePercent))
	}

	if ind.EnableOIData && len(sig.OI) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Open Interest ===\n", symbol))
		for _, dur := range e.durationOrder(ind.DataDurations) {
			if m, ok := sig.OI[dur]; ok {
				if p, ok := m["top:"+symbol]; ok {
					sb.WriteString(e.formatOIListLine(dur, "Increase", p))
				}
				if p, ok := m["low:"+symbol]; ok {
					sb.WriteString(e.formatOIListLine(dur, "Decrease", p))
				}
			}
		}
		sb.WriteString("\n")
	}

	if ind.EnableNetflowData && len(sig.Netflow) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Net Flow ===\n", symbol))
		for _, dur := range e.durationOrder(ind.DataDurations) {
			if m, ok := sig.Netflow[dur]; ok {
				if p, ok := m["top:"+symbol]; ok {
					sb.WriteString(e.formatNetflowListLine(dur, "inflow", p))
				}
				if p, ok := m["low:"+symbol]; ok {
					sb.WriteString(e.formatNetflowListLine(dur, "outflow", p))
				}
			}
		}
		sb.WriteString("\n")
	}

	if ind.EnablePriceData && len(sig.Price) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Price Change ===\n", symbol))
		for _, dur := range e.durationOrder(ind.DataDurations) {
			if m, ok := sig.Price[dur]; ok {
				if p, ok := m["top:"+symbol]; ok {
					sb.WriteString(e.formatPriceListLine(dur, p))
				}
				if p, ok := m["low:"+symbol]; ok {
					sb.WriteString(e.formatPriceListLine(dur, p))
				}
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (e *StrategyEngine) durationOrder(durs []string) []string {
	order := []string{"5m", "15m", "30m", "1h", "4h", "8h", "12h", "24h"}
	rank := map[string]int{}
	for i, d := range order {
		rank[d] = i
	}
	sort.SliceStable(durs, func(i, j int) bool { return rank[durs[i]] < rank[durs[j]] })
	return durs
}

func (e *StrategyEngine) formatOIListLine(dur, list string, p nofxos.OIPosition) string {
	return fmt.Sprintf("[%s \u00b7 %s] rank #%d | OI change %+.1f%% (%s) | price %+.1f%% | OI %s | long net %s / short net %s\n",
		dur, list, p.Rank,
		p.OIDeltaPercent, formatUSDCompact(p.OIDeltaValue),
		p.PriceDeltaPercent, formatUSDCompact(p.CurrentOI),
		formatUSDCompact(p.NetLong), formatUSDCompact(p.NetShort))
}

func (e *StrategyEngine) formatNetflowListLine(dur, dir string, p nofxos.NetFlowPosition) string {
	return fmt.Sprintf("[%s \u00b7 %s] rank #%d | net flow %s | price %.4f\n", dur, dir, p.Rank, formatUSDCompact(p.Amount), p.Price)
}

func (e *StrategyEngine) formatPriceListLine(dur string, p nofxos.PriceRankingItem) string {
	return fmt.Sprintf("[%s] price change %+.1f%% | spot %s / future %s | OI delta %s\n",
		dur, p.PriceDelta*100, formatUSDCompact(p.SpotFlow), formatUSDCompact(p.FutureFlow), formatUSDCompact(p.OIDeltaValue))
}

func formatUSDCompact(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("$%.1fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("$%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("$%.1fK", v/1e3)
	default:
		return fmt.Sprintf("$%.2f", v)
	}
}
```

Ensure `sort` and `nofxos` are imported in `kernel/engine_prompt.go` (`nofxos` may need adding).

- [ ] **Step 4: Wire into candidate + position loops**

In `BuildUserPrompt` candidate loop after the vergex block (line ~911), add:
```go
		sb.WriteString(e.formatPerCoinSignals(coin.Symbol, marketData.CurrentPrice))
```
In `formatPositionInfo`, after the vergex block (~982), add the same line using the position's `markPrice` (or `marketData.CurrentPrice` if present):
```go
		sb.WriteString(e.formatPerCoinSignals(pos.Symbol, marketData.CurrentPrice))
```
(A position always has market data in `ctx.MarketDataMap` since it was fetched; guard with the existing `if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok` block.)

- [ ] **Step 5: Run tests**

Run: `go test ./kernel/ -run 'TestFormatPerCoinSignals_|TestAttachPerCoinSignals_' -v`
Expected: PASS.

- [ ] **Step 6: gofmt + vet + commit**

Run: `gofmt -l ./kernel ./store ./trader` then `go vet ./kernel/...`
Commit:
```bash
git add kernel/engine_prompt.go kernel/engine_prompt_test.go
git commit -m "feat(kernel): render per-coin AI500/OI/netflow/price in candidate and position sections"
```

---

### Task 6: Frontend types + factory mapping

**Files:**
- Modify: `web/src/types/strategy.ts` (`IndicatorConfig`)
- Modify: `web/src/features/strategies/strategyFactory.ts` (`StrategyEditorForm`, `buildStrategyConfig`)
- Test: `web/src/features/strategies/strategyFactory.test.ts`

**Interfaces:**
- Consumes: `StrategyEditorForm` fields added here.
- Produces: `IndicatorConfig` fields `enable_ai500_data?`, `enable_oi_data?`, `enable_netflow_data?`, `enable_price_data?`, `data_durations?`; `StrategyEditorForm` adds `enableAI500Data`, `enableOIData`, `enableNetflowData`, `enablePriceData`, `enableEma`, `enableMacd`, `enableRsi`, `enableOi`, `enableFundingRate`, `durations: string[]`.

- [ ] **Step 1: Write failing tests**

In `strategyFactory.test.ts` add:

```ts
it('maps data-source toggles and durations into indicators', () => {
  const cfg = buildStrategyConfig({
    name: 'Test', custom_prompt: '', scan_interval_minutes: 15,
    btcEthMaxLeverage: 5, altcoinMaxLeverage: 5,
    btcEthPositionRatio: 5, altcoinPositionRatio: 5,
    isCrossMargin: true, selectedTimeframes: ['15m'], excludedCoins: [],
    decisionContext: { enabled: true, recent_count: 8, mode: 'structured' },
    scopeUnits: [freeUnit('gainers')], scopeMode: 'union',
    enableAI500Data: true, enableOIData: true, enableNetflowData: true,
    enablePriceData: true, dataDurations: ['15m', '1h'],
    enableEma: true, enableMacd: true, enableRsi: true,
  })
  const ind = cfg.ai_config?.indicators
  expect(ind?.enable_ai500_data).toBe(true)
  expect(ind?.enable_oi_data).toBe(true)
  expect(ind?.enable_netflow_data).toBe(true)
  expect(ind?.enable_price_data).toBe(true)
  expect(ind?.data_durations).toEqual(['15m', '1h'])
  expect(ind?.enable_ema).toBe(true)
  expect(ind?.enable_macd).toBe(true)
  expect(ind?.enable_rsi).toBe(true)
})
```

- [ ] **Step 2: Run to verify fail**

Run: `cd web && npx vitest run src/features/strategies/strategyFactory.test.ts`
Expected: FAIL — type/field unused or undefined.

- [ ] **Step 3: Add TS types**

In `web/src/types/strategy.ts` `IndicatorConfig`, add:
```ts
  // Free per-coin data sources exposed to the LLM prompt (optional toggles)
  enable_ai500_data?: boolean;
  enable_oi_data?: boolean;
  enable_netflow_data?: boolean;
  enable_price_data?: boolean;
  data_durations?: string[];
```

- [ ] **Step 4: Update StrategyEditorForm + buildStrategyConfig**

Add to `StrategyEditorForm`:
```ts
  enableAI500Data?: boolean
  enableOIData?: boolean
  enableNetflowData?: boolean
  enablePriceData?: boolean
  dataDurations?: string[]
  enableEma?: boolean
  enableMacd?: boolean
  enableRsi?: boolean
  enableOi?: boolean
  enableFundingRate?: boolean
```

In `buildStrategyConfig`, set the indicator values from the form (replacing the hardcoded `false`, keeping defaults for unchanged ones), e.g.:
```ts
        enable_ema: form.enableEma ?? false,
        enable_macd: form.enableMacd ?? false,
        enable_rsi: form.enableRsi ?? false,
        enable_oi: form.enableOi ?? false,
        enable_funding_rate: form.enableFundingRate ?? false,
        enable_ai500_data: form.enableAI500Data ?? false,
        enable_oi_data: form.enableOIData ?? false,
        enable_netflow_data: form.enableNetflowData ?? false,
        enable_price_data: form.enablePriceData ?? false,
        data_durations: form.dataDurations && form.dataDurations.length ? form.dataDurations : undefined,
```

Keep all previously hardcoded `false` for the indicators not surfaced (atr, boll, volume, quant, ranking).

- [ ] **Step 5: Run tests**

Run: `cd web && npx vitest run src/features/strategies/strategyFactory.test.ts && npx tsc --noEmit`
Expected: PASS, no type errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/types/strategy.ts web/src/features/strategies/strategyFactory.ts web/src/features/strategies/strategyFactory.test.ts
git commit -m "feat(web): map per-coin data-source toggles and durations into strategy config"
```

---

### Task 7: Frontend editor — toggles + duration multiselect + auto-default

**Files:**
- Modify: `web/src/features/strategies/EditorStepPage.tsx`
- Test: `web/src/features/strategies/EditorStepPage.test.tsx` (create if missing)

**Interfaces:**
- Consumes: Task 6 form fields; `scope.units` from draft store.
- Produces: editor UI wiring the new form fields into `buildStrategyConfig` and auto-defaulting sources from selected scope.

- [ ] **Step 1: Write failing tests** (if an EditorStepPage test file exists, extend; otherwise create a focused one)

Add assertions that:
- auto-default: when scope has an `ai500` unit and no AI500 toggle set, the resulting form passes `enableAI500Data: true`.
- equivalence for `nofxos_oi`→OI, `nofxos_netflow`→Netflow, `nofxos_price`→Price, `vergex`→none.

Implement the auto-default as a pure helper to make it testable:
```ts
export function defaultDataSources(units: ScopeUnit[]): {
  enableAI500Data: boolean; enableOIData: boolean;
  enableNetflowData: boolean; enablePriceData: boolean;
} {
  return {
    enableAI500Data: units.some((u) => u.source_type === 'ai500'),
    enableOIData: units.some((u) => u.source_type === 'nofxos_oi'),
    enableNetflowData: units.some((u) => u.source_type === 'nofxos_netflow'),
    enablePriceData: units.some((u) => u.source_type === 'nofxos_price'),
  }
}
```
Put this helper in a new module `web/src/features/strategies/dataSourceDefaults.ts` and import it in `EditorStepPage`.

- [ ] **Step 2: Run to verify fail** (helper not exported)

- [ ] **Step 3: Create helper + wire into EditorStepPage**

- Use `defaultDataSources(scope.units)` on mount/create to seed the toggle states (unless restoring an existing strategy in edit mode, in which case load from config and only fall back to defaults when the strategy has none of those fields set).
- Add a "Data sources for LLM" fieldset with `ToggleChip`/`Toggle` controls for each source and the basic indicators.
- Add a duration multiselect (15m/30m/1h/4h/8h/12h/24h) shown/enabled when any of AOI/Netflow/Price is on; default to `['1h','24h']`.
- Pass all new fields into `buildStrategyConfig`.

- [ ] **Step 4: Run tests**

Run: `cd web && npx vitest run src/features/strategies && npx tsc --noEmit && npm run build`
Expected: PASS, no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/strategies/EditorStepPage.tsx web/src/features/strategies/dataSourceDefaults.ts web/src/features/strategies/EditorStepPage.test.tsx
git commit -m "feat(web): per-coin data source toggles, durations, and scope auto-default in strategy editor"
```

---

### Task 8: Full verification

- [ ] **Step 1: Backend build + vet + fmt**

Run: `go build ./... && go vet ./... && gofmt -l .`
Expected: clean.

- [ ] **Step 2: Backend tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 3: Frontend checks**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: PASS.

- [ ] **Step 4: Manual end-to-end sanity**

Create an AI500 strategy via the UI with AI500 + OI data + `[1h, 24h]`; run a cycle and inspect the LLM user prompt (in the LLM decisions log) to confirm the per-coin sections appear, and that coins without rows omit them. Confirm the same applies to an open position's section.
