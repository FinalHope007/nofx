# NoFX Free Alternative Data Endpoints — Verification Report

> **Purpose:** Replace every paywalled `claw402.ai`/NoFXOS paid data source with a free, publicly accessible `vergex.trade` web endpoint, so the candidate pool (step 1) and per-coin detail (step 2) run without a Claw402 wallet.
>
> **Status:** All endpoints below were fetched **live** on 2026-08-11 (and re-verified inline against user correction) and their real response JSON compared against the backend Go structs/parsers. Sources are marked **GO / NEEDS-ADAPTER / NO-GO**. Per the handoff gate, **no code has been written** — this report is the review point before any implementation.
>
> **Correction note (v2):** an initial automated pass mislabeled the Bearer token as dead and the leaderboard as stock-only. **Both were wrong** — the token is valid (earlier failures used an unencoded symbol path and a corrupted token copy), and the leaderboard contains both crypto and stock rows. This revision corrects those verdicts.
>
> **Raw response snapshots** are in `/tmp/opencode/altdata/` (a-…, b-…, c-…, d-….txt), not committed.

---

## Headline verdicts

| Backend source | Free endpoint | Verdict |
|---|---|---|
| `vergex_signal` (bias bull/bear, crypto + stock) | `/api/v1/direction-change/leaderboard` | **GO** (lenient; crypto+stock rows) |
| `oi_top` | `/trending-crypto?tab=oi` → `top[]` | **NEEDS-ADAPTER** (envelope only) |
| `oi_low` | `/trending-crypto?tab=oi` → `low[]` | **NEEDS-ADAPTER** (envelope only) |
| `netflow_top` | `/trending-crypto?tab=net_flow` → `top[]` | **NEEDS-ADAPTER** (envelope only) |
| `netflow_low` | `/trending-crypto?tab=net_flow` → `low[]` | **NEEDS-ADAPTER** (envelope only) |
| `price_*` (gainers/losers) | `/trending-crypto?tab=price` | **NEEDS-ADAPTER** (envelope only) |
| `ai500` | `/trending-category?key=ai500` | **NEEDS-ADAPTER** (hand-picked ~3 coins = intended pool) |
| **SignalLab** (per-coin detail) | `/data-intelligence/markets/<mt>/<sym>/signals` | **GO** (no auth, opaque passthrough) |
| **CostLiquidationHeatmap** (per-coin) | `/data-intelligence/markets/<mt>/<sym>/riskbins` | **GO with token** (200 verified) |
| **FlowMarkets** (frontend) | `/data-intelligence/flow/markets` | **GO** (no auth, 1:1) |

### Token: valid (corrected)

The Bearer token in `paidsource-research.md:170` **works**. Verified with the exact token: `riskbins`, `summary`, `direction-change/BTC/current`, `structure-overview`, `holders`, `coverage`, `markets` list all return **HTTP 200**. Earlier 401s were from an unencoded symbol path (`core_perp:BTC` vs `core_perp%3ABTC`) and a corrupted token copy. The token has **no `exp` claim**, so its server-side lifetime is unknown — treat it as valid-now but plan a runtime token source/config for resilience (see Open questions).

---

## Scope card → verified free endpoint mapping

All `crypto-*` OI/netflow/price rows below use `GET https://vergex.trade/trending-crypto?tab=<T>&duration=<D>&limit=<N>` with `D ∈ {5m,15m,30m,1h,4h,8h,12h,24h}`.

