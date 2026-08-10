# NoFX Paid Data Sources — Endpoint Research

> **Purpose:** Catalog every backend `source_type` that currently requires a **paid** data provider (VergeX via Claw402, and NoFXOS data proxied through Claw402). The goal is to find **public / free API alternatives** that return equivalent rankings so the candidate-pool (step 1) and per-coin detail (step 2) can run without a Claw402 wallet/paywall.
>
> All paid requests are paid per-call with USDC via the **Claw402 x402** payment gateway. The two gateways share one wallet key: `CLAW402_WALLET_KEY`.

---

## How paid vs free is decided at runtime

At the kernel level (`kernel/engine.go`, `GetCandidateCoins`), the `coin_source.source_type` determines which data provider builds the candidate pool:

| `source_type` | Provider | Cost | Backend method |
|---|---|---|---|
| `static` | user-supplied list | free | — |
| `hyper_all` / `hyper_main` / `hyper_rank` | Hyperliquid-native | free | `GetAllCoinSymbols` / `GetMainCoinSymbols` / `GetFirstMeta` ranking |
| `ai500` | NoFXOS (or Claw402 proxy) | **paid** | `nofxosClient.GetTopRatedCoins` → `/api/ai500/list` |
| `oi_top` | NoFXOS (or Claw402 proxy) | **paid** | `nofxosClient.GetOITopPositions` → `/api/oi/top-ranking` |
| `oi_low` | NoFXOS (or Claw402 proxy) | **paid** | `nofxosClient.GetOILowPositions` → `/api/oi/low-ranking` |
| `vergex_signal` | VergeX via Claw402 | **paid** | `vergexClient.GetSignalRanking` → listing below |
| (`mixed`) | any combination above | mixed | — |

The per-coin **detail** step (`FetchVergexDataBatch`) is paid **only** when `source_type === "vergex_signal"`; non-vergex sources only fetch free raw OHLCV klines.

---

## The Claw402 x402 payment gateway

Two separate Claw402 clients route different families of paid calls:

1. **VergeX** (`provider/vergex/client.go`) — base `https://claw402.ai`, endpoints under `/api/v1/vergex/...`.
2. **NoFXOS data** (`provider/nofxos/claw402.go`) — routes NoFXOS endpoints to `https://claw402.ai/api/v1/nofx/...` with x402 payment.

Both send header `X-Client-ID: nofx` and sign the request with the Claw402 wallet private key (`payment.MakeClaw402SignFunc`, `payment.DoX402Request`). Cost is charged per request.

---

## 1. VergeX paid endpoints (`provider/vergex/client.go`)

Base URL: **`https://claw402.ai`**

| Endpoint (POST paid GET) | Method | Required params | Returns | Used for |
|---|---|---|---|---|
| `/api/v1/vergex/signal-ranking` | GET (x402) | `chain`, `liqBand` | ranked list: `rank`, `symbol`, `market_type`, `bias` (bullish/bearish/neutral), `confidence`, `score`, `category` | candidate pool for `vergex_signal` |
| `/api/v1/vergex/signal-lab` | GET (x402) | `marketType`, `symbol`, `chain`, `liqBand` | per-coin signal detail: `market`, `band`, `bias`, `confidence`, `structureRead`, `dimensions[]` (family/signal/direction/strength/percentile), `levels[]` (POC/magnet/resistance/support/value area), `metrics[]` (shortLiqAbove/longLiqBelow/cascadeVulnPct/...), `compositeZ` | `FetchVergexDataBatch` step 2 |
| `/api/v1/vergex/cost-liquidation-heatmap` | GET (x402) | `marketType`, `symbol`, `chain`, `liqBand` | per-coin heatmap: `binStep`, `bins[]` (bucketStartPrice/bucketEndPrice/longCost/shortCost/longLiq/shortLiq), `markPrice` | `FetchVergexDataBatch` step 2 |
| `/api/v1/vergex/flow-markets` | GET (x402) | `chain`, `window` (e.g. `1h`), `limit` | net-flow market list: `symbol`, `netFlow`, `buyNotional`, `sellNotional`, `trades`, `latestPrice` (used by frontend chart only, `api.getFlowMarkets`) | market flow (frontend) |

**Signal-ranking response parsing** is lenient: the client walks any JSON shape and pulls an array of rows with keys `symbol`/`ticker`/`base`/`coin`, `bias`/`direction`/`side`/`signal`, `compositeZ`/`score`, `confidence`, `marketType`/`market_type`/`venue`. So a replacement API returning any of those keys would parse with minimal changes.

