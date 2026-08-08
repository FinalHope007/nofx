# NOFX Handoff — Bifrost + Free Strategy Work

## Goal
Run NOFX AI traders 100% free: AI decisions via Bifrost, coin data from free Hyperliquid/Binance sources. Avoid Claw402/VergeX/NoFXOS (all paid).

## Architecture found (code-verified)
### Data sources (free vs paid)
- **hyper_all / hyper_main / hyper_rank** — FREE. Pulled directly from Hyperliquid public API (`api.hyperliquid.xyz/info`, `metaAndAssetCtxs` / `meta`), no Claw402/NoFXOS.
  - hyper_all = full USDC perp universe (crypto + XYZ stock/commodity)
  - hyper_main = top N by 24h volume
  - hyper_rank = sorted by 24h % change (gainers/losers) or volume; category crypto or XYZ
- **vergex_signal** — PAID (x402). Needs Claw402 wallet key for coin selection + Signal Lab + liquidation heatmap.
- **ai500 / oi_top / oi_low / NoFXOS quant / OI / netflow / price rankings** — PAID. `nofxos.ai` returns HTTP 402 for the hardcoded `DefaultAuthKey` (`cm_568c67eae410d912c54c`) — deprecated/paywalled.
- **Raw OHLCV klines + indicators (EMA/MACD/RSI/ATR/BOLL/volume/OI/funding)** — FREE. Computed locally from Binance futures klines (`fapi.binance.com`, `market.GetWithTimeframes`).

### Pipeline (each cycle)
1. candidate pool (coin source) → 2. per-coin OHLCV+indicators (free, Binance) → 3. optional paid NoFXOS/VergeX (disabled in free strategies) → 4. build prompt → 5. LLM via Bifrost (custom provider) → 6. execute on CEX.

### AI provider: Bifrost
- Bifrost is the local OpenAI-compatible gateway (port 9120, model `z-ai/glm-5.2`). NOFX routes AI through it via `provider="custom"`, `custom_api_url`, no API key needed.
- SSRF blocker: NOFX's `security/url_validator.go` blocks loopback by default; fixed via `ALLOW_LOCAL_CUSTOM_API=1` env exemption (gated; keeps SSRF on otherwise).

### Key files
- `api/handler_ai_model.go` — `handleGetSupportedModels` (determines which model cards show in frontend).
- `security/url_validator.go` — SSRF; env-gated loopback exemption.
- `api/handler_vergex.go` — `newVergexClientForRequest` → "claw402 wallet is not configured" when no claw402 model exists.
- `manager/trader_manager.go:756` / `resolveTraderDataWalletKey:796` — data wallet = claw402 model's API key; falls back to any claw402 model.
- `kernel/engine.go` — `NewStrategyEngine` (claw402 routing), `getHyperRankCoins` etc; engine requires `use_hyper_all:true` / `use_hyper_main:true` for those sources.
- `kernel/engine_analysis.go` — `fetchMarketDataWithStrategy` (per-coin OHLCV, $15M OI liquidity filter drops low-OI candidates); `GetFullDecisionWithStrategy`.
- `trader/auto_trader_loop.go` — `runCycle` → `GetFullDecisionWithStrategy → CallWithMessages(Bifrost)`.
- `store/strategy.go` — `StrategyConfig`/`RiskControlConfig`/`IndicatorConfig`/`KlineConfig`; runtime params split: strategy holds coin source+indicators+risk+prompts; trader (POST /api/traders) holds `scan_interval_minutes`, `is_cross_margin`, `initial_balance`, leverage (backward-compat).
- `store/strategy.go:182` — `ClampLimits` for `hyper_all` uses `HyperMainLimit`.
- Frontend: `web/src/pages/StrategyStudioPage.tsx` = strategy page; `defaultCoinSource()` force-set `vergex_signal` (fixed to preserve source_type). `TraderConfigModal.tsx` reads all strategies for dropdown.

## Decisions made
- **Use custom provider (Bifrost) for AI** instead of Claw402. Implemented via backend patch + `ALLOW_LOCAL_CUSTOM_API=1`.
- **Use free Hyperliquid sources** for coin selection. Only **hyper_rank (top gainers + top losers, crypto only)** created.
- **Frontend hides free options** — strategy page locked to vergex_signal; no coin-source selector, no strategy create UI. Free to surface via frontend rewrite (on laptop).
- **Run NOFX backend as direct proot process** (not Doki container) — glibc build; Doki removed for NOFX (kept installed, small).
- No code/build on phone going forward — build on laptop, copy binary/static assets over.

## Current state
- Two free strategies exist (API): "Hyper Rank Top Gainers (Crypto)" and "Hyper Rank Top Losers (Crypto)", both `source_type=hyper_rank`, `category=crypto`, limit 10, ALL free indicators enabled (raw klines/EMA/MACD/RSI/ATR/BOLL/volume/OI/funding), paid NoFXOS all off.
- One custom model (Bifrost) row; no traders currently running.
- Frontend served statically; original `StrategyStudioPage` still shows default Claw402 strategy; new strategies visible in trader-create dropdown.

## Next steps (for laptop coding agent)
1. Rewrite/replace `StrategyStudioPage.tsx` into a strategy manager: list, create, edit, duplicate; coin-source selector (hyper_all/main/rank/static/vergex); prompt + name + basic rules (AI interval, max leverage x2, max account leverage) + advanced (position mode, candles/indicators, excluded coins).
2. Surface custom/model + trader-level fields if hidden.
3. Rebuild frontend on laptop; scp static assets to phone `~/nofx/frontend/`.
4. Consider a "Trading Pace" toggle and "decisions context" knob (currently not exposed by NOFX).
