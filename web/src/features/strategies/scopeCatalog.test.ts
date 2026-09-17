import { describe, expect, it } from 'vitest'
import { SCOPE_CARD_DEFS, toScopeUnit } from './scopeCatalog'

describe('scope catalog', () => {
  it('exposes free crypto scope cards', () => {
    const free = SCOPE_CARD_DEFS.filter((c) => c.provider === 'free')
    expect(free.length).toBe(5)
    expect(free.every((c) => c.category === 'crypto')).toBe(true)
  })

  it('marks exactly the two Binance opportunity cards as discontinued', () => {
    const discontinued = SCOPE_CARD_DEFS.filter((c) => c.discontinued)
    expect(discontinued.map((c) => c.id)).toEqual([
      'crypto-binance-technical',
      'crypto-binance-sentiment',
    ])
  })

  it('has paid bias-radar stock cards', () => {
    const bias = SCOPE_CARD_DEFS.filter((c) => c.label.includes('Bias Radar'))
    expect(bias.length).toBe(4) // 2 crypto + 2 stock
    expect(bias.every((c) => c.provider === 'paid')).toBe(true)
  })

  it('produces a ScopeUnit carrying the limit and only relevant direction', () => {
    const gainers = SCOPE_CARD_DEFS.find((c) => c.id === 'crypto-top-gainers')!
    const unit = toScopeUnit(gainers, 10)
    expect(unit.source_type).toBe('hyper_rank')
    expect(unit.variant).toBe('gainers')
    expect(unit.limit).toBe(10)
    expect(unit.provider).toBe('free')

    const ai500 = SCOPE_CARD_DEFS.find((c) => c.id === 'crypto-ai500')!
    const aiUnit = toScopeUnit(ai500, 5)
    expect(aiUnit.source_type).toBe('ai500')
    expect(aiUnit.variant).toBeUndefined()
    expect(aiUnit.limit).toBe(5)
  })
})
