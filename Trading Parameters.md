# Trading Parameters — Complete Backend Trading-Gate Inventory

> All code-enforced gates that can block or alter order placement / position management at runtime, plus prompt-only "gates". Verified against source on 2026-08-12 (branch `dev`).
>
> **Legend**
> - **Enforced at:** `open` = order path · `close` = exit path · `cycle` = per-cycle loop · `ai` = AI-decision validation (before orders) · `prompt` = injected into AI prompt only, **not** code-enforced
> - **Scope:** `all` = every strategy · `autopilot` = AI autopilot (non-grid) · `vergex` = vergex_signal source · `grid` = grid strategy
> - **Config now?:** yes/<field> vs no (hardcoded)

## A. Global hard caps (config clamped to these bounds) — `store/strategy.go:15-33`

| Gate | Enforced value | File:line | Scope | Config now? |
|---|---|---|---|---|
| Candidate universe cap | `MaxCandidateCoins = 10` | strategy.go:15, clamp 40-56 | all | limit fields, hard-capped 10 |
| Max concurrent positions | `MaxPositions = 8` | strategy.go:16, clamp 96-102 | all | yes `max_positions` |
| Max timeframes / kline count | `MaxTimeframes=4`, `MinKlineCount=10`, `MaxKlineCount=30` | strategy.go:18-19, 59-73 | all | `selected_timeframes`/`primary_count` |
| Leverage bounds | `1 ≤ lev ≤ 20` (BTC/ETH & alt) | strategy.go:20-22, 105-116 | all | yes `btc_eth_max_leverage`, `altcoin_max_leverage` |
| Position-value ratio bounds | `0.5 ≤ ratio ≤ 10.0` | strategy.go:23-24, 119-130 | all | yes `*_max_position_value_ratio` |
| Risk-reward bounds | `1.0 ≤ rr ≤ 10.0` | strategy.go:25-26, 133-138 | all | yes `min_risk_reward_ratio` |
| Margin-usage bounds | `0.1 ≤ margin ≤ 1.0` | strategy.go:27-28, 139-144 | all | yes `max_margin_usage` |
| Position-size bounds | `10 ≤ size ≤ 1000` | strategy.go:29-30 | all | yes `min_position_size` |
| Confidence bounds | `50 ≤ conf ≤ 100` | strategy.go:31-32 | all | yes `min_confidence` |

## B. Anti-churn throttle (all hardcoded) — `auto_trader_throttle.go:12-30`, enforced `auto_trader_loop.go:318`

| Gate | Value | Enforced at | Scope | Config now? |
|---|---|---|---|---|
| Opens per hour cap | `autopilotMaxOpensPerHour = 3` | open | autopilot | no |
| Opens per cycle cap | `autopilotMaxOpensPerCycle = 2` | open | autopilot | no |
| Min hold before normal close | `autopilotMinHoldDuration = 90m` | close | autopilot | no |
| Noise-band close window | `autopilotNoiseCloseHoldDuration = 3h` (band −2%..+3%) | close | autopilot | no |
| Re-entry cooldown | `autopilotReentryCooldown = 4h` | open | autopilot | no |
| SL/TP bypass floors | `earlyCloseStopLossBypass=−3%`, `earlyCloseTakeProfitBypass=+8%` | close | autopilot | no |
| Noise band | `noiseCloseLossFloor=−2%`, `noiseCloseProfitCeiling=+3%` | close | autopilot | no |

> Tuned by replay and hardcoded; commit `574ddfb1` reverted per-strategy configurability.

## C. Decision validator (hardcoded, before orders) — `kernel/engine_position.go` (via `engine_analysis.go:431`)

