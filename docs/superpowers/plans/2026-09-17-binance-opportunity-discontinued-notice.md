# Binance Opportunity (technical + sentiment) — Discontinued Notice

## Context

Binance discontinued the "Binance Opportunity" data endpoints that back:

- candidate pool scopes `binance_technical` / `binance_sentiment`
- per-coin detail toggles `enable_binance_technical_data` / `enable_binance_sentiment_data`

Endpoints (now dead):

- `GET https://www.binance.com/bapi/apex/v1/friendly/apex/web/opportunity/assets`
- `GET https://www.binance.com/bapi/apex/v1/friendly/apex/web/opportunity/asset-details`

Decision: **do not remove any code.** Keep it as a reference implementation for a future
data source. Add visible notices so users and developers know the feature no longer works,
and leave the scopes selectable so existing saved strategies still load.

## Scope

Frontend (3 placements):

1. `web/src/features/strategies/scopeCatalog.ts` — add `discontinued?: boolean` to
   `ScopeCardDef`, set it on the two Binance cards, append a discontinued note to their
   descriptions, and render a `DISCONTINUED` badge in `ScopeStepPage.tsx`.
2. `web/src/features/strategies/ScopeStepPage.tsx` — inline warning banner when a
   discontinued scope card is selected (above the Direction / Interval rows).
3. `web/src/features/strategies/EditorStepPage.tsx` — inline warning banner in the
   "Binance data" per-coin toggle block. Toggles stay usable.

Backend (passive only — comments + log notes, no LLM prompt change, no behavior change):

4. `provider/binance/opportunity.go` — package/type + method doc comments marking the
   whole client and both endpoints as discontinued, with the date.
5. `kernel/engine.go` — comments on `PrefetchBinanceDetails` and
   `getBinanceOpportunityCoins`; `logger.Warnf` deprecation notice when a Binance scope
   or prefetch is actually exercised.
6. `kernel/engine_analysis.go` — comments on the per-coin Binance detail fetch paths.

Docs:

7. Add a "Discontinued" note to both existing Binance specs:
   - `docs/superpowers/specs/2026-08-20-binance-scopes-and-indicator-revamp-design.md`
   - `docs/superpowers/specs/2026-08-21-oi-aware-candidate-pool-and-precycle-prefetch-design.md`
   - also note in `HANDOFF/HANDOFF_2026-08-18.md`.

## Non-goals

- No removal/refactor of Binance code.
- No prompt/schema changes.
- No change to saved strategy config schema or defaults.
- No runtime failure when a Binance scope is selected (degrades to empty, by design).

## Verification

- `cd web && npx vitest run src/features/strategies`
- `go build ./...`
- `go vet ./provider/binance/... ./kernel/...`
