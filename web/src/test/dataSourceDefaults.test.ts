import { defaultDataSources } from '../features/strategies/dataSourceDefaults'

test('binance technical scope auto-enables binance technical detail', () => {
  const d = defaultDataSources({
    id: 'crypto-binance-technical', category: 'crypto', source_type: 'binance_technical',
    limit: 10, label: 'Binance Technical', provider: 'free', variant: 'binance',
  } as any)
  expect(d.enableBinanceTechnicalData).toBe(true)
  expect(d.enableBinanceSentimentData).toBe(false)
})
