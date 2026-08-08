# Strategy Manager Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `/strategy` route with a complete strategy manager: a strategy table plus a two-step create/edit wizard (Trading Scope → Enter Strategy), wiring existing backend endpoints and using structured placeholders for not-yet-built backend features.

**Architecture:** Frontend-first. `web/src/features/strategies/` holds a table page (`StrategyManagerPage`), two wizard steps (`ScopeStepPage`, `EditorStepPage`), a version-history side modal, a pure `strategyFactory` that builds the strategy config JSON, a scope catalog, a shared wizard draft store, and a typed strategy API layer with placeholder methods. A small patch to `TraderConfigModal` + `AITradersPage` enables "Use in Trader Agent" pre-selection. The old `StrategyStudioPage.tsx` is archived (moved, not deleted) out of the router.

**Tech Stack:** React 18, TypeScript, Vite, react-router-dom v7 (`useNavigate`/`useSearchParams`), Zustand (draft store), lightweight-charts (NAV sparklines; already a dependency), sonner (`notify`), vitest + @testing-library/react.

## Global Constraints

- English-only UI; local string literals (no global i18n keys).
- `npm run build` (tsc + vite), `npm run lint` (max-warnings 0), and `npm test` must all pass.
- Every numeric input is clamped to the backend's range: Top-N 1–50; interval ≥3 minutes; leverage 1–20; position-value ratio 0.5–10.
- All not-yet-implemented backend features render a consistent ⚠ "Backend pending / data source not configured" note and never throw.
- The old strategy page is archived at `web/src/pages/legacy/StrategyStudioPage.legacy.tsx` and removed from the router — NOT deleted.
- `coin_source.source_type` union gains `'custom'` (used only when >1 scope selected).

---

### Task 1: Add `custom` source_type + new config types

**Files:**
- Modify: `web/src/types/strategy.ts`

**Interfaces:**
- Produces:
  - `CoinSourceConfig.source_type` accepts `... | 'custom'` (union becomes `'static' | 'ai500' | 'oi_top' | 'oi_low' | 'hyper_all' | 'hyper_main' | 'hyper_rank' | 'vergex_signal' | 'custom'`).
  - `CoinSourceConfig.custom_scope?: CustomScopeConfig` and `CoinSourceConfig.scope_mode?: 'overlap' | 'union'`.
  - `ScopeUnit` and `CustomScopeConfig` types (exported for later tasks).
  - `AIStrategyConfig.decision_context?: DecisionContextConfig`.

- [ ] **Step 1: Edit the CoinSourceConfig source_type union**

In `web/src/types/strategy.ts`, change line 108 from:
```ts
  source_type: 'static' | 'ai500' | 'oi_top' | 'oi_low' | 'hyper_all' | 'hyper_main' | 'hyper_rank' | 'vergex_signal';
```
to:
```ts
  source_type:
    | 'static'
    | 'ai500'
    | 'oi_top'
    | 'oi_low'
    | 'hyper_all'
    | 'hyper_main'
    | 'hyper_rank'
    | 'vergex_signal'
    | 'custom';
  custom_scope?: CustomScopeConfig;
  scope_mode?: 'overlap' | 'union';
```

- [ ] **Step 2: Add the new types to the same file**

Append these interfaces at the end of `web/src/types/strategy.ts`:
```ts
// A single selected trading-scope card (from the scope wizard step).
export interface ScopeUnit {
  id: string
  category: 'crypto' | 'stock'
  source_type:
    | 'hyper_rank'
    | 'ai500'
    | 'oi_top'
    | 'oi_low'
    | 'vergex'
    | 'nofxos_netflow'
    | 'nofxos_oi'
    | 'nofxos_price'
    | 'other'
  // Optional per-source qualifier (only for sources that use a direction,
  // e.g. hyper_rank gainers/losers/volume). Absent for AI500/OI/Netflow etc.
  direction?: 'gainers' | 'losers' | 'volume'
  limit: number
  label: string
  provider: 'free' | 'paid'
}

// The full multi-scope selection plus Overlap/Union mode. Stored on the
// strategy so the future backend can resolve candidate pools in AND/OR form.
export interface CustomScopeConfig {
  scope_units: ScopeUnit[]
  mode: 'overlap' | 'union'
}

// Runtime prompt-context knobs. Backend prompt-builder wiring is pending;
// the frontend persists these so no data is lost.
export interface DecisionContextConfig {
  enabled: boolean
  recent_count: number
  mode: 'structured' | 'digest'
}
```

- [ ] **Step 3: Add decision_context to AIStrategyConfig**

In `web/src/types/strategy.ts`, add to `AIStrategyConfig` (around line 60-66):
```ts
export interface AIStrategyConfig {
  coin_source: CoinSourceConfig
  indicators: IndicatorConfig
  custom_prompt?: string
  risk_control: RiskControlConfig
  prompt_sections?: PromptSectionsConfig
  decision_context?: DecisionContextConfig // NEW
}
```

- [ ] **Step 4: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/strategy.ts
git commit -m "feat(strategy): add custom source_type and scope/decision-context types"
```

---

### Task 2: Create the scope catalog

**Files:**
- Create: `web/src/features/strategies/scopeCatalog.ts`
- Create: `web/src/features/strategies/scopeCatalog.test.ts`

**Interfaces:**
- Consumes: `ScopeUnit` (Task 1).
- Produces:
  - `ScopeCardDef` type: `{ id, category, label, description, provider: 'free'|'paid', source_type, direction?, defaultLimit }`.
  - `SCOPE_CARD_DEFS: ScopeCardDef[]` — all free + paid cards (Crypto and Stock).
  - `toScopeUnit(def: ScopeCardDef, limit: number): ScopeUnit`.

- [ ] **Step 1: Write the failing test**

`web/src/features/strategies/scopeCatalog.test.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { SCOPE_CARD_DEFS, toScopeUnit } from './scopeCatalog'

