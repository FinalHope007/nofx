# Strategy Backend Tasks — Single-Scope Cleanup + decision_context + Versions + Stats

**Date:** 2026-08-18

## Context

The Strategy Manager frontend (on `dev`) was built ahead of its backend. This spec covers:

- **Task 1 (frontend cleanup):** Remove the multi-scope AND/OR concept, reverting scope
  selection to **single scope only**. The `custom` source_type / `scope_mode` / `custom_scope`
  emit path is removed from the frontend. The backend was never touched for this (it has no
  `custom` case in `GetCandidateCoins` — it returns `unknown coin source type: custom` at
  runtime). Backend `store.CoinSourceConfig` is intentionally left unchanged.
- **Task 2:** Wire the persisted `ai_config.decision_context` into the prompt builder.
- **Task 4:** Strategy version/snapshot endpoints + `strategy_versions` table.
- **Task 5:** Strategy-level aggregate stats endpoint (7D yield, Sharpe, max drawdown).
- **Stale-warning cleanup:** Remove frontend "backend pending" warnings that Tasks 2/4/5 make
  obsolete.

Single scope remains fully functional: the selected scope card maps to one concrete backend
`source_type` that `GetCandidateCoins` already supports (`hyper_rank`, `ai500`, `oi_top`,
`oi_low`, `netflow_top`, `netflow_low`, `price_top`, `price_low`, `vergex_signal`).

## Constraints

- Backend: `go build ./...`, `go vet ./...`, `go test ./...` clean.
- Frontend: `cd web && npx tsc --noEmit && npm run build && npm test` pass.
- English-only UI strings. Backend errors via `SafeError` / `SafeInternalError` /
  `SanitizeError`.
- `store.*` is the only DB access layer. All timestamps UTC.
- HIGH-RISK actions (restore config, affecting live traders) require explicit gating.

---

## Task 1 — Remove multi-scope, keep single scope (frontend only)

### Files

- `web/src/types/strategy.ts`
- `web/src/features/strategies/strategyFactory.ts`
- `web/src/features/strategies/ScopeStepPage.tsx`
- `web/src/features/strategies/draftStore.ts`
- `web/src/features/strategies/EditorStepPage.tsx`
- `web/src/features/strategies/dataSourceDefaults.ts`
- Tests: `strategyFactory.test.ts`, `strategyFactoryPresets.test.ts`, `scopeCatalog.test.ts`

### `web/src/types/strategy.ts`

- Remove `'custom'` from `CoinSourceConfig.source_type` union.
- Remove `custom_scope?: CustomScopeConfig` and `scope_mode?: 'overlap' | 'union'` fields.
- Remove the `CustomScopeConfig` interface (lines 288-292).
- Keep `ScopeUnit` and `ScopeVariant` (used for the single scope card + factory mapping).
- Keep all concrete `CoinSourceConfig` fields (`hyper_rank_*`, `vergex_*`, `netflow_limit`,
  `price_limit`, etc.).

### `web/src/features/strategies/strategyFactory.ts`

- `buildCoinSource(unit: ScopeUnit)` — take a single unit; drop the `mode` parameter.
- Remove the `units.length > 1` `custom` branch.
- Remove `scope_mode: mode` from every single-source return block.
- Remove the fallback `custom` branch. Unreachable known card is impossible, but as a safe
  guard, fall back to an empty `static` pool (never emits an unknown `source_type`).
- `StrategyEditorForm`: remove `scopeMode`; change `scopeUnits: ScopeUnit[]` to
  `scopeUnit: ScopeUnit | null`.

### `web/src/features/strategies/draftStore.ts`

- Replace `scope: { units: ScopeUnit[], mode }` with `scope: ScopeUnit | null`.
- Methods: `setScope(unit)` (replace), `clearScope()`. Remove `setScopeMode`,
  `mergeScopeUnit`, `removeScopeUnit`, `setScopeUnits`.

### `web/src/features/strategies/ScopeStepPage.tsx`

- Remove the Union/Overlap toggle button and all `setScopeMode` / `scope.mode` usage.
- Change cards from multi-select to **single-select radio**: selecting a second card replaces
  the first (per user). Selecting an active card deselects it.
- Remove the `custom_scope` prefill path on edit (lines 82-97); prefill from a single concrete
  `coin_source` via existing `matchConcreteScope`.
- "Next" enabled when exactly one scope is selected.
- **Stale-warning cleanup:** keep the `FREE`/`PAID` badge, but remove the
  "Data source not configured / backend pending" warning (line 287-292).

