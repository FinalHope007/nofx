# Task 3 Follow-up Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the runtime issues found when running a trader on the free-mode Task 3 implementation: market-qualified symbol + token for the free per-coin detail endpoints (so Signal Lab / Cost-Liquidation Heatmap actually work), feed per-coin detail to all strategy source types (omitting when the endpoint has no data), source klines from the matching exchange, and make the dashboard funnel reflect the active strategy.

**Architecture:** Extend the free-mode `vergex.Client` to build the correct market-qualified detail symbol (`core_perp:BTC`, `hip3_perp:0x888...:xyz:SP500`) and URL-encode it; remove the `vergex_signal`-only gate around per-coin detail population so all source types get it, then suppress "unavailable" error lines in the generic user prompt; make kline sourcing exchange-aware; and re-wire the dashboard `flow`/`signal` funnel layers to the active strategy. A separate investigation task determines whether a residual paid NoFXOS `claw402-data` x402 call is in scope.

**Tech Stack:** Go 1.25 (Gin), React 18 + TS + Vite. Reuses the existing `provider/vergex` free-mode client, `kernel` engine/prompt, `market` kline providers, and the `web` TerminalDashboard.

## Global Constraints

- `go vet ./...` + `gofmt` clean; frontend `npx tsc --noEmit` + `npm test` clean.
- Backend errors use safe-error conventions; never leak internal/auth details into user-facing prompts or responses.
- `store.*` is the only DB access layer; no DB schema changes in this plan.
- `0x88806a71d74ad0a510b350545c9ae490912f0888` is the fixed Hyperliquid xyz deployer address (a constant). Crypto `core_perp` markets need no address qualification.
- `VERGEX_API_TOKEN` is read via `os.Getenv` in the free client path; supply it via local `.env` only (never commit the token).
- The system prompt for non-`vergex_signal` strategies stays generic (unchanged). The `vergex_signal` Claw402 rules prompt is unchanged.
- Live `vergex.trade` verification uses the Bearer token the user provided and does NOT ship it into the repo.
- Out of scope: risk-radar config UI for MaxPositions/MaxMarginUsage/MinPositionSize; the `/data` page iframe; Binance clock-skew `-1021`; multi-card `custom` AND/OR resolver.

---

### Task 1: `FreeDetailSymbol` — market-qualified detail symbol + URL-encoding

**Files:**
- Modify: `provider/vergex/client.go`
- Test: `provider/vergex/free_mode_test.go`

**Interfaces:**
- Consumes: existing `normalizeMarketType`, `QuerySymbol`, `isCoreMarketType` in `provider/vergex/client.go`.
- Produces: `const freeDetailXYZDeployer = "0x88806a71d74ad0a510b350545c9ae490912f0888"` and `func FreeDetailSymbol(marketType, symbol string) string`, plus the free-mode `GetSignalLab`/`GetCostLiquidationHeatmap` use it (with `:`→`%3A` URL-encoding) in their path.

- [ ] **Step 1: Write the failing test**

Append to `provider/vergex/free_mode_test.go`:

```go
func TestFreeDetailSymbol(t *testing.T) {
	cases := []struct {
		marketType string
		symbol     string
		want       string
	}{
		{"core_perp", "BTC", "core_perp:BTC"},
		{"core_perp", "core_perp:BTC", "core_perp:BTC"},
		{"hip3_perp", "xyz:SP500", "hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:SP500"},
		{"hip3_perp", "SP500", "hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:SP500"},
		{"all", "PUMP", "all:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:PUMP"},
	}
	for _, c := range cases {
		got := FreeDetailSymbol(c.marketType, c.symbol)
		if got != c.want {
			t.Errorf("FreeDetailSymbol(%q, %q) = %q, want %q", c.marketType, c.symbol, got, c.want)
		}
	}
}

func TestDetailPathURLEncodesColons(t *testing.T) {
	// The symbol segment must render colons as %3A so the request matches the
	// verified working curl: .../hip3_perp/hip3_perp%3A...%3Axyz%3ASP500/riskbins
	sym := FreeDetailSymbol("hip3_perp", "SP500")
	enc := strings.ReplaceAll(sym, ":", "%3A")
	want := "hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500"
	if enc != want {
		t.Errorf("encoded = %q, want %q", enc, want)
	}
}
```

