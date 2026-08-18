# Strategy Backend Tasks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the multi-scope AND/OR concept from the Strategy Manager (single-scope only), wire `decision_context` into the prompt builder, and add strategy version/snapshot + aggregate-stats endpoints.

**Architecture:** Frontend single-scope refactor (types → factory → draft store → wizard pages → tests) follows a strict single-scope data flow. Backend adds a persisted `DecisionContextConfig` threaded into both prompt builders, a new `strategy_versions` table + endpoints, and a `GET /strategies/:id/stats` endpoint that merges linked traders' equity curves. Frontend stale "backend pending" warnings are removed where the new backend makes them obsolete.

**Tech Stack:** Go 1.25 (Gin, GORM, zerolog), React 18 + TypeScript + Vite + Vitest + Zustand.

## Global Constraints

- Backend verification: `go build ./... && go vet ./... && go test ./...`
- Frontend verification: `cd web && npx tsc --noEmit && npm run build && npm test`
- `tsconfig.json` **excludes** `src/**/*.test.ts(x)` and `src/test` from `tsc`. Test files are verified by `npm test` (vitest) and the build, NOT by `tsc`.
- English-only UI strings. Backend errors use `SafeError` / `SafeInternalError` / `SanitizeError` (never leak internals).
- `store.*` is the only DB access layer. All timestamps UTC.
- HIGH-RISK: `POST /strategies/:id/restore` overwrites a strategy config → blocked while a running trader uses the strategy.
- Backend `store.CoinSourceConfig` is intentionally **unchanged** (Task 1 is frontend-only).
- Scope cards keep the `FREE`/`PAID` badge; only the "Data source not configured / backend pending" warning text is removed.

---

## File Structure

**Task 1 (frontend single-scope):**
- Modify: `web/src/types/strategy.ts`
- Modify: `web/src/features/strategies/strategyFactory.ts`
- Modify: `web/src/features/strategies/draftStore.ts`
- Modify: `web/src/features/strategies/ScopeStepPage.tsx`
- Modify: `web/src/features/strategies/EditorStepPage.tsx`
- Modify: `web/src/features/strategies/dataSourceDefaults.ts`
- Test: `web/src/features/strategies/strategyFactory.test.ts`
- Test: `web/src/features/strategies/strategyFactoryPresets.test.ts`
- Test: `web/src/features/strategies/scopeCatalog.test.ts` (unchanged)

**Task 2 (decision_context backend):**
- Modify: `store/strategy.go` (`AIStrategyConfig`, `MarshalJSON`/`UnmarshalJSON`)
- Modify: `kernel/engine.go` (`StrategyEngine` struct + `SetRecentDecisions`)
- Modify: `kernel/engine_prompt.go` (recent-decisions rendering in both prompt builders)
- Modify: `trader/auto_trader_loop.go` (`buildTradingContext`)
- Test: `store/strategy_decision_context_test.go` (new)
- Test: `kernel/engine_prompt_test.go` (new cases)

**Task 3 (decision_context frontend cleanup):**
- Modify: `web/src/features/strategies/EditorStepPage.tsx` (remove warning text)

**Task 4 (versions):**
- Create: `store/strategy_version.go`
- Modify: `store/store.go` (register sub-store)
- Modify: `store/strategy.go` (snapshot hooks in `Create`/`Update`/`Duplicate`/`Delete`)
- Modify: `api/strategy.go` (version + restore handlers)
- Modify: `api/server.go` (register routes)
- Modify: `web/src/features/strategies/strategyApi.ts` (real calls)
- Modify: `web/src/features/strategies/VersionHistoryModal.tsx`
- Test: `store/strategy_version_test.go` (new)
- Test: `api/strategy_version_test.go` (new)

**Task 5 (stats):**
- Modify: `api/strategy.go` (stats handler + merged-curve helper)
- Modify: `api/server.go` (register route)
- Modify: `web/src/features/strategies/strategyApi.ts` (single endpoint call)
- Test: `api/strategy_stats_test.go` (new)

---

### Task 1: Remove multi-scope from frontend types and factory

**Files:**
- Modify: `web/src/types/strategy.ts:109-150,258-300`
- Modify: `web/src/features/strategies/strategyFactory.ts:9-22,73-208,266-305`
- Test: `web/src/features/strategies/strategyFactory.test.ts`

**Interfaces:**
- Consumes: existing `ScopeUnit`, `ScopeVariant`, `CoinSourceConfig` types.
- Produces:
  - `CoinSourceConfig.source_type` union no longer contains `'custom'`; no `scope_mode` or `custom_scope` fields.
  - `CustomScopeConfig` interface removed.
  - `buildCoinSource(unit: ScopeUnit): CoinSourceConfig` (single-argument).
  - `StrategyEditorForm.scopeUnits` → `scopeUnit: ScopeUnit | null`; `scopeMode` removed.
  - `defaultRiskControl` unchanged.

- [ ] **Step 1: Remove `custom` / `scope_mode` / `custom_scope` / `CustomScopeConfig` from types**

In `web/src/types/strategy.ts`:
- From the `CoinSourceConfig.source_type` union (line ~110-123), remove the `| 'custom'` entry.
- Remove `custom_scope?: CustomScopeConfig` (line 124) and `scope_mode?: 'overlap' | 'union'` (line 125).
- Remove the `CustomScopeConfig` interface (lines ~288-292).
- Keep `ScopeUnit` (line 266) and `ScopeVariant` (line 259).

- [ ] **Step 2: Update `buildCoinSource` to single-unit**

In `web/src/features/strategies/strategyFactory.ts`, change:
```ts
export function buildCoinSource(
  units: ScopeUnit[],
  mode: 'overlap' | 'union' = 'union'
): CoinSourceConfig {
```
to:
```ts
export function buildCoinSource(unit: ScopeUnit | null): CoinSourceConfig {
  if (!unit) {
    return {
      source_type: 'static',
      static_coins: [],
      excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }
  const mode: 'overlap' | 'union' = 'union' // retained internally only for structural parity; not emitted
```
Then in each single-source block, remove `scope_mode: mode,` and replace `units[0]` references with `unit`. Remove the `if (units.length > 1)` custom branch (lines ~77-95) and the fallback `custom` branch (lines ~190-207).

Concretely replace each `unit?.` with `unit.` (e.g. `unit?.source_type === 'hyper_rank'` → `unit.source_type === 'hyper_rank'`).

- [ ] **Step 3: Update `StrategyEditorForm`**

In `web/src/features/strategies/strategyFactory.ts`, change the form fields:
```ts
  scopeUnits: ScopeUnit[]
  scopeMode: 'overlap' | 'union'
```
to:
```ts
  scopeUnit: ScopeUnit | null
```

- [ ] **Step 4: Update `buildStrategyConfig` call site**

In `web/src/features/strategies/strategyFactory.ts` (line ~272):
```ts
coin_source: buildCoinSource(form.scopeUnits, form.scopeMode),
```
to:
```ts
coin_source: buildCoinSource(form.scopeUnit),
```

- [ ] **Step 5: Rewrite `strategyFactory.test.ts`**

Replace the multi-scope test cases (`uses custom source_type when more than one scope is selected`, `persists overlap mode on a multi-scope custom source`, `persists overlap mode on a single concrete scope`, `buildStrategyConfig passes scopeMode into the coin source for a multi-scope form`) with single-scope assertions.

Keep `freeUnit` helper. Update its usages and add:

```ts
import type { ScopeUnit } from '../../types/strategy'

it('maps a single free scope to its concrete coin source without scope_mode/custom_scope', () => {
  const cs = buildCoinSource(freeUnit('gainers'))
  expect(cs.source_type).toBe('hyper_rank')
  expect(cs.hyper_rank_category).toBe('crypto')
  expect(cs.hyper_rank_direction).toBe('gainers')
  expect(cs.hyper_rank_limit).toBe(10)
  expect('scope_mode' in cs).toBe(false)
  expect('custom_scope' in cs).toBe(false)
})

it('maps a single paid scope to vergex_signal', () => {
  const paid: ScopeUnit = {
    id: 'crypto-bias-bull', category: 'crypto', source_type: 'vergex',
    limit: 10, label: 'Bias Radar (Bullish)', provider: 'paid',
  }
  const cs = buildCoinSource(paid)
  expect(cs.source_type).toBe('vergex_signal')
  expect('custom_scope' in cs).toBe(false)
})

it('falls back to an empty static pool when no scope is selected', () => {
  const cs = buildCoinSource(null)
  expect(cs.source_type).toBe('static')
  expect(cs.static_coins).toEqual([])
})
```

Update the `buildStrategyConfig` tests to pass `scopeUnit: freeUnit('gainers')` instead of `scopeUnits: [...]` / `scopeMode`.

- [ ] **Step 6: Update `strategyFactoryPresets.test.ts`**

In `web/src/features/strategies/strategyFactoryPresets.test.ts`, change:
```ts
  decisionContext: { mode: 'structured', count: 10 },
  scopeUnits: [], scopeMode: 'union',
```
to:
```ts
  decisionContext: { enabled: false, recent_count: 8, mode: 'structured' },
  scopeUnit: null,
```
(Note: the pre-existing `count` was a latent type error masked by tsconfig excluding test files; correct it to `recent_count` and add `enabled`.)

- [ ] **Step 7: Run frontend tests and typecheck**

Run: `cd web && npx tsc --noEmit && npm test -- strategyFactory`
Expected: PASS; tsc exits 0.

- [ ] **Step 8: Commit**

```bash
git add web/src/types/strategy.ts web/src/features/strategies/strategyFactory.ts web/src/features/strategies/strategyFactory.test.ts web/src/features/strategies/strategyFactoryPresets.test.ts
git commit -m "feat(strategy): single-scope coin source (remove custom/multi-scope)"
```

---

### Task 2: Update draft store and scope wizard to single-select

**Files:**
- Modify: `web/src/features/strategies/draftStore.ts`
- Modify: `web/src/features/strategies/ScopeStepPage.tsx`
- Modify: `web/src/features/strategies/dataSourceDefaults.ts`

**Interfaces:**
- Consumes: `buildCoinSource`/`StrategyEditorForm` from Task 1; `ScopeUnit`, `ScopeCardDef` from `scopeCatalog.ts`.
- Produces:
  - `useStrategyDraft` state: `{ scope: ScopeUnit | null }`.
  - `setScope(unit: ScopeUnit): void`, `clearScope(): void`.
  - `defaultDataSources(scope: ScopeUnit | null): { enableAI500Data, enableOIData, enableNetflowData, enablePriceData: boolean }`.

- [ ] **Step 1: Rewrite `draftStore.ts`**

Replace the whole file body with:
```ts
import { create } from 'zustand'
import type { ScopeUnit } from '../../types/strategy'

interface StrategyDraft {
  scope: ScopeUnit | null
  setScope: (unit: ScopeUnit) => void
  clearScope: () => void
  resetDraft: () => void
}

export const useStrategyDraft = create<StrategyDraft>((set) => ({
  scope: null,
  setScope: (unit) => set({ scope: unit }),
  clearScope: () => set({ scope: null }),
  resetDraft: () => set({ scope: null }),
}))

export function resetStrategyDraft() {
  useStrategyDraft.getState().resetDraft()
}
```

- [ ] **Step 2: Update `dataSourceDefaults.ts`**

Replace the whole file body with:
```ts
import type { ScopeUnit } from '../../types/strategy'

export function defaultDataSources(scope: ScopeUnit | null): {
  enableAI500Data: boolean
  enableOIData: boolean
  enableNetflowData: boolean
  enablePriceData: boolean
} {
  return {
    enableAI500Data: scope?.source_type === 'ai500',
    enableOIData: scope?.source_type === 'nofxos_oi',
    enableNetflowData: scope?.source_type === 'nofxos_netflow',
    enablePriceData: scope?.source_type === 'nofxos_price',
  }
}
```

- [ ] **Step 3: Update `ScopeStepPage.tsx` imports and draft access**

At the top of `web/src/features/strategies/ScopeStepPage.tsx`:
- Remove `mergeScopeUnit, removeScopeUnit, setScopeMode, setScopeUnits` from the `useStrategyDraft()` destructure; keep `scope` and add `setScope`, `clearScope`.

Change:
```ts
  const {
    scope,
    mergeScopeUnit,
    removeScopeUnit,
    setScopeMode,
    setScopeUnits,
  } = useStrategyDraft()
```
to:
```ts
  const { scope, setScope, clearScope } = useStrategyDraft()
```

- [ ] **Step 4: Simplify `cardActive` and `cardUnit`**

Keep `cardActive` (checks `units.some(...)`). Since `scope` is now a single `ScopeUnit | null`, rewrite helpers:
```ts
function cardActive(scope: ScopeUnit | null, def: ScopeCardDef): boolean {
  return scope?.id === def.id
}

function cardUnit(def: ScopeCardDef, limit: number): ScopeUnit {
  return toScopeUnit(def, limit)
}
```

- [ ] **Step 5: Rewrite the edit prefill effect**

Replace the body that handles `custom_scope` seeding. Remove the `custom_scope` branch. Keep the `matchConcreteScope` path:
```ts
        const unit = matchConcreteScope(coinSource)
        if (unit) {
          setScope(unit)
          setCategory(unit.category)
          setTopN({ [unit.id]: unit.limit })
        }
```

- [ ] **Step 6: Rewrite `toggleCard` / `updateLimit` to single-select**

Change:
```ts
  const toggleCard = (def: ScopeCardDef) => {
    if (cardActive(scope, def)) {
      clearScope()
    } else {
      setScope(cardUnit(def, topN[def.id] ?? def.defaultLimit))
    }
  }

  const updateLimit = (def: ScopeCardDef, raw: number) => {
    const limit = Math.min(50, Math.max(1, raw || 1))
    setTopN((prev) => ({ ...prev, [def.id]: limit }))
    if (cardActive(scope, def)) {
      setScope(cardUnit(def, limit))
    }
  }
```
Update `cardActive(scope.units, def)` call sites to `cardActive(scope, def)` (lines ~239).

- [ ] **Step 7: Remove the Union/Overlap toggle**

Delete the header button block (lines ~209-217) that calls `setScopeMode` and renders the `Union`/`Overlap` label. Remove the now-unused `scope.mode` references.

- [ ] **Step 8: Remove the "Data source not configured / backend pending" warning**

Replace the block at lines ~287-292:
```tsx
              {def.provider === 'paid' && (
                <div className="mt-2 flex items-center gap-1.5 text-[11px] text-nofx-danger">
                  <ShieldAlert className="h-3 w-3" />
                  Data source not configured / backend pending
                </div>
              )}
```
with nothing (delete the block). Keep the `FREE`/`PAID` badge at lines ~263-270.

- [ ] **Step 9: Fix the "Next" guard**

Change `disabled={scope.units.length === 0}` to `disabled={scope === null}`.

- [ ] **Step 10: Update `EditorStepPage.tsx` scope references**

In `web/src/features/strategies/EditorStepPage.tsx`:
- Line ~72: `defaultDataSources(scope.units)` → `defaultDataSources(scope)`.
- Line ~151: `defaultDataSources(scope.units)` → `defaultDataSources(scope)`.
- Line ~175: dependency `[mode, strategyId, token, scope.units]` → `[mode, strategyId, token, scope]`.
- Line ~220: `scope.units.length === 0` → `scope === null`.
- Lines ~272-279: `scopeUnits: scope.units, scopeMode: scope.mode` → `scopeUnit: scope`, and description `Strategy using ${scope.units.length} scope(s)` → `Strategy using ${scope ? 1 : 0} scope(s)`.

- [ ] **Step 11: Run frontend typecheck, build, and tests**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: all pass.

- [ ] **Step 12: Commit**

