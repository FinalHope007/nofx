# Configurable Trading Gates + Trading Style Presets — Design

**Date:** 2026-08-15
**Branch:** `dev`
**Status:** Approved for planning

## Overview

This feature makes a curated subset of the backend trading gates (Sections A/B/C/D of
`Trading Parameters.md`) editable per-strategy from the frontend strategy editor, and adds a
canned **Trading Style** preset (scalp / intraday / swing / default) that bulk-sets a cluster of
those gates. It is **personal-use only**; the user accepts the risk of changing hard/dangerous
parameters. Not all gates are surfaced — a curated subset only.

Two workstreams land together:

1. **Reconcile (Goal 1):** make the Section A config fields the single per-strategy source of
   truth that BOTH the decision validator (`kernel/engine_position.go`) and the runtime
   (`trader/auto_trader_risk.go`) read, eliminating the hardcoded shadows (12 / 60 / 3.0).
2. **Surface (Goal 2):** expose the curated fields in the strategy editor UI, plus the new
   **Throttling Settings (Risky)** section and the **Trading Style** preset.

Also in scope: fix the open-rate counting bug (HANDOFF2 "Issue 1"). The DB-leverage mismatch
(HANDOFF2 "Issue 2") is explicitly **deferred** (see Excluded).

## Decisions (recorded)

- **Approach:** Extend the existing per-strategy `RiskControlConfig` (Approach 1). Throttle lives
  in a new nested `ThrottlingConfig` struct (Approach 2 grouping). Trading style is a top-level
  strategy field.
- **Storage model:** Per-strategy values. "Global" means the Section A field **on this strategy's
  own config**; changing strategy A never affects strategy B. No shared/global config object.
- **Min-size collapse:** The validator's two-tier floor (general 12 / BTC-ETH 60) collapses into a
  single config value (`min_position_size`). Per-tier position sizing guidance remains via
  `btc_eth_max_position_value_ratio` / `altcoin_max_position_value_ratio`.
- **Margin:** Margin math (`marginOverheadFactor=1.01`, `takerFeeRate=0.001`,
  `positionSizeSafetyFactor=0.98`) and `max_margin_usage` are **untouched** — not reconciled, not
  newly enforced. `max_margin_usage` stays prompt-only/cosmetic and is surfaced in the UI only as
  **AI prompt guidance**, never labeled a hard limit.
- **Durations:** stored as **integer minutes**.
- **OI-liquidity filter:** boolean toggle + a numeric threshold box (disabled when toggle is off).
  The 15M threshold becomes configurable (default 15,000,000 USDT).
- **Preset apply:** **one-way, non-sticky** bulk-setter. Selecting a style overwrites the affected
  form fields; the user may then edit any field. "Default" resets those fields to defaults.
  `trading_style` is saved for record / UI highlight only.
- **Effect timing:** config changes take effect on **Save Strategy** via the existing
  trader remove → reload-from-store → restart path (`api/handler_trader.go:707-726`). No new
  hot-reload mechanism.

## Config surface map (what the user edits)

### Basic Rules (Section A additions — current editor only shows leverage + ratios)
| Field | Label | Clamp | Default |
|---|---|---|---|
| `max_positions` | Max concurrent positions | 1–8 | 3 |
| `min_position_size` | Min position size (USDT) | 10–1000 | 12 |

### Advanced Settings (Section A additions + OI)
| Field | Label | Clamp | Default | Note |
|---|---|---|---|---|
| `min_risk_reward_ratio` | Min risk/reward ratio | 1.0–10.0 | 3.0 | now enforced by validator |
| `max_margin_usage` | Max margin usage (%) | 0.1–1.0 | 1.0 | **prompt-only**; label "AI guidance", not hard limit |
| `min_confidence` | Min AI confidence | 50–100 | 78 | prompt-only |
| `enable_oi_liquidity_filter` | OI-liquidity filter | bool | true | gates the <15M candidate filter |
| `oi_liquidity_filter_min_usdt` | OI-liquidity min (USDT) | ≥0 | 15,000,000 | disabled box when toggle off |

### Throttling Settings (Risky) — new fieldset
| Field | Label | Default | Enforced at |
|---|---|---|---|
| `max_opens_per_hour` | Max opens per hour | 3 | open |
| `max_opens_per_cycle` | Max opens per cycle | 2 | open |
| `min_hold_duration_min` | Min hold before normal close (min) | 90 | close |
| `noise_close_hold_duration_min` | Noise-band close window (min) | 180 | close |
| `reentry_cooldown_min` | Re-entry cooldown (min) | 240 | open |
| `early_close_stop_loss_bypass_pct` | Early-close stop-loss bypass (%) | -3.0 | close |
| `early_close_take_profit_bypass_pct` | Early-close take-profit bypass (%) | 8.0 | close |
| `noise_close_loss_floor_pct` | Noise band loss floor (%) | -2.0 | close |
| `noise_close_profit_ceiling_pct` | Noise band profit ceiling (%) | 3.0 | close |

### Trading Style preset — new fieldset (top of editor)
4 chips: **Scalp / Intraday / Swing / Default**. One-way bulk-apply of the cluster below
(placeholder values; tuned later):