Market-type normalization handles: `hip3_perp` (default, = TradeFi synthetic), `core_perp`, `stock`, `commodity`, `forex`, `index`, `pre_ipo`, `all`, etc.

---

## 2. NoFXOS paid/data endpoints (`provider/nofxos/`)

Free path base: **`https://nofxos.ai`** — every `/api/...` call appends `?auth=<AUTH_KEY>` (a NoFXOS API key).
Paid path base: **`https://claw402.ai`** — same path becomes `/api/v1/nofx/...`, `auth=` stripped, paid with x402.

| Endpoint | Params | Returns | Source type |
|---|---|---|---|
| `/api/ai500/list` | — | `data.coins[]`: `pair`, `score` (AI-rated top coins) | `ai500` |
| `/api/oi/top-ranking?limit=&duration=` | `limit`, `duration` (e.g. `1h`/`4h`/`24h`) | `data.positions[]`: `rank`, `symbol`, `amount` (OI increase) | `oi_top` |
| `/api/oi/low-ranking?limit=&duration=` | `limit`, `duration` | `data.positions[]`: OI decrease | `oi_low` |
| `/api/netflow/top-ranking?limit=&duration=&type=&trade=` | `limit`, `duration`, `type` = `institution`\|`personal`, `trade` = `future` | `data.netflows[]`: `rank`, `symbol`, `amount` (inflow) | netflow (frontend) |
| `/api/netflow/low-ranking?...` | same | net outflows | netflow (frontend) |
| `/api/price/ranking?duration=&limit=` | `duration`, `limit` | gainers/losers price ranking | price (frontend) |
| `/api/coin/{symbol}?include=` | `include` (comma list of data blocks) | per-coin quantitative data | detail/analysis |

**Frontend chart endpoints** (not candidate-pool, but the same paid family, used by `api.getFlowMarkets` / `getSignalRanking` on the dashboard):
- `/api/v1/vergex/flow-markets` (paid via Claw402)
- `/api/v1/vergex/signal-ranking` (paid via Claw402)

---

## 3. Frontend `source_type` → what the backend must resolve (what's missing)

The frontend scope catalog (`web/src/features/strategies/scopeCatalog.ts`) introduced these `ScopeUnit.source_type` values; each maps to a backend provider family:

| Frontend `source_type` (ScopeUnit) | Backend `source_type` (CoinSourceConfig) | Data provider | Currently paid? |
|---|---|---|---|
| `hyper_rank` | `hyper_rank` | Hyperliquid-native | free |
| `vergex` | `vergex_signal` | VergeX via Claw402 | **paid** |
| `ai500` | `ai500` | NoFXOS / Claw402 | **paid** |
| `nofxos_oi` | `oi_top` / `oi_low` | NoFXOS / Claw402 | **paid** |
| `nofxos_netflow` | `netflow_*` (prompt context) | NoFXOS / Claw402 | **paid** |
| `nofxos_price` | `price_*` (prompt context) | NoFXOS / Claw402 | **paid** |

**Backend work needed (from the frontend plan) — not yet implemented:**
1. **`custom` multi-scope AND/OR resolver** — `source_type: 'custom'` stores `custom_scope.scope_units[]` + `mode` (`overlap`=AND / `union`=OR). The backend must resolve these candidate pools in AND/OR form. **This is the main remaining backend step-1 work.**
2. **`decision_context` prompt-builder wiring** — persisted but unused at runtime.
3. **Paid source data providers** — VergeX scrape / NoFXOS free auth.
4. **Strategy version/snapshot endpoints** (`getVersions`/`getVersion`/`restoreVersion` return placeholder local data on the frontend).
5. **Strategy-level aggregate stats** (7D yield / Sharpe / Max DD) — frontend `getStrategyStats` returns `null` for these; the table shows `—`.

---

## Goal for these endpoints

Find public / free APIs that return **similar ranking content** to each paid endpoint so the candidate pool and per-coin detail can be built/scraped from publicly available data (a public exchange or analytics website with a similar endpoint). Equivalent-priority targets:

- **signal-ranking** (bias/score/confidence across crypto + TradeFi) → look for a public perp/synthetic ranking.
- **signal-lab** (per-coin structure read + dimensions + levels + liquidation metrics) → look for public open-interest / liquidation-cluster / long-short metrics per symbol.
- **cost-liquidation-heatmap** (price-binned long/short cost & liquidation clusters) → look for public liquidation heatmap data.
- **flow-markets** (net inflow/outflow per market) → look for public exchange net-flow / CEx fund-flow rankings.
- **ai500** (AI-rated top coins by score) → look for a free "AI top coins" / market-cap / momentum ranking.
- **oi_top / oi_low** (open-interest change rankings) → public exchange OI-change leaderboards (e.g. CoinGlass-style OI delta).
- **netflow** (institution/retail fund flow) → look for public CEx institutional/whale net-flow.

