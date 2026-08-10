# Design — Replace Paid Data Sources with Verified Free Endpoints

> **Date:** 2026-08-11
> **Status:** Design (for review). Implementation plan follows only after approval.
> **Companion:** `data-alt-endpoints.md` (live verification report, repo root).

## 1. Goal

Remove the runtime dependency on the paywalled `claw402.ai`/`CLAW402_WALLET_KEY` data path, and make the candidate pool (kernel step 1) and per-coin detail (kernel step 2) run from **free, publicly accessible `vergex.trade` web endpoints**. The AI/LLM call (step 3) and order execution (step 4) are **out of scope** — they already work without Claw402.

Success = a strategy using any of the replaced source types yields a non-empty candidate pool (no more "No candidate coins available, cycle skipped") **without** a funded Claw402 wallet, using only free endpoints.

## 2. Scope

**In scope — candidate-pool replacements:**
- `oi_top`, `oi_low` via `trending-crypto?tab=oi` (top/low)
- `netflow_*` via `trending-crypto?tab=net_flow` (top/low)
- `price_*` via `trending-crypto?tab=price` (top/low)
- `vergex_signal` via `direction-change/leaderboard` (stock bias cards)

**In scope — per-coin detail:**
- SignalLab via `data-intelligence/markets/<mt>/<sym>/signals` (replaces paid `GetSignalLab`)
- FlowMarkets (frontend chart) via `data-intelligence/flow/markets`

**Out of scope / not replaced in this pass:**
- Stock bias "hot / movers" helper endpoints (`hl-stocks-hot`, `hl-stocks-movers`) — resolve symbol+rank but carry **no per-row bias/score/confidence**, so they can't drive a directional signal pool. Their scope cards stay on the paid path until a richer source is confirmed.
- `<trending-price>` HTML route — client-rendered, not server-scrapable.

**All four discarded subagent mislabels are corrected (data-alt-endpoints.md v2):**
- The Bearer token is **valid** (`riskbins`/`summary`/`direction-change`/`structure-overview`/`holders`/`coverage`/`markets` all return 200 with it).
- The leaderboard contains **both** `core_perp` (crypto) and `hip3_perp` (stock) rows, so crypto bias cards are **GO**.
- `ai500` is **NEEDS-ADAPTER** (a small hand-picked ~3-coin pool is the intended semantic, not a 500-pool).
- CostLiquidationHeatmap is **GO with token** (not NO-GO).

**Non-goals (do not build now):**
- No new DB schema, no strategy-version endpoints, no aggregate-stats endpoints (these are separate backend tasks in the handoff, not part of the data-source swap).
- No live order/position behavior changes (HIGH-RISK surface — untouched).

## 3. Approach (recommended)

**Add free-endpoint fetchers beside the existing paid clients; route `vergex.trade` for the GO sources; keep the paid clients for the not-yet-replaced (NO-GO) sources.** No deletion of paid code — leave it for any source that stays paywalled.

Concretely:
- A new lightweight HTTP client (or thin functions) that GETs the `vergex.trade` endpoints, unauthenticated, through the existing SSRF-safe `security.SafeGet`/`SafeHTTPClient` path.
- **Envelope adapters** for the strict-unmarshal structs (`OIPosition`/`NetFlowPosition`/`PriceRankingItem`): the free rows match the struct fields 1:1, only the outer envelope (`{top,low}` vs `{success,data{…}}`) differs. Adapters fetch the relevant array (`top`/`low`) and wrap it into the existing strict response envelope, reusing unchanged row structs.
- **Lenient reuse** for `vergex_signal`/SignalLab: the existing `parseRankItem` already consumes the leaderboard; SignalLab is opaque `json.RawMessage` passthrough. These need only a URL/base swap + a `directionScore`→`score` key mapping so `SignalRankItem.Score` is populated.
- **Token-gated calls** (heatmap `riskbins`, and other per-coin detail endpoints that need auth): send a configurable Bearer token (user-supplied via config/DB) and degrade gracefully on 401. No hard dependency.
- **Bias scope-card split**: the leaderboard mixes `core_perp` (crypto) + `hip3_perp` (stock); the getter filters rows by nested `market.marketType` to serve the correct crypto vs stock bull/bear card.
- **Source wiring** in `kernel/engine.go` getters: each `getOITopCoins`/`getOILowCoins`/netflow/price/`getAI500Coins` getter reads from the free adapter. `getVergexSignalCoins`/`FetchVergexDataBatch`(SignalLab + heatmap) read from the free endpoints.

