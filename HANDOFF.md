# NOFX Handoff — Backend Implementation for Strategy Manager (+ Free/Paid Data Sources)

> Replacement handoff. The frontend Strategy Manager is now built and on `dev`. This session is the **backend implementation pass**. Start by reading the priority task at the bottom (verify alternative data endpoints) and `paidsource-research.md`.

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

### 3. Paid source providers (the focus of the first task)
`vergex_signal`, `ai500`, `oi_top`, `oi_low`, and the netflow/price rankings currently require a Claw402 wallet (paid x402) or a NoFXOS auth key. See `paidsource-research.md` for every endpoint. **Goal: find public/free alternatives.** Details below under "First task".

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
- **paid sources**: `provider/vergex/client.go`, `provider/nofxos/*.go`, `kernel/engine.go` getters; `api/handler_vergex.go`; config for provider base URLs / auth keys (`provider/nofxos/client.go` `DefaultAuthKey`).
- **versions endpoint**: `api/strategy.go` (new handlers near `handleGetStrategy`), `store/strategy.go` new table/methods, register routes in `api/route_registry.go` / `api/server.go`.
- **stats endpoint**: new handler + store query that aggregates trader equity history per strategy.

## Constraints to respect
- `go vet ./...` + `gofmt` clean; frontend `tsc` + `npm test` pass if the frontend changes too.
- English-only UI strings (frontend); backend uses `SafeError`/`SafeInternalError`/`SanitizeError` for errors (never leak internals).
- HIGH-RISK: any change to live order/position behavior needs explicit confirmation. Stopping/deleting a trader does **NOT** close open positions by design.
- `store.*` is the only DB access layer. All timestamps UTC.

## First task — verify alternative (public/scraped) data endpoints
Read **`paidsource-research.md`** (repo root) for the full endpoint catalog. Then, for each paid source below, the NEXT LLM should **research and verify a public/free API endpoint** that returns equivalent ranking data, and report back with concrete URLs + response shapes + a feasibility note. Do NOT implement until alternatives are confirmed:
1. `vergex_signal` ranking (bias/score/confidence) — look for a public perp/synthetic high/low ranking.
2. `vergex` signal-lab (per-coin structure/levels/liquidation metrics).
3. `vergex` cost-liquidation-heatmap (price-binned long/short cost + liq clusters).
4. `vergex` flow-markets (net inflow/outflow per market).
5. `ai500` (AI-rated top coins) — a free momentum/cap ranking.
6. `oi_top` / `oi_low` (open-interest change leaderboard) — e.g. CoinGlass-style OI delta.
7. netflow (institution/retail fund flow) — CEx flow leaderboard.

**Deliverable:** a short markdown (`data-alt-endpoints.md` at repo root, or extend `paidsource-research.md`) listing, per source: the public endpoint URL(s), params, exact JSON field mapping to the current `SignalRankItem`/`OIPosition`/`NetFlowPosition`/`CoinData` structs, and a GO/NO-GO. Only after that review should any code be written.

## Quick verification
- Backend: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./... && go vet ./...`
- Frontend: `cd web && npx tsc --noEmit && npm run build && npm test`