| Gate | Value | Enforced at | Scope | Config now? |
|---|---|---|---|---|
| Min order size (general) | `minPositionSizeGeneral = 12 USDT` | ai | all | no (config `min_position_size` ignored here) |
| Min order size (BTC/ETH) | `minPositionSizeBTCETH = 60 USDT` | ai | BTC/ETH | no |
| Risk/reward floor | `≥ 3.0:1` (ignores `min_risk_reward_ratio`) | ai | all | no |
| Leverage cap | config tiers; auto-reduce if exceeded | ai | all | yes |
| Position-value cap | `equity × ratio` (+1% tol) | ai | all | yes |
| SL/TP ordering | long: SL<TP · short: SL>TP | ai | all | no (rule) |
| Invalid action reject | whitelist `open/close/hold/wait` | ai | all | no (rule) |

## D. Risk-control execution checks — `auto_trader_risk.go`, `auto_trader_orders.go`

| Gate | Value | Enforced at | Scope | Config now? |
|---|---|---|---|---|
| Position-value cap (BTC/ETH+XYZ) | `equity × BTCETHMaxPositionValueRatio` (default 5.0) | open | BTC/ETH+XYZ | yes |
| Position-value cap (altcoin) | `equity × AltcoinMaxPositionValueRatio` (default 1.0) | open | alt | yes |
| Min position size (runtime) | `RiskControl.MinPositionSize` (default 12) | open | all | yes `min_position_size` |
| Max positions | `RiskControl.MaxPositions` (default 3) | open | all | yes `max_positions` |
| Duplicate-direction guard | reject open if same symbol+side held/scheduled | open | all | no (rule) |
| Margin overhead factor | `marginOverheadFactor = 1.01` | open | all | no |
| Taker fee rate | `takerFeeRate = 0.001` | open | all | no |
| Size safety buffer | `positionSizeSafetyFactor = 0.98` | open | all | no |
| Insufficient-margin auto-shrink | `maxAffordable = avail / (1.01/lev + 0.001)`; cap `×0.98` | open | all | no |
| Drawdown profit-protection close | arm `+5%`, close `≥40%` giveback, check 1 min | close | all | no |
| `applyAutopilotFullSizeOpen` | force vergex opens to `equity×ratio` @ config lev | open | vergex | via ratio/lev |
| `ensureLongShortCoverage` (balanced top-up) | floor `|score| ≥ 0.4`, half `MaxPositions` long/short | cycle | vergex | score floor no; target via `max_positions` |
| OI-liquidity candidate filter | drop if `OI value < 15M USDT` (skips positions & XYZ) | ai/context | all | no |

## E. Account-level / safe-mode gates

| Gate | Value | Enforced at | Scope | Config now? |
|---|---|---|---|---|
| Safe-mode trigger | `consecutiveAIFailures ≥ 3` | cycle | all | no |
| Safe-mode action | blocks ALL opens; closes/holds still run | cycle | all | no |
| Safe-mode deactivate | AI success resets counter | cycle | all | no |
| (non-grid) stop-trading `stopUntil` | dead code — never assigned | cycle | all | fields exist but unwired |
| (non-grid) daily-loss / max-drawdown / stop-time | hints only, not enforced | — | all | no effect |
| Claw402 AI-wallet fee floor | `aiWalletLowThresholdUSDC = 1.0` (feeds safe-mode status) | cycle | claw402 | no |

## F. Grid-specific gates — `auto_trader_grid*.go`, `grid_regime.go`, `store/grid.go`

| Gate | Value | Enforced at | Scope | Config now? |
|---|---|---|---|---|
| Max drawdown emergency exit | `grid.max_drawdown_pct` (default 15%) → cancel+close+pause | cycle | grid | yes |
| Daily loss limit pause | `grid.daily_loss_limit_pct` (default 10%) → pause | cycle | grid | yes |
| Grid leverage | `grid.leverage` (1-20) | open | grid | yes |
| Per-level quantity cap | `(TotalInvestment/GridCount)×Leverage` | open | grid | yes |
| Absolute position-value safety cap | reject if `qty×price > TotalInvest×Lev×2` (hardcoded 2x) | open | grid | no (the 2x) |
| Total grid position limit | `≤ TotalInvestment×Leverage` | open | grid | yes |
| Per-level stop loss | `grid.stop_loss_pct` | close | grid | yes |
| Breakout → pause+cancel | price move `≥ 2%` (hardcoded) | cycle | grid | no |
| Breakout confirmation | `3` candles (hardcoded) | cycle | grid | no |
| Direction bias / direction-adjust | `grid.direction_bias_ratio` (0.7), `grid.enable_direction_adjust` | open/cycle | grid | yes |
| Regime leverage & position limits | Narrow/Standard/Wide/Volatile presets | display-only | grid | no effect |
| `PositionReductionPct=50` (box-short reduce) | set-but-not-applied | cycle | grid | inert |