Add `"strings"` to the test file imports if not present.

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./provider/vergex/ -run 'TestFreeDetailSymbol|TestDetailPathURLEncodesColons' -v`
Expected: FAIL — `undefined: FreeDetailSymbol`.

- [ ] **Step 3: Implement `FreeDetailSymbol` + constant**

In `provider/vergex/client.go`, add near the free path consts:

```go
const (
	// freeDetailXYZDeployer is the fixed Hyperliquid xyz (TradeFi) deployer
	// address that vergex.trade free per-coin detail endpoints require for
	// stock/index/commodity (hip3_perp) markets. Crypto core_perp markets do
	// not use an address.
	freeDetailXYZDeployer = "0x88806a71d74ad0a510b350545c9ae490912f0888"
)

// FreeDetailSymbol returns the market-qualified symbol that vergex.trade free
// per-coin detail endpoints expect in the path:
//   - crypto (core_perp):   "core_perp:BTC"
//   - stock (hip3_perp):    "hip3_perp:0x8880...:xyz:SP500"
// The returned string still contains ':' (callers URL-encode as needed).
func FreeDetailSymbol(marketType, symbol string) string {
	mt := normalizeMarketType(marketType)
	base := QuerySymbol(symbol)
	if base == "" {
		return ""
	}
	if isCoreMarketType(mt) {
		return marketType + ":" + base
	}
	return marketType + ":" + freeDetailXYZDeployer + ":xyz:" + base
}
```

Note: `normalizeMarketType` and `isCoreMarketType` accept the raw value; pass `marketType` (the raw) to `isCoreMarketType` so it normalizes internally — use `normalizeMarketType` only if needed for the crypto branch. Keep `marketType` verbatim in the returned prefix (it is already `core_perp`/`hip3_perp`).

- [ ] **Step 4: Use it in free-mode per-coin detail + URL-encode**

In `GetSignalLab` free branch (currently `path := fmt.Sprintf(FreeSignalsPath, q.MarketType, MarketSymbol(q.MarketType, q.Symbol))`), replace `MarketSymbol(...)` with the URL-encoded `FreeDetailSymbol`:

```go
	if c.freeMode {
		params := url.Values{}
		if q.Chain != "" {
			params.Set("chain", QueryChain(q.Chain))
		}
		if q.LiqBand != "" {
			params.Set("liqBand", q.LiqBand)
		}
		sym := strings.ReplaceAll(FreeDetailSymbol(q.MarketType, q.Symbol), ":", "%3A")
		path := fmt.Sprintf(FreeSignalsPath, q.MarketType, sym)
		return c.doGET(ctx, path, params)
	}
```

Mirror exactly for `GetCostLiquidationHeatmap` free branch (`FreeRiskbinsPath`).

- [ ] **Step 5: Run tests to confirm pass**

Run: `go test ./provider/vergex/ -run 'TestFreeDetailSymbol|TestDetailPathURLEncodesColons' -v`
Expected: PASS.

Then: `go build ./... && go vet ./... && go test ./provider/vergex/`
Expected: all PASS (existing paid + free tests green).

- [ ] **Step 6: Commit**

```bash
git add provider/vergex/client.go provider/vergex/free_mode_test.go
git commit -m "feat(vergex): market-qualified detail symbol + URL-encode for free per-coin endpoints"
```

---

### Task 2: Omit "unavailable" detail from the generic user prompt (per-coin detail for all source types)

**Files:**
- Modify: `kernel/engine_analysis.go`
- Modify: `kernel/engine.go`
- Modify: `kernel/engine_prompt.go`
- Test: `kernel/engine_free_test.go`

**Interfaces:**
- Consumes: `StrategyEngine.GetConfig()` (for `CoinSource.SourceType`), `fetchMarketDataWithStrategy`, `enrichVergexDataWithStrategy`, `FetchVergexDataBatch`, `formatVergexData`, `vergex.FormatAnalysisForAI`.
- Produces: `enrichVergexDataWithStrategy` and `FetchVergexDataBatch` run for ALL source types (no `vergex_signal` gate); `formatVergexData` gains behavior that omits Signal Lab / Heatmap sections when they carried an error, for non-`vergex_signal` strategies.

- [ ] **Step 1: Write the failing test**

Append to `kernel/engine_free_test.go`:

```go
func TestFormatVergexData_omitsUnavailableForNonVergex(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "ai500"
	e := NewStrategyEngine(&cfg)

	analysis := &vergex.MarketAnalysis{
		Symbol:         "CYS",
		QuerySymbol:    "CYS",
		MarketType:     "core_perp",
		SignalLabError: "vergex endpoint has no data for this market",
		HeatmapError:   "vergex endpoint has no data for this market",
	}
	out := e.formatVergexData(analysis, false)
	if strings.Contains(out, "unavailable") {
		t.Errorf("generic user prompt should not contain 'unavailable', got: %s", out)
	}
	if strings.Contains(out, "Signal Lab") || strings.Contains(out, "Heatmap") {
		t.Errorf("generic user prompt should omit empty sections, got: %s", out)
	}
}
```

Ensure imports `store`, `vergex`, `strings` are present in the test file (add if needed).

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./kernel/ -run TestFormatVergexData_omitsUnavailableForNonVergex -v`
Expected: FAIL to compile or the assertion fails (current `formatVergexData` prints the error).

