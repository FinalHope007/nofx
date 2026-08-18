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

// ----- Live per-strategy stats from the backend stats endpoint -----

export interface StrategyStats {
  aum: number
  symbols: string[]
  navPoints: { timestamp: string; total_equity: number }[]
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
