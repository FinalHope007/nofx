# NOFX Handoff — Backend Implementation for Strategy Manager (+ Free/Paid Data Sources)

> Handoff. The frontend Strategy Manager is built and on `dev`. **Task 3 (paid source providers → free vergex.trade) is now DONE.** This file is the reference for the remaining backend tasks (1, 2, 4, 5 below).

## Repo, branch, stack
- Go 1.25 backend (`go.mod`, module `nofx`) + React 18/TS/Vite frontend (`web/`). Branch: `dev`. Worktree clean.
- Backend layout: `api/` (Gin HTTP handlers), `kernel/` (strategy engine + prompt/decision), `store/` (GORM DB: sqlite/postgres), `trader/` (exchange adapters + auto-trader loop), `manager/` (trader lifecycle), `provider/` (data providers: `hyperliquid`, `nofxos`, `vergex`, `coinank`, `binance`), `mcp/` (LLM clients + x402 payment), `market/` (klines/indicators).
- AI calls go through local **Bifrost** gateway (`custom` provider, `ALLOW_LOCAL_CUSTOM_API=1` SSRF exemption). Backend + frontend run together; `.env` holds `JWT_SECRET` etc.

## What shipped (frontend, on `dev`, all committed)
A full **Strategy Manager** replacing the old `/strategy` page. New `web/src/features/strategies/`:
- `StrategyManagerPage` (table: #, Name/Version, AUM, Symbols, 7D Yield, Sharpe, MaxDD, Last Update, NAV curve, Actions incl. **Delete** w/ confirm).
- Two-step wizard: `ScopeStepPage` (scope cards + Overlap/Union toggle + Top-N) → `EditorStepPage` (name, prompt, interval, leverage, margin, candles, excluded coins, **decision-context** toggle).
- `scopeCatalog.ts` (all free + paid scope cards), `draftStore.ts` (zustand), `strategyFactory.ts` (config JSON builder), `strategyApi.ts` (typed API + placeholder methods), `VersionHistoryModal` (side drawer), `tableHelpers.tsx`.
- Wiring to existing backend: `api.createStrategy/updateStrategy/deleteStrategy/activateStrategy/duplicateStrategy/getStrategy`, `api.getTraders/getAccount/getPositions/getEquityHistoryBatch`.
- Old `web/src/pages/StrategyStudioPage.tsx` **archived** → `web/src/pages/legacy/StrategyStudioPage.legacy.tsx` (moved, NOT deleted).
- Trader-config leverage is seeded from the linked strategy's `risk_control` at create time, and the dashboard reads live leverage from the strategy.
- Editing a strategy is **blocked** while a running trader uses it (stop first).

## Frontend placeholders that need BACKEND implementation (priority for this session)
These are the gaps between the completed UI and the backend. The frontend already calls these; the backend does not fully support them yet.

### 1. `custom` multi-scope AND/OR resolver (MAIN step-1 backend work)
The wizard lets a user select **multiple scope cards** and combine them via **Overlap (=AND)** or **Union (=OR)**. When >1 scopes are selected, the factory sets:
```go
// coin_source:
source_type: "custom"
scope_mode: "overlap" | "union"
custom_scope: { scope_units: [ { id, category, source_type, direction?, limit, label, provider } ], mode }
```
The backend must resolve `custom_scope.scope_units[]` into a candidate pool:
- **Union**: any candidate present in ≥1 selected source.
- **Overlap**: candidate must be present in **all** selected sources.
Each `scope_unit.source_type` maps to one of the single-source getters already in `kernel/engine.go` (`getAI500Coins`, `getOITopCoins`, `getOILowCoins`, `getHyperRankCoins`, `getHyperAllCoins`, `getHyperMainCoins`, `getVergexSignalCoins`). Add a `case "custom":` in `GetCandidateCoins` that builds per-source sets then intersects/unions them. NOTE: a single selected scope already maps to its concrete `source_type` (free path works); only multi-scope is `custom`.

### 2. `decision_context` prompt-builder wiring
`ai_config.decision_context = { enabled, recent_count, mode: "structured"|"digest" }` is persisted by the UI but **unused** at runtime. Wire it into the prompt builder in `kernel/` (feed recent decisions into the system prompt when enabled; `recent_count` limits how many; `mode` chooses structured vs digest formatting).

### 3. Paid source providers — ✅ DONE (this session)
`vergex_signal`, `ai500`, `oi_top`, `oi_low`, and the netflow/price rankings now use **free `vergex.trade` endpoints** — no Claw402 wallet required for the candidate pool, per-coin detail, or the four frontend vergex handlers. `VERGEX_API_TOKEN` is optional (only `riskbins` needs it). See `data-alt-endpoints.md` + the follow-up fixes below. Do NOT redo this task; remaining work is only follow-ups, not the core paid→free migration.

### 4. Strategy version/snapshot endpoints
`web/src/features/strategies/strategyApi.ts` has placeholder `getVersions`/`getVersion`/`restoreVersion` that return only the current config as a synthetic `v1` snapshot. Backend needs real endpoints:
- `GET  /api/strategies/:id/versions`
- `GET  /api/strategies/:id/versions/:version`
- `POST /api/strategies/:id/restore` (snapshot-before-restore so it's reversible)
- New DB table `strategy_versions` (version number, snapshot of `config`, note, created_at, is_current). Create snapshot v1 on strategy create, snapshot the pre-edit state as a new version on each update.

### 5. Strategy-level aggregate stats
`getStrategyStats` in `web/src/features/strategies/strategyApi.ts` returns live AUM/symbols/NAV from trader endpoints but `sevenDayYield`, `sharpe`, `maxDd` are `null` (table shows `—`). Backend needs a merged-book windowed calc: merge linked traders' equity snapshots into one curve, then compute 7D yield, Sharpe, max drawdown.

## Key backend files to touch (by task)
- **custom resolver**: `kernel/engine.go` `GetCandidateCoins` switch; reuse single-source getters.
- **decision_context**: `kernel/engine_prompt.go` / `kernel/prompt_builder.go` system-prompt assembly; `kernel/engine_analysis.go` `GetFullDecisionWithStrategy`.
- **paid sources**: ✅ DONE — see `provider/vergex/client.go` (free/x402 mode switch, `FreeDetailSymbol`), `provider/nofxos/free.go`, `trader/symbols.go` (`ExchangeSymbol`), `kernel/engine.go` getters, `api/handler_vergex.go`, `data-alt-endpoints.md`. No further work here unless a new free-source gap is found.
- **versions endpoint**: `api/strategy.go` (new handlers near `handleGetStrategy`), `store/strategy.go` new table/methods, register routes in `api/route_registry.go` / `api/server.go`.
- **stats endpoint**: new handler + store query that aggregates trader equity history per strategy.

## Constraints to respect
- `go vet ./...` + `gofmt` clean; frontend `tsc` + `npm test` pass if the frontend changes too.
- English-only UI strings (frontend); backend uses `SafeError`/`SafeInternalError`/`SanitizeError` for errors (never leak internals).
- HIGH-RISK: any change to live order/position behavior needs explicit confirmation. Stopping/deleting a trader does **NOT** close open positions by design.
- `store.*` is the only DB access layer. All timestamps UTC.

## Paid-source → free vergex.trade migration (Task 3) — ✅ completed

This was delivered across the `dev` branch. Key design: the existing `provider/vergex.Client` gained a free/x402 **mode switch** (`NewFreeClient`, `doFreeGET`, free path consts, optional Bearer token) reusing the same methods (`GetSignalRanking`/`GetSignalLab`/`GetCostLiquidationHeatmap`/`GetFlowMarkets`) and `ParseSignalRanking`. `provider/nofxos/free.go` adds a thin `FreeTrendingClient` for the OI/netflow/price crypto `trending-crypto` endpoints (different envelope) + ai500. The engine and the 4 vergex handlers route through the free client, and `free mode is the production default`.

**Runtime follow-up fixes (all in this session, committed on `dev`):**
- **Market-qualified per-coin detail symbol + URL-encoding** — `FreeDetailSymbol` / `resolveDetailMarket` / `stripDetailSymbolQualifier` in `provider/vergex/client.go`. Per-coin `signals`/`riskbins` use the correct symbol form (`core_perp:BTC` for crypto, `hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:SP500` for stock), classified by the **symbol's asset family** (not marketType), and the path prefix uses the resolved concrete market type. Fixes the 400/404 on `perp` and `hip3_perp+PUMP`.
- **Per-coin detail for ALL source types** — removed the `vergex_signal`-only gate in `enrichVergexDataWithStrategy`/`FetchVergexDataBatch`; generic user prompt omits absent/error Signal Lab/Heatmap sections (non-mutating shallow copy). `vergex_signal` keeps its Claw402 rules prompt.
- **Crypto/stock market-family isolation** — `filterSignalRankingItems` now excludes `hip3_perp` rows from a `core_perp` (crypto) pool (and vice versa), so a crypto-bias strategy no longer leaks stocks into the pool or 404s its detail fetch.
- **Exchange-form symbol at the order layer** — `trader/symbols.go` `ExchangeSymbol(symbol, exchange)`; wired into the 4 order functions in `trader/auto_trader_orders.go`. `vergex_signal` crypto emits bare `XRP` (per the prompt contract), so on CEX adapters (binance/bybit/okx/bitget/gate/kucoin/aster) the order layer resolves it to `XRPUSDT` for the live calls + position matching; Hyperliquid/`xyz:` stay unchanged. Fixes Binance `code=-1121 Invalid symbol` (and analogous Gate/OKX/etc.).
- **Per-trader-exchange kline sourcing** — `market.GetWithTimeframesWithExchange` + `StrategyEngine.exchange`; binance→Binance, hyperliquid→Hyperliquid, else→CoinAnk.
- **Dashboard funnel** — `flow`/`signal` topology layers gated to `vergex_signal` strategies (empty otherwise); their paid x402 fetches are also gated so non-vergex dashboards stop polling them.

`VERGEX_API_TOKEN` (optional): set in local `.env` for `riskbins` (heatmap) live data; it has no `exp` claim. Do not commit it.

⚠️ **Verification note for the next session:** a backend rebuild + server restart is required to pick up newer commits. The free per-coin detail runtime behavior (Symbol type, market-family isolation, exchange-form symbols) was verified via unit tests + user live retests (crypto heatmap 200, Binance crypto open works); other exchanges' live order paths were not re-verified outside Binance.

## Current status & next steps (start of a new session)

- **Task 3 — Paid source providers: DONE** (see the migration + follow-up fixes above).
- **Remaining backend tasks, in order of the handoff list:**
  1. **`custom` multi-scope AND/OR resolver** (Task 1 above — the MAIN remaining backend work). Single-scope works; multi-scope emits `source_type:"custom"` which the backend does not yet handle (`GetCandidateCoins` has no `case "custom"`). Add it.
  2. **`decision_context` prompt wiring** (Task 2 above).
  3. **Strategy version/snapshot endpoints** (Task 4 above; placeholder `getVersions` etc. in `web/src/features/strategies/strategyApi.ts`).
  4. **Strategy-level aggregate stats** (Task 5 above).
- **Branch/etc.:** `dev`. Running tree clean. Frontend + backend pass `go test ./...` / `tsc --noEmit` / `npm test`.
- **Recommended read first:** `data-alt-endpoints.md`, `paidsource-research.md`, and the Task 3 section above so you don't re-implement what's done.

## Quick verification
- Backend: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./...`
- Frontend: `cd web && npx tsc --noEmit && npm run build && npm test`