| # | Scope card (`id`) | `source_type` | Free endpoint | Verdict |
|---|---|---|---|---|
| 1 | Bias Radar Bull crypto (`crypto-bias-bull`) | `vergex` | `GET /api/v1/direction-change/leaderboard` + filter `bias==bullish`, crypto | **GO** |
| 2 | Bias Radar Bear crypto (`crypto-bias-bear`) | `vergex` | same + filter `bias==bearish`, crypto | **GO** |
| 3 | AI500 (`crypto-ai500`) | `ai500` | `GET /trending-category?lang=en&key=ai500` | **NEEDS-ADAPTER** (small hand-picked pool) |
| 4 | OI Increase (`crypto-oi-increase`) | `nofxos_oi` | `/trending-crypto?tab=oi` → `top[]` | **NEEDS-ADAPTER** |
| 5 | OI Decrease (`crypto-oi-decrease`) | `nofxos_oi` | `/trending-crypto?tab=oi` → `low[]` | **NEEDS-ADAPTER** |
| 6 | Netflow Top (`crypto-netflow-top`) | `nofxos_netflow` | `/trending-crypto?tab=net_flow` → `top[]` | **NEEDS-ADAPTER** |
| 7 | Netflow Outflow Top (`crypto-netflow-outflow`) | `nofxos_netflow` | `/trending-crypto?tab=net_flow` → `low[]` | **NEEDS-ADAPTER** |
| 8 | Crypto Top Gainers (`crypto-gainers-nofxos`) | `nofxos_price` | `/trending-crypto?tab=price` → `top[]` | **NEEDS-ADAPTER** |
| 9 | Crypto Top Losers (`crypto-losers-nofxos`) | `nofxos_price` | `/trending-crypto?tab=price` → `low[]` | **NEEDS-ADAPTER** |
| 10 | Bias Radar Bull stock (`stock-bias-bull`) | `vergex` | `/direction-change/leaderboard` + filter `bias==bullish`, hip3_perp-equity | **GO** |
| 11 | Bias Radar Bear stock (`stock-bias-bear`) | `vergex` | same + `bias==bearish` | **GO** |
| 12 | Trending Stocks (`stock-trending`) | `vergex` | `GET /api/v1/market-data/hl-stocks-hot?limit=10` | **parse-GO / signal NO-GO** |
| 13 | Stock Gainers (`stock-gainers`) | `vergex` | `GET /api/v1/market-data/hl-stocks-movers?direction=gainers&limit=10` | **parse-GO / signal NO-GO** |
| 14 | Stock Losers (`stock-losers`) | `vergex` | `GET /api/v1/market-data/hl-stocks-movers?direction=losers&limit=10` | **parse-GO / signal NO-GO** |

Notes:
- Rows 12-14 resolve and populate `Symbol`+`Rank` via the lenient parser, but carry **no per-row `bias`/`score`/`confidence`** — they cannot drive a directional bull/bear signal pool correctly (a gainer is not necessarily a "bullish" signal). Treat as symbol/rank sources only, or NO-GO for signal use.
- Rows 1/2/10/11 (leaderboard) need no code change: `parseRankItem` already pulls `symbol`/`bias`/`market`(nested `.market.marketType`) and never skips rows. Score/Confidence parse as 0 because the response uses `directionScore` (not one of the score keys) and has no confidence field — add a 1-line key mapping only if Score>0 is required.

---

## Detailed verification

### 1. `vergex_signal` — `/api/v1/direction-change/leaderboard` — **GO**

- HTTP 200; **no auth required**; pure JSON.
- Root: `{"band":15, "items":[…], "rankBy":"directionScore", …}`.
- Row keys: `symbol`, `bias` (`bullish`/`bearish`/`neutral`), `directionScore`, `bullishCount`, `bearishCount`, `neutralCount`, `markPrice`, `rank`, `oiRank`, `factorDirections`, `market:{marketType,symbol}`.
- Mapping → `SignalRankItem` (via lenient `parseRankItem`):
  - `Symbol` ← `symbol` ✓ (never skipped)
  - `Bias` ← `bias` ✓
  - `MarketType` ← nested `market.marketType` ✓
  - `Rank` ← `rank` ✓
  - `Score` ← *(none)* — `directionScore` not in {`compositeZ`,`composite_z`,`score`,`rank_score`,`z`,`value`}
  - `Confidence` ← *(none)*
- The leaderboard contains **both** `core_perp` (crypto: e.g. PUMP, AAVE, XMR, ETH, LIT, BTC) **and** `hip3_perp` (stocks/indices/fx/commodities: `xyz:` prefixed). Verified: of 30 rows, ~10 `core_perp` (2 bull / 6 bear / 2 neutral) + ~20 `hip3_perp` (11 bull / 8 bear / 1 neutral). **Crypto and stock bias cards are both serviceable** — filter by nested `market.marketType` + the parsed `Bias`.
- **Verdict: GO** for both crypto and stock bias cards via a single endpoint.

### 2. `oi_top` / `oi_low` — `/trending-crypto?tab=oi` — **NEEDS-ADAPTER**