---

## Scope card → free endpoint mapping (fill in)

> These are the **9 paid crypto + 5 paid stock** scope cards from the frontend catalog (`web/src/features/strategies/scopeCatalog.ts`). Each card maps to a `source_type`; fill the **Free endpoint** column with the public URL you scrape from the VergeX strategy-creation page (or anywhere). The next session's LLM will analyze each endpoint, link it to the backend code (modifying the parsing if the returned shape differs from the paid endpoint), and flag any that don't link so we can decide whether to surface the data in the frontend.

| # | Scope card (`id`) | `source_type` | Backend source | Free endpoint |
|---|---|---|---|---|
| 1 | Bias Radar (Bullish) — crypto (`crypto-bias-bull`) | `vergex` | `vergex_signal` | (paste here) |
| 2 | Bias Radar (Bearish) — crypto (`crypto-bias-bear`) | `vergex` | `vergex_signal` | (paste here) |
| 3 | AI500 Data Provider (`crypto-ai500`) | `ai500` | `ai500` | (paste here) |
| 4 | OI Increase (`crypto-oi-increase`) | `nofxos_oi` | `oi_top` | (paste here) |
| 5 | OI Decrease (`crypto-oi-decrease`) | `nofxos_oi` | `oi_low` | (paste here) |
| 6 | Netflow Top (`crypto-netflow-top`) | `nofxos_netflow` | `netflow_*` | (paste here) |
| 7 | Netflow Outflow Top (`crypto-netflow-outflow`) | `nofxos_netflow` | `netflow_*` | (paste here) |
| 8 | Crypto Top Gainers (NOFXOS) (`crypto-gainers-nofxos`) | `nofxos_price` | `price_*` | (paste here) |
| 9 | Crypto Top Losers (NOFXOS) (`crypto-losers-nofxos`) | `nofxos_price` | `price_*` | (paste here) |
| 10 | Bias Radar (Bullish) — stock (`stock-bias-bull`) | `vergex` | `vergex_signal` | (paste here) |
| 11 | Bias Radar (Bearish) — stock (`stock-bias-bear`) | `vergex` | `vergex_signal` | (paste here) |
| 12 | Trending Stocks (`stock-trending`) | `vergex` | `vergex_signal` | (paste here) |
| 13 | Stock Gainers (`stock-gainers`) | `vergex` | `vergex_signal` | (paste here) |
| 14 | Stock Losers (`stock-losers`) | `vergex` | `vergex_signal` | (paste here) |

**Notes**
- `vergex`-sourced cards all resolve via the paid `vergex_signal` path (signal-ranking → candidate pool); the stock vs crypto category is carried by `hyper_rank_category` / market-type.
- Rows 6/7 share `nofxos_netflow`; rows 8/9 share `nofxos_price` — one free endpoint may back both rows of a pair.
- These 14 are the PAID scope cards; the 3 free crypto `hyper_rank` cards are excluded (already free).

---

## Other free endpoints (not tied to a scope card)

> Add any free endpoints you find that are **not** a 1:1 scope-card replacement (e.g. market-wide context: funding rates, long/short ratios, liquidations, OI aggregates, top movers, volume leaders, etc.). The next LLM will decide where each fits in the backend (prompt context, analyses, dashboards) or whether to surface to the frontend.

- (paste here)

---

## Per-coin detail (`FetchVergexDataBatch`) free endpoints

> These are the per-coin **detail** endpoints (paid via Claw402) that the backend calls for each candidate when `source_type = vergex_signal` (`kernel/engine.go` `FetchVergexDataBatch`). Add public alternatives that return equivalent per-symbol structure/liquidation data. If none link cleanly, we'll discuss whether to show the data on the frontend instead of using it in the strategy loop.

| # | Paid per-coin endpoint | Purpose (fields) | Free alternative endpoint |
|---|---|---|---|
| 1 | `https://claw402.ai/api/v1/vergex/signal-lab` | per-coin structure read: `market`, `band`, `bias`, `confidence`, `dimensions[]`, `levels[]` (POC/magnet/resistance/support/VWAP band), `metrics[]`, `compositeZ` | (paste here) |
| 2 | `https://claw402.ai/api/v1/vergex/cost-liquidation-heatmap` | per-coin liquidation/cost heatmap: `binStep`, `bins[]` (longCost/shortCost/longLiq/shortLiq per price band), `markPrice` | (paste here) |

---
