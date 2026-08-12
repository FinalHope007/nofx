import { describe, expect, it } from 'vitest'
import { defaultDataSources } from './dataSourceDefaults'
import type { ScopeUnit } from '../../types/strategy'

function unit(source_type: ScopeUnit['source_type']): ScopeUnit {
  return {
    id: source_type,
    category: 'crypto',
    source_type,
    limit: 10,
    label: source_type,
    provider: 'free',
  }
}

const ai500 = unit('ai500')
const oi = unit('nofxos_oi')
const netflow = unit('nofxos_netflow')
const price = unit('nofxos_price')
const vergex = unit('vergex')

describe('defaultDataSources', () => {
  it('enables AI500 data when an ai500 unit is selected', () => {
    expect(defaultDataSources([ai500])).toEqual({
      enableAI500Data: true,
      enableOIData: false,
      enableNetflowData: false,
      enablePriceData: false,
    })
  })

  it('enables OI data when a nofxos_oi unit is selected', () => {
    expect(defaultDataSources([oi]).enableOIData).toBe(true)
  })

  it('enables netflow data when a nofxos_netflow unit is selected', () => {
    expect(defaultDataSources([netflow]).enableNetflowData).toBe(true)
  })

  it('enables price data when a nofxos_price unit is selected', () => {
    expect(defaultDataSources([price]).enablePriceData).toBe(true)
  })

  it('disables all four toggles for a vergex unit (Bias Radar exclusion)', () => {
    expect(defaultDataSources([vergex])).toEqual({
      enableAI500Data: false,
      enableOIData: false,
      enableNetflowData: false,
      enablePriceData: false,
    })
  })

  it('ORs the mapped sources across mixed units', () => {
    expect(defaultDataSources([ai500, oi, price])).toEqual({
      enableAI500Data: true,
      enableOIData: true,
      enableNetflowData: false,
      enablePriceData: true,
    })
  })

  it('disables all toggles when no units are selected', () => {
    expect(defaultDataSources([])).toEqual({
      enableAI500Data: false,
      enableOIData: false,
      enableNetflowData: false,
      enablePriceData: false,
    })
  })
})
