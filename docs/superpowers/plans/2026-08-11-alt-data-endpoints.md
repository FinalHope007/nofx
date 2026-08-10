# Replace Paid Data Sources with Free `vergex.trade` Endpoints — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the runtime dependency on the paywalled `claw402.ai`/`CLAW402_WALLET_KEY` data path by routing every data source (candidate pool, per-coin detail, prompt context, and the four frontend vergex handlers) to free, publicly accessible `vergex.trade` endpoints, and fix the scope-card lossy-collapse so every card resolves to the correct free endpoint.

**Architecture:** A new `provider/vergex/free.go` `FreeClient` fetches `vergex.trade` api/v1 endpoints (signal leaderboard, stock hot/movers, per-coin signals/riskbins/summary/direction-change/flow-markets), reusing the existing `ParseSignalRanking` + `SignalRankItem` types. A new `provider/nofxos/free.go` `FreeTrendingClient` fetches the `vergex.trade/trending-crypto` endpoints (OI/netflow/price) and reuses the existing strict structs. The engine getters and vergex handlers route through these free clients; a configurable Bearer token (optional) is sent on authed detail calls. A unified `variant` field is added to the frontend `ScopeUnit` so each scope card carries its rank-direction qualifier, and `buildCoinSource` emits the correct backend `source_type` (`oi_top`/`oi_low`/`netflow_top`/`netflow_low`/`price_top`/`price_low`/`vergex_signal`+`vergex_direction`). The paid-only vergex client stays untouched; it is simply no longer required for these sources.

**Tech Stack:** Go 1.25 (Gin, GORM), React 18 + TS + Vite frontend. Verified live endpoints documented in `data-alt-endpoints.md`.

## Global Constraints

- `gofmt` + `go vet ./...` clean; frontend `npx tsc --noEmit` clean.
- Backend errors use the existing safe-error conventions (`SafeError`/`SafeInternalError`, `fmt.Errorf("...: %w", err)`, `logger.Warnf`); never leak internal details.
- All timestamps UTC (no new timestamps introduced by this plan).
- `store.*` remains the only DB access layer — this plan only adds config fields read/written via the existing `CoinSourceConfig` (already persisted), **no new DB tables**.
- No live order/position behavior changes (HIGH-RISK surface — untouched).
- `vergex.trade` endpoints from `data-alt-endpoints.md`; the Bearer token has no `exp` claim — must degrade gracefully on 401.
- Do NOT modify the existing Claw402 `provider/vergex/client.go` paid methods; add free clients alongside. No deletion of paid code (may stay for not-yet-replaced surfaces).
- New `ScopeUnit` field is `variant` (replaces `direction`); TypeScript union updated atomically with its consumers.

## File Structure

**Frontend (modified):**
- `web/src/types/strategy.ts` — `ScopeUnit`: replace `direction` with `variant`; add `CoinSourceConfig` source_type values.
- `web/src/features/strategies/scopeCatalog.ts` — add `variant` to `ScopeCardDef` + every card; `toScopeUnit`.
- `web/src/features/strategies/strategyFactory.ts` — `buildCoinSource` maps `variant` → correct `source_type`+qualifier.

**Backend — new free clients:**
- `provider/vergex/free.go` — `FreeClient` (vergex.trade api/v1; optional bearer token).
- `provider/nofxos/free.go` — `FreeTrendingClient` (vergex.trade/trending-crypto).
- `provider/vergex/free.go` reuses `ParseSignalRanking`/`SignalRankItem` from `client.go`.
- Tests: `provider/vergex/free_test.go`, `provider/nofxos/free_test.go`.

**Backend — schema (modified):**
- `store/strategy.go` — `CoinSourceConfig.VergexDirection` field; extend `normalizeCoinSourceType`, `NormalizeProductSchema`, `inferCoinSourceType` for netflow/price source types.
- `kernel/schema.go` — no change (source types are strings; validation in store).
- Tests: `store/strategy_schema_test.go`.

**Backend — engine (modified):**
- `kernel/engine.go` — hold a `*vergex.FreeClient` and `*nofxos.FreeTrendingClient`; add `GetCandidateCoins` cases for `netflow_top`/`netflow_low`/`price_top`/`price_low`; add `vergex_direction` routing in `getVergexSignalCoins`; change `getOITopCoins`/`getOILowCoins`/`getAI500Coins`/netflow/price to use free clients; add free fetchers for per-coin detail.
- Tests: `kernel/engine_*_test.go`.

**Backend — handlers (modified):**
- `api/handler_vergex.go` — route through free client + optional token; remove hard wallet requirement.
- `api/server.go` — update route doc text (cosmetic).
- `api/handler_trader.go` — if the create/update path seeds vergex fields, ensure no new hard requirement (verify only).

**Backend — config (modified):**
- `config/config.go` — add `VERGEX_API_TOKEN` env (optional) loaded once.
- Tests: `api/handler_vergex_test.go`, `api/server_test.go` (regression: vergex routes no longer 400 without wallet).

---

### Task 1: ScopeCardDef + ScopeUnit `variant` (frontend types)

**Files:**
- Modify: `web/src/types/strategy.ts`
- Modify: `web/src/features/strategies/scopeCatalog.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `ScopeUnit` gains `variant?: ScopeVariant` where `type ScopeVariant = 'bull'|'bear'|'trending'|'gainers'|'losers'|'top'|'low'|'inflow'|'outflow'`. `ScopeCardDef` gains `variant: ScopeVariant`. `toScopeUnit(def, limit): ScopeUnit` copies `variant`. `CoinSourceConfig.source_type` union gains `'netflow_top'|'netflow_low'|'price_top'|'price_low'`.

- [ ] **Step 1: Update `ScopeUnit` and source_type union**

In `web/src/types/strategy.ts`, replace lines 226-246 (the `ScopeUnit` interface) and add the new source_types. New `ScopeUnit`:

```ts
// A single selected trading-scope card (from the scope wizard step).
export type ScopeVariant =
  | 'bull' | 'bear' | 'trending'   // vergex bias + stock trending
  | 'gainers' | 'losers'           // hyper_rank / vergex movers / price
  | 'top' | 'low'                  // oi top/low, netflow, price (rank array)
  | 'inflow' | 'outflow'           // netflow direction
  | 'volume'                       // hyper_rank volume

export interface ScopeUnit {
  id: string
  category: 'crypto' | 'stock'
  source_type:
    | 'hyper_rank'
    | 'ai500'
    | 'oi_top'
    | 'oi_low'
    | 'vergex'
    | 'nofxos_netflow'
    | 'nofxos_oi'
    | 'nofxos_price'
    | 'other'
  // Per-source rank-direction qualifier so the backend can pick the exact
  // free endpoint (and top/low array) for the chosen card.
  variant?: ScopeVariant
  limit: number
  label: string
  provider: 'free' | 'paid'
}
```

In the same file, `CoinSourceConfig.source_type` union (lines 108-119) becomes:

```ts
  source_type:
    | 'static'
    | 'ai500'
    | 'oi_top'
    | 'oi_low'
    | 'netflow_top'
    | 'netflow_low'
    | 'price_top'
    | 'price_low'
    | 'hyper_all'
    | 'hyper_main'
    | 'hyper_rank'
    | 'vergex_signal'
    | 'custom'