```bash
git add web/src/features/strategies/draftStore.ts web/src/features/strategies/ScopeStepPage.tsx web/src/features/strategies/dataSourceDefaults.ts web/src/features/strategies/EditorStepPage.tsx
git commit -m "feat(strategy): single-select scope wizard, drop overlap/union + stale warning"
```

---

### Task 3: Remove decision-context "backend pending" warning (frontend)

**Files:**
- Modify: `web/src/features/strategies/EditorStepPage.tsx:684-686`

**Interfaces:**
- Consumes: nothing new.
- Produces: none (pure UI removal).

- [ ] **Step 1: Remove the warning text**

In `web/src/features/strategies/EditorStepPage.tsx`, delete the block:
```tsx
            <p className="mt-2 text-xs text-nofx-danger">
              Runtime prompt wiring is pending backend work.
            </p>
```
Leave the surrounding decision-context `fieldset` and its controls intact.

- [ ] **Step 2: Run frontend build + tests**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/strategies/EditorStepPage.tsx
git commit -m "fix(strategy): drop stale 'runtime prompt wiring pending' warning"
```

---

### Task 4: Add `DecisionContextConfig` to backend strategy config

**Files:**
- Modify: `store/strategy.go` (around `AIStrategyConfig` at 775-782, and `MarshalJSON` 792-823 / `UnmarshalJSON` 827-880)
- Test: `store/strategy_decision_context_test.go` (new)

**Interfaces:**
- Consumes: existing `StrategyConfig` JSON schema.
- Produces:
  - `type DecisionContextConfig struct { Enabled bool; RecentCount int; Mode string }` with JSON tags `enabled`, `recent_count`, `mode`.
  - `AIStrategyConfig.DecisionContext *DecisionContextConfig` JSON tag `decision_context,omitempty`.
  - `StrategyConfig` round-trips `ai_config.decision_context` through `MarshalJSON`/`UnmarshalJSON`.

- [ ] **Step 1: Write the failing test**

Create `store/strategy_decision_context_test.go`:
```go
package store

import (
	"encoding/json"
	"testing"
)

