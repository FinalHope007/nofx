import type { Strategy, StrategyConfig, StrategyVersion } from '../../types/strategy'
import { api } from '../../lib/api'
import { strategyApi } from '../../lib/api/strategies'

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

export async function getRunningTradersForStrategy(
  strategyId: string
): Promise<string[]> {
  const traders = await api.getTraders(true).catch(() => [])
  return traders
    .filter((t) => t.strategy_id === strategyId && t.is_running)
    .map((t) => t.trader_name)
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

  const aum = accounts.reduce((sum, a) => sum + (a?.total_equity ?? 0), 0)
  const symbols = Array.from(
    new Set(
      positions
        .flat()
        .map((p) => p?.symbol)
        .filter((s): s is string => Boolean(s))
    )
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

  return {
    aum,
    symbols,
    navPoints,
    sevenDayYield: null,
    sharpe: null,
    maxDd: null,
  }
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
  getRunningTradersForStrategy,
}