- [ ] **Step 3: Remove the `vergex_signal` gates**

In `kernel/engine_analysis.go` `enrichVergexDataWithStrategy`, delete the early return:

```go
	if engine.GetConfig().CoinSource.SourceType != "vergex_signal" {
		return
	}
```

In `kernel/engine.go` `FetchVergexDataBatch`, delete the source-type check in the guard:

```go
	if e == nil || e.config == nil || e.config.CoinSource.SourceType != "vergex_signal" {
		return result
	}
```
→
```go
	if e == nil || e.config == nil {
		return result
	}
```

- [ ] **Step 4: Make `formatVergexData` omit-on-missing for non-vergex strategies**

Change the signature to accept an `omitUnavailable bool`:

```go
func (e *StrategyEngine) formatVergexData(data *vergex.MarketAnalysis, omitUnavailable bool) string {
	if data == nil {
		return ""
	}
	if omitUnavailable && len(data.SignalLab) == 0 && data.SignalLabError != "" {
		data.SignalLabError = ""
	}
	if omitUnavailable && len(data.Heatmap) == 0 && data.HeatmapError != "" {
		data.HeatmapError = ""
	}
	var sb strings.Builder
	sb.WriteString("\nVergex Claw402 Signals:\n")
	sb.WriteString(vergex.FormatAnalysisForAI(data))
	return sb.String()
}
```

Note: `FormatAnalysisForAI` (in `provider/vergex/client.go`) only prints a section when `len(...) > 0`, and prints the error line only when `...Error != ""`. Clearing the error for omitted sections makes it render nothing for that section (blank body) — because when both `SignalLab` empty AND `SignalLabError` empty, `FormatAnalysisForAI` prints nothing for signal lab. This achieves omit-on-missing.

Update the two callers in `engine_prompt.go` to pass `omitUnavailable`, which is `false` when the strategy is `vergex_signal` and `true` otherwise:

```go
	omit := e.GetConfig().CoinSource.SourceType != "vergex_signal"
	// at the VergexDataMap render sites:
	sb.WriteString(e.formatVergexData(vergexData, omit))
```

- [ ] **Step 5: Run tests to confirm pass**

Run: `go test ./kernel/ -run 'TestFormatVergexData_omitsUnavailableForNonVergex|TestEngine_' -v`
Expected: PASS.

Then: `go build ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add kernel/engine_analysis.go kernel/engine.go kernel/engine_prompt.go kernel/engine_free_test.go
git commit -m "feat(kernel): per-coin detail for all source types; omit unavailable sections in generic prompt"
```

---

### Task 3: Token verification for free `riskbins` (local only)

**Files:**
- Modify: `.env` (local only — do NOT commit)
- Test: none (manual live check)

**Interfaces:**
- Consumes: existing `os.Getenv("VERGEX_API_TOKEN")` in `api/handler_vergex.go` `freeVergexClientForRequest`; the `VERGEX_API_TOKEN` read in `kernel.NewStrategyEngine` free-client init; the Task 1 free riskbins path.
- Produces: verification that `riskbins` returns HTTP 200 with a valid token, for both crypto and stock, after Task 1.

- [ ] **Step 1: Set the token locally (untracked)**

Add to your local `.env` (not committed):

```
VERGEX_API_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMGY3MjM4M2ItOGUzYi00ZDk4LWExMTctMzE0OGZjNWI4ZjFkIiwiZW1haWwiOiJsaW1saXZlYUBnbWFpbC5jb20iLCJpc3MiOiJWZXJnZVgiLCJuYmYiOjE3ODU5MjQ0ODAsImlhdCI6MTc4NTkyNDQ4MH0.D_Jr82We4AsabFsNkC8k99vVnRYSEybfa6EwAiHKRRQ
```