func TestStrategyConfigDecisionContextRoundTrip(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	cfg.AIConfig = &AIStrategyConfig{
		CoinSource:     cfg.CoinSource,
		Indicators:     cfg.Indicators,
		RiskControl:    cfg.RiskControl,
		DecisionContext: &DecisionContextConfig{
			Enabled:     true,
			RecentCount: 8,
			Mode:        "structured",
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out StrategyConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.AIConfig == nil || out.AIConfig.DecisionContext == nil {
		t.Fatalf("decision_context not round-tripped: %s", string(data))
	}
	dc := out.AIConfig.DecisionContext
	if !dc.Enabled || dc.RecentCount != 8 || dc.Mode != "structured" {
		t.Fatalf("decision_context = %+v", dc)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./store/ -run TestStrategyConfigDecisionContextRoundTrip -v`
Expected: FAIL (no `DecisionContext` field / marshal drops it).

- [ ] **Step 3: Add the type and wire it through marshal/unmarshal**

In `store/strategy.go`:
- Add the `DecisionContextConfig` struct near `AIStrategyConfig`.
- Add field to `AIStrategyConfig`:
```go
type AIStrategyConfig struct {
	CoinSource     CoinSourceConfig     `json:"coin_source"`
	Indicators     IndicatorConfig      `json:"indicators"`
	CustomPrompt   string               `json:"custom_prompt,omitempty"`
	RiskControl    RiskControlConfig    `json:"risk_control"`
	PromptSections PromptSectionsConfig `json:"prompt_sections,omitempty"`
	DecisionContext *DecisionContextConfig `json:"decision_context,omitempty"`
}
```
- In `MarshalJSON`, add `DecisionContext: c.DecisionContext` to the `AIStrategyConfig` literal (line ~813).
- In `UnmarshalJSON`: add `DecisionContext *DecisionContextConfig \`json:"decision_context"\`` to the `rawStrategyConfig` struct; after unmarshal, set `c.DecisionContext = raw.DecisionContext` in both the `raw.AIConfig != nil` branch and the flat branch.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./store/ -run TestStrategyConfigDecisionContextRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Run full backend checks**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add store/strategy.go store/strategy_decision_context_test.go
git commit -m "feat(strategy): persist decision_context in strategy config"
```

---

### Task 5: Fetch per-trader recent decisions and render into both system prompts

**Files:**
- Modify: `kernel/engine.go` (struct + setter)
- Modify: `kernel/engine_prompt.go` (render helper + both prompt builders)
- Modify: `trader/auto_trader_loop.go` (fetch + set)
- Test: `kernel/engine_prompt_test.go` (new cases)

**Interfaces:**
- Consumes: `store.DecisionContextConfig`, `store.DecisionRecord` (from `store.DecisionStore.GetLatestRecords`), `store.DecisionStore.GetLatestRecords(traderID string, n int) ([]*DecisionRecord, error)`.
- Produces:
  - `func (e *StrategyEngine) SetRecentDecisions(records []*store.DecisionRecord)`.
  - `StrategyEngine.recentDecisions []*store.DecisionRecord` field.
  - `func renderRecentDecisions(ctx *DecisionContextConfig, records []*DecisionRecord) string` in `kernel/engine_prompt.go`.

- [ ] **Step 1: Add the engine field + setter**

In `kernel/engine.go`, add a field to `StrategyEngine`:
```go
	recentDecisions []*store.DecisionRecord // prior-cycle assistant responses (per-trader)
```
Add a method near `NewStrategyEngine`:
```go
// SetRecentDecisions sets prior-cycle decision records (this trader only) so
// the prompt builder can feed them into the system prompt when configured.
func (e *StrategyEngine) SetRecentDecisions(records []*store.DecisionRecord) {
	e.recentDecisions = records
}
```

- [ ] **Step 2: Write the failing rendering test**

Add to `kernel/engine_prompt_test.go`:
```go
func TestRenderRecentDecisionsStructured(t *testing.T) {
	cfg := store.DecisionContextConfig{Enabled: true, RecentCount: 2, Mode: "structured"}
	records := []*store.DecisionRecord{
		{Timestamp: time.Now().Add(-2 * time.Hour), RawResponse: "decision A"},
		{Timestamp: time.Now().Add(-1 * time.Hour), RawResponse: "decision B"},
	}
	out := renderRecentDecisions(&cfg, records)
	if out == "" {
		t.Fatal("expected non-empty recent-decisions section")
	}
	if !strings.Contains(out, "decision A") || !strings.Contains(out, "decision B") {
		t.Fatalf("missing prior responses: %q", out)
	}
}

func TestRenderRecentDecisionsDigestAndDisabled(t *testing.T) {
	disabled := store.DecisionContextConfig{Enabled: false, RecentCount: 2, Mode: "structured"}
	if out := renderRecentDecisions(&disabled, nil); out != "" {
		t.Fatalf("expected empty when disabled, got %q", out)
	}
	digest := store.DecisionContextConfig{Enabled: true, RecentCount: 1, Mode: "digest"}
	records := []*store.DecisionRecord{{Timestamp: time.Now(), RawResponse: "a very long response " + strings.Repeat("x", 200)}}
	out := renderRecentDecisions(&digest, records)
	if !strings.Contains(out, "a very long response") {
		t.Fatalf("digest missing snippet: %q", out)
	}
}
```
Add needed imports (`time`, `strings`) if not already present in the test file.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./kernel/ -run 'TestRenderRecentDecisions' -v`
Expected: FAIL (undefined function `renderRecentDecisions`).

- [ ] **Step 4: Implement the render helper**

In `kernel/engine_prompt.go`, add:
```go
// renderRecentDecisions formats prior-cycle assistant responses for the system
// prompt. Only the raw assistant response (RawResponse) is included — parsed
// decisions are intentionally omitted so the LLM does not re-call past actions.
// Structured = one delimited, timestamped entry per cycle; digest = a single
// truncated snippet per cycle. Returns "" when disabled or no records.
func renderRecentDecisions(cfg *store.DecisionContextConfig, records []*store.DecisionRecord) string {
	if cfg == nil || !cfg.Enabled || len(records) == 0 {
		return ""
	}
	n := cfg.RecentCount
	if n <= 0 {
		n = len(records)
	}
	if n > len(records) {
		n = len(records)
	}
	var sb strings.Builder
	sb.WriteString("# Recent Decisions\n\n")
	sb.WriteString("These are the assistant responses from previous cycles of THIS trader. Use them for continuity; do NOT re-execute the same actions. Treat them as context only.\n\n")
	if cfg.Mode == "digest" {
		for _, r := range records[len(records)-n:] {
			snippet := r.RawResponse
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			snippet = strings.ReplaceAll(snippet, "\n", " ")
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", r.Timestamp.UTC().Format("2006-01-02 15:04:05"), snippet))
		}
	} else {
		for _, r := range records[len(records)-n:] {
			sb.WriteString(fmt.Sprintf("### Cycle %s\n\n%s\n\n", r.Timestamp.UTC().Format("2006-01-02 15:04:05"), r.RawResponse))
		}
	}
	sb.WriteString("\n---\n\n")
	return sb.String()
}
```
Ensure `fmt` is imported in `engine_prompt.go`.

- [ ] **Step 4b: Add a nil-safe decision-context accessor**

`store.StrategyConfig.AIConfig` is a `*AIStrategyConfig` and can be nil for flat legacy configs. Add a helper on the engine to read the decision context safely. In `kernel/engine.go`:
```go
// DecisionContextConfig returns the strategy's decision_context config, or nil.
func (e *StrategyEngine) DecisionContextConfig() *store.DecisionContextConfig {
	if e == nil || e.config == nil || e.config.AIConfig == nil {
		return nil
	}
	return e.config.AIConfig.DecisionContext
}
```

- [ ] **Step 5: Render into the generic prompt builder**

In `StrategyEngine.BuildSystemPrompt` (line ~38), after computing `legacyZhConfig`, before the vergex branch:
```go
	if recent := renderRecentDecisions(e.DecisionContextConfig(), e.recentDecisions); recent != "" {
		sb.WriteString(recent)
	}
```
Move the `sb` declaration above the vergex branch so both paths can use it (currently `var sb strings.Builder` is at line 20). Insert the recent-decisions section early so it applies to both branches — but `buildVergexSystemPrompt` is a separate function and doesn't receive `sb`. Therefore:
- For the **generic** path, write the recent section at the top of `sb` (before `GetSchemaPrompt`).
- For the **Claw402/vergex** path, pass the recent section into `buildVergexSystemPrompt` via a new parameter `recent string`, and write it near the top of its `sb` (after `writeVergexSchemaPrompt`).

- [ ] **Step 6: Render into the vergex prompt builder**

Change `buildVergexSystemPrompt` signature to accept a `recent string` parameter:
```go
func (e *StrategyEngine) buildVergexSystemPrompt(accountEquity float64, variant string, lang Language, zh bool, singleSymbol bool, primarySymbol string, recent string) string {
```
At the top of the function body (after `writeVergexSchemaPrompt`), if `recent != ""`, `sb.WriteString(recent)`.

Update the call site in `BuildSystemPrompt`:
```go
	if e.usesVergexSignalPrompt() {
		return e.buildVergexSystemPrompt(accountEquity, variant, lang, zh, singleSymbol, primarySymbol, renderRecentDecisions(e.DecisionContextConfig(), e.recentDecisions))
	}
```

- [ ] **Step 7: Fetch recent decisions in the trader loop**

In `trader/auto_trader_loop.go` `buildTradingContext` (line 460), after the engine is known to exist and before returning the context, add:
```go
	// Feed prior-cycle decisions (this trader only) into the prompt builder when
	// the strategy enables decision_context.
	if at.strategyEngine != nil && at.store != nil {
		if dc := at.strategyEngine.DecisionContextConfig(); dc != nil && dc.Enabled {
			recentCount := dc.RecentCount
			if recentCount <= 0 {
				recentCount = 8
			}
			records, err := at.store.Decision().GetLatestRecords(at.id, recentCount)
			if err != nil {
				at.logWarnf("⚠️ Failed to load recent decisions for decision_context: %v", err)
			} else {
				at.strategyEngine.SetRecentDecisions(records)
			}
		}
	}
```
Note: ensure `AIConfig` is non-nil before dereferencing `at.strategyEngine.GetConfig().AIConfig.DecisionContext` — guard with `cfg := at.strategyEngine.GetConfig(); if cfg.AIConfig != nil && cfg.AIConfig.DecisionContext != nil`.

- [ ] **Step 8: Run tests and build**

Run: `go build ./... && go vet ./... && go test ./kernel/ ./store/ ./trader/`
Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add kernel/engine.go kernel/engine_prompt.go kernel/engine_prompt_test.go trader/auto_trader_loop.go
git commit -m "feat(kernel): feed recent per-trader decisions into system prompts via decision_context"
```

---

### Task 6: Create `strategy_versions` table and store

**Files:**
- Create: `store/strategy_version.go`
- Modify: `store/store.go` (lazy getter + `initTables`)
- Test: `store/strategy_version_test.go` (new)

**Interfaces:**
- Consumes: `store.Strategy` (`.ID`, `.UserID`, `.Config`), GORM `*gorm.DB` via `store.Store`.
- Produces:
  - `type StrategyVersion struct { ID int64; StrategyID string; UserID string; Version int; Config string; Note string; IsCurrent bool; CreatedAt time.Time }` (table `strategy_versions`).
  - `func NewStrategyVersionStore(db *gorm.DB) *StrategyVersionStore`.
  - Methods on `*StrategyVersionStore`:
    - `List(strategyID, userID string) ([]*StrategyVersion, error)`
    - `Get(strategyID, userID string, version int) (*StrategyVersion, error)`
    - `CreateSnapshot(strategyID, userID, config, note string) (*StrategyVersion, error)` (returns the created version)
    - `SetCurrent(strategyID, userID string, version int) error`
    - `DeleteForStrategy(strategyID, userID string) error`
  - `func (s *Store) StrategyVersion() *StrategyVersionStore`.

- [ ] **Step 1: Write the failing test**

Create `store/strategy_version_test.go` (mirrors the in-memory sqlite pattern in `store/position_test.go`):
```go
package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newStrategyVersionTestStore(t *testing.T) *StrategyVersionStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	s := NewStrategyVersionStore(db)
	if err := s.initTables(); err != nil {
		t.Fatalf("init strategy_versions table: %v", err)
	}
	return s
}

func TestStrategyVersionStoreLifecycle(t *testing.T) {
	s := newStrategyVersionTestStore(t)

	v1, err := s.CreateSnapshot("strat-1", "user-1", `{"a":1}`, "v1")
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if v1.Version != 1 {
		t.Fatalf("first version = %d, want 1", v1.Version)
	}
	if err := s.SetCurrent("strat-1", "user-1", v1.Version); err != nil {
		t.Fatalf("set current v1: %v", err)
	}

	v2, err := s.CreateSnapshot("strat-1", "user-1", `{"a":2}`, "Snapshot before edit")
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("second version = %d, want 2", v2.Version)
	}

	list, err := s.List("strat-1", "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2", len(list))
	}

	got, err := s.Get("strat-1", "user-1", 1)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if got.Config != `{"a":1}` {
		t.Fatalf("v1 config = %q", got.Config)
	}

	if err := s.DeleteForStrategy("strat-1", "user-1"); err != nil {
		t.Fatalf("delete for strategy: %v", err)
	}
	if remaining, err := s.List("strat-1", "user-1"); err != nil || len(remaining) != 0 {
		t.Fatalf("expected no versions after delete, got %d (err=%v)", len(remaining), err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./store/ -run TestStrategyVersionStoreLifecycle -v`
Expected: FAIL (type `StrategyVersion` undefined).

- [ ] **Step 3: Create `store/strategy_version.go`**

```go
package store

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// StrategyVersion a point-in-time snapshot of a strategy config.
type StrategyVersion struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID string    `gorm:"column:strategy_id;not null;index:idx_strategy_versions_strategy" json:"strategy_id"`
	UserID     string    `gorm:"column:user_id;not null;index" json:"-"`
	Version    int       `gorm:"column:version;not null" json:"version"`
	Config     string    `gorm:"column:config;not null" json:"config"`
	Note       string    `gorm:"column:note;default:''" json:"note"`
	IsCurrent  bool      `gorm:"column:is_current;default:false" json:"is_current"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (StrategyVersion) TableName() string { return "strategy_versions" }

type StrategyVersionStore struct {
	db *gorm.DB
}

func NewStrategyVersionStore(db *gorm.DB) *StrategyVersionStore {
	return &StrategyVersionStore{db: db}
}

func (s *StrategyVersionStore) initTables() error {
	if s.db.Dialector.Name() == "postgres" {
		var exists int64
		s.db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'strategy_versions'`).Scan(&exists)
		if exists > 0 {
			return nil
		}
	}
	return s.db.AutoMigrate(&StrategyVersion{})
}

func (s *StrategyVersionStore) List(strategyID, userID string) ([]*StrategyVersion, error) {
	var versions []*StrategyVersion
	if err := s.db.Where("strategy_id = ? AND user_id = ?", strategyID, userID).
		Order("version ASC").Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("failed to list strategy versions: %w", err)
	}
	return versions, nil
}