- HTTP 200; **no auth**; pure JSON `{"top":[…],"low":[…]}`; honors `duration` & `limit`.
- Row keys (both arrays, identical): `current_oi, net_long, net_short, oi_delta, oi_delta_percent, oi_delta_value, price, price_delta_percent, rank, symbol`.
- Mapping → `OIPosition`: **name-for-name, all 10 fields match**. `oi_delta_percent`/`price_delta_percent` already x100 (15.457 = +15.5%). `low` rows are true decreases (negative `oi_delta`/`oi_delta_percent`).
- Only gap: outer envelope is bare `{top,low}`; the strict target is `OIRankingResponse{success,code,data{positions,count,exchange,time_range,time_range_param,rank_type,limit}}`.
- **Adapter**: fetch `tab=oi`, read `top` (oi_top) or `low` (oi_low), wrap into `data.positions` + synthesize `success=true, code=0, count=len, time_range=duration`. No key renames or type surgery.

### 3. `netflow_top` / `netflow_low` — `/trending-crypto?tab=net_flow` — **NEEDS-ADAPTER**

- HTTP 200; **no auth**; pure JSON `{"top":[…],"low":[…]}`; honors `duration` & `limit`.
- Row keys: `amount, price, price_delta_percent, rank, symbol`.
- Mapping → `NetFlowPosition`: `rank←rank`, `symbol←symbol`, `amount←amount` (positive inflow in `top`, negative outflow in `low`), `price←price`. Extra `price_delta_percent` is ignored by strict unmarshal.
- Only gap: envelope. **Adapter**: read `top` (netflow_top) or `low` (netflow_low) into `data.netflows`; direction chosen by which array the adapter reads (there is no paid `type`/`trade` discriminator on the free side).

### 4. `price_*` — `/trending-crypto?tab=price` — **NEEDS-ADAPTER**

- HTTP 200; **no auth**; pure JSON `{"top":[…],"low":[…]}`; honors `duration` & `limit`.
- Row keys: `pair, symbol, price_delta, price, future_flow, spot_flow, oi, oi_delta, oi_delta_value`.
- Mapping → `PriceRankingItem`: **exact 1:1, key-for-key**, and `price_delta` is already **decimal** (0.4825 = 48.25%) matching the struct contract.
- Only gap: envelope is bare `{top,low}`; strict target is `PriceRankingResponse{success,data{durations[],limit,data{map[duration]{top,low}}}}`. **Adapter**: wrap `top`/`low` under `data.data[duration]`.
- Alternative `/trending-price?duration=&limit=` is HTML client-rendered (**NO-GO**, not server-scrapable).

### 5. `ai500` — `/trending-category?lang=en&key=ai500` — **NEEDS-ADAPTER** (small hand-picked pool)

- HTTP 200; **no auth**; pure JSON.
- Row path `.category.assets[]`; keys `symbol, pair, quote, name, price, change, volume, signal("Peak 87"), score, startPrice, startTime, changePctValue`.
- Mapping → `CoinData`: `Pair`←`pair`, `Score`←`score`, `StartTime`←`startTime`, `StartPrice`←`startPrice`, `IncreasePercent`←`changePctValue` (already x100). `LastScore`/`MaxScore`/`MaxPrice` have no direct key (MaxScore derivable from the `signal` string).
- **Pool size is per design.** This is NOT a 500-coin pool — it is the AI hand-picked coin set (a handful of candidates with an AI confidence `score`). Verified: returns exactly **3 assets** (CYS, TUT, BTW) with scores 75/71/61. For the `ai500` source_type, a ~3-coin pool is the correct semantic (the paid NofxOS `/api/ai500/list` was the same idea, a top-rated shortlist). Usable as long as a ≥3 candidate pool is acceptable for a strategy.
- **Adapter needed**: the free envelope is `{category:{assets[]}}`, not the strict `AI500Response{success,data{coins[]}}`. Wrap `.category.assets[]` into `data.coins`, preserving `Pair`/`Score` (+ map the keys above).

### 6. SignalLab (per-coin) — `/data-intelligence/markets/<mt>/<sym>/signals?chain=mainnet&liqBand=15` — **GO**

- HTTP 200; **no auth**; JSON wrapped in `{"data":{…},"meta":{…}}`.
- Shape: `market, band, bias, structureRead, confidence, dimensions[7]{key,family,label,what,kind,direction,strength,percentile,detail}, levels{markPrice,poc,pocDistPct,magnet,resistance,support,valueAreaHigh/Low}, metrics{shortLiqAbove,longLiqBelow,longOverhangPnl,shortOverhangPnl,gLong,gShort,cascadeVulnPct,top10Pct,convexity,includedPositions,state}, markPriceSource, markPriceAsOf, markPriceLive, compositeZ, rank, universeSize`.
- Backs the opaque `MarketAnalysis.SignalLab json.RawMessage` — passthrough to the LLM, so exact formatting is safe.
- Verified for both crypto (`core_perp:BTC`) and stock (`hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:MSFT`).
- **Verdict: GO.** Replaces the paid SignalLab call directly.