## 4. Component boundaries

| Unit | Responsibility | Depends on |
|---|---|---|
| `provider/vergex/` free-client | GET `vergex.trade` endpoints | `security.SafeGet`, HTTP |
| `provider/nofxos/` envelope adapters | wrap `{top,low}` into `OIRankingResponse`/`NetFlowResponse`/`PriceRankingResponse` | `security`, row structs |
| `kernel/engine.go` getters | pick free vs paid per `source_type`; build `CandidateCoin[]` / `MarketAnalysis` | providers, `store.MaxCandidateCoins` |

Each unit is independently testable: free-client tested via `httptest`; adapters tested as pure JSON wrapper functions; engine getters tested with a stubbed provider.

## 5. Data flow

1. Trader cycle → `kernel.GetCandidateCoins()` → per `source_type`, the getter calls the free adapter.
2. `trending-crypto?tab=oi&duration&limit` returns `{top[],low[]}`; adapter wraps into `data.positions` → `OIPosition[]` → `CandidateCoin{Symbol,Sources:["oi_top"]}` → `filterExcludedCoins`.
3. For `vergex_signal`, `direction-change/leaderboard` → lenient `parseRankItem` → `SignalRankItem[]` → directional candidate pool (existing logic).
4. For per-coin detail, `FetchVergexDataBatch` calls the free `/signals` endpoint per symbol → `MarketAnalysis.SignalLab` raw JSON → LLM prompt.

## 6. Error handling

- Free endpoint failure is **non-fatal to the wallet**: log a warning and degrade. For candidate-pool getters, an empty result is already a normal, handled condition ("cycle skipped"); failure must not panic or crash the loop.
- Reuse existing safe-error wrapping (`fmt.Errorf("...: %w", err)`) and `logger.Warnf`; never leak internals to clients.
- Token-gated endpoints (heatmap) that 401 must be logged and skipped, not retried endlessly.

## 7. Testing

- **Adapters:** table-driven unit tests feeding `{top,low}` fixtures; assert typed `OIPosition`/`NetFlowPosition`/`PriceRankingItem` output and envelope fields.
- **Free client:** `httptest.Server` returning the captured real snapshots; assert decoded rows.
- **Engine getters:** stub provider; assert `CandidateCoin` ordering/limit/exclusion.
- **Regression:** existing `provider/vergex/client_test.go` and engine tests must stay green; run `go build ./... && go vet ./...`.

## 8. Open questions (resolve before implementation)

1. **Runtime token source** — `riskbins` (heatmap) and some per-coin detail endpoints need a valid Bearer token. The recorded token works but has no `exp` claim, so its lifetime is unknown. Options: (a) user-supplied token (`.env`/DB) sent on authed calls with graceful 401 degradation; (b) leave heatmap absent. **Recommend: (a) optional token, degrade gracefully.**
2. **`directionScore`→score mapping** — confirmed yes: add the key mapping so `SignalRankItem.Score` is populated.
3. **ai500 pool size** — free ai500 returns ~3 hand-picked coins. Confirm that's an acceptable strategy pool (paid API was the same shortlist concept). **Default: yes.**
4. **Crypto vs stock bias split** — engine must filter leaderboard by `market.marketType` (`core_perp` vs `hip3_perp`) for the correct scope card. Confirm existing `VergexMarketType`/category handling covers this or needs a filter addition.

## 9. Deliverable flow

1. This design (spec) + `data-alt-endpoints.md` reviewed/approved.
2. `writing-plans` → implementation plan at `docs/superpowers/plans/2026-08-11-alt-data-endpoints.md`.
3. Implementation for sources resolved GO/NEEDS-ADAPTER (all candidate-pool sources + signal-lab + heatmap + flow-markets + leaderboard). Only the stock hot/movers helper cards (no per-row bias) stay unserved.
