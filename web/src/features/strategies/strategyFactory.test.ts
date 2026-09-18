import { describe, expect, it } from 'vitest'
import {
  buildCoinSource,
  buildStrategyConfig,
  defaultRiskControl,
} from './strategyFactory'
import type { ScopeUnit } from '../../types/strategy'

const freeUnit = (variant: 'gainers' | 'losers' | 'volume'): ScopeUnit => ({
  id: `crypto-${variant}`,
  category: 'crypto',
  source_type: 'hyper_rank',
  variant,
  limit: 10,
  label: `Top ${variant}`,
  provider: 'free',
})

describe('strategy factory', () => {
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
      id: 'crypto-bias-bull',
      category: 'crypto',
      source_type: 'vergex',
      limit: 10,
      label: 'Bias Radar (Bullish)',
      provider: 'paid',
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

  it('maps data-source toggles and durations into indicators', () => {
    const cfg = buildStrategyConfig({
      name: 'Test', custom_prompt: '', scan_interval_minutes: 15,
      btcEthMaxLeverage: 5, altcoinMaxLeverage: 5,
      btcEthPositionRatio: 5, altcoinPositionRatio: 5,
      isCrossMargin: true, selectedTimeframes: ['15m'], excludedCoins: [],
      decisionContext: { enabled: true, recent_count: 8, mode: 'structured' },
      scopeUnit: freeUnit('gainers'),
      enableAI500Data: true, enableOIData: true, enableNetflowData: true,
      enablePriceData: true, dataDurations: ['15m', '1h'],
      enableEma: true, enableMacd: true, enableRsi: true,
      maxPositions: 3, minPositionSize: 12, minRiskRewardRatio: 3.0,
      maxMarginUsage: 1.0, minConfidence: 78,
      enableOILiquidityFilter: true, oiLiquidityFilterMinUSDT: 15000000,
      maxOpensPerHour: 3, maxOpensPerCycle: 2,
      minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240,
      earlyCloseStopLossBypassPct: -3.0, earlyCloseTakeProfitBypassPct: 8.0,
      noiseCloseLossFloorPct: -2.0, noiseCloseProfitCeilingPct: 3.0,
    })
    const ind = cfg.ai_config?.indicators
    expect(ind?.enable_ai500_data).toBe(true)
    expect(ind?.enable_oi_data).toBe(true)
    expect(ind?.enable_netflow_data).toBe(true)
    expect(ind?.enable_price_data).toBe(true)
    expect(ind?.data_durations).toEqual(['15m', '1h'])
    expect(ind?.enable_ema).toBe(true)
    expect(ind?.enable_macd).toBe(true)
    expect(ind?.enable_rsi).toBe(true)
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
      scopeUnit: freeUnit('gainers'),
      maxPositions: 3, minPositionSize: 12, minRiskRewardRatio: 3.0,
      maxMarginUsage: 1.0, minConfidence: 78,
      enableOILiquidityFilter: true, oiLiquidityFilterMinUSDT: 15000000,
      maxOpensPerHour: 3, maxOpensPerCycle: 2,
      minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240,
      earlyCloseStopLossBypassPct: -3.0, earlyCloseTakeProfitBypassPct: 8.0,
      noiseCloseLossFloorPct: -2.0, noiseCloseProfitCeilingPct: 3.0,
    })
    expect(cfg.ai_config?.custom_prompt).toBe('hello')
    expect(cfg.ai_config?.decision_context).toEqual({
      enabled: true,
      recent_count: 8,
      mode: 'digest',
    })
  })

  it('emits an explicit empty indicator period list when the category is enabled', () => {
    const base = {
      name: 'Test', custom_prompt: '', scan_interval_minutes: 15,
      btcEthMaxLeverage: 5, altcoinMaxLeverage: 5,
      btcEthPositionRatio: 5, altcoinPositionRatio: 5,
      isCrossMargin: true, selectedTimeframes: ['15m'], excludedCoins: [],
      maxPositions: 3, minPositionSize: 12, minRiskRewardRatio: 3.0,
      maxMarginUsage: 1.0, minConfidence: 78,
      enableOILiquidityFilter: true, oiLiquidityFilterMinUSDT: 15000000,
      maxOpensPerHour: 3, maxOpensPerCycle: 2,
      minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240,
      earlyCloseStopLossBypassPct: -3.0, earlyCloseTakeProfitBypassPct: 8.0,
      noiseCloseLossFloorPct: -2.0, noiseCloseProfitCeilingPct: 3.0,
    }
    const enabled = buildStrategyConfig({ ...base, enableEma: true, emaPeriods: [] })
    expect(enabled.ai_config?.indicators?.ema_periods).toEqual([])

    const disabled = buildStrategyConfig({ ...base, enableEma: false, emaPeriods: [] })
    expect(disabled.ai_config?.indicators?.ema_periods).toBeUndefined()
  })

  it('buildCoinSource maps binance_technical unit to concrete source', () => {
    const cs = buildCoinSource({
      id: 'crypto-binance-technical',
      category: 'crypto',
      source_type: 'binance_technical',
      limit: 10,
      label: 'Binance Technical',
      provider: 'free',
      variant: 'binance',
      interval: '1h',
      direction: 'top',
    })
    expect(cs.source_type).toBe('binance_technical')
    expect(cs.binance_technical_interval).toBe('1h')
    expect(cs.binance_technical_direction).toBe('top')
    expect(cs.binance_technical_limit).toBe(10)
    expect(cs.scope_mode).toBeUndefined()
  })
})