### `web/src/features/strategies/EditorStepPage.tsx`

- Update scope references from `scope.units` / `scope.units.length` to the single `scope`.
- Build form with `scopeUnit` instead of `scopeUnits` / `scopeMode`.
- **Stale-warning cleanup:** remove the "Runtime prompt wiring is pending backend work." text
  (lines 684-686).

### `web/src/features/strategies/dataSourceDefaults.ts`

- Change signature to `defaultDataSources(scope: ScopeUnit | null)`; replace `.some()` with
  direct equality checks.

### Tests

- `strategyFactory.test.ts`: remove multi-scope / overlap / custom cases; add single-scope
  assertions (concrete `source_type`, no `scope_mode` / `custom_scope`).
- `strategyFactoryPresets.test.ts`: update form fixtures (`scopeUnit`, drop `scopeMode`).
- `scopeCatalog.test.ts`: unchanged (still tests `toScopeUnit` / cards).

---

## Task 2 — Wire `decision_context` into the prompt builder

### Persist in backend config

Add to `store/strategy.go`:

```go
type DecisionContextConfig struct {
    Enabled     bool   `json:"enabled"`
    RecentCount int    `json:"recent_count"`
    Mode        string `json:"mode"` // "structured" | "digest"
}
```

- Add `DecisionContext *DecisionContextConfig` to `AIStrategyConfig`.
- Wire through `StrategyConfig.MarshalJSON` / `UnmarshalJSON` (mirror how `PromptSections`
  flows). No migration needed — persisted in the existing config JSON.

### Fetch recent decisions

- In `trader/auto_trader_loop.go` `buildTradingContext` (has `at.store` and `at.id`): when
  the strategy's `decision_context.enabled` is true, call
  `at.store.Decision().GetLatestRecords(at.id, recent_count)`.
- Set the results on the engine before the decision call via a setter, e.g.
  `engine.SetRecentDecisions(records []*store.DecisionRecord)`.
- Scoped to **this trader only** — never mix another trader's decisions into this trader's
  system prompt.

### Render into the system prompt

- `StrategyEngine.BuildSystemPrompt` reads the engine config `DecisionContext` and the set
  recent decisions. When `enabled`, render a recent-decisions section into the system prompt
  for **both** the Claw402 prompt (`buildVergexSystemPrompt`) and the generic prompt
  (`buildSystemPromptEN` / ZH path).
- **Only the raw assistant response text (`RawResponse`) from each prior cycle is included** —
  do NOT include the parsed `DecisionAction` list (prevents the LLM re-calling past actions).
  Do NOT include prior system/user prompts.
- `recent_count` caps how many prior cycles are rendered.
- `mode`:
  - `structured`: each prior response rendered as its own clearly-delimited entry (timestamped).
  - `digest`: each prior response truncated to a short single-line snippet per cycle (no AI
    summarization — plain text truncation only).

Add a formatting helper in `kernel/engine_prompt.go`.

---

## Task 4 — Strategy version/snapshot endpoints

### New table + store

New file `store/strategy_version.go`:

```go
type StrategyVersion struct {
    ID         int64     `gorm:"primaryKey;autoIncrement"`
    StrategyID string    `gorm:"column:strategy_id;not null;index"`
    UserID     string    `gorm:"column:user_id;not null;index"`
    Version    int       `gorm:"column:version;not null"`
    Config     string    `gorm:"column:config;not null"` // JSON snapshot of full config
    Note       string    `gorm:"column:note;default:''"`
    IsCurrent  bool      `gorm:"column:is_current;default:false"`
    CreatedAt  time.Time `gorm:"column:created_at"`
}
```

`StrategyVersionStore` methods: `List(strategyID, userID)`, `Get(strategyID, userID, version)`,
`CreateSnapshot(strategyID, userID, config, note)`, `SetCurrent(strategyID, userID, version)`,
`ClearCurrent(strategyID, userID)`, `DeleteForStrategy(strategyID, userID)`.

Register in `store/store.go` (`StrategyVersion()` lazy getter + `initTables()` AutoMigrate).

### Snapshot lifecycle (hooked into `StrategyStore`)

- **On create:** snapshot v1 of the saved config, mark `IsCurrent`.
- **On update:** snapshot the **pre-edit** config as a new version (bump `MAX(version)+1`),
  then apply the update. Do NOT mark the new pre-edit snapshot current — the live config
  remains the latest state. (The pre-edit snapshot preserves what the strategy looked like
  before this edit.)