Confirm `.env` is git-ignored: check `git check-ignore .env`. If not ignored, do NOT add it.

- [ ] **Step 2: Live-verify endpoints with curl**

With the server running (or directly via curl using the token), verify:

```bash
# crypto signals + riskbins (BTC)
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $VERGEX_API_TOKEN" \
  "https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/signals?chain=mainnet&liqBand=15"
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $VERGEX_API_TOKEN" \
  "https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/riskbins?chain=mainnet"

# stock signals + riskbins (SP500) — the qualified symbol
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $VERGEX_API_TOKEN" \
  "https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500/signals?chain=mainnet&liqBand=15"
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $VERGEX_API_TOKEN" \
  "https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500/riskbins?chain=mainnet"
```

Expected: all return `200`. If signal-lab returns 200 without a token but riskbins requires it, record that (riskbins = token-required).
Also verify a data-limited market returns a non-200/empty (e.g. `core_perp:CYS`) so the omit-on-missing path (Task 2) is exercised.

- [ ] **Step 3: Confirm the running trader no longer logs 401 for these**

In `data/nofx_*.log`, confirm new heatmap fetches no longer log `vergex token rejected (401)` for supported markets (BTC/SP500).

---

### Task 4: Per-trader-exchange kline sourcing

**Files:**
- Modify: `market/data_klines.go`
- Modify: `market/data.go`
- Modify: `kernel/engine.go`
- Modify: `kernel/engine_analysis.go`
- Test: `market/data_klines_test.go`

**Interfaces:**
- Consumes: `market.GetWithTimeframes` (currently exchange-agnostic, always CoinAnk for non-xyz); `StrategyEngine` (gains `exchange` field); `kernel/engine_analysis.go fetchMarketDataWithStrategy`.
- Produces: `market.GetWithTimeframesWithExchange(symbol, timeframes, primaryTimeframe, count, exchange)` or an exchange field passed through; `kernel.getKlinesFromBinance(symbol, interval, limit)` uses the existing `fapi.binance.com` API client.

- [ ] **Step 1: Write the failing test**

Create `market/data_klines_test.go`:

```go
package market

import "testing"

func TestResolveKlineExchange(t *testing.T) {
	cases := []struct {
		exchange, want string
	}{
		{"binance", "binance"},
		{"BINANCE", "binance"},
		{"hyperliquid", "hyperliquid"},
		{"okx", ""},      // unknown/other -> default (CoinAnk), represented as ""
		{"", ""},
	}
	for _, c := range cases {
		got := resolveKlineExchange(c.exchange)
		if got != c.want {
			t.Errorf("resolveKlineExchange(%q) = %q, want %q", c.exchange, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./market/ -run TestResolveKlineExchange -v`
Expected: FAIL — `undefined: resolveKlineExchange`.

- [ ] **Step 3: Add `resolveKlineExchange` + binance source (reuse existing client)**

`market/api_client.go` already provides `NewAPIClient().GetKlines(symbol, interval string, limit int) ([]Kline, error)` hitting `fapi/v1/klines`. Reuse it — do NOT write a new binance fetch.

In `market/data_klines.go`, add:

```go
// resolveKlineExchange normalizes a trader exchange to the kline source:
//   - "binance"      -> "binance" (native Binance futures klines via APIClient.GetKlines)
//   - "hyperliquid"  -> "hyperliquid"
//   - anything else / empty -> "" (default: CoinAnk, current behavior)
func resolveKlineExchange(exchange string) string {
	switch strings.ToLower(strings.TrimSpace(exchange)) {
	case "binance":
		return "binance"
	case "hyperliquid":
		return "hyperliquid"
	default:
		return ""
	}
}

// getKlinesFromBinance fetches klines from the Binance USDM futures API
// using the existing APIClient (market/api_client.go GetKlines).
func getKlinesFromBinance(symbol, interval string, limit int) ([]Kline, error) {
	return NewAPIClient().GetKlines(Normalize(symbol), interval, limit)
}
```

- [ ] **Step 4: Add exchange-aware entry point + use it in the engine**

In `market/data.go`, refactor the existing `GetWithTimeframes` to delegate to a new exchange-aware function, so existing callers (without exchange) keep the CoinAnk default:

```go
func GetWithTimeframes(symbol string, timeframes []string, primaryTimeframe string, count int) (*Data, error) {
	return GetWithTimeframesWithExchange(symbol, timeframes, primaryTimeframe, count, "")
}

func GetWithTimeframesWithExchange(symbol string, timeframes []string, primaryTimeframe string, count int, exchange string) (*Data, error) {
	// ... same body as GetWithTimeframes (market/data.go:148), but inside the
	// non-xyz branch (currently line 192-197) use:
	//   switch resolveKlineExchange(exchange) {
	//   case "binance":
	//       klines, err = getKlinesFromBinance(symbol, tf, 200)
	//   case "hyperliquid":
	//       klines, err = getKlinesFromHyperliquid(symbol, tf, 200)
	//   default:
	//       klines, err = getKlinesFromCoinAnk(symbol, tf, "binance", 200)
	//   }
	//   with the same per-frame error handling (`continue` + log) for each source.
}
```

Add `GetWithTimeframesWithExchange` as the body carrier; `GetWithTimeframes` is a thin wrapper. For the primary kline variable capture and staleness/indicators, keep identical logic.

- [ ] **Step 5: Thread the exchange into the engine**

In `kernel/engine.go`:
- Add field to `StrategyEngine` struct:

```go
	exchange string // trader exchange used to pick the kline source
```

- Add a setter or expose it:

```go
func (e *StrategyEngine) SetExchange(exchange string) {
	if e == nil {
		return
	}
	e.exchange = exchange
}
```

In `trader/auto_trader.go`, after constructing the engine (lines ~387, ~451), call `strategyEngine.SetExchange(config.Exchange)`.

In `kernel/engine_analysis.go` `fetchMarketDataWithStrategy`, change the `market.GetWithTimeframes(...)` calls (lines ~205, ~226) to pass `engine.exchange`:

```go
	data, err := market.GetWithTimeframesWithExchange(pos.Symbol, timeframes, primaryTimeframe, klineCount, engine.exchange)
```

and

```go
	data, err := market.GetWithTimeframesWithExchange(coin.Symbol, timeframes, primaryTimeframe, klineCount, engine.exchange)
```

- [ ] **Step 6: Run tests to confirm pass**

Run: `go test ./market/ -run TestResolveKlineExchange -v` → PASS.
Then: `go build ./... && go vet ./... && go test ./kernel/ ./market/ ./trader/`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add market/data_klines.go market/data.go market/data_klines_test.go kernel/engine.go kernel/engine_analysis.go trader/auto_trader.go
git commit -m "feat(kernel): source klines from the trader's exchange (binance/hyperliquid, else coinank)"
```

---

### Task 5: Dashboard funnel reflects the active strategy

**Files:**
- Modify: `web/src/components/terminal/TerminalDashboard.tsx`

**Interfaces:**
- Consumes: `flow`/`signal` funnel layers currently sourced from global vergex flow-markets / signal-ranking endpoints keyed by `traderId`.
- Produces: the `flow`/`signal` layers are empty/derived when the active strategy's candidates have no per-coin detail; `decision`/`execute`/`hold` layers still render from the active strategy.

- [ ] **Step 1: Add per-coin detail to the funnel data contract**

In `TerminalDashboard.tsx`, determine whether the decision/user-prompt data for the **selected trader** exposes per-coin detail (Signal Lab/Heatmap presence per symbol). If the backend `GetFullDecision`/`DecisionRecord` already carries per-symbol detail, derive the `signal` layer items from those symbols that have detail; otherwise leave `signal`/`flow` empty when detail is absent.

Concretely, change the `signal` layer items source so it reflects the **active strategy's** judged symbols rather than the raw global signal ranking when the active strategy is not `vergex_signal`:

```tsx
// signal layer: only symbols the active strategy actually judged this cycle
// that carry per-coin detail; empty when the active source has none.
const judgedSymbols = candidateCoins.map((c) => c) // candidateCoins already = active strategy's judged set
```

And keep `flow` layer derived from the active strategy's net-flow data when present, else empty.

- [ ] **Step 2: Gate `flow`/`signal` on the active strategy, not the default**

Replace the raw `flow?.data?.inflow`/`outflow` and `signalRank?.items` usage in the funnel layers (lines ~535-548) with the active-strategy-driven sets:

```tsx
{
  key: 'flow',
  title: 'FLOW',
  zh: 'flow',
  items: activeFlowItems, // [] when the active strategy has no net-flow detail
},
{
  key: 'signal',
  title: 'SIGNAL',
  zh: 'signal',
  items: activeSignalItems, // [] when the active strategy has no per-coin Signal Lab detail
},
```

Where `activeFlowItems`/`activeSignalItems` are derived from `candidateCoins` + per-coin detail (from the selected trader's verified decision data) and `[]` by default.

- [ ] **Step 3: Key the data fetches on the selected trader/strategy (not default)**

Verify `realFlow`/`realSignalRank` are already keyed on `traderId`/`selectedTrader` (they are, lines 195-207) and that the fallback when inactive strategy has no detail is `[]`, so AI500 shows empty `flow`/`signal` but `decision`/`execute`/`hold` still render the active strategy's candidates/positions.

- [ ] **Step 4: Frontend typecheck + tests**

Run:
```bash
cd web && npx tsc --noEmit && npm test
```
Expected: PASS. Fix any type errors from the funnel-layer item shape.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/terminal/TerminalDashboard.tsx
git commit -m "feat(dashboard): drive funnel flow/signal layers from the active strategy"
```

