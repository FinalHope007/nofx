# Strategy Manager Frontend — Design Spec

Date: 2026-08-08
Status: Approved for implementation (frontend-first)

## Goal

Replace the existing `StrategyStudioPage` (mounted at `/strategy`) with a complete
**strategy manager**: a table of the user's strategies plus a two-step create/edit wizard
(Select Trading Scope → Enter Trading Strategy). The frontend must be **complete** —
every feature and requirement in the UI — wiring whatever backend already exists today and
using structured **placeholders** for backend features that are not yet implemented (to be
built in a later backend pass).

## Scope & non-goals

- Frontend-first. Wire all currently-existing backend endpoints into the UI.
- Backend features that do not exist yet are exposed in the UI with a uniform ⚠ "Backend
  pending" placeholder; each placeholder is defined by an API-contract + config shape so the
  later backend pass can implement to it without frontend changes.
- Language: **English only** (personal-use project), local strings.
- The old `StrategyStudioPage.tsx` is **archived, not deleted** (moved to
  `web/src/pages/legacy/`).

## Key decisions (recorded)

- Replace `/strategy` route entirely with the new manager.
- **Approach A**: small `pages/` entry + a `features/strategies/` folder of focused
  components (one clear purpose each), matching existing app conventions.
- Table row-click navigates to edit; per-row actions: Version History / Use in Trader Agent /
  Edit.
- **Version History**: server-side snapshots (new backend); a **side modal** containing a
  version dropdown (view prompt + all params read-only) and a **Restore** button. Restore
  works both directions (snapshot-before-restore so you can go back and forth).
- **Trading scope**: select **multiple scope cards**; combine with **Overlap** (=AND,
  candidate must be in every selected scope) or **Union** (=OR, candidate in any selected
  scope).
- `source_type = 'custom'` when **>1 scopes** are selected (backend does AND/OR across the
  selected sources — new backend). A single scope uses that scope's concrete `source_type`
  (e.g. `hyper_rank`, `vergex`, `ai500`, ...).
- Each scope card has a **Top-N number input**; the scope returns that many candidates.
- `direction` on a scope is **optional / discriminated** — only present for sources that use
  it (e.g. `hyper_rank` gainers/losers/volume); AI500/OI/Netflow/bias-radar carry their own
  qualifiers.
- **Multi-trader stats** aggregate a strategy as "one book": merge linked traders' equity
  snapshots into a combined curve; Sharpe/MaxDD computed from the merged book.
- **AI decision interval + margin mode**: stored in the strategy wizard, but per **Option A**
  they only **seed** the trader's creation defaults; runtime reads them from the trader, and
  the user can still override them during trader creation.
- **Leverage fields split** to match the backend (BTC/ETH vs altcoin, position leverage vs
  account leverage-ratio).

## Trading scope options (free vs paid)

### Crypto — FREE (Hyperliquid-native; works with existing backend today)
| Card | source_type | category | direction | limit |
|---|---|---|---|---|
| Crypto Top Gainers | `hyper_rank` | crypto | gainers | Top-N |
| Crypto Top Losers | `hyper_rank` | crypto | losers | Top-N |
| Crypto Trending · Top Volume | `hyper_rank` | crypto | volume | Top-N |

### Crypto — PAID / provider pending (⚠)
Bias Radar Bullish, Bias Radar Bearish (Vergex/Claw402), AI500 Data Provider, OI Increase,
OI Decrease, Netflow Top, Netflow Outflow Top, Crypto Top Gainers (NOFXOS), Crypto Top
Losers (NOFXOS).

### Stock — PAID, VergeX/Claw402 (⚠)
Bias Radar Bullish, Bias Radar Bearish, Trending Stocks, Stock Gainers, Stock Losers.

Paid cards render complete and selectable (frontend complete), carry a ⚠ "Data source not
configured / backend pending" note, and produce a saved placeholder scope entry. A single
paid scope uses its own concrete `source_type`.

## Routes

```
/strategy                     StrategyManagerPage      (table/list)
/strategy/create/scope        ScopeStepPage            (step 1, create)
/strategy/create/editor       EditorStepPage           (step 2, create)
/strategy/:id/edit/scope       ScopeStepPage            (step 1, edit — prefilled)
/strategy/:id/edit/editor      EditorStepPage           (step 2, edit — prefilled)
```

- Create: `/strategy/create/scope` → Next → `/strategy/create/editor` → Save → back to table.
- Edit: loads strategy, prefills scope on step 1, Back/Next, Save on step 2.
- Scope/editor steps share components; a `mode: 'create' | 'edit'` prop + optional
  `strategyId` distinguish them.
- In-progress draft carried across the two pages via a shared draft provider/store.

## Page structure & files (features/strategies)

The manager lives under `web/src/features/strategies/`:

- `StrategyManagerPage.tsx` — table (§2); mounted directly at `/strategy` via `AppRoutes`.
- `ScopeStepPage.tsx` — scope wizard (§3).
- `EditorStepPage.tsx` — editor wizard (§5).
- `VersionHistoryModal.tsx` — side modal (§4).
- `strategyFactory.ts` — build config JSON from form/draft state (pure, unit-tested).
- `scopeCatalog.tsx` — the free/paid scope card definitions.
- `draftStore.ts` — shared in-progress wizard draft (React context/store).
- `strategyApi.ts` — typed API + placeholder methods for missing endpoints.
- `web/src/pages/legacy/StrategyStudioPage.legacy.tsx` — archived old page/helpers (not routed).

## §2 — Strategy Manager table

