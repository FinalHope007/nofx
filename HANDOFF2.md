# HANDOFF2 — Next-Session Context (NOFX trading gates / full frontend control)

## Current state (branch `dev`)

- Working tree was clean. Prior feature (per-coin data-source prompt enrichment) is landed and verified (docs in `docs/superpowers/specs/2026-08-12-per-coin-data-source-prompt-design.md` and `.../plans/2026-08-12-per-coin-data-source-prompt.md`).
- A user-facing audit was completed and written to the repo root as **`Trading Parameters.md`** (complete backend trading-gate inventory). No trading code was changed for the audit itself.
- This file (`HANDOFF2.md`) is the handoff for the next session.
- The user is in a **planning/decision phase**: they want to make a curated subset of trading parameters (incl. hard/dangerous ones) editable in the frontend, **for personal use only** (risk accepted). They have NOT yet chosen which parameters to surface.

## Recent diagnostic findings (bug class, NOT fixed — by user decision)

1. **Trader-id attribution bug (shared-exchange case):**
   - Symptom: a closed position didn't show in /dashboard recent trades.
   - Root cause: open booked under one `trader_id`, close under another → `PositionBuilder.handleClose` (`store/position_builder.go:104-114`) found no OPEN position, logged "No matching open position ... skipping", no CLOSED record created.
   - Cause: two traders shared ONE exchange account (`exchange_id`) → order-sync attribution collided (open under `..._1786476551`, close under `..._1786541319`).
   - **Decision: leave unfixed.** Real-run topology = 1 trader : 1 distinct Binance subaccount (no shared API keys), which avoids it by construction. The remaining hazard in the current TEST setup (two exchange rows sharing one API key/account) is position-routing conflict, not this attribution bug.
   - Safe-state fact: `binanceSyncState` is keyed by `exchangeID` (per account UUID). Distinct `exchange_id` = isolated. Only same-`exchange_id` sharing is unsafe.

2. **Trade throttle applies to ALL autopilot strategies (not vergex-only):**
   - Confirmed: `tradeThrottleReason` (`auto_trader_loop.go:318`) runs for every trader/source. `autopilotReentryCooldown=4h` blocked a re-open on `Test New AI500` (AI500 source) → user saw "closed 42m ago, wait 3h18m".
   - Throttle is hardcoded and was made non-configurable on purpose (git `574ddfb1 revert: drop exit-gate configurability, hardcode the replay-validated values`). It blocks short-term/scalping strategies.

3. **Backend log non-200s (benign, expected):** VergeX free detail endpoints return HTTP 503 `data_unavailable` / HTTP 404 `market not found` for unsupported markets (e.g. `xyz:CYS`). Swallowed, never surfaced into the prompt. Not trading bugs.

## The audit deliverable

**`Trading Parameters.md`** (repo root) is the full inventory: A (global hard caps), B (throttle), C (decision validator), D (risk-control execution), E (account-level/safe-mode), F (grid), G (AI/context), H (prompt-only). Highlights:

- **Hardcoded & not configurable today** (main candidates to expose for scalping): all throttle gates (B), risk/reward ≥3.0 + min-size 12/60 in validator (C), margin factors 1.01/0.001/0.98 + shrink (D), drawdown close 5%/40% (D), safe-mode threshold 3 (E), OI-liquidity 15M filter (D), grid 2x cap + 2% breakout + 3-candle confirm (F), context limit 131072 (G).
- **Prompt-only / inert** (appear configured, not enforced): `max_margin_usage`, `min_confidence`, `min_risk_reward_ratio` (all prompt-only), non-grid `max_daily_loss`/`max_drawdown`/`stop_trading_time` (unwired dead code), grid regime presets + `PositionReductionPct` (inert).
- **Configurable today:** `max_positions`, `btc_eth_max_leverage`, `altcoin_max_leverage`, `*_max_position_value_ratio`, `min_position_size`, `min_risk_reward_ratio`, `max_margin_usage`, `min_confidence`, grid `grid_count`/`total_investment`/`leverage`/`stop_loss_pct`/`daily_loss_limit_pct`/`max_drawdown_pct`/`direction_bias_ratio`/`enable_direction_adjust`.

## Recommended next steps for the next session

1. The user will decide WHICH parameters to surface (not all) from the `Trading Parameters.md` matrix. Ask them to pick before implementing.
2. Likely first target: re-introduce the throttle + validator hardcoded values (3.0 / 12 / 60) as **strategy-config fields**, since these gate scalping. A prior implementation existed and was reverted — see `git log` on `trader/auto_trader_throttle.go` (commits `434301cb` then `574ddfb1`) for the pattern to reintroduce.
3. Propose a **canned trading-style preset** (scalp / intraday / swing / autopilot-default) with per-field override, plus a curated "advanced" section in the strategy editor for the hard/dangerous params.
4. Decide what stays hard-locked (recommend: margin 1.01 / fee 0.001 / safety 0.98, SL<TP rule, duplicate-direction guard, safe-mode "block opens on repeated AI failure", `MaxCandidateCoins`).

## Constraints / verification (from AGENTS.md + repo)

- `go vet ./...` + `gofmt -l` clean; frontend `tsc --noEmit` + `npm run build` + `npm test`.
- English-only UI strings.
- Backend uses `SafeError`/`SafeInternalError`/`SanitizeError`; never leak internals.
- `store.*` is the only DB access layer; timestamps UTC.
- HIGH-RISK: live order/position changes need explicit confirmation. Stopping/deleting a trader does not close open positions.
- `ALLOW_LOCAL_CUSTOM_API=1` is used in tests to bypass SSRF for httptest loopback.
- Free `vergex.trade` endpoints: `/trending-crypto?tab=oi|net_flow|price`, `/trending-category?lang=en&key=ai500` (see `paidsource-research.md`).
- AI calls go through local Bifrost gateway (custom provider); `.env` holds `JWT_SECRET` etc.

## Decisions (recorded 2026-08-12)

- Configurability focus is **Section A, B, C, D only** (global hard caps, throttle,
  decision validator, risk-control execution). Sections E/F/G/H are out of scope
  for surfacing to the frontend for now.
- Two goals agreed:
  1. **Reconcile conflicting pairs** into single config-backed fields used
     consistently by both the decision validator and the runtime. The full set of
     conflicting pairs is tabulated in `Trading Parameters.md` §I.
  2. **Surface the chosen Section A/B/C/D parameters in the frontend.**
- The **exact list of fields/parameters to surface** in each section is NOT yet
  decided — the next session LLM and the user will finalize this together before
  writing the implementation plan.
- Design intent: for personal use; user accepts risk of changing hard/dangerous
  parameters. Not all parameters will be surfaced (curated subset).
- A likely implementation anchor: reintroduce throttle + validator hardcoded
  values as config fields (see reverted commits `434301cb` then `574ddfb1` on
  `trader/auto_trader_throttle.go`).
