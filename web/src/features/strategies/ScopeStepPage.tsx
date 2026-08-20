import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, ChevronRight, Loader2 } from 'lucide-react'
import { useStrategyDraft } from './draftStore'
import { SCOPE_CARD_DEFS, toScopeUnit } from './scopeCatalog'
import { strategyManagerApi } from './strategyApi'
import type { ScopeCardDef } from './scopeCatalog'
import type { ScopeUnit } from '../../types/strategy'
import { api } from '../../lib/api'
import { notify } from '../../lib/notify'

type Mode = 'create' | 'edit'

function cardActive(scope: ScopeUnit | null, def: ScopeCardDef): boolean {
  return scope?.id === def.id
}

function cardUnit(def: ScopeCardDef, limit: number): ScopeUnit {
  return toScopeUnit(def, limit)
}

function matchConcreteScope(
  cs: import('../../types/strategy').CoinSourceConfig
): ScopeUnit | null {
  const scopeSource =
    cs.source_type === 'vergex_signal' ? 'vergex' : cs.source_type
  const def = SCOPE_CARD_DEFS.find((c) => c.source_type === scopeSource)
  if (!def) return null
  let limit: number
  switch (cs.source_type) {
    case 'ai500':
      limit = cs.ai500_limit ?? def.defaultLimit
      break
    case 'vergex_signal':
      limit = cs.vergex_limit ?? def.defaultLimit
      break
    case 'binance_technical':
      limit = cs.binance_technical_limit ?? def.defaultLimit
      break
    case 'binance_sentiment':
      limit = cs.binance_sentiment_limit ?? def.defaultLimit
      break
    default:
      limit = cs.hyper_rank_limit ?? def.defaultLimit
  }
  const unit = toScopeUnit(def, limit)
  if (cs.source_type === 'binance_technical') {
    unit.interval = cs.binance_technical_interval ?? '1h'
    unit.direction = cs.binance_technical_direction ?? 'top'
  }
  if (cs.source_type === 'binance_sentiment') {
    unit.direction = cs.binance_sentiment_direction ?? 'top'
  }
  return unit
}

