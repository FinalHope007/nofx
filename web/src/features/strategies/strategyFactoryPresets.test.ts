import { describe, it, expect } from 'vitest'
import { TRADING_STYLE_PRESETS, applyTradingStyle } from './strategyFactory'
import type { StrategyEditorForm } from './strategyFactory'

const baseForm: StrategyEditorForm = {
  name: 'x', custom_prompt: '', scan_interval_minutes: 5,
  btcEthMaxLeverage: 5, altcoinMaxLeverage: 5,
  btcEthPositionRatio: 5, altcoinPositionRatio: 1,
  isCrossMargin: true, selectedTimeframes: ['15m'], excludedCoins: [],
  decisionContext: { mode: 'structured', count: 10 },
  scopeUnits: [], scopeMode: 'union',
  // Section A / throttle form fields (all new required fields):
  maxPositions: 3, minPositionSize: 12, minRiskRewardRatio: 3.0,
  maxMarginUsage: 1.0, minConfidence: 78,
  enableOILiquidityFilter: true, oiLiquidityFilterMinUSDT: 15000000,
  maxOpensPerHour: 3, maxOpensPerCycle: 2,
  minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240,
  earlyCloseStopLossBypassPct: -3.0, earlyCloseTakeProfitBypassPct: 8.0,
  noiseCloseLossFloorPct: -2.0, noiseCloseProfitCeilingPct: 3.0,
}

describe('applyTradingStyle', () => {
  it('applies scalp values and clamps to section A caps', () => {
    const out = applyTradingStyle('scalp', { ...baseForm })
    expect(out.maxPositions).toBe(5)
    expect(out.minHoldDurationMin).toBe(10)
    expect(out.reentryCooldownMin).toBe(30)
    expect(out.minRiskRewardRatio).toBeGreaterThanOrEqual(1.0)
  })

  it('default style resets to baseline', () => {
    const out = applyTradingStyle('default', { ...baseForm })
    expect(out.maxPositions).toBe(3)
    expect(out.minHoldDurationMin).toBe(90)
  })
})