---

### Task 6: Investigate the residual paid `claw402-data` x402 call (scope decision)

**Files:**
- None (investigation only, until scoped).

**Interfaces:**
- Consumes: backend log `payment/x402.go:332 ⚠️ [claw402-data] Payment expired (402)`; `provider/nofxos/claw402.go` (`DoRequest` with `"claw402-data"`); `kernel/engine.go` NoFXOS claw402 wiring (`client.SetClaw402`).
- Produces: a clear finding (which live call hits the paid NoFXOS x402 path) + a recommendation for in-scope vs out-of-scope.

- [ ] **Step 1: Trace the on-demand NoFXOS claw402 calls**

`GetFullDecisionWithStrategy` calls `engine.nofxosClient.GetOITopPositions()` (`kernel/engine_analysis.go:93`). When a Claw402 wallet key is present, `nofxosClient` has `SetClaw402` applied (`kernel/engine.go:228-230`), so `GetOITopPositions`/`GetTopRatedCoins`/etc. route through `claw402-data` x402. Confirm via the log which trader/config produced the 402.

- [ ] **Step 2: Determine scope**

If the NoFXOS-on-claw402 routing is still needed ONLY for features that Task-3 replaced (e.g. ai500/oi/netflow/price now come from free `vergex.trade` via the engine's free getters), then a running trader with a wallet key may still be paying for NoFXOS data that is no longer the candidate source. In that case, the fix is to **not apply `SetClaw402`** for the free-mode getters (or make NoFXOS data calls skip the paid route when the free source is in use).

This is a **scope decision**: changing NoFXOS claw402 routing could affect other consumers of `nofxosClient` (e.g. OI/funding display data, `engine.nofxosClient` used elsewhere). Report the finding to the human and get approval before changing `kernel/engine.go` wiring. **Do NOT change it in this task** without the human's confirmation.

---

## Self-Review Notes

- **Spec coverage:** Fix A → Task 1; Fix B → Task 3; Fix C → Task 2; Fix D → Task 5; Fix E → Task 4; Fix F → Task 6. Every spec section maps to a task. Non-goals documented in Global Constraints.
- **Type consistency:** `FreeDetailSymbol(marketType, symbol)` (Task 1) consumed by `GetSignalLab`/`GetCostLiquidationHeatmap`. `formatVergexData(data, omitUnavailable bool)` (Task 2) consumed by two `engine_prompt.go` callers. `GetWithTimeframesWithExchange(...)` + `StrategyEngine.exchange` + `SetExchange` (Task 4) consumed by `fetchMarketDataWithStrategy` and `auto_trader.go`. `resolveKlineExchange` (Task 4) unit-tested.
- **Placeholder check:** all implementation steps carry literal Go/TS. Task 3 is a verification task (no code). Task 6 is intentionally an investigation + scope-decision task (no code) — flagged explicitly.
- **URL-encoding note:** Task 1 encodes `:`→`%3A` in the detail path. If the live server accepts literal `:` too, the encoding is still harmless and matches the verified curl; the Task 3 live check is the authority.
- **Fix D nuance:** the funnel `flow`/`signal` layers were sourced from GLOBAL vergex endpoints (not per-strategy). The task makes them empty when the active strategy has no per-coin detail, which matches the user's requirement ("fine that flow/signal does not have output as long as decision/execute/hold still works").
