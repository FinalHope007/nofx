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

| # | Scope card (`id`) | `source_type` | Backend source | Free endpoint | Note |
|---|---|---|---|---|---|
| 1 | Bias Radar (Bullish) — crypto (`crypto-bias-bull`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/direction-change/leaderboard` | This endpoint returns a mixed market (crypto, stocks, commodities, indices) ranking of bias radar and oi rank. Rank 1 = most bullish. Need to filter out for crypto and bullish only. |
| 2 | Bias Radar (Bearish) — crypto (`crypto-bias-bear`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/direction-change/leaderboard` | Same as bullish, but for bearish access larger rank. |
| 3 | AI500 Data Provider (`crypto-ai500`) | `ai500` | `ai500` | `https://vergex.trade/trending-category?lang=en&key=ai500` | This endpoint returns a N out of 100 score for potential candidate, alert start time and start price, and price change percentage since alert start. |
| 4 | OI Increase (`crypto-oi-increase`) | `nofxos_oi` | `oi_top` | `https://vergex.trade/trending-crypto?tab=oi&duration=24h&limit=50` | This endpoint returns both top and low ranking oi data with the key "top" and "low" respectively. Options for duration: 5m, 15m, 30m, 1h, 4h, 8h, 12h, 24h. Includes price and price change percentage. |
| 5 | OI Decrease (`crypto-oi-decrease`) | `nofxos_oi` | `oi_low` | `https://vergex.trade/trending-crypto?tab=oi&duration=24h&limit=50` | Same as top-ranking |
| 6 | Netflow Top (`crypto-netflow-top`) | `nofxos_netflow` | `netflow_*` | `https://vergex.trade/trending-crypto?tab=net_flow&duration=24h&limit=50` | This endpoint returns both inflow and outflow ranking net flow data with the key "top" and "low" respectively. Options for duration: 5m, 15m, 30m, 1h, 4h, 8h, 12h, 24h. Includes price and price change percentage. 
| 7 | Netflow Outflow Top (`crypto-netflow-outflow`) | `nofxos_netflow` | `netflow_*` | `https://vergex.trade/trending-crypto?tab=net_flow&duration=24h&limit=50` | Same as top-ranking |
| 8 | Crypto Top Gainers (NOFXOS) (`crypto-gainers-nofxos`) | `nofxos_price` | `price_*` | `https://vergex.trade/trending-crypto?tab=price&duration=24h&limit=50` | This endpoint returns both top and low ranking price data with the key "top" and "low" respectively. Options for duration: 15m, 30m, 1h, 4h, 8h, 12h, 24h. Includes price and price change percentage. |
| 9 | Crypto Top Losers (NOFXOS) (`crypto-losers-nofxos`) | `nofxos_price` | `price_*` | `https://vergex.trade/trending-crypto?tab=price&duration=24h&limit=50` | Same as top-ranking |
| 10 | Bias Radar (Bullish) — stock (`stock-bias-bull`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/direction-change/leaderboard` | This endpoint returns a mixed market (crypto, stocks, commodities, indices) ranking of bias radar and oi rank. Rank 1 = most bullish. Need to filter out for stock and bullish only. |
| 11 | Bias Radar (Bearish) — stock (`stock-bias-bear`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/direction-change/leaderboard` | Same as bullish, but for bearish access larger rank. |
| 12 | Trending Stocks (`stock-trending`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/market-data/hl-stocks-hot?limit=10` | - |
| 13 | Stock Gainers (`stock-gainers`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/market-data/hl-stocks-movers?direction=gainers&limit=10` | - |
| 14 | Stock Losers (`stock-losers`) | `vergex` | `vergex_signal` | `https://vergex.trade/api/v1/market-data/hl-stocks-movers?direction=losers&limit=10` | - |

**Notes**
- `vergex`-sourced cards all resolve via the paid `vergex_signal` path (signal-ranking → candidate pool); the stock vs crypto category is carried by `hyper_rank_category` / market-type.
- Rows 6/7 share `nofxos_netflow`; rows 8/9 share `nofxos_price` — one free endpoint may back both rows of a pair.
- These 14 are the PAID scope cards; the 3 free crypto `hyper_rank` cards are excluded (already free).
- All of the endpoints for candidate pools sourcing do not require authorization unlike the per-coin detail endpoint shown below.

---

## Other free endpoints for candidate pools sourcing (not tied to a scope card)

> Add any free endpoints you find that are **not** a 1:1 scope-card replacement (e.g. market-wide context: funding rates, long/short ratios, liquidations, OI aggregates, top movers, volume leaders, etc.). The next LLM will decide where each fits in the backend (prompt context, analyses, dashboards) or whether to surface to the frontend. If none link cleanly, we'll discuss whether to show the data on the frontend instead of using it in the strategy loop.

(Somewhat useful endpoints for candidate pools sourcing):
- `https://vergex.trade/trending-crypto?tab=depth&limit=20` | This endpoint returns the top depth difference between bid and ask volumes for future and spot markets.
- `https://vergex.trade/trending-crypto?tab=rates&limit=20` | This endpoint returns the top and low funding rate.
- `https://vergex.trade/trending-hl?category=crypto` | This endpoint returns an unranked data for the category. Options for category: `crypto`, `stocks`, `indices`, `commodities`, `fx`, `other&sub=preipo`.
- `https://vergex.trade/api/v1/market-data/hl-stocks-universe?sortBy=baseAsset` | Same as `https://vergex.trade/trending-hl?category=stocks`, but sorted by base asset (in alphabetical order).
- `https://vergex.trade/api/v1/market-data/hot-assets?exchange=hyperliquid&limit=30` | This endpoint returns the top 30 hot assets on Hyperliquid, mixed markets.
- `https://vergex.trade/api/v1/data-intelligence/flow/markets?window=1h&limit=25` | This endpoint returns the top inflow and outflow volume for the mixed markets (crypto, stocks, indices, commodities, fx) with the key `inflow` and `outflow`. Options for window: 5m, 15m, 1h, 4h, 8h, 12h, 24h. Useful for netflow analysis for market other than crypto. For crypto, see #6 or #7 (crypto-netflow-top/crypto-netflow-outflow) scope card.

(Not useful endpoints but have informative descriptions for building frontend UI):
- `https://vergex.trade/api/v1/market-data/agent-assets` | This endpoint returns all the assets available in vergex and a short description for the a list of symbols (mixed markets).
- `https://vergex.trade/api/v1/market-data/stock-profiles` | This endpoint returns stock profiles description for the a list of stocks symbols.


---

## Per-coin detail (`FetchVergexDataBatch`) free endpoints

> These are the per-coin **detail** endpoints (paid via Claw402) that the backend calls for each candidate when `source_type = vergex_signal` (`kernel/engine.go` `FetchVergexDataBatch`). Add public alternatives that return equivalent per-symbol structure/liquidation data.

| # | Paid per-coin endpoint | Purpose (fields) | Free alternative endpoint | Auth | Notes |
|---|---|---|---|---|---|
| 1 | `https://claw402.ai/api/v1/vergex/signal-lab` | per-coin structure read: `market`, `band`, `bias`, `confidence`, `dimensions[]`, `levels[]` (POC/magnet/resistance/support/VWAP band), `metrics[]`, `compositeZ` | `https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/signals?chain=mainnet&liqBand=15` | Needed | Requires marketType+symbol (core_perp%3ABTC for crypto or hip3_perp%3AMSFT for stocks), Optional liqBand tunes the liquidation banding |
| 2 | `https://claw402.ai/api/v1/vergex/cost-liquidation-heatmap` | per-coin liquidation/cost heatmap: `binStep`, `bins[]` (longCost/shortCost/longLiq/shortLiq per price band), `markPrice` | `https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/riskbins` | Needed | Requires marketType+symbol (core_perp%3ABTC for crypto or hip3_perp%3AMSFT for stocks), Optional liqBand tunes the liquidation banding |

**Notes**
- Most of the endpoints for per-coin detail need to include authorization headers `Bearer Vergex_Auth_Key` in order to access the endpoint. I have included my current authorization headers here. We need to discuss on how the backend obtains this token in the future.
- For /signals and /riskbins endpoints, when calling for stock market data, use this format: `https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500/riskbins`, `https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3AMSFT/summary`. `0x88806a71d74ad0a510b350545c9ae490912f0888` is fixed contract address for hyperliquid deployer.

---

## Other free endpoints for per-coin detail sourcing (not tied to the `FetchVergexDataBatch` call)
| Endpoint | Auth | Notes |
| --- | --- | --- |
(For market data):
- `https://vergex.trade/api/v1/market-data/market-cap?symbol=BTC` | Not needed | For stock, use symbol=`SP500` or `MSFT` directly
- https://vergex.trade/api/v1/direction-change/BTC/current | Needed | For stock, use `xyz%3ASP500` or `xyz%3AMSFT` directly
- https://vergex.trade/api/v1/direction-change/BTC/history?type=all&page=1&page_size=20 | Needed | For stock, use `xyz%3ASP500` or `xyz%3AMSFT` directly
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/summary | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/structure-overview?chain=mainnet&marketId=core_perp%3ABTC | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://fapi.binance.com/fapi/v1/ticker/24hr?symbol=BTCUSDT | Not Needed | Unknown for stocks

(For position data such as position ranking, whale position changes, and position groups):
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/cohorts | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/coverage?scope=market&marketType=core_perp&marketId=core_perp%3ABTC | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/group-migration?dimension=notional | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/group-stats?dimension=risk | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/group-stats?dimension=wallet | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/holders?view=ranked&rankType=overall&limit=50 | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/group-flow/snapshot-diff?dimension=notional&window=4h | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format
- https://vergex.trade/api/v1/data-intelligence/markets/core_perp/core_perp%3ABTC/whale-activity?window=4h&minChange=10000&limit=500 | Needed | For stock, use `hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500` format

- https://vergex.trade/api/v1/data-intelligence/markets?status=active&sort=openInterest&limit=50 | Needed | Get a list of market data from a list of mixed market types (crypto, stocks, and more)