func (s *StrategyVersionStore) Get(strategyID, userID string, version int) (*StrategyVersion, error) {
	var v StrategyVersion
	if err := s.db.Where("strategy_id = ? AND user_id = ? AND version = ?", strategyID, userID, version).
		First(&v).Error; err != nil {
		return nil, fmt.Errorf("failed to get strategy version: %w", err)
	}
	return &v, nil
}

func (s *StrategyVersionStore) CreateSnapshot(strategyID, userID, config, note string) (*StrategyVersion, error) {
	var maxVersion *int
	s.db.Model(&StrategyVersion{}).Where("strategy_id = ? AND user_id = ?", strategyID, userID).
		Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)
	next := 1
	if maxVersion != nil {
		next = *maxVersion + 1
	}
	v := &StrategyVersion{
		StrategyID: strategyID,
		UserID:     userID,
		Version:    next,
		Config:     config,
		Note:       note,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, fmt.Errorf("failed to create strategy version: %w", err)
	}
	return v, nil
}

func (s *StrategyVersionStore) SetCurrent(strategyID, userID string, version int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&StrategyVersion{}).Where("strategy_id = ? AND user_id = ?", strategyID, userID).
			Update("is_current", false).Error; err != nil {
			return err
		}
		return tx.Model(&StrategyVersion{}).Where("strategy_id = ? AND user_id = ? AND version = ?", strategyID, userID, version).
			Update("is_current", true).Error
	})
}

func (s *StrategyVersionStore) DeleteForStrategy(strategyID, userID string) error {
	return s.db.Where("strategy_id = ? AND user_id = ?", strategyID, userID).
		Delete(&StrategyVersion{}).Error
}
```

- [ ] **Step 4: Register the sub-store in `store/store.go`**

In `store/store.go`:
- Add a `strategyVersion *StrategyVersionStore` field to the `Store` struct (if the struct uses lazy fields; mirror how `Strategy()` is stored).
- Add `if err := s.StrategyVersion().initTables(); err != nil { return err }` in `initTables` (line ~149, near the Strategy init).
- Add the lazy getter:
```go
func (s *Store) StrategyVersion() *StrategyVersionStore {
	if s.strategyVersion == nil {
		s.strategyVersion = NewStrategyVersionStore(s.gdb)
	}
	return s.strategyVersion
}
```
(Confirm the exact `Store` struct field convention — use `s.gdb` or the same DB handle `Strategy()` uses.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./store/ -run TestStrategyVersionStoreLifecycle -v`
Expected: PASS.

- [ ] **Step 6: Run backend checks**

Run: `go build ./... && go vet ./... && go test ./store/`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add store/strategy_version.go store/store.go store/strategy_version_test.go
git commit -m "feat(store): add strategy_versions table and store"
```

---

### Task 7: Hook version snapshots into strategy lifecycle

**Files:**
- Modify: `store/strategy.go` (`Create`, `Update`, `Duplicate`, `Delete`)

**Interfaces:**
- Consumes: `StrategyVersionStore` (from Task 6), `StrategyStore.db`.
- Produces: version snapshot side effects on create/update/duplicate/delete. No signature changes to existing `StrategyStore` methods.

- [ ] **Step 1: Update `Create` to snapshot v1**

In `store/strategy.go`, change `Create` to create the strategy then snapshot v1:
```go
func (s *StrategyStore) Create(strategy *Strategy) error {
	if err := s.db.Create(strategy).Error; err != nil {
		return err
	}
	_, err := s.StrategyVersion().CreateSnapshot(strategy.ID, strategy.UserID, strategy.Config, "v1")
	return err
}
```
Add a helper on `StrategyStore` to reach the version store, or inject it. Since `StrategyStore` holds only `db`, add a field or a package-level constructor that also gets the version store. Recommended: add a `version *StrategyVersionStore` field to `StrategyStore` set in its constructor `NewStrategyStore`. Verify the constructor signature and wire `version: NewStrategyVersionStore(db)`.

- [ ] **Step 2: Update `Update` to snapshot the pre-edit config**

Before applying the update, snapshot the **existing** config (pre-edit state) as a new version. `Update` currently receives a `*Strategy` whose `.Config` is the new config; the pre-edit config must be loaded first:
```go
func (s *StrategyStore) Update(strategy *Strategy) error {
	// Snapshot pre-edit state as a new version (preserves what the strategy
	// looked like before this edit).
	var existing Strategy
	if err := s.db.Where("id = ? AND user_id = ?", strategy.ID, strategy.UserID).First(&existing).Error; err == nil && existing.Config != "" {
		if _, err := s.StrategyVersion().CreateSnapshot(existing.ID, existing.UserID, existing.Config, "Snapshot before edit"); err != nil {
			return err
		}
	}
	// ... existing update query unchanged ...
}
```

- [ ] **Step 3: Update `Duplicate` to snapshot v1 for the new strategy**

After `Create` in `Duplicate`, snapshot v1 for the new strategy. Since `Duplicate` calls `s.Create(newStrategy)` (Task 7 Step 1 already snapshots v1), no additional change is needed if Step 1 is in place.

- [ ] **Step 4: Update `Delete` to cascade-delete versions**

At the end of `StrategyStore.Delete`, after the strategy row is deleted, delete its versions:
```go
	// ... existing checks and delete ...
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Strategy{}).Error; err != nil {
		return err
	}
	return s.StrategyVersion().DeleteForStrategy(id, userID)
```

- [ ] **Step 5: Write/verify lifecycle test**

Add assertions to `store/strategy_version_test.go` covering: create produces v1 current; update produces a pre-edit snapshot and bumps version; delete removes versions. Run:
Run: `go test ./store/ -run TestStrategyVersion -v`
Expected: PASS.

- [ ] **Step 6: Run backend checks**

Run: `go build ./... && go vet ./... && go test ./store/`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add store/strategy.go store/strategy_version_test.go
git commit -m "feat(strategy): snapshot strategy versions on create/update/duplicate/delete"
```

---

### Task 8: Add version + restore API endpoints

**Files:**
- Modify: `api/strategy.go` (new handlers)
- Modify: `api/server.go` (register routes)
- Test: `api/strategy_version_test.go` (new)