### 7. CostLiquidationHeatmap (per-coin) — `/data-intelligence/markets/<mt>/<sym>/riskbins` — **GO with token (corrected)**

- HTTP **200** with the valid Bearer token (verified); HTTP 401 `AUTHORIZATION_HEADER_MISSING` without it. Use the URL-encoded symbol path (`core_perp%3ABTC`).
- Shape: `{"data":{market, markPrice, markPriceLive, markPriceSource, markPriceAsOf, binStep, costAddrs, liqAddrs, bins[{px, bucketStartPrice, bucketEndPrice, longCost, shortCost, longLiq, shortLiq}]}}`.
- Backs `MarketAnalysis.Heatmap json.RawMessage` — passthrough to the LLM, so exact formatting is safe.
- **Verdict: GO** — but **requires a valid Bearer token at runtime** (see Open questions — token source/config).

### 8. FlowMarkets (frontend) — `/data-intelligence/flow/markets?window=1h&limit=25` — **GO**

- HTTP 200; **no auth**; JSON `{"data":{"by":"markets","window":"1h","inflow":[…],"outflow":[…]}}`.
- Row keys: `key(market ref), marketType, symbol, netFlow(string), buyNotional, sellNotional, trades, latestPrice, latestPriceAsOf, priceChangePct`.
- Maps 1:1 to the target `symbol / netFlow / buyNotional / sellNotional / trades / latestPrice` (note: `netFlow` is a **string**). Replaces the paid Claw402 flow-markets used by the frontend chart.

---

## Backend code-linkage summary

| Backend anchor | Struct / parser | Free endpoint required to feed it | Needed change |
|---|---|---|---|
| `kernel.GetCandidateCoins` case `vergex_signal` → `getVergexSignalCoins` | `SignalRankItem` via lenient `parseRankItem` | `/direction-change/leaderboard` | none (lenient); score keys optional |
| `… case oi_top` → `getOITopCoins` | `OIPosition` (strict `OIRankingResponse`) | `/trending-crypto?tab=oi` `top` | **envelope adapter** |
| `… case oi_low` → `getOILowCoins` | `OIPosition` | `/trending-crypto?tab=oi` `low` | **envelope adapter** |
| `… case ai500` → `getAI500Coins` | `CoinData` (strict `AI500Response`) | `/trending-category?key=ai500` | **envelope adapter** (hand-picked ~3-coin pool) |
| netflow prompts | `NetFlowPosition` (strict `NetFlowResponse`) | `/trending-crypto?tab=net_flow` | **envelope adapter** |
| price prompts | `PriceRankingItem` | `/trending-crypto?tab=price` | **envelope adapter** |
| `FetchVergexDataBatch` → `GetSignalLab` | `MarketAnalysis.SignalLab json.RawMessage` | `/…/signals` | URL/query swap (free base) |
| `FetchVergexDataBatch` → `GetCostLiquidationHeatmap` | `MarketAnalysis.Heatmap json.RawMessage` | `/…/riskbins` | URL swap + **Bearer token** |
| frontend `getFlowMarkets` | flow row | `/data-intelligence/flow/markets` | URL swap |

---

## Open questions to resolve (before implementation)

1. **Runtime token source** — `riskbins` (heatmap) and some per-coin endpoints need a valid `vergex.trade` Bearer token; the recorded token works but has **no `exp` claim**, so its lifetime is unknown. Options: (a) store a user-supplied token (`.env`/DB) and send it on authed calls, with graceful degradation (skip + log) when it returns 401; (b) leave the heatmap absent. **Recommend: (a) — optional token, degrade gracefully.**
2. **`directionScore`→score mapping** — confirmed: add a one-line key mapping so `SignalRankItem.Score` carries `directionScore` (otherwise parses as 0). **Default: yes.**
3. **ai500 pool size** — the free ai500 returns ~3 hand-picked coins. Confirm that's acceptable as the strategy pool (the paid API was the same shortlist concept). **Default: yes, acceptable.**
4. **Crypto vs stock bias splitting** — the leaderboard mixes `core_perp` + `hip3_perp`; the engine must filter by `market.marketType` for the correct scope card. Confirm existing `VergexMarketType`/category handling covers this or needs a filter addition.

---

*Generated 2026-08-11. Live-verified via subagent, raw snapshots in `/tmp/opencode/altdata/`.*