```

- [ ] **Step 2: Update `scopeCatalog.ts` — `ScopeCardDef` and `toScopeUnit`**

In `web/src/features/strategies/scopeCatalog.ts`, change `ScopeCardDef` (lines 3-12): replace `direction?: 'gainers' | 'losers' | 'volume'` with `variant: ScopeVariant`. Import `ScopeVariant` from `'../../types/strategy'`.

```ts
export interface ScopeCardDef {
  id: string
  category: 'crypto' | 'stock'
  label: string
  description: string
  provider: 'free' | 'paid'
  source_type: ScopeUnit['source_type']
  variant: ScopeVariant
  defaultLimit: number
}
```

- [ ] **Step 3: Add `variant` to every card**

For `freeHyperRank` (lines 15-29), the two gainers/losers/volume cards each need a variant. Update the helper signature to take `variant`:

```ts
const freeHyperRank = (
  id: string,
  label: string,
  description: string,
  variant: ScopeVariant
): ScopeCardDef => ({
  id, category: 'crypto', label, description, provider: 'free',
  source_type: 'hyper_rank', variant, defaultLimit: 10,
})
```

Then the three calls (lines 32-49):
```ts
freeHyperRank('crypto-top-gainers', 'Crypto Top Gainers', 'Top % gainers on Hyperliquid', 'gainers'),
freeHyperRank('crypto-top-losers',   'Crypto Top Losers',   'Top % losers on Hyperliquid',  'losers'),
freeHyperRank('crypto-trending',     'Crypto Trending · Top Volume', 'Top traders by volume on Hyperliquid', 'volume'),
```

Add `variant` to every other card object. Map each card's `id` to its variant:

| id | variant |
|---|---|
| `crypto-bias-bull` | `bull` |
| `crypto-bias-bear` | `bear` |
| `crypto-ai500` | (omit — ai500 has no rank-direction) |
| `crypto-oi-increase` | `top` |
| `crypto-oi-decrease` | `low` |
| `crypto-netflow-top` | `inflow` |
| `crypto-netflow-outflow` | `outflow` |
| `crypto-gainers-nofxos` | `gainers` |
| `crypto-losers-nofxos` | `losers` |
| `stock-bias-bull` | `bull` |
| `stock-bias-bear` | `bear` |
| `stock-trending` | `trending` |
| `stock-gainers` | `gainers` |
| `stock-losers` | `losers` |

For the bias objects (e.g. `crypto-bias-bull`), the object literal is:
```ts
{
  id: 'crypto-bias-bull', category: 'crypto', label: 'Bias Radar (Bullish)',
  description: 'VergeX bullish bias radar', provider: 'paid',
  source_type: 'vergex', variant: 'bull', defaultLimit: 10,
},
```

- [ ] **Step 4: Update `toScopeUnit`**

Replace the `direction` spread (lines 182-191) with `variant`:

```ts
export function toScopeUnit(def: ScopeCardDef, limit: number): ScopeUnit {
  return {
    id: def.id,
    category: def.category,
    source_type: def.source_type,
    limit,
    label: def.label,
    provider: def.provider,
    ...(def.variant ? { variant: def.variant } : {}),
  }
}
```

- [ ] **Step 5: Typecheck the frontend (expected to fail until Task 2 fixes the factory referencing `direction`)**

Run: `cd web && npx tsc --noEmit`
Expected: errors in `strategyFactory.ts` referencing `.direction`. These are resolved in Task 2. Do not fix here.

- [ ] **Step 6: Commit**

```bash
git add web/src/types/strategy.ts web/src/features/strategies/scopeCatalog.ts
git commit -m "feat(strategy): add ScopeUnit variant to disambiguate scope cards"
```

---

### Task 2: `buildCoinSource` maps `variant` → correct backend source config

**Files:**
- Modify: `web/src/features/strategies/strategyFactory.ts`

**Interfaces:**
- Consumes: `ScopeUnit` (with `variant`) from Task 1.
- Produces: `buildCoinSource(units, mode): CoinSourceConfig` emitting the correct `source_type` + qualifier for all 4 families. Later backend tasks rely on these emitted values: `oi_top`/`oi_low`, `netflow_top`/`netflow_low`, `price_top`/`price_low`, and `vergex_signal` with `vergex_market_type`+`vergex_direction`.

- [ ] **Step 1: Rewrite `buildCoinSource` single-card branches**

Replace the `vergex` branch (lines 94-110) and add branches for `nofxos_oi`, `nofxos_netflow`, `nofxos_price`. Keep the `hyper_rank`/`ai500` branches but use `unit.variant` for hyper_rank. The full new function body (after the `>1` custom branch and the `hyper_rank`/`ai500` branches, i.e. replacing everything from the `vergex` branch onward):

```ts
  if (unit?.source_type === 'hyper_rank') {
    return {
      source_type: 'hyper_rank',
      scope_mode: mode,
      hyper_rank_category: unit.category,
      hyper_rank_direction: unit.variant || 'gainers',
      hyper_rank_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'oi_top' || unit?.source_type === 'oi_low') {
    const isTop = unit.source_type === 'oi_top' || unit.variant === 'top'
    return {
      source_type: isTop ? 'oi_top' : 'oi_low',
      scope_mode: mode,
      use_oi_top: isTop,
      oi_top_limit: isTop ? clamp(unit.limit, 1, 50) : 0,
      use_oi_low: !isTop,
      oi_low_limit: !isTop ? clamp(unit.limit, 1, 50) : 0,
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'nofxos_netflow') {
    const isInflow = unit.variant !== 'outflow'
    return {
      source_type: isInflow ? 'netflow_top' : 'netflow_low',
      scope_mode: mode,
      netflow_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'nofxos_price') {
    const isGainers = unit.variant !== 'losers'
    return {
      source_type: isGainers ? 'price_top' : 'price_low',
      scope_mode: mode,
      price_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'vergex') {
    const marketType = unit.category === 'stock' ? 'hip3_perp' : 'core_perp'
    return {
      source_type: 'vergex_signal',
      scope_mode: mode,
      vergex_limit: clamp(unit.limit, 1, 50),
      vergex_market_type: marketType,
      vergex_direction: unit.variant,
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false,
    }
  }
```

Note: `unit.source_type` for `nofxos_oi` cards is `'nofxos_oi'` per scopeCatalog — but the branch above checks `'oi_top'/'oi_low'`. **Correction:** the scope catalog `source_type` for OI cards is `'nofxos_oi'`, not `'oi_top'/'oi_low'`. Change the OI branch condition to `unit?.source_type === 'nofxos_oi'`:

```ts
  if (unit?.source_type === 'nofxos_oi') {
    const isTop = unit.variant === 'top'
    return {
      source_type: isTop ? 'oi_top' : 'oi_low',
      scope_mode: mode,
      use_oi_top: isTop,
      oi_top_limit: isTop ? clamp(unit.limit, 1, 50) : 0,
      use_oi_low: !isTop,
      oi_low_limit: !isTop ? clamp(unit.limit, 1, 50) : 0,
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }
```

The new `CoinSourceConfig` fields `netflow_limit`, `price_limit`, `vergex_direction` must be added to `web/src/types/strategy.ts` `CoinSourceConfig` in this task:

```ts
  // Netflow / price ranking pool (candidate source) limits
  netflow_limit?: number;
  price_limit?: number;
  // Vergex sub-card selector: 'bull'|'bear'|'trending'|'gainers'|'losers'
  vergex_direction?: string;
```

- [ ] **Step 2: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS (no references to the removed `.direction`). If the fallback `custom` branch (formerly lines 112-129) still references removed fields, delete it — single cards should now always match a branch above.

- [ ] **Step 3: Run frontend tests**

Run: `cd web && npm test`
Expected: PASS (existing tests should not exercise the removed `direction`; if any do, update them to `variant`).

- [ ] **Step 4: Commit**

```bash
git add web/src/features/strategies/strategyFactory.ts web/src/types/strategy.ts
git commit -m "feat(strategy): emit distinct coin_source per scope card variant"
```

---

### Task 3: Backend schema — `VergexDirection`, new source types in normalizer

**Files:**
- Modify: `store/strategy.go`
- Modify: `provider/vergex/free_test.go` (see Task 5 — placeholder not created here)

**Interfaces:**
- Consumes: frontend-emitted `netflow_top`/`netflow_low`/`price_top`/`price_low` source_types and `vergex_direction`.
- Produces: `CoinSourceConfig.VergexDirection string`, `CoinSourceConfig.NetflowLimit int`, `CoinSourceConfig.PriceLimit int`; `normalizeCoinSourceType` and `NormalizeProductSchema` recognize the new source types; `inferCoinSourceType` returns them.

- [ ] **Step 1: Wrap an existing test to prove current behavior (failing-forward not needed; this is additive). Write the failing normalizer test first.**

Create `store/strategy_normalize_alt_test.go`:

```go
package store

import (
	"testing"
)

func TestNormalizeProductSchema_AltSourceTypes(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.CoinSource.SourceType = "netflow_top"
	cfg.CoinSource.NetflowLimit = 10
	cfg.NormalizeProductSchema()
	if cfg.CoinSource.SourceType != "netflow_top" {
		t.Fatalf("SourceType = %q, want netflow_top", cfg.CoinSource.SourceType)
	}

	cfg2 := &StrategyConfig{}
	cfg2.CoinSource.SourceType = "price_low"
	cfg2.CoinSource.PriceLimit = 10
	cfg2.NormalizeProductSchema()
	if cfg2.CoinSource.SourceType != "price_low" {
		t.Fatalf("SourceType = %q, want price_low", cfg2.CoinSource.SourceType)
	}

	cfg3 := &StrategyConfig{}
	cfg3.CoinSource.SourceType = "vergex_signal"
	cfg3.CoinSource.VergexDirection = "gainers"
	cfg3.CoinSource.VergexLimit = 10
	cfg3.NormalizeProductSchema()
	if cfg3.CoinSource.VergexMarketType != "all" {
		t.Fatalf("VergexMarketType = %q, want all (default preserved)", cfg3.CoinSource.VergexMarketType)
	}
	if cfg3.CoinSource.VergexDirection != "gainers" {
		t.Fatalf("VergexDirection = %q, want gainers", cfg3.CoinSource.VergexDirection)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./store/ -run TestNormalizeProductSchema_AltSourceTypes -v`
Expected: FAIL — `SourceType` normalized to `vergex_signal` (default branch) because normalizer doesn't recognize `netflow_top`.

- [ ] **Step 3: Add struct fields**

In `store/strategy.go`, extend `CoinSourceConfig` after `VergexLiqBand string` (line 829):

```go
	// Netflow/price candidate-pool limits (source_type netflow_top/low, price_top/low)
	NetflowLimit int `json:"netflow_limit,omitempty"`
	PriceLimit   int `json:"price_limit,omitempty"`
	// Vergex sub-card selector: "bull"|"bear"|"trending"|"gainers"|"losers"
	VergexDirection string `json:"vergex_direction,omitempty"`
```

- [ ] **Step 4: Extend `normalizeCoinSourceType`**

Add cases before the `default` (line 302):

```go
	case strings.Contains(compact, "netflowtop") || strings.Contains(value, "netflow top") || strings.Contains(value, "net flow top"):
		return "netflow_top"
	case strings.Contains(compact, "netflowlow") || strings.Contains(value, "netflow low") || strings.Contains(value, "net flow low"):
		return "netflow_low"
	case strings.Contains(compact, "pricetop") || strings.Contains(value, "price top"):
		return "price_top"
	case strings.Contains(compact, "pricelow") || strings.Contains(value, "price low"):
		return "price_low"
```

- [ ] **Step 5: Extend `NormalizeProductSchema` switch + vergex hunk**

Add cases to the switch in `NormalizeProductSchema` after the `oi_low` case (line 176):

```go
	case "netflow_top":
		c.CoinSource.UseAI500 = false
		c.CoinSource.UseOITop = false
		c.CoinSource.UseOILow = false
		c.CoinSource.UseHyperAll = false
		c.CoinSource.UseHyperMain = false
		if c.CoinSource.NetflowLimit <= 0 {
			c.CoinSource.NetflowLimit = 10
		}
	case "netflow_low":
		c.CoinSource.UseAI500 = false
		c.CoinSource.UseOITop = false
		c.CoinSource.UseOILow = false
		c.CoinSource.UseHyperAll = false
		c.CoinSource.UseHyperMain = false
		if c.CoinSource.NetflowLimit <= 0 {
			c.CoinSource.NetflowLimit = 10
		}
	case "price_top":
		c.CoinSource.UseAI500 = false
		c.CoinSource.UseOITop = false
		c.CoinSource.UseOILow = false
		c.CoinSource.UseHyperAll = false
		c.CoinSource.UseHyperMain = false
		if c.CoinSource.PriceLimit <= 0 {
			c.CoinSource.PriceLimit = 10
		}
	case "price_low":
		c.CoinSource.UseAI500 = false
		c.CoinSource.UseOITop = false
		c.CoinSource.UseOILow = false
		c.CoinSource.UseHyperAll = false
		c.CoinSource.UseHyperMain = false
		if c.CoinSource.PriceLimit <= 0 {
			c.CoinSource.PriceLimit = 10
		}
```

In the `vergex_signal` case (lines 213-234), no change needed for `VergexDirection` (it's carried as-is). Good.

- [ ] **Step 6: Extend `inferCoinSourceType`**

Add cases before `default` (line 326):

```go
	case source.NetflowLimit > 0:
		return "netflow_top"
	case source.PriceLimit > 0:
		return "price_top"
```

- [ ] **Step 7: Build + test**

Run:
```bash
go build ./store/ && go vet ./store/ && go test ./store/ -run TestNormalizeProductSchema_AltSourceTypes -v
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add store/strategy.go store/strategy_normalize_alt_test.go
git commit -m "feat(strategy): schema support for netflow/price source types + vergex_direction"
```

---

### Task 4: `provider/vergex/free.go` — FreeClient (signal leaderboard, stock hot/movers, detail)

**Files:**
- Create: `provider/vergex/free.go`
- Test: `provider/vergex/free_test.go`

**Interfaces:**
- Consumes: `parseRankItem`/`ParseSignalRanking`/`SignalRankItem`/`Query`/`MarketSymbol`/`TradableSymbolForMarket` from `provider/vergex/client.go`; `security.SafeHTTPClient`.
- Produces:
  - `func NewFreeClient(authToken string) *FreeClient`
  - `func (c *FreeClient) GetLeaderboard() (*SignalRankingData, error)` — GET `/api/v1/direction-change/leaderboard`
  - `func (c *FreeClient) GetStockTrending(limit int) (*SignalRankingData, error)` — GET `/api/v1/market-data/hl-stocks-hot?limit=`
  - `func (c *FreeClient) GetStockMovers(direction string, limit int) (*SignalRankingData, error)` — GET `/api/v1/market-data/hl-stocks-movers?direction=&limit=`
  - `func (c *FreeClient) GetRaw(path string, params url.Values) (json.RawMessage, error)` — generic authed GET returning raw bytes
  - Helper free-endpoint path constants: `LeaderboardPath`, `StocksHotPath`, `StocksMoversPath`, and detail paths `SignalSignalsPath`, `SignalRiskbinsPath`, `SignalSummaryPath`, `DirectionCurrentPath`, `DirectionHistoryPath`, `StructOverviewPath`, `CoveragePath`, `HoldersPath`, `MarketsPath`, `FlowMarketsPath`.

- [ ] **Step 1: Write the failing test**

Create `provider/vergex/free_test.go`:

```go
package vergex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFreeClient_GetLeaderboard(t *testing.T) {
	body := `{"band":15,"items":[
	  {"symbol":"PUMP","bias":"bullish","directionScore":4,"rank":1,"market":{"marketType":"core_perp"}},
	  {"symbol":"xyz:MSFT","bias":"bullish","directionScore":3,"rank":2,"market":{"marketType":"hip3_perp"}}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/direction-change/leaderboard" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeClient("tok")
	c.baseURL = srv.URL + "/api/v1"
	data, err := c.GetLeaderboard()
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if len(data.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(data.Items))
	}
	if data.Items[0].Symbol != "PUMP" || data.Items[0].Bias != "bullish" || data.Items[0].MarketType != "core_perp" {
		t.Fatalf("item[0] = %+v", data.Items[0])
	}
}

func TestFreeClient_GetStockMovers(t *testing.T) {
	body := `{"requestId":"","direction":"gainers","entries":[
	  {"symbol":"SMSNUSDC","rank":1,"change24hPct":5.2}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/market-data/hl-stocks-movers" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("direction") != "gainers" {
			t.Fatalf("direction = %q, want gainers", r.URL.Query().Get("direction"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeClient("")
	c.baseURL = srv.URL + "/api/v1"
	data, err := c.GetStockMovers("gainers", 10)
	if err != nil {
		t.Fatalf("GetStockMovers: %v", err)
	}
	if len(data.Items) != 1 || data.Items[0].Symbol != "SMSN" {
		t.Fatalf("unexpected players %+v", data.Items)
	}
}

func TestFreeClient_GetRaw_AddsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("missing token header")
		}
		w.Write([]byte(`{"data":{"bins":[{"px":1}]}}`))
	}))
	defer srv.Close()
	c := NewFreeClient("tok")
	c.baseURL = srv.URL
	raw, err := c.GetRaw("/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/riskbins", nil)
	if err != nil {
		t.Fatalf("GetRaw: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["data"]; !ok {
		t.Fatalf("missing data key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/vergex/ -run TestFreeClient -v`
Expected: FAIL — `undefined: NewFreeClient`.

- [ ] **Step 3: Write `free.go`**

Create `provider/vergex/free.go`:

```go
package vergex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"nofx/security"
	"strings"
	"time"
)

const (
	DefaultFreeBaseURL  = "https://vergex.trade"
	DefaultFreeTimeout  = 30 * time.Second
	LeaderboardPath     = "/api/v1/direction-change/leaderboard"
	StocksHotPath       = "/api/v1/market-data/hl-stocks-hot"
	StocksMoversPath    = "/api/v1/market-data/hl-stocks-movers"
	SignalSignalsPath   = "/api/v1/data-intelligence/markets"
	SignalRiskbinsPath  = "/api/v1/data-intelligence/markets"
	SignalSummaryPath   = "/api/v1/data-intelligence/markets"
	DirectionCurrentPath = "/api/v1/direction-change"
	DirectionHistoryPath = "/api/v1/direction-change"
	StructOverviewPath   = "/api/v1/data-intelligence/markets/structure-overview"
	MarketsPath          = "/api/v1/data-intelligence/markets"
	FlowMarketsPath      = "/api/v1/data-intelligence/flow/markets"
)

// FreeClient fetches the free vergex.trade endpoints (no x402/Claw402
// payment). An optional Bearer token is attached for endpoints that require
// it (per-coin detail). It reuses the lenient SignalRankItem parser.
type FreeClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewFreeClient(authToken string) *FreeClient {
	base := strings.TrimRight(DefaultFreeBaseURL, "/")
	return &FreeClient{
		baseURL: base,
		token:   strings.TrimSpace(authToken),
		http:    security.SafeHTTPClient(DefaultFreeTimeout),
	}
}

// GetLeaderboard returns the direction-change (bias radar) leaderboard.
func (c *FreeClient) GetLeaderboard() (*SignalRankingData, error) {
	return c.getSignalRanking(LeaderboardPath, nil)
}

// GetStockTrending returns the hot/trending US stocks list.
func (c *FreeClient) GetStockTrending(limit int) (*SignalRankingData, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.getSignalRanking(StocksHotPath, params)
}

// GetStockMovers returns stock gainers (direction= gainers) or losers.
func (c *FreeClient) GetStockMovers(direction string, limit int) (*SignalRankingData, error) {
	params := url.Values{}
	params.Set("direction", direction)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.getSignalRanking(StocksMoversPath, params)
}

func (c *FreeClient) getSignalRanking(path string, params url.Values) (*SignalRankingData, error) {
	body, err := c.GetRaw(path, params)
	if err != nil {
		return nil, err
	}
	return ParseSignalRanking(body)
}

// GetRaw performs an authenticated GET to a vergex.trade api/v1 path and
// returns the raw response bytes. If c.token is set it is sent as
// Authorization: Bearer <token>.
func (c *FreeClient) GetRaw(path string, params url.Values) (json.RawMessage, error) {
	fullURL := c.baseURL + path
	if encoded := params.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vergex free request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; nofx)")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vergex free GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return io.ReadAll(resp.Body)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("vergex token rejected (401) for %s", path)
	}
	return nil, fmt.Errorf("vergex free GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./provider/vergex/ -run TestFreeClient -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add provider/vergex/free.go provider/vergex/free_test.go
git commit -m "feat(vergex): free vergex.trade client (leaderboard, stock movers, raw detail)"
```

---

### Task 5: `provider/nofxos/free.go` — FreeTrendingClient (OI/netflow/price)

**Files:**
- Create: `provider/nofxos/free.go`
- Test: `provider/nofxos/free_test.go`

**Interfaces:**
- Consumes: `OIPosition`, `NetFlowPosition`, `PriceRankingItem` types from `provider/nofxos`; `security.SafeHTTPClient`.
- Produces:
  - `func NewFreeTrendingClient() *FreeTrendingClient`
  - `(c *FreeTrendingClient) GetOITop(limit) ([]OIPosition, error)` / `GetOILow(limit) ([]OIPosition, error)` — `trending-crypto?tab=oi` → `top`/`low`
  - `(c *FreeTrendingClient) GetNetFlowTop(limit) ([]NetFlowPosition, error)` / `GetNetFlowLow(limit) ([]NetFlowPosition, error)` — `tab=net_flow`
  - `(c *FreeTrendingClient) GetPriceTop(limit) ([]PriceRankingItem, error)` / `GetPriceLow(limit) ([]PriceRankingItem, error)` — `tab=price`
  - `(c *FreeTrendingClient) GetAI500() ([]CoinData, error)` — `trending-category?lang=en&key=ai500` → wraps `.category.assets[]` into `CoinData`.

- [ ] **Step 1: Write the failing test**

Create `provider/nofxos/free_test.go`:

```go
package nofxos

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFreeTrendingClient_GetOITop(t *testing.T) {
	body := `{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":100,"oi_delta":5,"oi_delta_percent":15.5,"oi_delta_value":150000,"price_delta_percent":2.1,"net_long":10,"net_short":5}],"low":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tab") != "oi" {
			t.Fatalf("tab = %q, want oi", r.URL.Query().Get("tab"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	pos, err := c.GetOITop(10)
	if err != nil {
		t.Fatalf("GetOITop: %v", err)
	}
	if len(pos) != 1 || pos[0].Symbol != "BTC" || pos[0].OIDeltaPercent != 15.5 {
		t.Fatalf("unexpected %+v", pos)
	}
}

func TestFreeTrendingClient_GetPriceLow(t *testing.T) {
	body := `{"top":[],"low":[{"pair":"ETHUSDT","symbol":"ETH","price_delta":-0.12,"price":3000,"future_flow":0,"spot_flow":0,"oi":0,"oi_delta":0,"oi_delta_value":0}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tab") != "price" {
			t.Fatalf("tab = %q, want price", r.URL.Query().Get("tab"))
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	items, err := c.GetPriceLow(10)
	if err != nil {
		t.Fatalf("GetPriceLow: %v", err)
	}
	if len(items) != 1 || items[0].Symbol != "ETH" || items[0].PriceDelta != -0.12 {
		t.Fatalf("unexpected %+v", items)
	}
}

func TestFreeTrendingClient_GetAI500(t *testing.T) {
	body := `{"category":{"assets":[{"symbol":"CYS","pair":"CYSUSDT","score":75.0,"startTime":1785852000,"startPrice":0.51,"changePctValue":143.7}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	coins, err := c.GetAI500()
	if err != nil {
		t.Fatalf("GetAI500: %v", err)
	}
	if len(coins) != 1 || coins[0].Pair != "CYSUSDT" || coins[0].Score != 75.0 {
		t.Fatalf("unexpected %+v", coins)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/nofxos/ -run TestFreeTrendingClient -v`
Expected: FAIL — `undefined: NewFreeTrendingClient`.

- [ ] **Step 3: Write `free.go`**

Create `provider/nofxos/free.go`:

```go
package nofxos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"nofx/security"
	"strings"
	"time"
)

const (
	DefaultFreeTrendingBase = "https://vergex.trade"
	trendingPath            = "/trending-crypto"
	categoryPath            = "/trending-category"
)

// FreeTrendingClient fetches the free vergex.trade trending-crypto endpoints
// (OI / netflow / price) and the ai500 trending-category endpoint. No payment.
type FreeTrendingClient struct {
	baseURL string
	http    *http.Client
}

func NewFreeTrendingClient() *FreeTrendingClient {
	return &FreeTrendingClient{
		baseURL: strings.TrimRight(DefaultFreeTrendingBase, "/"),
		http:    security.SafeHTTPClient(30 * time.Second),
	}
}

func (c *FreeTrendingClient) GetOITop(limit int) ([]OIPosition, error) {
	return c.getOIArray("top", limit)
}

func (c *FreeTrendingClient) GetOILow(limit int) ([]OIPosition, error) {
	return c.getOIArray("low", limit)
}

func (c *FreeTrendingClient) getOIArray(which string, limit int) ([]OIPosition, error) {
	raw, err := c.getTrending("oi", limit)
	if err != nil {
		return nil, err
	}
	var env struct {
		Top []OIPosition `json:"top"`
		Low []OIPosition `json:"low"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending oi: %w", err)
	}
	if which == "low" {
		return env.Low, nil
	}
	return env.Top, nil
}

func (c *FreeTrendingClient) GetNetFlowTop(limit int) ([]NetFlowPosition, error) {
	return c.getNetFlowArray("top", limit)
}

func (c *FreeTrendingClient) GetNetFlowLow(limit int) ([]NetFlowPosition, error) {
	return c.getNetFlowArray("low", limit)
}

func (c *FreeTrendingClient) getNetFlowArray(which string, limit int) ([]NetFlowPosition, error) {
	raw, err := c.getTrending("net_flow", limit)
	if err != nil {
		return nil, err
	}
	var env struct {
		Top []NetFlowPosition `json:"top"`
		Low []NetFlowPosition `json:"low"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending net_flow: %w", err)
	}
	if which == "low" {
		return env.Low, nil
	}
	return env.Top, nil
}

func (c *FreeTrendingClient) GetPriceTop(limit int) ([]PriceRankingItem, error) {
	return c.getPriceArray("top", limit)
}

func (c *FreeTrendingClient) GetPriceLow(limit int) ([]PriceRankingItem, error) {
	return c.getPriceArray("low", limit)
}

func (c *FreeTrendingClient) getPriceArray(which string, limit int) ([]PriceRankingItem, error) {
	raw, err := c.getTrending("price", limit)
	if err != nil {
		return nil, err
	}
	var env struct {
		Top []PriceRankingItem `json:"top"`
		Low []PriceRankingItem `json:"low"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending price: %w", err)
	}
	if which == "low" {
		return env.Low, nil
	}
	return env.Top, nil
}

func (c *FreeTrendingClient) getTrending(tab string, limit int) (json.RawMessage, error) {
	params := url.Values{}
	params.Set("tab", tab)
	params.Set("duration", "24h")
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.get(c.trendingPath(params))
}

func (c *FreeTrendingClient) GetAI500() ([]CoinData, error) {
	params := url.Values{}
	params.Set("lang", "en")
	params.Set("key", "ai500")
	raw, err := c.get(c.baseURL + categoryPath + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Category struct {
			Assets []struct {
				Symbol          string  `json:"symbol"`
				Pair            string  `json:"pair"`
				Score           float64 `json:"score"`
				StartTime       int64   `json:"startTime"`
				StartPrice      float64 `json:"startPrice"`
				ChangePctValue  float64 `json:"changePctValue"`
			} `json:"assets"`
		} `json:"category"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse trending-category ai500: %w", err)
	}
	coins := make([]CoinData, 0, len(resp.Category.Assets))
	for _, a := range resp.Category.Assets {
		coins = append(coins, CoinData{
			Pair:            a.Pair,
			Score:           a.Score,
			StartTime:       a.StartTime,
			StartPrice:      a.StartPrice,
			IncreasePercent: a.ChangePctValue,
			IsAvailable:     true,
		})
	}
	return coins, nil
}

func (c *FreeTrendingClient) trendingPath(params url.Values) string {
	return c.baseURL + trendingPath + "?" + params.Encode()
}

func (c *FreeTrendingClient) get(fullURL string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("trending request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; nofx)")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trending GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trending GET: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./provider/nofxos/ -run TestFreeTrendingClient -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add provider/nofxos/free.go provider/nofxos/free_test.go
git commit -m "feat(nofxos): free vergex.trade trending client (oi/netflow/price/ai500)"
```

---

### Task 6: Engine wiring — use free clients in candidate getters

**Files:**
- Modify: `kernel/engine.go`

**Interfaces:**
- Consumes: `FreeClient` (Task 4), `FreeTrendingClient` (Task 5), existing `nofxosClient`/`vergexClient`.
- Produces: `StrategyEngine` gains `freeClient *vergex.FreeClient` and `trending *nofxos.FreeTrendingClient`. `getAI500Coins`, `getOITopCoins`, `getOILowCoins`, netflow/price fetchers use free clients. New handler-compatible methods for netflow/price pools.

- [ ] **Step 1: Write failing tests for the new getter outputs**

Create `kernel/engine_free_test.go`:

```go
package kernel

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestEngineWithFree(t *testing.T) *StrategyEngine {
	t.Helper()
	srvOI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":1,"oi_delta":1,"oi_delta_percent":1,"oi_delta_value":1,"price_delta_percent":1,"net_long":1,"net_short":1}],"low":[]}`))
	}))
	srvLeader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"symbol":"PUMP","bias":"bullish","directionScore":4,"rank":1,"market":{"marketType":"core_perp"}}]}`))
	}))
	t.Cleanup(func() { srvOI.Close(); srvLeader.Close() })

	e := &StrategyEngine{config: nil}
	trending := nofxosFreeTestClient(srvOI.URL)
	e.trending = trending
	e.freeClient = vergexFreeTestClient(srvLeader.URL)
	return e
}
```

Add a helper file to avoid import noise — actually the engine already imports `nofxos`, `vergex`, `market`, `store`, `logger`. For the test, construct the free clients directly (they expose `baseURL` unexported — test is `package kernel`, so can't set unexported). **Use `httptest` servers but set the client's http transport to route to the server via a RoundTripper, or add an exported setter.** Simplest: add an exported `SetBaseURL` to each free client in Tasks 4/5. Update those files:

In `provider/vergex/free.go` add:
```go
// SetBaseURL overrides the base URL (test/diagnostics).
func (c *FreeClient) SetBaseURL(u string) { c.baseURL = strings.TrimRight(u, "/") }
```
In `provider/nofxos/free.go` add:
```go
// SetBaseURL overrides the base URL (test/diagnostics).
func (c *FreeTrendingClient) SetBaseURL(u string) { c.baseURL = strings.TrimRight(u, "/") }
```

Write test `kernel/engine_free_test.go`:

```go
package kernel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nofx/provider/nofxos"
	"nofx/provider/vergex"
)

func TestEngine_getOITopCoins_usesFree(t *testing.T) {
	srvOI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tab") != "oi" {
			t.Fatalf("tab=%q want oi", r.URL.Query().Get("tab"))
		}
		w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":1,"oi_delta":1,"oi_delta_percent":1,"oi_delta_value":1,"price_delta_percent":1,"net_long":1,"net_short":1}],"low":[]}`))
	}))
	defer srvOI.Close()

	e := &StrategyEngine{}
	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srvOI.URL)
	e.trending = tr

	coins, err := e.getOITopCoins(5)
	if err != nil {
		t.Fatalf("getOITopCoins: %v", err)
	}
	if len(coins) != 1 || coins[0].Symbol != "BTCUSDT" || coins[0].Sources[0] != "oi_top" {
		t.Fatalf("unexpected %+v", coins)
	}
}

func TestEngine_getVergexSignalCoins_usesFreeLeaderboard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"symbol":"PUMP","bias":"bullish","directionScore":4,"rank":1,"market":{"marketType":"core_perp"}}]}`))
	}))
	defer srv.Close()

	e := &StrategyEngine{}
	fc := vergex.NewFreeClient("")
	fc.SetBaseURL(srv.URL)
	e.freeClient = fc

	coins, err := e.getVergexSignalCoins(5, "core_perp", "", "", "all", nil)
	if err != nil {
		t.Fatalf("getVergexSignalCoins: %v", err)
	}
	if len(coins) != 1 || coins[0].Symbol != "PUMP" {
		t.Fatalf("unexpected %+v", coins)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./kernel/ -run 'TestEngine_getOITopCoins_usesFree|TestEngine_getVergexSignalCoins_usesFreeLeaderboard' -v`
Expected: FAIL — `e.trending is nil` / `undefined field` / `getOITopCoins` uses paid client.

- [ ] **Step 3: Add free clients to `StrategyEngine` + constructor**

In `kernel/engine.go`, the `StrategyEngine` struct (around lines 100-110) add fields:

```go
	// Free vergex.trade / trending clients (no Claw402 needed)
	freeClient *vergex.FreeClient
	trending   *nofxos.FreeTrendingClient
```

In `NewStrategyEngine(config *store.StrategyConfig, claw402WalletKey ...string)` (engine.go:197, body lines ~200-245), initialize both unconditionally before the wallet check:

```go
	freeVergex := vergex.NewFreeClient("")
	trendingClient := nofxos.NewFreeTrendingClient()
```

and include them in the returned `StrategyEngine` in **both** the wallet and no-wallet branches.

- [ ] **Step 4: Rewrite `getAI500Coins`, `getOITopCoins`, `getOILowCoins` to use free clients**

Replace line 537 (`symbols, err := e.nofxosClient.GetTopRatedCoins(limit)`) with:

```go
	coins, err := e.trending.GetAI500()
	if err != nil {
		return nil, err
	}
	var candidates []CandidateCoin
	for _, c := range coins {
		symbol := market.Normalize(c.Pair)
		candidates = append(candidates, CandidateCoin{Symbol: symbol, Sources: []string{"ai500"}})
	}
	return candidates, nil
```

Replace `getOITopCoins` body (lines 552-574) to use the free client:

```go
func (e *StrategyEngine) getOITopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetOITop(limit)
	if err != nil {
		return nil, err
	}
	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(pos.Symbol), Sources: []string{"oi_top"}})
	}
	return candidates, nil
}
```

Replace `getOILowCoins` similarly with `e.trending.GetOILow(limit)` and `Sources: []string{"oi_low"}`.

- [ ] **Step 5: Add new getters for netflow/price pools**

Add these methods (used by new `GetCandidateCoins` cases in Task 7):

```go
func (e *StrategyEngine) getNetflowTopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetNetFlowTop(limit)
	if err != nil {
		return nil, err
	}
	return netflowPositionsToCandidates(positions, "netflow_top", limit)
}

func (e *StrategyEngine) getNetflowLowCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetNetFlowLow(limit)
	if err != nil {
		return nil, err
	}
	return netflowPositionsToCandidates(positions, "netflow_low", limit)
}

func netflowPositionsToCandidates(positions []nofxos.NetFlowPosition, source string, limit int) ([]CandidateCoin, error) {
	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(pos.Symbol), Sources: []string{source}})
	}
	return candidates, nil
}

func (e *StrategyEngine) getPriceTopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	items, err := e.trending.GetPriceTop(limit)
	if err != nil {
		return nil, err
	}
	return priceItemsToCandidates(items, "price_top", limit)
}

func (e *StrategyEngine) getPriceLowCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	items, err := e.trending.GetPriceLow(limit)
	if err != nil {
		return nil, err
	}
	return priceItemsToCandidates(items, "price_low", limit)
}

func priceItemsToCandidates(items []nofxos.PriceRankingItem, source string, limit int) ([]CandidateCoin, error) {
	var candidates []CandidateCoin
	for i, it := range items {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(it.Symbol), Sources: []string{source}})
	}
	return candidates, nil
}
```

- [ ] **Step 6: Run the free-engine tests**

Run: `go test ./kernel/ -run 'TestEngine_|TestNetflow|TestPrice' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add kernel/engine.go kernel/engine_free_test.go
git commit -m "feat(kernel): use free vergex.trade clients in candidate getters"
```

---

### Task 7: Engine wiring — `GetCandidateCoins` cases + `vergex_direction` routing

**Files:**
- Modify: `kernel/engine.go`

**Interfaces:**
- Consumes: `getNetflowTopCoins`/`getNetflowLowCoins`/`getPriceTopCoins`/`getPriceLowCoins` (Task 6); `freeClient.GetStockTrending`/`GetStockMovers` (Task 4); `vergex_direction` config (Task 3).
- Produces: `GetCandidateCoins` handles `netflow_top`/`netflow_low`/`price_top`/`price_low` and routes `vergex_signal` sub-cards by direction.

- [ ] **Step 1: Add `netflow_top/low` + `price_top/low` cases to `GetCandidateCoins`**

In `kernel/engine.go` `GetCandidateCoins` switch, after the `oi_low` case (line 367), add:

```go
	case "netflow_top":
		coins, err := e.getNetflowTopCoins(coinSource.NetflowLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "netflow_low":
		coins, err := e.getNetflowLowCoins(coinSource.NetflowLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "price_top":
		coins, err := e.getPriceTopCoins(coinSource.PriceLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "price_low":
		coins, err := e.getPriceLowCoins(coinSource.PriceLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil
```

- [ ] **Step 2: Route `vergex_direction` in `getVergexSignalCoins`**

Modify `getVergexSignalCoins` (engine.go:730). Replace the wallet-required guard and the ranking source so it prefers the free client by direction. New guard (replace lines 731-733):

```go
	if marketType == "" {
		marketType = vergex.DefaultMarketType
	}
```

Replace the ranking fetch block (lines 746-769) so it uses the free client and the `vergex_direction` sub-selector. Add a `direction` parameter to the signature: change the method to accept `direction string` (the callers in `GetCandidateCoins` pass `coinSource.VergexDirection`). Update the call at line 415-422 to pass `coinSource.VergexDirection` as a new arg:

```go
	case "vergex_signal":
		coins, err := e.getVergexSignalCoins(
			coinSource.VergexLimit,
			coinSource.VergexMarketType,
			coinSource.VergexChain,
			coinSource.VergexLiqBand,
			coinSource.HyperRankCategory,
			coinSource.StaticCoins,
			coinSource.VergexDirection,
		)
```

New signature: `func (e *StrategyEngine) getVergexSignalCoins(limit int, marketType, chain, liqBand, category string, selectedSymbols []string, direction string) ([]CandidateCoin, error)`.

Inside, replace the `e.vergexClient` ranking fetch with a direction-based free fetch:

```go
	direction = strings.ToLower(strings.TrimSpace(direction))
	var ranking *vergex.SignalRankingData
	var fetchErr error
	switch direction {
	case "trending":
		data, err := e.freeClient.GetStockTrending(limit)
		ranking, fetchErr = data, err
	case "gainers", "losers":
		data, err := e.freeClient.GetStockMovers(direction, limit)
		ranking, fetchErr = data, err
	case "" , "bull", "bear", "all":
		data, err := e.freeClient.GetLeaderboard()
		ranking, fetchErr = data, err
	default:
		data, err := e.freeClient.GetLeaderboard()
		ranking, fetchErr = data, err
	}
	if fetchErr != nil {
		return nil, fmt.Errorf("failed to fetch Vergex data: %w", fetchErr)
	}
```

Then use `ranking.Items` in place of the old paid `ranking.Items` everywhere below (the `rankedItems := vergex.FilterSignalRankingItems(ranking.Items, ...)` and the directional selection). For the `gainers`/`losers`/`trending` free endpoints the rows carry no `bias`; the existing directional interleave (`engine.go:806-848`) will bucket them into `otherItems` and still emit them — acceptable (they are symbol pools).

Note: `getVergexSignalCoins` currently errors when `e.vergexClient == nil`. Since we now use `e.freeClient`, remove that guard. The vergex detail cache (`e.vergexRankingCache`) population can stay.

- [ ] **Step 3: Build + run free-engine + existing engine tests**

Run:
```bash
go build ./... && go vet ./... && go test ./kernel/ ./store/ ./api/ -run 'TestGetCandidateCoins|TestEngine_|TestNormalizeProductSchema|TestHandleVergex' -count=1
```
Expected: PASS (no regressions).

- [ ] **Step 4: Commit**

```bash
git add kernel/engine.go
git commit -m "feat(kernel): netflow/price candidate pools + vergex_direction routing"
```

---

### Task 8: Replace vergex HTTP handlers with free client + token

**Files:**
- Modify: `api/handler_vergex.go`
- Modify: `api/server.go` (route doc text, cosmetic)

**Interfaces:**
- Consumes: `vergex.NewFreeClient` (Task 4), `resolveStrategyDataWalletKey` (existing, only as token fallback), config token via `config.Get()`.
- Produces: the 4 vergex routes no longer require a Claw402 wallet, and return free-endpoint data.

- [ ] **Step 1: Write failing test (regression: vergex route 400s without wallet)**

Create `api/handler_vergex_free_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)
```

Note: constructing a full `Server` requires store/config/manager wiring. Given the existing test infra doesn't include a vergex-handler test and building one is heavy, use a lighter unit test that calls the handler logic via a constructed `Server` configured for tests if available; otherwise assert the client construction path. **If no lightweight path exists, keep the test minimal** and rely on `go build`/`go vet` + manual verification via `curl`. Write the test only if the existing `api` test helpers allow a real server (check `api/server_test.go` for a `newTestServer`). If `newTestServer` exists, use it; otherwise skip the automated test and add an inline TODO noting manual verification.

- [ ] **Step 2: Rewrite `newVergexClientForRequest` and the 4 handlers**

Replace `newVergexClientForRequest` (handler_vergex.go:98-119) with a free-client factory that does not require a wallet:

```go
func (s *Server) freeVergexClientForRequest(c *gin.Context) *vergex.FreeClient {
	userID := c.GetString("user_id")
	_ = userID // free endpoints are public; token is a global config, not per-user
	tok := s.config.VergexAPIKey() // see Task 9
	return vergex.NewFreeClient(tok)
}
```

Update each handler to use it. `handleVergexSignalRanking` becomes:

```go
func (s *Server) handleVergexSignalRanking(c *gin.Context) {
	client := s.freeVergexClientForRequest(c)
	data, err := client.GetLeaderboard()
	if err != nil {
		logger.Warnf("Vergex signal-ranking failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	limit := parsePositiveInt(c.Query("limit"), vergex.MaxSignalRankingItems)
	marketType := strings.TrimSpace(c.Query("marketType"))
	if strings.TrimSpace(c.Query("direction")) != "" {
		switch d := strings.TrimSpace(c.Query("direction")); d {
		case "gainers", "losers":
			dd, derr := client.GetStockMovers(d, limit)
			if derr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": derr.Error()})
				return
			}
			data = dd
		case "trending":
			dd, derr := client.GetStockTrending(limit)
			if derr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": derr.Error()})
				return
			}
			data = dd
		}
	}
	items := vergex.FilterSignalRankingItems(data.Items, marketType, limit)
	c.JSON(http.StatusOK, gin.H{"items": items, "raw": data.Raw})
}
```

`handleVergexSignalLab` and `handleVergexCostLiquidationHeatmap` become free-client `GetRaw` calls with the market/symbol paths:

```go
func (s *Server) handleVergexSignalLab(c *gin.Context) {
	client := s.freeVergexClientForRequest(c)
	marketType := withDefault(strings.TrimSpace(c.Query("marketType")), vergex.DefaultMarketType)
	symbol := strings.TrimSpace(c.Query("symbol"))
	params := url.Values{}
	chain := strings.TrimSpace(c.Query("chain"))
	if chain != "" {
		params.Set("chain", chain)
	}
	if lb := strings.TrimSpace(c.Query("liqBand")); lb != "" {
		params.Set("liqBand", lb)
	}
	path := detailMarketPath(marketType, symbol) + "/signals"
	body, err := client.GetRaw(path, params)
	if err != nil {
		logger.Warnf("Vergex signal-lab failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}
```

Add a helper `detailMarketPath(marketType, symbol)`:

```go
func detailMarketPath(marketType, symbol string) string {
	mt := marketType
	if mt == "" {
		mt = vergex.DefaultMarketType
	}
	return "/api/v1/data-intelligence/markets/" + mt + "/" + vergex.MarketSymbol(mt, symbol)
}
```

`handleVergexCostLiquidationHeatmap` mirrors it with `"/riskbins"`.

`handleVergexFlowMarkets` becomes:

```go
func (s *Server) handleVergexFlowMarkets(c *gin.Context) {
	client := s.freeVergexClientForRequest(c)
	window := withDefault(strings.TrimSpace(c.Query("window")), "1h")
	limit := parsePositiveInt(c.Query("limit"), 25)
	params := url.Values{}
	params.Set("window", window)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if ch := strings.TrimSpace(c.Query("chain")); ch != "" {
		params.Set("chain", ch)
	}
	body, err := client.GetRaw(vergex.FlowMarketsPath, params)
	if err != nil {
		logger.Warnf("Vergex flow-markets failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}
```

Add `"net/url"` to imports.

- [ ] **Step 3: Update route doc text in `server.go`**

Change lines 214-217 to reflect free endpoints:
```go
s.route(protected, "GET", "/vergex/signal-ranking", "Vergex signal ranking via free vergex.trade (?marketType=all&limit=30&direction=)", s.handleVergexSignalRanking)
s.route(protected, "GET", "/vergex/signal-lab", "Vergex signal lab via free vergex.trade (?marketType=hip3_perp&symbol=AAPL)", s.handleVergexSignalLab)
s.route(protected, "GET", "/vergex/cost-liquidation-heatmap", "Vergex cost/liquidation heatmap via free vergex.trade (?marketType=hip3_perp&symbol=AAPL)", s.handleVergexCostLiquidationHeatmap)
s.route(protected, "GET", "/vergex/flow-markets", "Vergex net-flow market ranking via free vergex.trade (?chain=mainnet&window=1h&limit=25)", s.handleVergexFlowMarkets)
```

- [ ] **Step 4: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/handler_vergex.go api/server.go
git commit -m "feat(api): serve vergex endpoints from free vergex.trade without claw402 wallet"
```

---

### Task 9: Token config — `VERGEX_API_TOKEN`

**Files:**
- Modify: `api/handler_vergex.go`
- Modify: `.env.example`

**Interfaces:**
- Consumes: nothing.
- Produces: `freeVergexClientForRequest` reads `os.Getenv("VERGEX_API_TOKEN")` (optional) and passes it to `vergex.NewFreeClient`.

Note: the `api.Server` struct has **no `config` field** (verified `api/server.go:21-31`), so the token is read directly from the environment rather than threaded through `config`. This avoids adding an unused config method.

- [ ] **Step 1: Wire token into `freeVergexClientForRequest`**

In `api/handler_vergex.go`, ensure `freeVergexClientForRequest` reads the token from env:

```go
func (s *Server) freeVergexClientForRequest(c *gin.Context) *vergex.FreeClient {
	_ = c.GetString("user_id")
	return vergex.NewFreeClient(os.Getenv("VERGEX_API_TOKEN"))
}
```

Add `"os"` to imports if not present.

- [ ] **Step 2: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 3: Document env var in `.env.example`**

Append to `.env.example`:
```
# Optional vergex.trade Bearer token for per-coin detail endpoints (riskbins etc.)
VERGEX_API_TOKEN=
```

- [ ] **Step 4: Commit**

```bash
git add config/config.go api/handler_vergex.go .env.example
git commit -m "feat(config): optional VERGEX_API_TOKEN for authed detail endpoints"
```

---

### Task 10: Full verification

**Files:**
- (no source changes unless a step reveals a regression)

- [ ] **Step 1: Backend build + vet + full test**

Run:
```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./... && go test ./... 2>&1 | tail -30
```
Expected: all PASS.

- [ ] **Step 2: Frontend typecheck + tests**

Run:
```bash
cd web && npx tsc --noEmit && npm test
```
Expected: PASS.

- [ ] **Step 3: gofmt check**

Run: `gofmt -l .`
Expected: empty (no unformatted files).

- [ ] **Step 4: Manual live smoke test (optional, requires network)**

Start the server, then:
```bash
curl -s 'http://localhost:8080/api/vergex/signal-ranking?marketType=all&limit=5'
curl -s 'http://localhost:8080/api/vergex/signal-lab?marketType=core_perp&symbol=core_perp:BTC'
curl -s 'http://localhost:8080/api/vergex/cost-liquidation-heatmap?marketType=core_perp&symbol=core_perp:BTC'
curl -s 'http://localhost:8080/api/vergex/flow-markets?window=1h&limit=5'
```
Expected: `200` with data; heatmap returns `401` (with a warning logged) if no `VERGEX_API_TOKEN` set — acceptable graceful degradation.

- [ ] **Step 5: Commit any final fixes**

```bash
git add -A
git commit -m "chore(data-alt): final verification fixes"
```
(Only run if Step 1-3 produced fixes.)

---

## Self-Review Notes

- **Spec coverage:** Tasks 1-2 (variant plumbing / factory), 3 (schema), 4-5 (free clients), 6-7 (engine), 8 (handlers), 9 (token), 10 (verify) cover every item in the corrected design: replace paid sources, fix 4 collapsing families, crypto/stock bias split, token config, no new source types beyond the needed netflow/price enums.
- **Type consistency:** `ScopeVariant` values match what `buildCoinSource` and `getVergexSignalCoins` consume (`bull/bear/trending/gainers/losers/top/low/inflow/outflow/volume`). Backend config field names (`VergexDirection`, `NetflowLimit`, `PriceLimit`, source_types `netflow_top`/`netflow_low`/`price_top`/`price_low`) match across tasks.
- **Placeholder check:** all test files contain literal test code; all implementation steps contain literal Go/TS. Task 8's automated-test step is conditional (depends on whether `newTestServer` exists) — flagged explicitly rather than left vague.
