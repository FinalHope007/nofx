import { useEffect, useState } from 'react'
import { X, RotateCcw, Loader2 } from 'lucide-react'
import { strategyManagerApi } from './strategyApi'
import { notify } from '../../lib/notify'
import type { Strategy, StrategyVersion } from '../../types/strategy'

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
      .getVersions(strategy.id)
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
        notify.warning(res.error || 'Restore failed')
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
          <h2 className="text-lg font-semibold text-nofx-text">
            Version History
          </h2>
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
              <div className="text-sm font-semibold text-nofx-text">
                Parameters
              </div>
              <dl className="mt-2 space-y-1 text-xs text-nofx-text-muted">
                <dt className="font-semibold text-nofx-text">Scope</dt>
                <dd>
                  {current.config.ai_config?.coin_source.source_type ??
                    current.config.coin_source?.source_type ??
                    '-'}
                </dd>
                <dt className="font-semibold text-nofx-text">Candles</dt>
                <dd>
                  {(
                    current.config.ai_config?.indicators.klines
                      .selected_timeframes ?? []
                  ).join(', ') || '-'}
                </dd>
                <dt className="font-semibold text-nofx-text">Risk</dt>
                <dd>
                  {JSON.stringify(current.config.ai_config?.risk_control ?? {})}
                </dd>
                <dt className="font-semibold text-nofx-text">
                  Decision context
                </dt>
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
              Restoring switches the strategy back to this version.
            </p>
          </div>
        ) : (
          <p className="text-sm text-nofx-text-muted">No versions available.</p>
        )}
      </div>
    </div>
  )
}
