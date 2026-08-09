import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, ShieldAlert, ChevronRight } from 'lucide-react'
import { useStrategyDraft } from './draftStore'
import { SCOPE_CARD_DEFS, toScopeUnit } from './scopeCatalog'
import type { ScopeCardDef } from './scopeCatalog'
import type { ScopeUnit } from '../../types/strategy'

type Mode = 'create' | 'edit'

function cardActive(units: ScopeUnit[], def: ScopeCardDef): boolean {
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
      mergeScopeUnit(
        cardUnit(scope.units, def, topN[def.id] ?? def.defaultLimit)
      )
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
                  onChange={(e) => updateLimit(def, Number(e.target.value))}
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