## G. AI-pipeline / context guards

| Gate | Value | Enforced at | Scope | Config now? |
|---|---|---|---|---|
| Context-limit block | default `131072` tokens (or provider limit), warn @80% | ai | all | provider-based; default no |
| Vergex detail concurrency | `2` concurrent fetches | cycle | vergex | no |

## H. Prompt-only (configurable in UI but NOT code-enforced)

| Field | Reality |
|---|---|
| `max_margin_usage` | "≤100%" in prompt; **no code check** (orders use margin-overhead math) |
| `min_confidence` | "≥78" in prompt; **no code check** on decision.Confidence |
| `min_risk_reward_ratio` | in prompt; actual block is hardcoded `≥3.0` in `engine_position.go:126` |
| `trading_frequency` prompt section | prompt text only |

## I. Conflicting / Duplicated Parameters (must reconcile)

These are cases where more than one parameter governs the same concept, and one
enforcement ignores or shadows the configurable field. Before exposing any of
these to the frontend, they must be unified into a single config-backed source
of truth used by BOTH the decision validator (`kernel/engine_position.go`) and
the runtime (`trader/auto_trader_risk.go` / `trader/auto_trader_throttle.go`).

| Concept | Config field (clamp, default) | Other/hardcoded enforcement | Where the config field is ignored/shadowed | Effective behavior today |
|---|---|---|---|---|
| Min position size (general) | `min_position_size` (10–1000; runtime default 12) | `minPositionSizeGeneral = 12` — validator, `engine_position.go:66` | validator ignores `min_position_size` | config < 12 has no effect; validator rejects |
| Min position size (BTC/ETH) | `min_position_size` (same field) | `minPositionSizeBTCETH = 60` — validator, `engine_position.go:67` | validator hardcodes 60 for BTC/ETH, ignores config | BTC/ETH floor is always 60 regardless of config |
| Risk/reward floor | `min_risk_reward_ratio` (1.0–10.0; prompt-only) | `≥ 3.0` hardcoded — validator, `engine_position.go:126` | validator ignores config | cannot be lowered below 3.0 via config |
| Max margin usage | `max_margin_usage` (0.1–1.0; prompt-only) | margin math `marginOverheadFactor = 1.01`, `takerFeeRate = 0.001` — `auto_trader_orders.go:17-18` | config not read as a gate | config is cosmetic; real cap from margin math |
| Min confidence | `min_confidence` (50–100; prompt-only) | no code check on `decision.Confidence` | nothing reads it | config is cosmetic (prompt only) |
| Max concurrent positions (held) | `max_positions` (1–8; default 3) | `MaxPositions = 8` const — `store/strategy.go:16` (clamp ceiling) | distinct from open-rate throttle | max held = min(config, 8); default 3 |
| Max new opens (rate) | — (no config) | `autopilotMaxOpensPerCycle = 2`, `autopilotMaxOpensPerHour = 3` — throttle | throttle hardcoded; separate from held cap | new opens rate-limited regardless of max_positions |
| Leverage (tiered) | `btc_eth_max_leverage` / `altcoin_max_leverage` (1–20) | `MaxBTCETHLeverage = 20` / `MaxAltLeverage = 20` — clamp | consistent between validator + runtime; only clamp applies | cannot exceed 20 |

**Reconcile rule:** each concept should have ONE config field that both the
validator and the runtime read. Remove the hardcoded shadow values (12 / 60 /
3.0 / margin math) or make them fall back to config when the config field is
set.