Columns: `#`, Name / Version, Total AUM, Trading Symbols, 7D Yield, Sharpe, Max DD,
Last Update, NAV Curve, Actions.

- Load `getStrategies()` (existing) and `getTraders()` (existing); group traders by
  `strategy_id`.
- Live per-strategy values aggregated client-side from existing trader endpoints:
  - **Total AUM** = Σ `getAccount(traderId).total_equity` over linked traders.
  - **Trading Symbols** = union of `getPositions(traderId)` symbols.
  - **NAV Curve** = merge `getEquityHistoryBatch(traderIds)` into one combined curve →
    mini SVG/lightweight-charts sparkline.
- **Placeholder** cells (7D Yield, Sharpe, Max DD): render `—` with tooltip "Aggregation
  backend pending". These need windowed / merged calculation that is new backend work.
- Placeholder state when a strategy has no linked traders → "No traders using this strategy
  yet"; on live-fetch failure → "Data unavailable".
- Actions/row-click behavior (§decisions above).
- Create button top-right → `/strategy/create/scope`.

## §3 — Scope step wizard

- Two-step indicator, strategy name, **Overlap / Union** mode toggle.
- Scope cards grouped by category (Crypto / Stock), each with name, description, free/paid
  marker, and a **Top-N number input** (default 10, clamp 1–50).
- Multi-select supported.
- Draft stored as `{ scopeUnits: ScopeUnit[], mode: 'overlap' | 'union' }`.

`ScopeUnit`:
```ts
{
  id: string
  category: 'crypto' | 'stock'
  source_type: 'hyper_rank' | 'ai500' | 'oi_top' | 'oi_low' | 'vergex' | 'nofxos_netflow' | 'nofxos_oi' | 'nofxos_price' | other
  direction?: 'gainers' | 'losers' | 'volume'   // optional, per-source
  limit: number
  label: string
  provider: 'free' | 'paid'
}
```

Save mapping:
- **1 scope selected** → `coin_source.source_type` = that scope's concrete type; set real
  fields (category/direction/limit). If paid, still saved (inert until provider wired).
- **>1 scopes selected** → `coin_source.source_type = 'custom'`; store the full
  `scopeUnits` + `mode` in `coin_source` (new config shape for the future backend AND/OR).
- Top-N value always captured into `limit`.
- Backend multi-source (`custom`) is new work flagged for the post-frontend pass.

## §4 — Version History modal (side drawer)

- Opens on the "Version History" row action.
- Contains a **version dropdown** (v5 current … v1), each with timestamp + note.
- Selecting a version shows its **full detail**: prompt (`custom_prompt` + prompt sections)
  and all parameters (scope, indicators/candles, risk/leverage, excluded coins, interval,
  margin, decision-context) read-only.
- **Restore** button — makes that version current and snapshots the previous state (reversible).
- Backend first pass renders via placeholder API methods
  (`strategyApi.getVersions`, `getVersion`, `restoreVersion`) showing a ⚠ "Version history
  backend pending" note while still rendering the dropdown + detail from the current
  config. API contract is fixed for later backend implementation.

## §5 — Editor step (Enter Strategy)

**Trading Strategy Prompt** — textarea (`custom_prompt`). [backend: exists]
**Strategy Name** — text input (`name`). [exists]
**Basic Rules**
- AI decision interval (min, default 15) → `scan_interval_minutes`. [trader; seeds creation]
- Max position leverage — BTC/ETH: `btc_eth_max_leverage`; Altcoin: `altcoin_max_leverage`
  (1–20). [exists]
- Max account leverage (position notional × equity) — BTC/ETH:
  `btc_eth_max_position_value_ratio`; Altcoin: `altcoin_max_position_value_ratio` (0.5–10).
  [exists]

**Advanced**
- Position mode: Cross margin / Isolated → `is_cross_margin`. [trader; seeds creation]
- Candles multi-select `1m,5m,15m,1h,4h,1d` → `selected_timeframes`. [exists]
- Excluded coins (tag input) → `excluded_coins`. [exists]
- Recent decisions context: Enable/Disable → **NEW** field, stored in config, ⚠ runtime
  wiring pending.
- Decisions in context (count) → **NEW** field ⚠.
- Context mode: Structured / Digest → **NEW** field ⚠.

The three **NEW** fields save into `ai_config.decision_context = { enabled, recent_count,
mode }` so no data is lost; prompt-builder uses them later.

Save (create) → `createStrategy` (also creates snapshot v1) → return to table.
Save (edit) → `updateStrategy` (creates a snapshot of the pre-edit state = new version after
current) → return to table.

## New backend work (post-frontend, documented here)

- Strategy version/snapshot table + list/get/restore endpoints.
- Strategy-level stats aggregation (AUM, 7D yield, merged Sharpe/MaxDD/NAV).
- `custom` multi-scope AND/OR candidate resolver.
- `decision_context` prompt-builder wiring.
- Paid scope data providers (VergeX scrape, nofxos.ai).

## §6 — Error handling & testing

- API failures → `notify.error` toast + inline banner; table renders rows with stat cells at
  fallback "—".
- Wizard validation on Next/Save + clamping of numeric inputs.
- Load/save spinners; placeholder shimmer on resolving stat cells.
- Placeholder paths never throw; uniform ⚠ tooltip.

Tests (vitest + @testing-library/react):
- `strategyFactory` config-mapping: free single scope → concrete coin_source; >1 scope →
  `custom`; single paid scope → its source_type; the 4 leverage fields → correct
  `risk_control` fields.
- `ScopeUnit` serialization / optional-direction shape.

Verification: `cd web && npm run build`, `npm run lint`, `npm test` all must pass.