**Interfaces:**
- Consumes: `s.store.StrategyVersion()`, `s.store.Strategy()`, `s.store.Trader()` (for in-use check), `s.route`/`s.routeWithSchema` registration helpers.
- Produces:
  - `func (s *Server) handleListStrategyVersions(c *gin.Context)` → `GET /api/strategies/:id/versions`
  - `func (s *Server) handleGetStrategyVersion(c *gin.Context)` → `GET /api/strategies/:id/versions/:version`
  - `func (s *Server) handleRestoreStrategyVersion(c *gin.Context)` → `POST /api/strategies/:id/restore`
  - Response shape: `{version, strategy_id, label, note, config, created_at, is_current}`; list wraps as `{"versions": [...]}`. `label` = `v{n}`.

- [ ] **Step 1: Write the failing handler tests**

Create `api/strategy_version_test.go` using a store-backed server (mirror `api/server_test.go` for the gin setup; build a real `store.Store` on in-memory sqlite so `s.store.Strategy()`/`StrategyVersion()` work):
```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

func newStrategyVersionTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New(), store: st}
	s.setupRoutes()
	return s
}

// callHandler invokes a gin handler directly with a hermetic context that has
// user_id set (avoids JWT middleware in tests).
func callHandler(t *testing.T, h gin.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Params = gin.Params{{Key: "id", Value: "strat-1"}}
	c.Set("user_id", "user-1")
	w := httptest.NewRecorder()
	c.Writer = w
	h(c)
	return w
}

func TestStrategyVersionEndpoints(t *testing.T) {
	s := newStrategyVersionTestServer(t)

	// Create a strategy via the store directly to seed v1.
	strategy := &store.Strategy{ID: "strat-1", UserID: "user-1", Name: "Test", Config: `{"strategy_type":"ai_trading"}`}
	if err := s.store.Strategy().Create(strategy); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	// (StrategyStore.Create snapshots v1 after Task 7; if not yet wired, call
	// s.store.StrategyVersion().CreateSnapshot("strat-1","user-1", strategy.Config, "v1").)

	// GET /versions (direct handler call — hermetic, no JWT).
	w := callHandler(t, s.handleListStrategyVersions, http.MethodGet, "/api/strategies/strat-1/versions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET versions status = %d, body=%s", w.Code, w.Body.String())
	}
	var listResp struct {
		Versions []strategyVersionDTO `json:"versions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResp.Versions) != 1 || listResp.Versions[0].Version != 1 {
		t.Fatalf("versions = %+v", listResp.Versions)
	}

	// GET /versions/:version
	w = callHandler(t, s.handleGetStrategyVersion, http.MethodGet, "/api/strategies/strat-1/versions/1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET version status = %d, body=%s", w.Code, w.Body.String())
	}

	// POST /restore
	w = callHandler(t, s.handleRestoreStrategyVersion, http.MethodPost, "/api/strategies/strat-1/restore", `{"version":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST restore status = %d, body=%s", w.Code, w.Body.String())
	}
}
```
Note: handlers read `user_id` from the gin context (`c.GetString("user_id")`). The `protected` route group wraps handlers in `s.authMiddleware()` (validates a real JWT at `api/server.go:646-682`). The `callHandler` helper (defined in the test file above) invokes handlers directly with `user_id` set, keeping the test hermetic and JWT-free. The store is `store.New(":memory:")` (there is no `NewInMemory`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/ -run TestStrategyVersionEndpoints -v`
Expected: FAIL — the test does not compile because `strategyVersionDTO` and the `s.handleListStrategyVersions` / `s.handleGetStrategyVersion` / `s.handleRestoreStrategyVersion` handlers are not yet defined (red state).

- [ ] **Step 3: Implement the handlers in `api/strategy.go`**

```go
type strategyVersionDTO struct {
	Version    int             `json:"version"`
	StrategyID string          `json:"strategy_id"`
	Label      string          `json:"label"`
	Note       string          `json:"note"`
	Config     json.RawMessage `json:"config"`
	CreatedAt  time.Time       `json:"created_at"`
	IsCurrent  bool            `json:"is_current"`
}

func toStrategyVersionDTO(v *store.StrategyVersion) strategyVersionDTO {
	return strategyVersionDTO{
		Version:    v.Version,
		StrategyID: v.StrategyID,
		Label:      fmt.Sprintf("v%d", v.Version),
		Note:       v.Note,
		Config:     json.RawMessage(v.Config),
		CreatedAt:  v.CreatedAt,
		IsCurrent:  v.IsCurrent,
	}
}

func (s *Server) handleListStrategyVersions(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if _, err := s.store.Strategy().Get(userID, strategyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	versions, err := s.store.StrategyVersion().List(strategyID, userID)
	if err != nil {
		SafeInternalError(c, "Failed to get strategy versions", err)
		return
	}
	out := make([]strategyVersionDTO, 0, len(versions))
	for _, v := range versions {
		out = append(out, toStrategyVersionDTO(v))
	}
	c.JSON(http.StatusOK, gin.H{"versions": out})
}

func (s *Server) handleGetStrategyVersion(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	versionStr := c.Param("version")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var version int
	if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
		SafeBadRequest(c, "Invalid version")
		return
	}
	v, err := s.store.StrategyVersion().Get(strategyID, userID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	c.JSON(http.StatusOK, toStrategyVersionDTO(v))
}

func (s *Server) handleRestoreStrategyVersion(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var req struct {
		Version int `json:"version" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	// Blocked while a running trader uses the strategy (same guard as the editor).
	running, err := s.runningTradersForStrategy(userID, strategyID)
	if err != nil {
		SafeInternalError(c, "Check running traders", err)
		return
	}
	if len(running) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot restore a strategy while a trader is running on it. Stop the trader first."})
		return
	}
	v, err := s.store.StrategyVersion().Get(strategyID, userID, req.Version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	strategy, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	strategy.Config = v.Config
	if err := s.store.Strategy().Update(strategy); err != nil {
		SafeInternalError(c, "Failed to restore strategy", err)
		return
	}
	if err := s.store.StrategyVersion().SetCurrent(strategyID, userID, v.Version); err != nil {
		SafeInternalError(c, "Failed to mark version current", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Strategy restored to v%d", "version": v.Version})
}
```
Implement a helper `runningTradersForStrategy` that lists the user's traders and returns names whose `strategy_id == strategyID` and `is_running` (mirror the frontend `getRunningTradersForStrategy`):
```go
// runningTradersForStrategy returns the names of running traders linked to a strategy.
func (s *Server) runningTradersForStrategy(userID, strategyID string) ([]string, error) {
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		return nil, err
	}
	var running []string
	for _, t := range traders {
		if t.StrategyID == strategyID && t.IsRunning {
			running = append(running, t.Name)
		}
	}
	return running, nil
}
```

- [ ] **Step 4: Register the routes**

In `api/server.go`, near the other strategy routes (line ~342-410), add:
```go
			s.route(protected, "GET", "/strategies/:id/versions", "List strategy version snapshots", s.handleListStrategyVersions)
			s.route(protected, "GET", "/strategies/:id/versions/:version", "Get a single strategy version snapshot", s.handleGetStrategyVersion)
			s.routeWithSchema(protected, "POST", "/strategies/:id/restore", "Restore a strategy to a saved version",
				`Body: {"version":<int, from GET /strategies/:id/versions>}. Applies that version's config. Blocked while a running trader uses the strategy.`,
				s.handleRestoreStrategyVersion)
```
Confirm the `s.routeWithSchema` signature accepts the handler as its last arg (check an existing call at line ~334-337).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./api/ -run TestStrategyVersion -v`
Expected: PASS.

- [ ] **Step 6: Run backend checks**

Run: `go build ./... && go vet ./... && go test ./api/ ./store/`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add api/strategy.go api/server.go api/strategy_version_test.go
git commit -m "feat(api): strategy version list/get/restore endpoints"
```

---

### Task 9: Wire the frontend version-history to the real endpoints

**Files:**
- Modify: `web/src/features/strategies/strategyApi.ts`
- Modify: `web/src/features/strategies/VersionHistoryModal.tsx`

**Interfaces:**
- Consumes: backend endpoints from Task 8.
- Produces:
  - `getVersions(strategyId: string): Promise<StrategyVersion[]>` (drop the `currentConfig` param)
  - `getVersion(strategyId: string, version: number): Promise<StrategyVersion | null>`
  - `restoreVersion(strategyId: string, version: number): Promise<{ ok: boolean }>`
  - Removes the `snapshotVersion` placeholder helper and the `// Backend pending` comments.

- [ ] **Step 1: Add HTTP methods to `lib/api/strategies.ts`**

In `web/src/lib/api/strategies.ts`, add methods to the `strategyApi` object following the existing `httpClient` pattern:
```ts
  async getVersions(strategyId: string): Promise<StrategyVersion[]> {
    const result = await httpClient.get<{ versions: StrategyVersion[] }>(
      `${API_BASE}/strategies/${strategyId}/versions`
    )
    if (!result.success) throw new Error('Failed to fetch strategy versions')
    return Array.isArray(result.data?.versions) ? result.data!.versions : []
  },

  async getVersion(strategyId: string, version: number): Promise<StrategyVersion | null> {
    const result = await httpClient.get<StrategyVersion>(
      `${API_BASE}/strategies/${strategyId}/versions/${version}`
    )
    if (!result.success) throw new Error('Failed to fetch strategy version')
    return result.data ?? null
  },

  async restoreVersion(strategyId: string, version: number): Promise<{ ok: boolean }> {
    const result = await httpClient.post<{ message: string }>(
      `${API_BASE}/strategies/${strategyId}/restore`,
      { version }
    )
    return { ok: result.success }
  },
```
Import `StrategyVersion` into `strategies.ts` (from `../../types/strategy` or add it to the type imports).

`StrategyVersion` is currently declared in `web/src/features/strategies/strategyApi.ts:5`. To avoid an awkward cross-import, move that interface to `web/src/types/strategy.ts` and have the feature file re-import it (or import from `../../types/strategy`). Update both files accordingly.

- [ ] **Step 2: Wire the feature `strategyApi.ts` functions to them**

In `web/src/features/strategies/strategyApi.ts`, replace the placeholder section (lines 41-95) with:
```ts
export async function getVersions(strategyId: string): Promise<StrategyVersion[]> {
  return strategyApi.getVersions(strategyId)
}

export async function getVersion(
  strategyId: string,
  version: number
): Promise<StrategyVersion | null> {
  return strategyApi.getVersion(strategyId, version)
}

export async function restoreVersion(
  strategyId: string,
  version: number
): Promise<{ ok: boolean }> {
  return strategyApi.restoreVersion(strategyId, version)
}
```
Remove the `snapshotVersion` helper, the `// Backend pending` comments, and the now-unused `api` import if it becomes unused. Note: the feature file imports `strategyApi` from `../../lib/api/strategies` (line 3) — this is the low-level client, distinct from the composite `api` (line 2). Keep whichever is still used.

- [ ] **Step 3: Update `VersionHistoryModal.tsx` call sites**

In `web/src/features/strategies/VersionHistoryModal.tsx` (lines 25, 42), update:
- `strategyManagerApi.getVersions(strategy.id, strategy.config)` → `strategyManagerApi.getVersions(strategy.id)`.
- `strategyManagerApi.restoreVersion(...)` call stays the same signature.

- [ ] **Step 4: Run frontend checks**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/strategies/strategyApi.ts web/src/features/strategies/VersionHistoryModal.tsx web/src/lib/api/strategies.ts web/src/types/strategy.ts
git commit -m "feat(strategy): wire version history to real backend endpoints"
```

---

### Task 10: Add strategy aggregate-stats endpoint

**Files:**
- Modify: `api/strategy.go` (stats handler + merged-curve helper)
- Modify: `api/server.go` (register route)
- Test: `api/strategy_stats_test.go` (new)

**Interfaces:**
- Consumes: `s.store.Strategy()`, `s.store.Trader().List(userID)`, `s.store.Equity().GetByTimeRange(traderID, start, end)`, `s.store.Position().GetOpenPositions(traderID)`.
- Produces:
  - `func (s *Server) handleGetStrategyStats(c *gin.Context)` → `GET /api/strategies/:id/stats`
  - `type strategyStatsDTO struct { AUM float64; Symbols []string; NavPoints []navPointDTO; SevenDayYield *float64; Sharpe *float64; MaxDrawdown *float64 }`
  - Metric helpers `sevenDayYield(nav []navPointDTO) *float64`, `maxDrawdown(nav []navPointDTO) *float64`, `sharpeFromCurve(nav []navPointDTO) *float64`.

- [ ] **Step 1: Write the failing test**

Create `api/strategy_stats_test.go`. Seed a strategy + two linked traders with equity snapshots at two distinct timestamps, then call the handler directly via the `callHandler` helper (from `strategy_version_test.go`, same package). Assert the merged curve sums `total_equity` across traders at the same timestamp and that `seven_day_yield` is computed:
```go
package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

func TestStrategyStatsMergesLinkedTraders(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	gin.SetMode(gin.TestMode)
	s := &Server{store: st}

	if err := st.Strategy().Create(&store.Strategy{ID: "strat-1", UserID: "user-1", Name: "S", Config: `{"strategy_type":"ai_trading"}`}); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	mk := func(id string) {
		if err := st.Trader().Create(&store.Trader{ID: id, UserID: "user-1", Name: id, AIModelID: "m", ExchangeID: "e", StrategyID: "strat-1"}); err != nil {
			t.Fatalf("create trader %s: %v", id, err)
		}
	}
	mk("t1")
	mk("t2")

	t0 := time.Now().UTC().Add(-6 * 24 * time.Hour)
	t1 := time.Now().UTC()
	// Both traders share the same two timestamps so the merge sums them.
	for _, snap := range []store.EquitySnapshot{
		{TraderID: "t1", Timestamp: t0, TotalEquity: 100},
		{TraderID: "t2", Timestamp: t0, TotalEquity: 200},
		{TraderID: "t1", Timestamp: t1, TotalEquity: 110},
		{TraderID: "t2", Timestamp: t1, TotalEquity: 220},
	} {
		if err := st.Equity().Save(&snap); err != nil {
			t.Fatalf("save equity: %v", err)
		}
	}

	w := callHandler(t, s.handleGetStrategyStats, http.MethodGet, "/api/strategies/strat-1/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body=%s", w.Code, w.Body.String())
	}
	var dto strategyStatsDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if len(dto.NavPoints) != 2 {
		t.Fatalf("nav_points length = %d, want 2", len(dto.NavPoints))
	}
	// Merged first point = 100 + 200 = 300; last = 110 + 220 = 330.
	if dto.NavPoints[0].TotalEquity != 300 || dto.NavPoints[1].TotalEquity != 330 {
		t.Fatalf("nav_points = %+v", dto.NavPoints)
	}
	if dto.SevenDayYield == nil {
		t.Fatalf("expected non-nil seven_day_yield, got %+v", dto)
	}
	if dto.AUM != 330 {
		t.Fatalf("aum = %v, want 330", dto.AUM)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/ -run TestStrategyStatsMergesLinkedTraders -v`
Expected: FAIL — the test does not compile because `strategyStatsDTO` and `handleGetStrategyStats` are not yet defined (red state).

- [ ] **Step 3: Implement the merged-curve + metrics helper**

In `api/strategy.go`:
```go
type navPointDTO struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalEquity float64   `json:"total_equity"`
}

type strategyStatsDTO struct {
	AUM           float64      `json:"aum"`
	Symbols       []string     `json:"symbols"`
	NavPoints     []navPointDTO `json:"nav_points"`
	SevenDayYield *float64     `json:"seven_day_yield"`
	Sharpe        *float64     `json:"sharpe"`
	MaxDrawdown   *float64     `json:"max_drawdown"`
}

func (s *Server) handleGetStrategyStats(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if _, err := s.store.Strategy().Get(userID, strategyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		SafeInternalError(c, "List traders", err)
		return
	}
	linked := make([]*store.Trader, 0)
	for _, t := range traders {
		if t.StrategyID == strategyID {
			linked = append(linked, t)
		}
	}

	// Merged equity curve over the last 7 days.
	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)
	series := make(map[int64]float64)
	order := make([]int64, 0)
	for _, t := range linked {
		snaps, err := s.store.Equity().GetByTimeRange(t.ID, start, now)
		if err != nil {
			continue
		}
		for _, snap := range snaps {
			key := snap.Timestamp.UTC().Unix()
			if _, ok := series[key]; !ok {
				order = append(order, key)
			}
			series[key] += snap.TotalEquity
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	nav := make([]navPointDTO, 0, len(order))
	for _, key := range order {
		nav = append(nav, navPointDTO{Timestamp: time.Unix(key, 0).UTC(), TotalEquity: series[key]})
	}

	dto := strategyStatsDTO{NavPoints: nav, Symbols: []string{}}
	// AUM = sum of latest equity per linked trader.
	var aum float64
	for _, t := range linked {
		if latest, err := s.store.Equity().GetLatest(t.ID, 1); err == nil && len(latest) > 0 {
			aum += latest[len(latest)-1].TotalEquity
		}
	}
	dto.AUM = aum

	// Open-position symbols across linked traders (union).
	symbolSet := make(map[string]bool)
	for _, t := range linked {
		if positions, err := s.store.Position().GetOpenPositions(t.ID); err == nil {
			for _, p := range positions {
				symbolSet[p.Symbol] = true
			}
		}
	}
	for sym := range symbolSet {
		dto.Symbols = append(dto.Symbols, sym)
	}

	// Metrics.
	dto.SevenDayYield = sevenDayYield(nav)
	dto.MaxDrawdown = maxDrawdown(nav)
	dto.Sharpe = sharpeFromCurve(nav)

	c.JSON(http.StatusOK, dto)
}
```
Add the metric helpers (pure functions, unit-testable):
```go
func sevenDayYield(nav []navPointDTO) *float64 {
	if len(nav) < 2 || nav[0].TotalEquity == 0 {
		return nil
	}
	y := (nav[len(nav)-1].TotalEquity - nav[0].TotalEquity) / nav[0].TotalEquity * 100
	return &y
}

func maxDrawdown(nav []navPointDTO) *float64 {
	if len(nav) < 2 {
		return nil
	}
	peak := nav[0].TotalEquity
	var maxDD float64
	for _, p := range nav {
		if p.TotalEquity > peak {
			peak = p.TotalEquity
		}
		if peak > 0 {
			dd := (peak - p.TotalEquity) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return &maxDD
}

// sharpeFromCurve computes annualized Sharpe from per-interval returns.
func sharpeFromCurve(nav []navPointDTO) *float64 {
	if len(nav) < 3 {
		return nil
	}
	returns := make([]float64, 0, len(nav)-1)
	for i := 1; i < len(nav); i++ {
		if nav[i-1].TotalEquity == 0 {
			return nil
		}
		returns = append(returns, (nav[i].TotalEquity-nav[i-1].TotalEquity)/nav[i-1].TotalEquity)
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns))
	if variance == 0 {
		return nil
	}
	std := math.Sqrt(variance)
	annual := math.Sqrt(float64(len(returns))) // per-sample annualization factor
	out := mean / std * annual
	return &out
}
```
Add imports `math` and `sort` to `api/strategy.go`. The open-position method is `store.Position().GetOpenPositions(traderID string) ([]*TraderPosition, error)` (defined in `store/position.go`); `TraderPosition` exposes `Symbol`.

- [ ] **Step 4: Register the route**

In `api/server.go` near the strategy routes, add:
```go
			s.routeWithSchema(protected, "GET", "/strategies/:id/stats", "Aggregate strategy stats (7D yield, Sharpe, max drawdown, merged NAV)",
				`Returns: {"aum":<number>,"symbols":[<string>],"nav_points":[{"timestamp":"<RFC3339>","total_equity":<number>}],"seven_day_yield":<number|null>,"sharpe":<number|null>,"max_drawdown":<number|null>}`,
				s.handleGetStrategyStats)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./api/ -run TestStrategyStats -v`
Expected: PASS.

- [ ] **Step 6: Run backend checks**

Run: `go build ./... && go vet ./... && go test ./api/ ./store/`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add api/strategy.go api/server.go api/strategy_stats_test.go
git commit -m "feat(api): strategy aggregate stats endpoint (7D yield, Sharpe, max drawdown)"
```

---

### Task 11: Wire the frontend stats to the real endpoint + remove placeholder `null`s

**Files:**
- Modify: `web/src/features/strategies/strategyApi.ts` (`getStrategyStats`)

**Interfaces:**
- Consumes: backend `GET /strategies/:id/stats` from Task 10.
- Produces: `getStrategyStats(strategyId: string): Promise<StrategyStats>` calling the endpoint; removes the client-side merge + `// Aggregation backend pending` comment and the hardcoded `null`s.

- [ ] **Step 1: Add the stats HTTP method and rewrite `getStrategyStats`**

First, add `getStrategyStats` to `web/src/lib/api/strategies.ts` (following the `httpClient` pattern):
```ts
  async getStrategyStats(strategyId: string): Promise<StrategyStats> {
    const result = await httpClient.get<StrategyStatsResponse>(
      `${API_BASE}/strategies/${strategyId}/stats`
    )
    if (!result.success) throw new Error('Failed to fetch strategy stats')
    return result.data!
  },
```
Define the backend shape in `strategies.ts`:
```ts
interface StrategyStatsResponse {
  aum: number
  symbols: string[]
  nav_points: { timestamp: string; total_equity: number }[]
  seven_day_yield: number | null
  sharpe: number | null
  max_drawdown: number | null
}
```

Then rewrite the feature `getStrategyStats` in `web/src/features/strategies/strategyApi.ts` (lines 118-175) to map the backend response to the frontend `StrategyStats` interface:
```ts
export async function getStrategyStats(
  strategyId: string
): Promise<StrategyStats> {
  const d = await strategyApi.getStrategyStats(strategyId)
  return {
    aum: d.aum ?? 0,
    symbols: d.symbols ?? [],
    navPoints: (d.nav_points ?? []).map((p) => ({
      timestamp: p.timestamp,
      total_equity: p.total_equity,
    })),
    sevenDayYield: d.seven_day_yield ?? null,
    sharpe: d.sharpe ?? null,
    maxDd: d.max_drawdown ?? null,
  }
}
```
Remove the old client-side merge, the `// Aggregation backend pending for these` comment, and the hardcoded `null`s. `getRunningTradersForStrategy` is still used by `ScopeStepPage`/`EditorStepPage` for the in-use guard, so keep it (only the equity-batch merge inside the old `getStrategyStats` is removed).

- [ ] **Step 2: Run frontend checks**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/strategies/strategyApi.ts
git commit -m "feat(strategy): consume real strategy stats endpoint"
```

---

### Task 12: Final verification

**Files:**
- None (verification only).

- [ ] **Step 1: Backend full verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass.

- [ ] **Step 2: Frontend full verification**

Run: `cd web && npx tsc --noEmit && npm run build && npm test`
Expected: all pass.

- [ ] **Step 3: Confirm stale warnings removed**

Search for `backend pending`, `Runtime prompt wiring`, `Data source not configured`:
Run: `grep -rn "backend pending\|Runtime prompt wiring\|Data source not configured" web/src`
Expected: no matches.

- [ ] **Step 4: Confirm single-scope workflow**

Verify `web/src/features/strategies/ScopeStepPage.tsx` has no `scope.mode` / `setScopeMode` / `custom_scope` references, and `buildCoinSource` takes a single `ScopeUnit | null`.
Run: `grep -rn "scope_mode\|setScopeMode\|custom_scope\|mergeScopeUnit" web/src`
Expected: no matches (except none).

- [ ] **Step 5: Final commit (if any uncommitted changes)**

```bash
git status
git add -A && git commit -m "chore: final verification cleanups"
```
(Only if there are uncommitted changes.)