describe('scope catalog', () => {
  it('exposes free hyper rank crypto option', () => {
    const free = SCOPE_CARD_DEFS.filter((c) => c.provider === 'free')
    expect(free.length).toBe(3)
    expect(free.every((c) => c.source_type === 'hyper_rank')).toBe(true)
    expect(free.every((c) => c.category === 'crypto')).toBe(true)
  })

  it('has paid bias-radar stock cards', () => {
    const bias = SCOPE_CARD_DEFS.filter((c) => c.label.includes('Bias Radar'))
    expect(bias.length).toBe(4) // 2 crypto + 2 stock
    expect(bias.every((c) => c.provider === 'paid')).toBe(true)
  })

  it('produces a ScopeUnit carrying the limit and only relevant direction', () => {
    const gainers = SCOPE_CARD_DEFS.find((c) => c.id === 'crypto-top-gainers')!
    const unit = toScopeUnit(gainers, 10)
    expect(unit.source_type).toBe('hyper_rank')
    expect(unit.direction).toBe('gainers')
    expect(unit.limit).toBe(10)
    expect(unit.provider).toBe('free')

    const ai500 = SCOPE_CARD_DEFS.find((c) => c.id === 'crypto-ai500')!
    const aiUnit = toScopeUnit(ai500, 5)
    expect(aiUnit.source_type).toBe('ai500')
    expect(aiUnit.direction).toBeUndefined()
    expect(aiUnit.limit).toBe(5)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/scopeCatalog.test.ts`
Expected: FAIL (module/function not found).

- [ ] **Step 3: Write the implementation**

Create `web/src/features/strategies/scopeCatalog.ts`:
```ts
import type { ScopeUnit } from '../../types/strategy'

export interface ScopeCardDef {
  id: string
  category: 'crypto' | 'stock'
  label: string
  description: string
  provider: 'free' | 'paid'
  source_type: ScopeUnit['source_type']
  direction?: 'gainers' | 'losers' | 'volume'
  defaultLimit: number
}

// Free = Hyperliquid-native, works with existing backend today.
const freeHyperRank = (
  id: string,
  label: string,
  description: string,
  direction: 'gainers' | 'losers' | 'volume'
): ScopeCardDef => ({
  id,
  category: 'crypto',
  label,
  description,
  provider: 'free',
  source_type: 'hyper_rank',
  direction,
  defaultLimit: 10,
})

export const SCOPE_CARD_DEFS: ScopeCardDef[] = [
  freeHyperRank('crypto-top-gainers', 'Crypto Top Gainers', 'Top % gainers on Hyperliquid', 'gainers'),
  freeHyperRank('crypto-top-losers', 'Crypto Top Losers', 'Top % losers on Hyperliquid', 'losers'),
  freeHyperRank('crypto-trending', 'Crypto Trending · Top Volume', 'Top traders by volume on Hyperliquid', 'volume'),

  // Crypto — PAID / provider pending
  { id: 'crypto-bias-bull', category: 'crypto', label: 'Bias Radar (Bullish)', description: 'VergeX bullish bias radar', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
  { id: 'crypto-bias-bear', category: 'crypto', label: 'Bias Radar (Bearish)', description: 'VergeX bearish bias radar', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
  { id: 'crypto-ai500', category: 'crypto', label: 'AI500 Data Provider', description: 'AI500 data provider (nofxos)', provider: 'paid', source_type: 'ai500', defaultLimit: 10 },
  { id: 'crypto-oi-increase', category: 'crypto', label: 'OI Increase', description: 'Open interest increase (nofxos)', provider: 'paid', source_type: 'nofxos_oi', defaultLimit: 10 },
  { id: 'crypto-oi-decrease', category: 'crypto', label: 'OI Decrease', description: 'Open interest decrease (nofxos)', provider: 'paid', source_type: 'nofxos_oi', defaultLimit: 10 },
  { id: 'crypto-netflow-top', category: 'crypto', label: 'Netflow Top', description: 'Top netflow (nofxos)', provider: 'paid', source_type: 'nofxos_netflow', defaultLimit: 10 },
  { id: 'crypto-netflow-outflow', category: 'crypto', label: 'Netflow Outflow Top', description: 'Top net outflow (nofxos)', provider: 'paid', source_type: 'nofxos_netflow', defaultLimit: 10 },
  { id: 'crypto-gainers-nofxos', category: 'crypto', label: 'Crypto Top Gainers (NOFXOS)', description: 'Top gainers via nofxos', provider: 'paid', source_type: 'nofxos_price', defaultLimit: 10 },
  { id: 'crypto-losers-nofxos', category: 'crypto', label: 'Crypto Top Losers (NOFXOS)', description: 'Top losers via nofxos', provider: 'paid', source_type: 'nofxos_price', defaultLimit: 10 },

  // Stock — PAID, VergeX/Claw402
  { id: 'stock-bias-bull', category: 'stock', label: 'Bias Radar (Bullish)', description: 'VergeX US-stock bullish bias radar', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
  { id: 'stock-bias-bear', category: 'stock', label: 'Bias Radar (Bearish)', description: 'VergeX US-stock bearish bias radar', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
  { id: 'stock-trending', category: 'stock', label: 'Trending Stocks', description: 'Trending US stocks (VergeX)', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
  { id: 'stock-gainers', category: 'stock', label: 'Stock Gainers', description: 'US stock gainers (VergeX)', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
  { id: 'stock-losers', category: 'stock', label: 'Stock Losers', description: 'US stock losers (VergeX)', provider: 'paid', source_type: 'vergex', defaultLimit: 10 },
]

export function toScopeUnit(
  def: ScopeCardDef,
  limit: number
): ScopeUnit {
  return {
    id: def.id,
    category: def.category,
    source_type: def.source_type,
    limit,
    label: def.label,
    provider: def.provider,
    ...(def.direction ? { direction: def.direction } : {}),
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/strategies/scopeCatalog.test.ts`
Expected: PASS (3 tests green).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/strategies/scopeCatalog.ts web/src/features/strategies/scopeCatalog.test.ts
git commit -m "feat(strategy): add trading scope catalog"
```

---

### Task 3: Strategy draft store (shared across wizard pages)

**Files:**
- Create: `web/src/features/strategies/draftStore.ts`

**Interfaces:**
- Consumes: `CustomScopeConfig`, `ScopeUnit` (Task 1), `SCOPE_CARD_DEFS`/`toScopeUnit` (Task 2).
- Produces (exported from a Zustand store):
  - `draft.scope: { units: ScopeUnit[]; mode: 'overlap' | 'union' }`
  - `setScopeUnits(units: ScopeUnit[])`
  - `setScopeMode(mode)`
  - `mergeScopeUnit(unit: ScopeUnit)` and `removeScopeUnit(id: string)` (toggle helpers)
  - `resetDraft()`

- [ ] **Step 1: Write the implementation**

Create `web/src/features/strategies/draftStore.ts`:
```ts
import { create } from 'zustand'
import type { ScopeUnit } from '../../types/strategy'

interface StrategyDraft {
  scope: {
    units: ScopeUnit[]
    mode: 'overlap' | 'union'
  }
  setScopeUnits: (units: ScopeUnit[]) => void
  setScopeMode: (mode: 'overlap' | 'union') => void
  mergeScopeUnit: (unit: ScopeUnit) => void
  removeScopeUnit: (id: string) => void
  resetDraft: () => void
}

export const useStrategyDraft = create<StrategyDraft>((set) => ({
  scope: { units: [], mode: 'union' },
  setScopeUnits: (units) => set({ scope: { mode: 'union', units } }),
  setScopeMode: (mode) =>
    set((s) => ({ scope: { ...s.scope, mode } })),
  mergeScopeUnit: (unit) =>
    set((s) => {
      const exists = s.scope.units.some((u) => u.id === unit.id)
      const units = exists
        ? s.scope.units.map((u) => (u.id === unit.id ? unit : u))
        : [...s.scope.units, unit]
      return { scope: { ...s.scope, units } }
    }),
  removeScopeUnit: (id) =>
    set((s) => ({
      scope: { ...s.scope, units: s.scope.units.filter((u) => u.id !== id) },
    })),
  resetDraft: () => set({ scope: { units: [], mode: 'union' } }),
}))

export function resetStrategyDraft() {
  useStrategyDraft.getState().resetDraft()
}
```

- [ ] **Step 2: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/strategies/draftStore.ts
git commit -m "feat(strategy): add shared wizard draft store"
```

---

### Task 4: Strategy config factory (pure, tested)

**Files:**
- Create: `web/src/features/strategies/strategyFactory.ts`
- Create: `web/src/features/strategies/strategyFactory.test.ts`

**Interfaces:**
- Consumes: `ScopeUnit`, `StrategyConfig`, `CoinSourceConfig`, `IndicatorConfig`, `RiskControlConfig`, `DecisionContextConfig` (Task 1).
- Produces:
  - `buildCoinSource(units: ScopeUnit[]): CoinSourceConfig` — returns a `CoinSourceConfig`; sets `source_type='custom'` + `custom_scope` when >1 units, else the single unit's concrete source; always writes `scope_mode`.
  - `defaultRiskControl(options): RiskControlConfig` — maps the four leverage/notional inputs.
  - `buildStrategyConfig(input): StrategyConfig` — composes the full config for the editor.
  - `StrategyEditorForm` type: `{ name, custom_prompt, scan_interval_minutes, btcEthMaxLeverage, altcoinMaxLeverage, btcEthPositionRatio, altcoinPositionRatio, isCrossMargin, selectedTimeframes, excludedCoins, decisionContext, scopeUnits, scopeMode }`.

- [ ] **Step 1: Write the failing test**

`web/src/features/strategies/strategyFactory.test.ts`:
```ts
import { describe, expect, it } from 'vitest'
import {
  buildCoinSource,
  buildStrategyConfig,
  defaultRiskControl,
} from './strategyFactory'
import type { ScopeUnit } from '../../types/strategy'

const freeUnit = (
  direction: 'gainers' | 'losers' | 'volume'
): ScopeUnit => ({
  id: `crypto-${direction}`,
  category: 'crypto',
  source_type: 'hyper_rank',
  direction,
  limit: 10,
  label: `Top ${direction}`,
  provider: 'free',
})

describe('strategy factory', () => {
  it('maps a single free scope to its concrete coin source', () => {
    const cs = buildCoinSource([freeUnit('gainers')])
    expect(cs.source_type).toBe('hyper_rank')
    expect(cs.hyper_rank_category).toBe('crypto')
    expect(cs.hyper_rank_direction).toBe('gainers')
    expect(cs.hyper_rank_limit).toBe(10)
    expect(cs.scope_mode).toBe('union')
  })

  it('maps a single paid scope to its own concrete source', () => {
    const paid: ScopeUnit = {
      id: 'crypto-bias-bull',
      category: 'crypto',
      source_type: 'vergex',
      limit: 10,
      label: 'Bias Radar (Bullish)',
      provider: 'paid',
    }
    const cs = buildCoinSource([paid])
    expect(cs.source_type).toBe('vergex')
    expect(cs.custom_scope).toBeUndefined()
  })

  it('uses custom source_type when more than one scope is selected', () => {
    const cs = buildCoinSource([freeUnit('gainers'), freeUnit('losers')])
    expect(cs.source_type).toBe('custom')
    expect(cs.custom_scope?.scope_units).toHaveLength(2)
    expect(cs.custom_scope?.mode).toBe('union')
  })

  it('writes the four leverage/notional controls into risk control', () => {
    const risk = defaultRiskControl({
      btcEthMaxLeverage: 5,
      altcoinMaxLeverage: 3,
      btcEthPositionRatio: 5,
      altcoinPositionRatio: 3,
    })
    expect(risk.btc_eth_max_leverage).toBe(5)
    expect(risk.altcoin_max_leverage).toBe(3)
    expect(risk.btc_eth_max_position_value_ratio).toBe(5)
    expect(risk.altcoin_max_position_value_ratio).toBe(3)
  })

  it('builds a full config with decision context persisted', () => {
    const cfg = buildStrategyConfig({
      name: 'Test',
      custom_prompt: 'hello',
      scan_interval_minutes: 15,
      btcEthMaxLeverage: 5,
      altcoinMaxLeverage: 5,
      btcEthPositionRatio: 5,
      altcoinPositionRatio: 5,
      isCrossMargin: true,
      selectedTimeframes: ['15m'],
      excludedCoins: ['SAMECOIN'],
      decisionContext: { enabled: true, recent_count: 8, mode: 'digest' },
      scopeUnits: [freeUnit('gainers')],
      scopeMode: 'union',
    })
    expect(cfg.ai_config?.custom_prompt).toBe('hello')
    expect(cfg.ai_config?.decision_context).toEqual({
      enabled: true,
      recent_count: 8,
      mode: 'digest',
    })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/strategyFactory.test.ts`
Expected: FAIL (module not found).

- [ ] **Step 3: Write the implementation**

Create `web/src/features/strategies/strategyFactory.ts`:
```ts
import type {
  CoinSourceConfig,
  DecisionContextConfig,
  RiskControlConfig,
  ScopeUnit,
  StrategyConfig,
} from '../../types/strategy'

export interface StrategyEditorForm {
  name: string
  custom_prompt: string
  scan_interval_minutes: number
  btcEthMaxLeverage: number
  altcoinMaxLeverage: number
  btcEthPositionRatio: number
  altcoinPositionRatio: number
  isCrossMargin: boolean
  selectedTimeframes: string[]
  excludedCoins: string[]
  decisionContext: DecisionContextConfig
  scopeUnits: ScopeUnit[]
  scopeMode: 'overlap' | 'union'
}

function clamp(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

export function buildCoinSource(units: ScopeUnit[]): CoinSourceConfig {
  const mode = 'union' // ScopeStepPage persists mode into the draft; factory keeps default here
  if (units.length > 1) {
    return {
      source_type: 'custom',
      scope_mode: mode,
      custom_scope: { scope_units: units, mode },
      static_coins: [],
      excluded_coins: [],
      use_ai500: false,
      ai500_limit: 0,
      use_oi_top: false,
      oi_top_limit: 0,
      use_oi_low: false,
      oi_low_limit: 0,
      use_hyper_all: false,
      use_hyper_main: false,
      hyper_main_limit: 0,
      vergex_limit: 0,
    }
  }

  const unit = units[0]
  if (unit?.source_type === 'hyper_rank') {
    return {
      source_type: 'hyper_rank',
      scope_mode: mode,
      hyper_rank_category: unit.category,
      hyper_rank_direction: unit.direction || 'gainers',
      hyper_rank_limit: clamp(unit.limit, 1, 50),
      static_coins: [],
      excluded_coins: [],
      use_ai500: false,
      ai500_limit: 0,
      use_oi_top: false,
      oi_top_limit: 0,
      use_oi_low: false,
      oi_low_limit: 0,
      use_hyper_all: false,
      use_hyper_main: false,
      vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'ai500') {
    return {
      source_type: 'ai500',
      scope_mode: mode,
      use_ai500: true,
      ai500_limit: clamp(unit.limit, 1, 50),
      static_coins: [],
      excluded_coins: [],
      use_oi_top: false,
      oi_top_limit: 0,
      use_oi_low: false,
      oi_low_limit: 0,
      use_hyper_all: false,
      use_hyper_main: false,
      vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'vergex') {
    return {
      source_type: 'vergex_signal',
      scope_mode: mode,
      vergex_limit: clamp(unit.limit, 1, 50),
      static_coins: [],
      excluded_coins: [],
      use_ai500: false,
      ai500_limit: 0,
      use_oi_top: false,
      oi_top_limit: 0,
      use_oi_low: false,
      oi_low_limit: 0,
      use_hyper_all: false,
      use_hyper_main: false,
    }
  }

  // Any other paid/provider-pending source: mark custom with the units but
  // fall back to a safe static/empty pool so runtime never errors today.
  return {
    source_type: 'custom',
    scope_mode: mode,
    custom_scope: { scope_units: units, mode },
    static_coins: [],
    excluded_coins: [],
    use_ai500: false,
    ai500_limit: 0,
    use_oi_top: false,
    oi_top_limit: 0,
    use_oi_low: false,
    oi_low_limit: 0,
    use_hyper_all: false,
    use_hyper_main: false,
    vergex_limit: 0,
  }
}

export function defaultRiskControl(input?: {
  btcEthMaxLeverage?: number
  altcoinMaxLeverage?: number
  btcEthPositionRatio?: number
  altcoinPositionRatio?: number
}): RiskControlConfig {
  return {
    max_positions: 2,
    btc_eth_max_leverage: clamp(input?.btcEthMaxLeverage ?? 5, 1, 20),
    altcoin_max_leverage: clamp(input?.altcoinMaxLeverage ?? 5, 1, 20),
    btc_eth_max_position_value_ratio: clamp(
      input?.btcEthPositionRatio ?? 5,
      0.5,
      10
    ),
    altcoin_max_position_value_ratio: clamp(
      input?.altcoinPositionRatio ?? 5,
      0.5,
      10
    ),
    max_margin_usage: 1.0,
    min_position_size: 12,
    min_risk_reward_ratio: 3,
    min_confidence: 78,
  }
}

export function buildStrategyConfig(form: StrategyEditorForm): StrategyConfig {
  return {
    strategy_type: 'ai_trading',
    language: 'en',
    ai_config: {
      coin_source: buildCoinSource(form.scopeUnits),
      indicators: {
        klines: {
          primary_timeframe: form.selectedTimeframes[0] ?? '15m',
          primary_count: 30,
          enable_multi_timeframe: form.selectedTimeframes.length > 1,
          selected_timeframes: form.selectedTimeframes,
        },
        enable_raw_klines: true,
        enable_ema: false,
        enable_macd: false,
        enable_rsi: false,
        enable_atr: false,
        enable_boll: false,
        enable_volume: false,
        enable_oi: false,
        enable_funding_rate: false,
        nofxos_api_key: '',
        enable_quant_data: false,
        enable_quant_oi: false,
        enable_quant_netflow: false,
        enable_oi_ranking: false,
        enable_netflow_ranking: false,
        enable_price_ranking: false,
      },
      custom_prompt: form.custom_prompt,
      risk_control: defaultRiskControl(form),
      decision_context: form.decisionContext,
    },
    grid_config: null,
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/strategies/strategyFactory.test.ts`
Expected: PASS (5 tests green).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/strategies/strategyFactory.ts web/src/features/strategies/strategyFactory.test.ts
git commit -m "feat(strategy): add config factory with scope/leverage mapping"
```

---

### Task 5: Strategy manager API layer (with placeholders)

**Files:**
- Create: `web/src/features/strategies/strategyApi.ts`

**Interfaces:**
- Consumes: `Strategy`, `StrategyConfig` types; global `api.strategyApi` methods (`getStrategies`, `createStrategy`, `updateStrategy`, `deleteStrategy`, `activateStrategy`, `duplicateStrategy`); `api.getTraders`, `api.getAccount`, `api.getPositions`, `api.getEquityHistoryBatch`.
- Produces:
  - `StrategyVersion` type: `{ version: number; strategy_id: string; label: string; note: string; config: StrategyConfig; created_at: string; is_current: boolean }`.
  - `getStrategies()`, `createStrategy`, `updateStrategy`, `deleteStrategy`, `activateStrategy`, `duplicateStrategy` (re-export pass-through).
  - Placeholders (return deterministic local snapshot data, never throw):
    - `getVersions(strategyId, currentConfig): Promise<StrategyVersion[]>`
    - `getVersion(strategyId, version, currentConfig)` — resolves a single version.
    - `restoreVersion(strategyId, version)` — resolves `{ ok: true }` (backend pending).
  - `getStrategyStats(strategyId)` aggregate helper returning `{ aum, symbols, navPoints }` from existing trader endpoints (live), with `7dYield/sharpe/maxDd` left as `null` (placeholder).

- [ ] **Step 1: Write the implementation**

Create `web/src/features/strategies/strategyApi.ts`:
```ts
import type {
  Strategy,
  StrategyConfig,
} from '../../types/strategy'
import { api } from '../../lib/api'
import { strategyApi } from '../../lib/api/strategies'

export interface StrategyVersion {
  version: number
  strategy_id: string
  label: string
  note: string
  config: StrategyConfig
  created_at: string
  is_current: boolean
}

async function getStrategies(): Promise<Strategy[]> {
  return strategyApi.getStrategies()
}
async function createStrategy(data: {
  name: string
  description: string
  config: StrategyConfig
}): Promise<Strategy> {
  return strategyApi.createStrategy(data)
}
async function updateStrategy(
  id: string,
  data: { name?: string; description?: string; config?: StrategyConfig }
): Promise<Strategy> {
  return strategyApi.updateStrategy(id, data)
}
async function activateStrategy(id: string): Promise<Strategy> {
  return strategyApi.activateStrategy(id)
}
async function duplicateStrategy(id: string): Promise<Strategy> {
  return strategyApi.duplicateStrategy(id)
}
async function deleteStrategy(id: string): Promise<void> {
  return strategyApi.deleteStrategy(id)
}

// ----- Version history (backend pending; local deterministic snapshot) -----

function snapshotVersion(
  strategyId: string,
  version: number,
  config: StrategyConfig,
  label: string,
  note: string,
  isCurrent: boolean
): StrategyVersion {
  return {
    version,
    strategy_id: strategyId,
    label,
    note,
    config,
    created_at: new Date().toISOString(),
    is_current: isCurrent,
  }
}

export async function getVersions(
  strategyId: string,
  currentConfig: StrategyConfig
): Promise<StrategyVersion[]> {
  // Backend pending: expose the current config as a single v1 snapshot so the
  // UI renders the full dropdown layout. Replace with GET /strategies/:id/versions.
  const current = snapshotVersion(
    strategyId,
    1,
    currentConfig,
    'v1',
    'Current configuration',
    true
  )
  return [current]
}

export async function getVersion(
  strategyId: string,
  version: number,
  currentConfig: StrategyConfig
): Promise<StrategyVersion | null> {
  const versions = await getVersions(strategyId, currentConfig)
  return versions.find((v) => v.version === version) ?? versions[0] ?? null
}

export async function restoreVersion(
  _strategyId: string,
  _version: number
): Promise<{ ok: boolean }> {
  // Backend pending: POST /strategies/:id/restore. Until implemented, report
  // that no change happened so the UI can show a consistent "pending" state.
  return { ok: false }
}

// ----- Live per-strategy stats from existing trader endpoints -----

export interface StrategyStats {
  aum: number
  symbols: string[]
  navPoints: { timestamp: string; total_equity: number }[]
  // Aggregation backend pending for these:
  sevenDayYield: number | null
  sharpe: number | null
  maxDd: number | null
}

export async function getStrategyStats(
  strategyId: string
): Promise<StrategyStats> {
  const traders = await api.getTraders(true)
  const linked = traders.filter((t) => t.strategy_id === strategyId)
  const ids = linked.map((t) => t.trader_id)

  if (ids.length === 0) {
    return {
      aum: 0,
      symbols: [],
      navPoints: [],
      sevenDayYield: null,
      sharpe: null,
      maxDd: null,
    }
  }

  const [accounts, positions, equityBatch] = await Promise.all([
    Promise.all(ids.map((id) => api.getAccount(id, true).catch(() => null))),
    Promise.all(ids.map((id) => api.getPositions(id, true).catch(() => null))),
    api.getEquityHistoryBatch(ids).catch(() => null),
  ])

  const aum = accounts.reduce(
    (sum, a) => sum + (a?.total_equity ?? 0),
    0
  )
  const symbols = Array.from(
    new Set(positions.flat().map((p) => p?.symbol).filter(Boolean))
  )
  const histories = equityBatch?.histories ?? {}
  let navPoints: { timestamp: string; total_equity: number }[] = []
  if (ids.length > 0) {
    const seriesByTs = new Map<string, number>()
    for (const id of ids) {
      for (const point of histories[id] ?? []) {
        const t = point.timestamp
        seriesByTs.set(t, (seriesByTs.get(t) ?? 0) + (point.total_equity ?? 0))
      }
    }
    navPoints = Array.from(seriesByTs, ([timestamp, total_equity]) => ({
      timestamp,
      total_equity,
    })).sort((a, b) => a.timestamp.localeCompare(b.timestamp))
  }

  return { aum, symbols, navPoints, sevenDayYield: null, sharpe: null, maxDd: null }
}

export const strategyManagerApi = {
  getStrategies,
  createStrategy,
  updateStrategy,
  activateStrategy,
  duplicateStrategy,
  deleteStrategy,
  getVersions,
  getVersion,
  restoreVersion,
  getStrategyStats,
}
```

- [ ] **Step 2: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/strategies/strategyApi.ts
git commit -m "feat(strategy): add API layer with version history placeholders"
```

---

### Task 6: Scope step page

**Files:**
- Create: `web/src/features/strategies/ScopeStepPage.tsx`

**Interfaces:**
- Consumes: `SCOPE_CARD_DEFS`, `toScopeUnit` (Task 2); `useStrategyDraft` (Task 3).
- Produces: a page at `scope` route; on "Next" pushes to `/strategy/<mode>/editor`. Renders the scope cards grouped by category, a Top-N number input per card, an Overlap/Union toggle, and a ⚠ note for paid cards.

- [ ] **Step 1: Write the implementation**

Create `web/src/features/strategies/ScopeStepPage.tsx`:
```tsx
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  ArrowLeft,
  ShieldAlert,
  ChevronRight,
} from 'lucide-react'
import { useStrategyDraft } from './draftStore'
import { SCOPE_CARD_DEFS, toScopeUnit } from './scopeCatalog'
import type { ScopeCardDef } from './scopeCatalog'
import type { ScopeUnit } from '../../types/strategy'

type Mode = 'create' | 'edit'

function cardActive(
  units: ScopeUnit[],
  def: ScopeCardDef
): boolean {
  return units.some((u) => u.id === def.id)
}

function cardUnit(
  units: ScopeUnit[],
  def: ScopeCardDef,
  limit: number
): ScopeUnit {
  const existing = units.find((u) => u.id === def.id)
  return toScopeUnit(def, existing?.limit ?? limit)
}

export function ScopeStepPage() {
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const mode: Mode = params.id ? 'edit' : 'create'
  const strategyId = params.id
  const { scope, mergeScopeUnit, removeScopeUnit, setScopeMode } =
    useStrategyDraft()
  const [topN, setTopN] = useState<Record<string, number>>({})
  const [category, setCategory] = useState<'crypto' | 'stock'>('crypto')

  const cards = SCOPE_CARD_DEFS.filter((c) => c.category === category)
  const nextPath =
    mode === 'create'
      ? '/strategy/create/editor'
      : `/strategy/${strategyId}/edit/editor`
  const backPath = '/strategy'

  const toggleCard = (def: ScopeCardDef) => {
    if (cardActive(scope.units, def)) {
      removeScopeUnit(def.id)
    } else {
      mergeScopeUnit(cardUnit(scope.units, def, topN[def.id] ?? def.defaultLimit))
    }
  }

  const updateLimit = (def: ScopeCardDef, raw: number) => {
    const limit = Math.min(50, Math.max(1, raw || 1))
    setTopN((prev) => ({ ...prev, [def.id]: limit }))
    if (cardActive(scope.units, def)) {
      mergeScopeUnit(cardUnit(scope.units, def, limit))
    }
  }

  return (
    <div className="mx-auto max-w-4xl p-6">
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => navigate(backPath)}
            className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-2 text-nofx-text-muted hover:text-nofx-text"
            aria-label="Back to strategies"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <h1 className="text-xl font-semibold text-nofx-text">
              {mode === 'create' ? 'New Strategy' : 'Edit Strategy'}
            </h1>
            <p className="mt-1 text-sm text-nofx-text-muted">
              Step 1 of 2 · Select Trading Scope
            </p>
          </div>
        </div>
        <button
          type="button"
          onClick={() =>
            setScopeMode(scope.mode === 'union' ? 'overlap' : 'union')
          }
          className="rounded-lg border border-nofx-gold/30 bg-nofx-gold/10 px-4 py-2 text-sm font-medium text-nofx-gold"
        >
          {scope.mode === 'union' ? 'Union' : 'Overlap'}
        </button>
      </div>

      <div className="mb-6 flex gap-2">
        {(['crypto', 'stock'] as const).map((c) => (
          <button
            key={c}
            type="button"
            onClick={() => setCategory(c)}
            className={`rounded-lg border px-4 py-2 text-sm ${
              category === c
                ? 'border-nofx-gold bg-nofx-gold/10 text-nofx-gold'
                : 'border-[rgba(26,24,19,0.14)] text-nofx-text-muted'
            }`}
          >
            {c === 'crypto' ? 'Crypto' : 'Stock'}
          </button>
        ))}
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        {cards.map((def) => {
          const active = cardActive(scope.units, def)
          return (
            <div
              key={def.id}
              role="button"
              tabIndex={0}
              onClick={() => toggleCard(def)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  toggleCard(def)
                }
              }}
              className={`cursor-pointer rounded-lg border p-4 transition ${
                active
                  ? 'border-nofx-gold bg-nofx-gold/10'
                  : 'border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper'
              }`}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm font-semibold text-nofx-text">
                  {def.label}
                </span>
                <span
                  className={`rounded px-2 py-0.5 text-[10px] font-semibold ${
                    def.provider === 'free'
                      ? 'bg-nofx-success/15 text-nofx-success'
                      : 'bg-nofx-danger/15 text-nofx-danger'
                  }`}
                >
                  {def.provider === 'free' ? 'FREE' : 'PAID'}
                </span>
              </div>
              <p className="mt-1 text-xs text-nofx-text-muted">
                {def.description}
              </p>
              <div className="mt-3 flex items-center gap-2">
                <label className="text-xs text-nofx-text-muted">Top</label>
                <input
                  type="number"
                  min={1}
                  max={50}
                  value={topN[def.id] ?? def.defaultLimit}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) =>
                    updateLimit(def, Number(e.target.value))
                  }
                  className="w-20 rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text"
                />
              </div>
              {def.provider === 'paid' && (
                <div className="mt-2 flex items-center gap-1.5 text-[11px] text-nofx-danger">
                  <ShieldAlert className="h-3 w-3" />
                  Data source not configured / backend pending
                </div>
              )}
            </div>
          )
        })}
      </div>

      <div className="mt-8 flex justify-end">
        <button
          type="button"
          disabled={scope.units.length === 0}
          onClick={() => navigate(nextPath)}
          className="inline-flex items-center gap-2 rounded-lg bg-nofx-gold px-5 py-2 text-sm font-semibold text-nofx-bg disabled:cursor-not-allowed disabled:opacity-40"
        >
          Next <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/strategies/ScopeStepPage.tsx
git commit -m "feat(strategy): add scope step wizard page"
```

---

### Task 7: Editor step page

**Files:**
- Create: `web/src/features/strategies/EditorStepPage.tsx`

**Interfaces:**
- Consumes: `useStrategyDraft` (Task 3), `buildStrategyConfig`/`StrategyEditorForm` (Task 4), `strategyManagerApi` (Task 5), `successToast`? — uses `notify` from `../../lib/notify`.
- Produces: step-2 page; on Save calls create or update and navigates back to `/strategy`. On Back goes to scope step.

- [ ] **Step 1: Write the implementation**

Create `web/src/features/strategies/EditorStepPage.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  ArrowLeft,
  Loader2,
  Save,
} from 'lucide-react'
import { useStrategyDraft } from './draftStore'
import { strategyManagerApi } from './strategyApi'
import { notify } from '../../lib/notify'
import { api } from '../../lib/api'
import { useAuth } from '../../contexts/AuthContext'

type Mode = 'create' | 'edit'

const TIMEFRAMES = ['1m', '5m', '15m', '1h', '4h', '1d']

export function EditorStepPage() {
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const mode: Mode = params.id ? 'edit' : 'create'
  const strategyId = params.id
  const { scope } = useStrategyDraft()
  const { token } = useAuth()
  const [loading, setLoading] = useState(mode === 'edit')

  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [interval, setInterval] = useState(15)
  const [btcEthLeverage, setBtcEthLeverage] = useState(5)
  const [altLeverage, setAltLeverage] = useState(5)
  const [btcEthRatio, setBtcEthRatio] = useState(5)
  const [altRatio, setAltRatio] = useState(5)
  const [isCross, setIsCross] = useState(true)
  const [timeframes, setTimeframes] = useState<string[]>(['15m'])
  const [excluded, setExcluded] = useState('')
  const [decisionEnabled, setDecisionEnabled] = useState(true)
  const [decisionCount, setDecisionCount] = useState(8)
  const [contextMode, setContextMode] = useState<'structured' | 'digest'>('structured')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (mode !== 'edit' || !strategyId || !token) {
      setLoading(false)
      return
    }
    ;(async () => {
      try {
        const strategy = await api.getStrategy(strategyId)
        const ai = strategy.config.ai_config
        setName(strategy.name)
        setPrompt(ai?.custom_prompt ?? '')
        setBtcEthLeverage(ai?.risk_control.btc_eth_max_leverage ?? 5)
        setAltLeverage(ai?.risk_control.altcoin_max_leverage ?? 5)
        setBtcEthRatio(
          ai?.risk_control.btc_eth_max_position_value_ratio ?? 5
        )
        setAltRatio(
          ai?.risk_control.altcoin_max_position_value_ratio ?? 5
        )
        setTimeframes(
          ai?.indicators.klines.selected_timeframes ?? ['15m']
        )
        setExcluded((ai?.coin_source.excluded_coins ?? []).join(', '))
      } catch (err) {
        notify.error(err instanceof Error ? err.message : 'Failed to load strategy')
      } finally {
        setLoading(false)
      }
    })()
  }, [mode, strategyId, token])

  const backPath =
    mode === 'create'
      ? '/strategy/create/scope'
      : `/strategy/${strategyId}/edit/scope`

  const toggleTimeframe = (tf: string) => {
    setTimeframes((prev) =>
      prev.includes(tf) ? prev.filter((t) => t !== tf) : [...prev, tf]
    )
  }

  const handleSave = async () => {
    if (saving) return
    if (!name.trim()) {
      notify.error('Strategy name is required')
      return
    }
    setSaving(true)
    try {
      const config = buildStrategyConfig({
        name,
        custom_prompt: prompt,
        scan_interval_minutes: Math.max(3, interval),
        btcEthMaxLeverage: btcEthLeverage,
        altcoinMaxLeverage: altLeverage,
        btcEthPositionRatio: btcEthRatio,
        altcoinPositionRatio: altRatio,
        isCrossMargin: isCross,
        selectedTimeframes: timeframes.length ? timeframes : ['15m'],
        excludedCoins: excluded.split(',').map((s) => s.trim()).filter(Boolean),
        decisionContext: {
          enabled: decisionEnabled,
          recent_count: decisionCount,
          mode: contextMode,
        },
        scopeUnits: scope.units,
        scopeMode: scope.mode,
      })

      if (mode === 'create') {
        await strategyManagerApi.createStrategy({
          name: name.trim(),
          description: `Strategy using ${scope.units.length} scope(s)`,
          config,
        })
        notify.success('Strategy created')
      } else if (strategyId) {
        await strategyManagerApi.updateStrategy(strategyId, {
          name: name.trim(),
          config,
        })
        notify.success('Strategy saved')
      }
      navigate('/strategy')
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to save strategy')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-7 w-7 animate-spin text-nofx-gold" />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl p-6">
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => navigate(backPath)}
            className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-2 text-nofx-text-muted hover:text-nofx-text"
            aria-label="Back to scope"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <h1 className="text-xl font-semibold text-nofx-text">
              {mode === 'create' ? 'New Strategy' : 'Edit Strategy'}
            </h1>
            <p className="mt-1 text-sm text-nofx-text-muted">
              Step 2 of 2 · Enter Trading Strategy
            </p>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        <label className="block">
          <span className="text-sm font-medium text-nofx-text">Strategy Name</span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper px-3 py-2 text-sm text-nofx-text"
            placeholder="e.g. Hyper Top Gainers"
          />
        </label>

        <label className="block">
          <span className="text-sm font-medium text-nofx-text">Trading Strategy Prompt</span>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={4}
            className="mt-1 w-full resize-none rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper px-3 py-2 text-sm text-nofx-text"
            placeholder="Instructions for the AI trading loop..."
          />
        </label>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Basic Rules
          </legend>
          <div className="grid gap-4 sm:grid-cols-2">
            <NumberField label="AI decision interval (min)" value={interval} onChange={setInterval} min={3} />
            <NumberField label="Max position leverage — BTC/ETH" value={btcEthLeverage} onChange={setBtcEthLeverage} min={1} max={20} />
            <NumberField label="Max position leverage — Altcoin" value={altLeverage} onChange={setAltLeverage} min={1} max={20} />
            <NumberField label="Max account leverage (notional × equity) — BTC/ETH" value={btcEthRatio} onChange={setBtcEthRatio} min={0.5} max={10} step={0.5} />
            <NumberField label="Max account leverage (notional × equity) — Altcoin" value={altRatio} onChange={setAltRatio} min={0.5} max={10} step={0.5} />
          </div>
        </fieldset>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Advanced Settings
          </legend>
          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Position mode</span>
            <div className="mt-1 flex gap-2">
              <ToggleChip label="Cross margin" active={isCross} onClick={() => setIsCross(true)} />
              <ToggleChip label="Isolated" active={!isCross} onClick={() => setIsCross(false)} />
            </div>
          </div>

          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Candles</span>
            <div className="mt-1 flex flex-wrap gap-2">
              {TIMEFRAMES.map((tf) => (
                <ToggleChip
                  key={tf}
                  label={tf}
                  active={timeframes.includes(tf)}
                  onClick={() => toggleTimeframe(tf)}
                />
              ))}
            </div>
          </div>

          <label className="mb-4 block">
            <span className="text-sm text-nofx-text-muted">Excluded coins (comma separated)</span>
            <input
              type="text"
              value={excluded}
              onChange={(e) => setExcluded(e.target.value)}
              className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-3 py-2 text-sm text-nofx-text"
              placeholder="SAMECOIN, JUNKCOIN"
            />
          </label>

          <div className="rounded-md border border-nofx-danger/25 bg-nofx-danger/10 p-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-nofx-text">Recent decisions context</span>
              <ToggleChip label={decisionEnabled ? 'Enable' : 'Disable'} active={decisionEnabled} onClick={() => setDecisionEnabled(!decisionEnabled)} />
            </div>
            <div className="mt-3 grid gap-3 text-nofx-text-muted sm:grid-cols-2">
              <NumberField label="Decisions in context" value={decisionCount} onChange={setDecisionCount} min={1} max={50} />
              <div>
                <span className="text-sm">Context mode</span>
                <div className="mt-1 flex gap-2">
                  <ToggleChip label="Structured" active={contextMode === 'structured'} onClick={() => setContextMode('structured')} />
                  <ToggleChip label="Digest" active={contextMode === 'digest'} onClick={() => setContextMode('digest')} />
                </div>
              </div>
            </div>
            <p className="mt-2 text-xs text-nofx-danger">Runtime prompt wiring is pending backend work.</p>
          </div>
        </fieldset>
      </div>

      <div className="mt-8 flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="inline-flex items-center gap-2 rounded-lg bg-nofx-gold px-5 py-2 text-sm font-semibold text-nofx-bg disabled:cursor-not-allowed disabled:opacity-40"
        >
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          Save Strategy
        </button>
      </div>
    </div>
  )
}

function NumberField({
  label,
  value,
  onChange,
  min,
  max,
  step = 1,
}: {
  label: string
  value: number
  onChange: (n: number) => void
  min: number
  max?: number
  step?: number
}) {
  return (
    <label className="block">
      <span className="text-sm text-nofx-text-muted">{label}</span>
      <input
        type="number"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-3 py-2 text-sm text-nofx-text"
      />
    </label>
  )
}

function ToggleChip({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-lg border px-3 py-2 text-sm ${
        active
          ? 'border-nofx-gold bg-nofx-gold/10 text-nofx-gold'
          : 'border-[rgba(26,24,19,0.14)] text-nofx-text-muted'
      }`}
    >
      {label}
    </button>
  )
}
```

- [ ] **Step 2: Add buildStrategyConfig import**

In `EditorStepPage.tsx`, the code references `buildStrategyConfig` — it needs a top-level import:
```ts
import { buildStrategyConfig } from './strategyFactory'
```
Add this line with the other imports at the top of the file (right after `import { api } from '../../lib/api'`).

- [ ] **Step 3: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors. (If `api.getStrategy` isn't exported on the shared `api` object, use `strategyApi.getStrategy` instead — verify the export in `lib/api/index.ts`.)

- [ ] **Step 4: Commit**

```bash
git add web/src/features/strategies/EditorStepPage.tsx
git commit -m "feat(strategy): add editor step wizard page"
```

---

### Task 8: Version history side modal

**Files:**
- Create: `web/src/features/strategies/VersionHistoryModal.tsx`
- Create: `web/src/features/strategies/VersionHistoryModal.test.tsx`

**Interfaces:**
- Consumes: `strategyManagerApi.getVersions/getVersion/restoreVersion` (Task 5), `Strategy`, `StrategyConfig`.
- Produces: `<VersionHistoryModal strategy isOpen onClose onVersionNote />` — a right-hand drawer with a version dropdown and detail view (prompt + params) and a Restore button.

- [ ] **Step 1: Write the failing test**

`web/src/features/strategies/VersionHistoryModal.test.tsx`:
```tsx
import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { VersionHistoryModal } from './VersionHistoryModal'
import type { Strategy } from '../../types/strategy'

const strategy: Strategy = {
  id: 's1',
  name: 'Test strategy',
  description: '',
  is_active: true,
  is_default: false,
  is_public: false,
  config_visible: true,
  config: { strategy_type: 'ai_trading', language: 'en' },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

describe('VersionHistoryModal', () => {
  it('shows current config version and a restore button', async () => {
    render(
      <VersionHistoryModal
        strategy={strategy}
        isOpen
        onClose={() => {}}
      />
    )
    await waitFor(() => {
      expect(screen.getByText(/v1/)).toBeInTheDocument()
      expect(screen.getByText(/Restore/)).toBeInTheDocument()
    })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/VersionHistoryModal.test.tsx`
Expected: FAIL (module not found).

- [ ] **Step 3: Write the implementation**

Create `web/src/features/strategies/VersionHistoryModal.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { X, RotateCcw, Loader2 } from 'lucide-react'
import { strategyManagerApi, type StrategyVersion } from './strategyApi'
import { notify } from '../../lib/notify'
import type { Strategy } from '../../types/strategy'

export function VersionHistoryModal({
  strategy,
  isOpen,
  onClose,
}: {
  strategy: Strategy
  isOpen: boolean
  onClose: () => void
}) {
  const [versions, setVersions] = useState<StrategyVersion[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [restoring, setRestoring] = useState(false)

  useEffect(() => {
    if (!isOpen) return
    setLoading(true)
    strategyManagerApi
      .getVersions(strategy.id, strategy.config)
      .then((list) => {
        setVersions(list)
        setSelected(list[0]?.version ?? null)
      })
      .catch(() => notify.error('Failed to load version history'))
      .finally(() => setLoading(false))
  }, [isOpen, strategy.id, strategy.config])

  if (!isOpen) return null

  const current = versions.find((v) => v.version === selected) ?? versions[0]

  const handleRestore = async () => {
    if (!current || restoring) return
    setRestoring(true)
    try {
      const res = await strategyManagerApi.restoreVersion(
        strategy.id,
        current.version
      )
      if (!res.ok) {
        notify.warning('Version restore requires backend support (pending).')
      } else {
        notify.success('Strategy restored')
      }
    } finally {
      setRestoring(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black bg-opacity-40">
      <div className="h-full w-full max-w-md overflow-y-auto border-l border-[rgba(26,24,19,0.14)] bg-nofx-bg-lighter p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-nofx-text">Version History</h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg p-1 text-nofx-text-muted hover:text-nofx-text"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <p className="mb-1 text-sm text-nofx-text">Version</p>
        <select
          value={selected ?? ''}
          onChange={(e) => setSelected(Number(e.target.value))}
          className="mb-4 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-3 py-2 text-sm text-nofx-text"
        >
          {versions.map((v) => (
            <option key={v.version} value={v.version}>
              {v.label} — {v.note}
            </option>
          ))}
        </select>

        {loading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-nofx-gold" />
          </div>
        ) : current ? (
          <div className="space-y-4">
            <section className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
              <div className="text-sm font-semibold text-nofx-text">Prompt</div>
              <pre className="mt-2 whitespace-pre-wrap rounded bg-nofx-bg p-3 text-xs text-nofx-text-muted">
                {current.config.ai_config?.custom_prompt || '(no prompt)'}
              </pre>
            </section>

            <section className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
              <div className="text-sm font-semibold text-nofx-text">Parameters</div>
              <dl className="mt-2 space-y-1 text-xs text-nofx-text-muted">
                <dt className="font-semibold text-nofx-text">Scope</dt>
                <dd>
                  {current.config.ai_config?.coin_source.source_type ??
                    (current.config.coin_source?.source_type ?? '-')}
                </dd>
                <dt className="font-semibold text-nofx-text">Candles</dt>
                <dd>
                  {(
                    current.config.ai_config?.indicators.klines.selected_timeframes ??
                    []
                  ).join(', ') || '-'}
                </dd>
                <dt className="font-semibold text-nofx-text">Risk</dt>
                <dd>
                  {JSON.stringify(
                    current.config.ai_config?.risk_control ?? {}
                  )}
                </dd>
                <dt className="font-semibold text-nofx-text">Decision context</dt>
                <dd>
                  {JSON.stringify(
                    current.config.ai_config?.decision_context ?? {}
                  )}
                </dd>
              </dl>
            </section>

            <button
              type="button"
              onClick={handleRestore}
              disabled={restoring}
              className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-nofx-gold px-4 py-2 text-sm font-semibold text-nofx-bg disabled:opacity-50"
            >
              {restoring ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RotateCcw className="h-4 w-4" />
              )}
              Restore
            </button>
            <p className="text-center text-[11px] text-nofx-text-muted">
              Version history backend is pending; restore is not yet applied.
            </p>
          </div>
        ) : (
          <p className="text-sm text-nofx-text-muted">No versions available.</p>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/strategies/VersionHistoryModal.test.tsx`
Expected: PASS.

- [ ] **Step 5: Run full typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/strategies/VersionHistoryModal.tsx web/src/features/strategies/VersionHistoryModal.test.tsx
git commit -m "feat(strategy): add version history side modal"
```

---

### Task 9: Strategy manager table page

**Files:**
- Create: `web/src/features/strategies/StrategyManagerPage.tsx`

**Interfaces:**
- Consumes: `strategyManagerApi.getStrategies/getStrategyStats` (Task 5); `useNavigate`.
- Produces: the main table page mounted at `/strategy`.

- [ ] **Step 1: Write the implementation**

Create `web/src/features/strategies/StrategyManagerPage.tsx`:
```tsx
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Plus, Pencil, History, Bot } from 'lucide-react'
import { strategyManagerApi, type StrategyStats } from './strategyApi'
import { VersionHistoryModal } from './VersionHistoryModal'
import { notify } from '../../lib/notify'
import type { Strategy } from '../../types/strategy'
import { formatMoney, EquitySparkline } from './tableHelpers'

interface Row {
  strategy: Strategy
  stats: StrategyStats
}

export function StrategyManagerPage() {
  const navigate = useNavigate()
  const [rows, setRows] = useState<Row[]>([])
  const [loading, setLoading] = useState(true)
  const [historyFor, setHistoryFor] = useState<Strategy | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const strategies = await strategyManagerApi.getStrategies()
      const statsResults = await Promise.all(
        strategies.map((s) => strategyManagerApi.getStrategyStats(s.id))
      )
      setRows(
        strategies.map((s, i) => ({ strategy: s, stats: statsResults[i] }))
      )
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to load strategies')
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const useInTrader = (strategyId: string) => {
    navigate(`/traders?strategy=${encodeURIComponent(strategyId)}&open=new`)
  }

  if (loading) {
    return (
      <div className="flex min-h-[70vh] items-center justify-center text-nofx-text-muted">
        Loading strategies...
      </div>
    )
  }

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-nofx-text">Strategies</h1>
        <button
          type="button"
          onClick={() => navigate('/strategy/create/scope')}
          className="inline-flex items-center gap-2 rounded-lg bg-nofx-gold px-4 py-2 text-sm font-semibold text-nofx-bg"
        >
          <Plus className="h-4 w-4" /> Create Strategy
        </button>
      </div>

      <div className="overflow-x-auto rounded-lg border border-[rgba(26,24,19,0.14)]">
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-[rgba(26,24,19,0.14)] text-left text-xs uppercase tracking-wide text-nofx-text-muted">
              <th className="px-3 py-3">#</th>
              <th className="px-3 py-3">Name / Version</th>
              <th className="px-3 py-3">Total AUM</th>
              <th className="px-3 py-3">Trading Symbols</th>
              <th className="px-3 py-3">7D Yield</th>
              <th className="px-3 py-3">Sharpe</th>
              <th className="px-3 py-3">Max DD</th>
              <th className="px-3 py-3">Last Update</th>
              <th className="px-3 py-3">NAV Curve</th>
              <th className="px-3 py-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => {
              const s = row.strategy
              return (
                <tr
                  key={s.id}
                  onClick={() => navigate(`/strategy/${s.id}/edit/scope`)}
                  className="cursor-pointer border-b border-[rgba(26,24,19,0.14)] text-nofx-text hover:bg-nofx-bg-deeper"
                >
                  <td className="px-3 py-3 text-nofx-text-muted">{index + 1}</td>
                  <td className="px-3 py-3">
                    <div className="font-medium">{s.name}</div>
                    <div className="text-xs text-nofx-text-muted">
                      {s.is_active ? 'Active' : 'Inactive'}
                    </div>
                  </td>
                  <td className="px-3 py-3">
                    {formatMoney(row.stats.aum)}
                  </td>
                  <td className="max-w-[180px] truncate px-3 py-3 text-xs">
                    {row.stats.symbols.length
                      ? row.stats.symbols.join(', ')
                      : '—'}
                  </td>
                  <td className="px-3 py-3">
                    {row.stats.sevenDayYield !== null
                      ? `${row.stats.sevenDayYield.toFixed(2)}%`
                      : '—'}
                  </td>
                  <td className="px-3 py-3">
                    {row.stats.sharpe !== null ? row.stats.sharpe.toFixed(2) : '—'}
                  </td>
                  <td className="px-3 py-3">
                    {row.stats.maxDd !== null ? `${row.stats.maxDd.toFixed(2)}%` : '—'}
                  </td>
                  <td className="px-3 py-3 text-xs">
                    {new Date(s.updated_at).toLocaleDateString()}
                  </td>
                  <td className="px-3 py-3">
                    {sparkline(row.stats.navPoints)}
                  </td>
                  <td className="px-3 py-3">
                    <div
                      className="flex gap-1"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <IconBtn title="Version History" onClick={() => setHistoryFor(s)}>
                        <History className="h-4 w-4" />
                      </IconBtn>
                      <IconBtn title="Use in Trader Agent" onClick={() => useInTrader(s.id)}>
                        <Bot className="h-4 w-4" />
                      </IconBtn>
                      <IconBtn title="Edit" onClick={() => navigate(`/strategy/${s.id}/edit/scope`)}>
                        <Pencil className="h-4 w-4" />
                      </IconBtn>
                    </div>
                  </td>
                </tr>
              )
            })}
            {rows.length === 0 && (
              <tr>
                <td colSpan={10} className="px-3 py-10 text-center text-nofx-text-muted">
                  No strategies yet. Click "Create Strategy" to begin.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {historyFor && (
        <VersionHistoryModal
          strategy={historyFor}
          isOpen
          onClose={() => setHistoryFor(null)}
        />
      )}
    </div>
  )
}

function IconBtn({
  title,
  onClick,
  children,
}: {
  title: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      className="rounded-lg border border-[rgba(26,24,19,0.14)] p-1.5 text-nofx-text-muted hover:text-nofx-text"
    >
      {children}
    </button>
  )
}
```

- [ ] **Step 2: Create the presentation helpers**

Create `web/src/features/strategies/tableHelpers.tsx`:
```tsx
export function formatMoney(value: number): string {
  if (!value) return '$0'
  const abs = Math.abs(value)
  if (abs >= 1_000_000_000)
    return `$${(value / 1_000_000_000).toFixed(2)}B`
  if (abs >= 1_000_000) return `$${(value / 1_000_000).toFixed(2)}M`
  if (abs >= 1_000) return `$${(value / 1_000).toFixed(2)}K`
  return `$${value.toFixed(2)}`
}

export function EquitySparkline({
  points,
}: {
  points: { timestamp: string; total_equity: number }[]
}) {
  if (points.length < 2) return <span className="text-nofx-text-muted">—</span>
  const values = points.map((p) => p.total_equity)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const w = 80
  const h = 24
  const coords = values
    .map((v, i) => {
      const x = (i / (values.length - 1)) * w
      const y = h - ((v - min) / span) * h
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="inline-block">
      <polyline
        fill="none"
        stroke="#D4AF37"
        strokeWidth={1.5}
        points={coords}
      />
    </svg>
  )
}
```

Update `StrategyManagerPage.tsx` (from Step 1) so its imports use the real component and the NAV cell renders actual markup:
```tsx
import { formatMoney, EquitySparkline } from './tableHelpers'
```
and replace the NAV-curve cell:
```tsx
<td className="px-3 py-3">
  <EquitySparkline points={row.stats.navPoints} />
</td>
```
Do NOT add a `sparkline` string helper or a `tableHelpers.ts` file — that was a mistaken sketch; ship only the `.tsx` component above.

- [ ] **Step 3: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors. Verify the `JSX` global is unused (removed).

- [ ] **Step 4: Commit**

```bash
git add web/src/features/strategies/StrategyManagerPage.tsx web/src/features/strategies/tableHelpers.tsx
git commit -m "feat(strategy): add strategy manager table page"
```

---

### Task 10: Wire routes + archive the old page

**Files:**
- Modify: `web/src/router/AppRoutes.tsx`
- Move: `web/src/pages/StrategyStudioPage.tsx` → `web/src/pages/legacy/StrategyStudioPage.legacy.tsx`

**Interfaces:**
- Consumes: `StrategyManagerPage`, `ScopeStepPage`, `EditorStepPage` (prior tasks).
- Produces: `/strategy` renders `StrategyManagerPage`; new child routes `/strategy/create/scope`, `/strategy/create/editor`, `/strategy/:id/edit/scope`, `/strategy/:id/edit/editor`.

- [ ] **Step 1: Move the old page out of the router**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
mkdir -p web/src/pages/legacy
git mv web/src/pages/StrategyStudioPage.tsx web/src/pages/legacy/StrategyStudioPage.legacy.tsx
```
Verify no remaining imports reference `StrategyStudioPage`:
Run: `grep -rn "StrategyStudioPage" web/src --include="*.tsx" --include="*.ts"`
Expected: only the moved file (the internal references inside the file itself are fine).

- [ ] **Step 2: Rewrite the strategy routes**

Edit `web/src/router/AppRoutes.tsx`:
- Remove `import { StrategyStudioPage } from '../pages/StrategyStudioPage'`.
- Add:
```tsx
import { StrategyManagerPage } from '../features/strategies/StrategyManagerPage'
import { ScopeStepPage } from '../features/strategies/ScopeStepPage'
import { EditorStepPage } from '../features/strategies/EditorStepPage'
```
- Replace the `ROUTES.strategy` element block (currently renders `<StrategyStudioPage />`) with:
```tsx
<Route
  path={ROUTES.strategy}
  element={
    isAuthenticated ? (
      <AppChrome currentPage="strategy" animateContent>
        <StrategyManagerPage />
      </AppChrome>
    ) : (
      <LandingPage />
    )
  }
/>
<Route
  path={`${ROUTES.strategy}/create/scope`}
  element={
    isAuthenticated ? (
      <AppChrome currentPage="strategy" animateContent>
        <ScopeStepPage />
      </AppChrome>
    ) : (
      <LandingPage />
    )
  }
/>
<Route
  path={`${ROUTES.strategy}/create/editor`}
  element={
    isAuthenticated ? (
      <AppChrome currentPage="strategy" animateContent>
        <EditorStepPage />
      </AppChrome>
    ) : (
      <LandingPage />
    )
  }
/>
<Route
  path={`${ROUTES.strategy}/:id/edit/scope`}
  element={
    isAuthenticated ? (
      <AppChrome currentPage="strategy" animateContent>
        <ScopeStepPage />
      </AppChrome>
    ) : (
      <LandingPage />
    )
  }
/>
<Route
  path={`${ROUTES.strategy}/:id/edit/editor`}
  element={
    isAuthenticated ? (
      <AppChrome currentPage="strategy" animateContent>
        <EditorStepPage />
      </AppChrome>
    ) : (
      <LandingPage />
    )
  }
/>
```

- [ ] **Step 3: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Run the full frontend build**

Run: `cd web && npm run build`
Expected: builds successfully (tsc + vite).

- [ ] **Step 5: Commit**

```bash
git add web/src/router/AppRoutes.tsx
git commit -m "feat(strategy): mount strategy manager and wizard routes, archive old page"
```

---

### Task 11: Use-in-Trader-Agent pre-selection

**Files:**
- Modify: `web/src/components/trader/TraderConfigModal.tsx`
- Modify: `web/src/components/trader/AITradersPage.tsx`

**Interfaces:**
- Consumes: none new.
- Produces:
  - `TraderConfigModal` gains optional prop `preselectedStrategyId?: string`; when set, the strategy `select` default becomes that strategy (falling back to the existing active/first behavior).
  - `AITradersPage` reads `?open=new` + `?strategy=<id>` on mount, opens the create modal with `preselectedStrategyId`, then clears those query params.

- [ ] **Step 1: Add preselectedStrategyId to TraderConfigModal**

In `web/src/components/trader/TraderConfigModal.tsx`:
- Add to `TraderConfigModalProps`:
```ts
preselectedStrategyId?: string
```
- Derive it in the destructure:
```tsx
export function TraderConfigModal({
  isOpen,
  onClose,
  traderData,
  isEditMode = false,
  availableModels = [],
  availableExchanges = [],
  onSave,
  preselectedStrategyId,
}: TraderConfigModalProps) {
```
- In the `fetchStrategies` effect (lines ~109-141), change the strategy default resolution so a preselected id wins:
```ts
if (!formData.strategy_id && !isEditMode) {
  const preselected = preselectedStrategyId
    ? strategyList.find((s) => s.id === preselectedStrategyId)
    : undefined
  const activeStrategy = preselected ?? strategyList.find((s) => s.is_active)
  if (activeStrategy) {
    setFormData((prev) => ({ ...prev, strategy_id: activeStrategy.id }))
  } else if (strategyList.length > 0) {
    setFormData((prev) => ({ ...prev, strategy_id: strategyList[0].id }))
  }
}
```
- Add `preselectedStrategyId` to the effect's dependency array.

- [ ] **Step 2: Handle `open=new` + `strategy` params in AITradersPage**

In `web/src/components/trader/AITradersPage.tsx`:
- Add a state:
```ts
const [preselectedStrategyId, setPreselectedStrategyId] = useState<string | null>(null)
```
- Add a `useEffect` (near the existing `setup` effect) that opens the create modal:
```ts
useEffect(() => {
  if (!user || !token) return
  const openNew = searchParams.get('open')
  if (openNew !== 'new') return
  const strategyId = searchParams.get('strategy')
  setPreselectedStrategyId(strategyId)
  setShowCreateModal(true)
  const nextParams = new URLSearchParams(searchParams)
  nextParams.delete('open')
  nextParams.delete('strategy')
  setSearchParams(nextParams, { replace: true })
}, [searchParams, setSearchParams, token, user])
```
- Pass the prop to the create modal (line ~832):
```tsx
<TraderConfigModal
  isOpen={showCreateModal}
  isEditMode={false}
  availableModels={enabledModels}
  availableExchanges={enabledExchanges}
  onSave={handleCreateTrader}
  onClose={() => setShowCreateModal(false)}
  preselectedStrategyId={preselectedStrategyId ?? undefined}
/>
```

- [ ] **Step 3: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Run full build + lint + tests**

Run:
```bash
cd web && npm run build && npm run lint && npm test
```
Expected: all three pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/trader/TraderConfigModal.tsx web/src/components/trader/AITradersPage.tsx
git commit -m "feat(strategy): pre-select strategy when launching a trader from a strategy"
```

---

### Task 12: Final verification pass

**Files:** none (verification only).

- [ ] **Step 1: Run the full check suite**

Run:
```bash
cd web && npm run build && npm run lint && npm test
```
Expected: all pass, lint with `--max-warnings 0`.

- [ ] **Step 2: Manual smoke test of wiring**

Run the dev server:
```bash
cd web && npm run dev
```
- Confirm `/strategy` shows the new table (empty state when no strategies).
- Create a strategy: `/strategy/create/scope` → select "Crypto Top Gainers" (free) → Next → editor → fill name + Save → returns to table, row appears.
- Edit: click the row → opens `/strategy/<id>/edit/scope` with scope preserved → Next → editor prefilled → Save.
- Version History: opens the side modal, shows v1 dropdown + detail + Restore (with backend-pending note).
- Delete/duplicate actions are backend-provided; the actions are wired via `strategyManagerApi`.

- [ ] **Step 3: Note any follow-up items**

If anything in Step 2 doesn't match, fix and re-run Step 1 before finishing.

- [ ] **Step 4: Final commit (if any changes)**

```bash
git add -A
git commit -m "fix(strategy): address verification findings"
```
(Only commit if Step 2 produced changes.)

---

## New backend work documented for later (not in this plan)

- Strategy version/snapshot table + list/get/restore endpoints (`getVersions`/`getVersion`/`restoreVersion` currently return placeholder local data).
- Strategy-level aggregate stats for 7D Yield / Sharpe / Max DD (`sevenDayYield`/`sharpe`/`maxDd` currently return `null`; the table shows `—`).
- `custom` multi-scope AND/OR candidate resolver using `custom_scope.scope_units` + `scope_mode`.
- `decision_context` prompt-builder wiring (currently persisted but unused at runtime).
- Paid scope data providers (VergeX scrape / nofxos.ai).
