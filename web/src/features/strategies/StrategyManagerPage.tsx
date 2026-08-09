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
      notify.error(
        err instanceof Error ? err.message : 'Failed to load strategies'
      )
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const openInTraderAgent = (strategyId: string) => {
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
                  <td className="px-3 py-3 text-nofx-text-muted">
                    {index + 1}
                  </td>
                  <td className="px-3 py-3">
                    <div className="font-medium">{s.name}</div>
                    <div className="text-xs text-nofx-text-muted">
                      {s.is_active ? 'Active' : 'Inactive'}
                    </div>
                  </td>
                  <td className="px-3 py-3">{formatMoney(row.stats.aum)}</td>
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
                    {row.stats.sharpe !== null
                      ? row.stats.sharpe.toFixed(2)
                      : '—'}
                  </td>
                  <td className="px-3 py-3">
                    {row.stats.maxDd !== null
                      ? `${row.stats.maxDd.toFixed(2)}%`
                      : '—'}
                  </td>
                  <td className="px-3 py-3 text-xs">
                    {new Date(s.updated_at).toLocaleDateString()}
                  </td>
                  <td className="px-3 py-3">
                    <EquitySparkline points={row.stats.navPoints} />
                  </td>
                  <td className="px-3 py-3">
                    <div
                      className="flex gap-1"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <IconBtn
                        title="Version History"
                        onClick={() => setHistoryFor(s)}
                      >
                        <History className="h-4 w-4" />
                      </IconBtn>
                      <IconBtn
                        title="Use in Trader Agent"
                        onClick={() => openInTraderAgent(s.id)}
                      >
                        <Bot className="h-4 w-4" />
                      </IconBtn>
                      <IconBtn
                        title="Edit"
                        onClick={() => navigate(`/strategy/${s.id}/edit/scope`)}
                      >
                        <Pencil className="h-4 w-4" />
                      </IconBtn>
                    </div>
                  </td>
                </tr>
              )
            })}
            {rows.length === 0 && (
              <tr>
                <td
                  colSpan={10}
                  className="px-3 py-10 text-center text-nofx-text-muted"
                >
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
