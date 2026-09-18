import type { ScopeUnit } from '../../types/strategy'

export function defaultDataSources(scope: ScopeUnit | null): {
  enableAI500Data: boolean
  enableOIData: boolean
  enableNetflowData: boolean
  enablePriceData: boolean
  enableBinanceTechnicalData: boolean
  enableBinanceSentimentData: boolean
  enableAltFinsData: boolean
  enableVergexSignalLabData: boolean
  enableVergexHeatmapData: boolean
} {
  return {
    enableAI500Data: scope?.source_type === 'ai500',
    enableOIData: scope?.source_type === 'nofxos_oi',
    enableNetflowData: scope?.source_type === 'nofxos_netflow',
    enablePriceData: scope?.source_type === 'nofxos_price',
    enableBinanceTechnicalData: scope?.source_type === 'binance_technical',
    enableBinanceSentimentData: scope?.source_type === 'binance_sentiment',
    enableAltFinsData: false,
    enableVergexSignalLabData: false,
    enableVergexHeatmapData: false,
  }
}
