import type { ScopeUnit, ScopeVariant } from '../../types/strategy'

export interface ScopeCardDef {
  id: string
  category: 'crypto' | 'stock'
  label: string
  description: string
  provider: 'free' | 'paid'
  source_type: ScopeUnit['source_type']
  variant?: ScopeVariant
  defaultLimit: number
}

// Free = Hyperliquid-native, works with existing backend today.
const freeHyperRank = (
  id: string,
  label: string,
  description: string,
  variant: ScopeVariant
): ScopeCardDef => ({
  id,
  category: 'crypto',
  label,
  description,
  provider: 'free',
  source_type: 'hyper_rank',
  variant,
  defaultLimit: 10,
})

const freeBinanceOpportunity = (
  id: string,
  label: string,
  description: string,
  source_type: ScopeUnit['source_type'],
): ScopeCardDef => ({
  id,
  category: 'crypto',
  label,
  description,
  provider: 'free',
  source_type,
  variant: 'binance',
  defaultLimit: 10,
})

export const SCOPE_CARD_DEFS: ScopeCardDef[] = [
  freeHyperRank(
    'crypto-top-gainers',
    'Crypto Top Gainers',
    'Top % gainers on Hyperliquid',
    'gainers'
  ),
  freeHyperRank(
    'crypto-top-losers',
    'Crypto Top Losers',
    'Top % losers on Hyperliquid',
    'losers'
  ),
  freeHyperRank(
    'crypto-trending',
    'Crypto Trending · Top Volume',
    'Top traders by volume on Hyperliquid',
    'volume'
  ),

  freeBinanceOpportunity('crypto-binance-technical', 'Binance Technical', 'Top technical score (1h/24h) on Binance Opportunity', 'binance_technical'),
  freeBinanceOpportunity('crypto-binance-sentiment', 'Binance Sentiment', 'Top sentiment score on Binance Opportunity', 'binance_sentiment'),

  // Crypto — PAID / provider pending
  {
    id: 'crypto-bias-bull',
    category: 'crypto',
    label: 'Bias Radar (Bullish)',
    description: 'VergeX bullish bias radar',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'bull',
    defaultLimit: 10,
  },
  {
    id: 'crypto-bias-bear',
    category: 'crypto',
    label: 'Bias Radar (Bearish)',
    description: 'VergeX bearish bias radar',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'bear',
    defaultLimit: 10,
  },
  {
    id: 'crypto-ai500',
    category: 'crypto',
    label: 'AI500 Data Provider',
    description: 'AI500 data provider (nofxos)',
    provider: 'paid',
    source_type: 'ai500',
    defaultLimit: 10,
  },
  {
    id: 'crypto-oi-increase',
    category: 'crypto',
    label: 'OI Increase',
    description: 'Open interest increase (nofxos)',
    provider: 'paid',
    source_type: 'nofxos_oi',
    variant: 'top',
    defaultLimit: 10,
  },
  {
    id: 'crypto-oi-decrease',
    category: 'crypto',
    label: 'OI Decrease',
    description: 'Open interest decrease (nofxos)',
    provider: 'paid',
    source_type: 'nofxos_oi',
    variant: 'low',
    defaultLimit: 10,
  },
  {
    id: 'crypto-netflow-top',
    category: 'crypto',
    label: 'Netflow Top',
    description: 'Top netflow (nofxos)',
    provider: 'paid',
    source_type: 'nofxos_netflow',
    variant: 'inflow',
    defaultLimit: 10,
  },
  {
    id: 'crypto-netflow-outflow',
    category: 'crypto',
    label: 'Netflow Outflow Top',
    description: 'Top net outflow (nofxos)',
    provider: 'paid',
    source_type: 'nofxos_netflow',
    variant: 'outflow',
    defaultLimit: 10,
  },
  {
    id: 'crypto-gainers-nofxos',
    category: 'crypto',
    label: 'Crypto Top Gainers (NOFXOS)',
    description: 'Top gainers via nofxos',
    provider: 'paid',
    source_type: 'nofxos_price',
    variant: 'gainers',
    defaultLimit: 10,
  },
  {
    id: 'crypto-losers-nofxos',
    category: 'crypto',
    label: 'Crypto Top Losers (NOFXOS)',
    description: 'Top losers via nofxos',
    provider: 'paid',
    source_type: 'nofxos_price',
    variant: 'losers',
    defaultLimit: 10,
  },

  // Stock — PAID, VergeX/Claw402
  {
    id: 'stock-bias-bull',
    category: 'stock',
    label: 'Bias Radar (Bullish)',
    description: 'VergeX US-stock bullish bias radar',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'bull',
    defaultLimit: 10,
  },
  {
    id: 'stock-bias-bear',
    category: 'stock',
    label: 'Bias Radar (Bearish)',
    description: 'VergeX US-stock bearish bias radar',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'bear',
    defaultLimit: 10,
  },
  {
    id: 'stock-trending',
    category: 'stock',
    label: 'Trending Stocks',
    description: 'Trending US stocks (VergeX)',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'trending',
    defaultLimit: 10,
  },
  {
    id: 'stock-gainers',
    category: 'stock',
    label: 'Stock Gainers',
    description: 'US stock gainers (VergeX)',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'gainers',
    defaultLimit: 10,
  },
  {
    id: 'stock-losers',
    category: 'stock',
    label: 'Stock Losers',
    description: 'US stock losers (VergeX)',
    provider: 'paid',
    source_type: 'vergex',
    variant: 'losers',
    defaultLimit: 10,
  },
]

export function toScopeUnit(def: ScopeCardDef, limit: number): ScopeUnit {
  return {
    id: def.id,
    category: def.category,
    source_type: def.source_type,
    limit,
    label: def.label,
    provider: def.provider,
    ...(def.variant ? { variant: def.variant } : {}),
  }
}