| Field | Default | Scalp | Intraday | Swing |
|---|---|---|---|---|
| `max_positions` | 3 | 5 | 4 | 2 |
| `max_opens_per_hour` | 3 | 8 | 5 | 2 |
| `max_opens_per_cycle` | 2 | 4 | 3 | 1 |
| `min_hold_duration_min` | 90 | 10 | 45 | 360 |
| `noise_close_hold_duration_min` | 180 | 30 | 90 | 720 |
| `reentry_cooldown_min` | 240 | 30 | 120 | 480 |
| `min_risk_reward_ratio` | 3.0 | 1.5 | 2.0 | 3.0 |
| `min_position_size` | 12 | 12 | 12 | 12 |

Values are clamped to the Section A hard caps on save (`StrategyConfig.ClampLimits()`).

## Backend changes

### 1. Reconcile the decision validator (`kernel/engine_position.go`)
- Extend `validateDecision` / `validateDecisions` / `parseFullDecisionResponse` signatures to
  accept `minPositionSize float64` and `minRiskRewardRatio float64`.
- Replace `const minPositionSizeGeneral = 12.0` / `minPositionSizeBTCETH = 60.0` and the
  two-branch (BTC-ETH vs general) size check with a single `minPositionSize` check.
- Replace the hardcoded `riskRewardRatio < 3.0` with `< minRiskRewardRatio`.
- Thread the two values from the strategy `RiskControlConfig` at the caller
  (`kernel/engine_analysis.go:431` → `parseFullDecisionResponse`).

### 2. New config model (`store/strategy.go`, `store/strategy_schema.go`)
- Add a nested **`ThrottlingConfig`** struct to `RiskControlConfig` with the fields from the
  Throttling table above (snake_case JSON tags, `omitempty`).
- Add the two OI-liquidity fields to `RiskControlConfig`.
- Add `TradingStyle` enum field (scalp / intraday / swing / default) to `StrategyConfig`.
- Update `ClampLimits()` to bound throttle/OI/style fields.
- Columns auto-added by the existing `AutoMigrate` on `Strategy` (`store/strategy.go:1025-1028`).

### 3. Runtime wiring (`trader/auto_trader_throttle.go`)
- Replace the package consts (`autopilotMaxOpensPerHour`, `autopilotMinHoldDuration`, etc.) with
  reads from `at.config.StrategyConfig.RiskControl.ThrottlingConfig` (live per cycle, matching how
  `auto_trader_risk.go` already reads `RiskControl`).
- Keep a package-level default struct so behavior is unchanged for strategies that never set the
  fields (back-compat / zero-value safety).

### 4. OI-liquidity filter wiring
- Gate the existing `< 15M` candidate filter on `enable_oi_liquidity_filter` and use
  `oi_liquidity_filter_min_usdt` as the threshold instead of the hardcoded 15M. Default true/15M
  preserves current behavior.

### 5. Bug fix — Issue 1 (open-rate counting)
- `countRecentOpenOrders` (`trader/auto_trader_throttle.go:233-249`): count **distinct
  position-open events** (group `open_` order rows by symbol/side into one logical open) instead of
  raw order rows, so a single position opened as multiple fills counts once.
- `findRecentCloseOrder` (re-entry cooldown): treat multi-fill closes for the same symbol/side as
  one close event.

## Frontend changes

- `web/src/types/strategy.ts`: extend `RiskControlConfig` with `ThrottlingConfig` + OI fields; add
  `trading_style` to `StrategyConfig`.
- `web/src/features/strategies/strategyFactory.ts`: extend `StrategyEditorForm` and
  `defaultRiskControl`/`buildStrategyConfig` to read/write the new fields; add the preset value map
  and a helper to apply a style to the form state.
- `web/src/features/strategies/EditorStepPage.tsx`:
  - **Basic Rules:** add `max_positions`, `min_position_size` inputs.
  - **Advanced Settings:** add `min_risk_reward_ratio`, `max_margin_usage` (labeled "AI guidance"),
    `min_confidence`, OI toggle + threshold box (disabled when off).
  - **Throttling Settings (Risky):** new fieldset with the 9 throttle inputs, styled with the
    existing `nofx-danger` warning pattern (like "Recent decisions context").
  - **Trading Style:** new fieldset at top with 4 chips; selecting applies the preset values and
    re-highlights the current style.
- English-only strings. No new backend endpoint required — reuse the existing strategy save path.

## Effect timing

Saved config is applied by the existing trader update flow:
`api/handler_trader.go:707-726` removes the trader, reloads it from store (re-reading strategy
config), and restarts it if it was running. No separate hot-reload.

## Excluded / stays hard-locked

- Margin math (`1.01` / `0.001` / `0.98`) and `max_margin_usage` enforcement — untouched.
- SL/TP ordering rule, duplicate-direction guard, safe-mode, `MaxCandidateCoins`,
  `MaxPositions = 8` clamp ceiling, timeframes/kline counts, grid-specific gates, Sections E/F/G.
- **HANDOFF2 Issue 2 (DB leverage mismatch, `store/position_builder.go:71`)** — deferred. It is not
  a one-line fix: leverage is absent from per-trade sync events and would need threading through 8
  exchange order-sync adapters + tests. Recorded as a follow-up known-issue.

## Verification

- `go vet ./...`, `gofmt -l` clean.
- Backend: `go test ./...` (CI runs with `-race`). Add/update tests for: validator config
  threading (min-size collapse + configurable RR), `ThrottlingConfig` defaults + clamping,
  throttle live-config reads, Issue-1 distinct-open counting, OI filter toggle/threshold.
- Frontend: `tsc --noEmit`, `npm run build`, `npm test`.
- English-only UI. Backend uses `SafeError`/`SafeInternalError`/`SanitizeError`; never leak
  internals. `store.*` is the only DB access layer.
