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
  it('maps a single free scope to its concrete coin source', () => {
    const cs = buildCoinSource([freeUnit('gainers')])
    expect(cs.source_type).toBe('hyper_rank')
    expect(cs.hyper_rank_category).toBe('crypto')
    expect(cs.hyper_rank_direction).toBe('gainers')
    expect(cs.hyper_rank_limit).toBe(10)
    expect(cs.scope_mode).toBe('union')
  })

  it('maps a single paid scope to its own concrete source', () => {
    const paid: ScopeUnit = {
      id: 'crypto-bias-bull',
      category: 'crypto',
      source_type: 'vergex',
      limit: 10,
      label: 'Bias Radar (Bullish)',
      provider: 'paid',
    }
    const cs = buildCoinSource([paid])
    expect(cs.source_type).toBe('vergex_signal')
    expect(cs.custom_scope).toBeUndefined()
  })

  it('uses custom source_type when more than one scope is selected', () => {
    const cs = buildCoinSource([freeUnit('gainers'), freeUnit('losers')])
    expect(cs.source_type).toBe('custom')
    expect(cs.custom_scope?.scope_units).toHaveLength(2)
    expect(cs.custom_scope?.mode).toBe('union')
  })

  it('persists overlap mode on a multi-scope custom source', () => {
    const cs = buildCoinSource(
      [freeUnit('gainers'), freeUnit('losers')],
      'overlap'
    )
    expect(cs.source_type).toBe('custom')
    expect(cs.scope_mode).toBe('overlap')
    expect(cs.custom_scope?.mode).toBe('overlap')
  })

  it('persists overlap mode on a single concrete scope', () => {
    const cs = buildCoinSource([freeUnit('gainers')], 'overlap')
    expect(cs.source_type).toBe('hyper_rank')
    expect(cs.scope_mode).toBe('overlap')
    expect(cs.custom_scope).toBeUndefined()
  })

  it('buildStrategyConfig passes scopeMode into the coin source for a multi-scope form', () => {
    const cfg = buildStrategyConfig({
      name: 'Overlap',
      custom_prompt: '',
      scan_interval_minutes: 15,
      btcEthMaxLeverage: 5,
      altcoinMaxLeverage: 5,
      btcEthPositionRatio: 5,
      altcoinPositionRatio: 5,
      isCrossMargin: true,
      selectedTimeframes: ['15m'],
      excludedCoins: [],
      decisionContext: { enabled: true, recent_count: 8, mode: 'structured' },
      scopeUnits: [freeUnit('gainers'), freeUnit('losers')],
      scopeMode: 'overlap',
    })
    const cs = cfg.ai_config?.coin_source
    expect(cs?.source_type).toBe('custom')
    expect(cs?.scope_mode).toBe('overlap')
    expect(cs?.custom_scope?.mode).toBe('overlap')
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
      scopeUnits: [freeUnit('gainers')],
      scopeMode: 'union',
    })
    expect(cfg.ai_config?.custom_prompt).toBe('hello')
    expect(cfg.ai_config?.decision_context).toEqual({
      enabled: true,
      recent_count: 8,
      mode: 'digest',
    })
  })
})