- **On duplicate:** snapshot v1 of the copied config for the new strategy.
- **On delete:** cascade-delete all versions for the strategy.

Version numbering: monotonic per strategy (`MAX(version)+1`), v1 on create.

### Endpoints (in `api/strategy.go`, registered in `api/server.go` protected routes)

- `GET /api/strategies/:id/versions` → `{versions: [{version, strategy_id, label, note,
  config, created_at, is_current}]}`
- `GET /api/strategies/:id/versions/:version` → single `{version, strategy_id, label, note,
  config, created_at, is_current}`
- `POST /api/strategies/:id/restore` — body `{version}`. Applies the target version's `config`
  as the current strategy config. **No new snapshot is created on restore**; all existing
  versions stay in the DB so the user can switch back to undo. **Blocked while a running
  trader uses the strategy** (reuse the editor's in-use check).

Response shape matches the existing frontend `StrategyVersion` interface
(`web/src/features/strategies/strategyApi.ts`): `{version, strategy_id, label, note, config,
created_at, is_current}`. `label` derived as `v{n}`; `note` from the DB column.

### Frontend

- `web/src/features/strategies/strategyApi.ts`: replace the local placeholder
  `getVersions` / `getVersion` / `restoreVersion` with real calls to these endpoints.
  Remove the `// Backend pending` placeholder comments/`snapshotVersion`.
- `web/src/features/strategies/VersionHistoryModal.tsx`: consumes real versions + restore
  (already renders a dropdown; now wired to real data).

---

## Task 5 — Strategy-level aggregate stats

### Endpoint

`GET /api/strategies/:id/stats` (new handler in `api/strategy.go`, registered in
`api/server.go`), returning:

```json
{
  "aum": <number>,
  "symbols": [<string>],
  "nav_points": [ {"timestamp": "<RFC3339>", "total_equity": <number>} ],
  "seven_day_yield": <number|null>,
  "sharpe": <number|null>,
  "max_drawdown": <number|null>
}
```

### Computation (server-side, merged book)

1. **Linked traders:** `store.Trader().List(userID)` → filter `strategy_id == :id`.
2. **Merged equity curve:** per linked trader, `store.Equity().GetByTimeRange(traderID,
   now-7d, now)`; merge by timestamp, summing `total_equity` across traders at each
   timestamp. Produces `nav_points`.
3. **7D yield:** `(last_equity - first_equity) / first_equity * 100`. `null` if < 2 points.
4. **Sharpe:** per-interval returns from the merged curve; `mean/stddev * sqrt(annualization)`
   (annualize from the snapshot interval). `null` if insufficient data (e.g. < 2 returns or
   zero variance).
5. **Max drawdown:** `max((peak - equity) / peak) * 100` over the curve. `null` if < 2 points.
6. **AUM:** sum of latest `total_equity` across linked traders.
7. **Symbols:** union of open position symbols across linked traders.

### Frontend

- `web/src/features/strategies/strategyApi.ts` `getStrategyStats`: replace the client-side
  merge with a single call to `GET /strategies/:id/stats`. Remove the
  `// Aggregation backend pending for these` comment and the hardcoded `null`s.
- `StrategyManagerPage.tsx` / `StrategyStats` interface remain compatible (same field names).

---

## Stale-warning cleanup summary

| Location | Warning | Action |
|----------|---------|--------|
| `ScopeStepPage.tsx:287-292` | "Data source not configured / backend pending" | Remove text; keep `PAID`/`FREE` badge |
| `EditorStepPage.tsx:684-686` | "Runtime prompt wiring is pending backend work." | Remove (Task 2 makes it real) |
| `strategyApi.ts` version-history | "Backend pending" placeholder comments + `snapshotVersion` | Replace with real calls (Task 4) |
| `strategyApi.ts` stats | "Aggregation backend pending" comment + `null` fields | Replace with endpoint (Task 5) |

---

## Verification

- Backend: `go build ./... && go vet ./... && go test ./...`
- Frontend: `cd web && npx tsc --noEmit && npm run build && npm test`
- Manual workflow: create a strategy → select exactly one scope card (free or paid) → editor →
  save → confirm the emitted `coin_source.source_type` is a concrete supported value that
  `GetCandidateCoins` handles; confirm the decision-context and version-history UIs reflect
  the now-implemented backend.
