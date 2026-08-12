import type { ScopeUnit } from '../../types/strategy'

export function defaultDataSources(units: ScopeUnit[]): {
  enableAI500Data: boolean
  enableOIData: boolean
  enableNetflowData: boolean
  enablePriceData: boolean
} {
  return {
    enableAI500Data: units.some((u) => u.source_type === 'ai500'),
    enableOIData: units.some((u) => u.source_type === 'nofxos_oi'),
    enableNetflowData: units.some((u) => u.source_type === 'nofxos_netflow'),
    enablePriceData: units.some((u) => u.source_type === 'nofxos_price'),
  }
}