export function ScopeStepPage() {
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const mode: Mode = params.id ? 'edit' : 'create'
  const strategyId = params.id
  const { scope, setScope, clearScope } = useStrategyDraft()
  const [topN, setTopN] = useState<Record<string, number>>({})
  const [category, setCategory] = useState<'crypto' | 'stock'>('crypto')
  const [loading, setLoading] = useState(mode === 'edit')
  const [blocked, setBlocked] = useState<string[]>([])

  useEffect(() => {
    if (mode !== 'edit' || !strategyId) {
      setLoading(false)
      return
    }
    ;(async () => {
      try {
        const running =
          await strategyManagerApi.getRunningTradersForStrategy(strategyId)
        if (running.length > 0) {
          setBlocked(running)
          setLoading(false)
          return
        }
        const strategy = await api.getStrategy(strategyId)
        const coinSource = strategy.config.ai_config?.coin_source
        if (!coinSource) return

        const unit = matchConcreteScope(coinSource)
        if (unit) {
          setScope(unit)
          setCategory(unit.category)
          setTopN({ [unit.id]: unit.limit })
        }
      } catch (err) {
        notify.error(
          err instanceof Error ? err.message : 'Failed to load strategy scope'
        )
      } finally {
        setLoading(false)
      }
    })()
  }, [strategyId, mode])

  const cards = SCOPE_CARD_DEFS.filter((c) => c.category === category)
  const nextPath =
    mode === 'create'
      ? '/strategy/create/editor'
      : `/strategy/${strategyId}/edit/editor`
  const backPath = '/strategy'

  const toggleCard = (def: ScopeCardDef) => {
    if (cardActive(scope, def)) {
      clearScope()
    } else {
      const unit = cardUnit(def, topN[def.id] ?? def.defaultLimit)
      if (def.source_type === 'binance_technical') {
        unit.interval = '1h'
        unit.direction = 'top'
      }
      if (def.source_type === 'binance_sentiment') {
        unit.direction = 'top'
      }
      setScope(unit)
    }
  }

  const updateLimit = (def: ScopeCardDef, raw: number) => {
    const limit = Math.min(50, Math.max(1, raw || 1))
    setTopN((prev) => ({ ...prev, [def.id]: limit }))
    if (cardActive(scope, def)) {
      const unit = cardUnit(def, limit)
      if (def.source_type === 'binance_technical' || def.source_type === 'binance_sentiment') {
        unit.interval = scope?.interval
        unit.direction = scope?.direction
      }
      setScope(unit)
    }
  }

  if (loading) {
    return (
      <div className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-7 w-7 animate-spin text-nofx-gold" />
      </div>
    )
  }

  if (blocked.length > 0) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <div className="rounded-lg border border-nofx-danger/30 bg-nofx-danger/10 p-6">
          <h1 className="text-lg font-semibold text-nofx-text">
            This strategy is in use
          </h1>
          <p className="mt-2 text-sm text-nofx-text-muted">
            Editing a strategy while a trader is live on it can cause
            discrepancies between open positions and your parameters (for
            example, leverage changes). Stop the following trader
            {blocked.length > 1 ? 's' : ''} before editing this strategy:
          </p>
          <ul className="mt-3 list-inside list-disc text-sm text-nofx-danger">
            {blocked.map((name) => (
              <li key={name}>{name}</li>
            ))}
          </ul>
          <button
            type="button"
            onClick={() => navigate('/traders')}
            className="mt-5 rounded-lg bg-nofx-gold px-4 py-2 text-sm font-semibold text-nofx-bg"
          >
            Go to Traders to stop it
          </button>
          <button
            type="button"
            onClick={() => navigate('/strategy')}
            className="ml-3 mt-5 rounded-lg border border-[rgba(26,24,19,0.14)] px-4 py-2 text-sm text-nofx-text-muted hover:text-nofx-text"
          >
            Back to strategies
          </button>
        </div>
      </div>
    )
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
          const active = cardActive(scope, def)
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

              {active &&
                (def.source_type === 'binance_technical' ||
                  def.source_type === 'binance_sentiment') && (
                  <div
                    className="mt-2 flex items-center gap-2"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <span className="text-xs text-nofx-text-muted">
                      Direction
                    </span>
                    <div className="flex gap-1">
                      {(['top', 'bottom'] as const).map((d) => (
                        <button
                          key={d}
                          type="button"
                          onClick={() => {
                            const next = { ...scope!, direction: d }
                            setScope(next)
                          }}
                          className={`rounded px-2 py-0.5 text-xs capitalize ${
                            scope?.direction === d
                              ? 'bg-nofx-gold/20 text-nofx-gold'
                              : 'text-nofx-text-muted hover:text-nofx-text'
                          }`}
                        >
                          {d}
                        </button>
                      ))}
                    </div>
                  </div>
                )}

              {active && def.source_type === 'binance_technical' && (
                <div
                  className="mt-2 flex items-center gap-2"
                  onClick={(e) => e.stopPropagation()}
                >
                  <span className="text-xs text-nofx-text-muted">
                    Interval
                  </span>
                  <div className="flex gap-1">
                    {(['1h', '24h'] as const).map((iv) => (
                      <button
                        key={iv}
                        type="button"
                        onClick={() => {
                          const next = { ...scope!, interval: iv }
                          setScope(next)
                        }}
                        className={`rounded px-2 py-0.5 text-xs ${
                          scope?.interval === iv
                            ? 'bg-nofx-gold/20 text-nofx-gold'
                            : 'text-nofx-text-muted hover:text-nofx-text'
                        }`}
                      >
                        {iv}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>

      <div className="mt-8 flex justify-end">
        <button
          type="button"
          disabled={scope === null}
          onClick={() => navigate(nextPath)}
          className="inline-flex items-center gap-2 rounded-lg bg-nofx-gold px-5 py-2 text-sm font-semibold text-nofx-bg disabled:cursor-not-allowed disabled:opacity-40"
        >
          Next <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}
