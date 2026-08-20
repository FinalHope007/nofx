import type {
  CoinSourceConfig,
  DecisionContextConfig,
  RiskControlConfig,
  ScopeUnit,
  StrategyConfig,
} from '../../types/strategy'

export interface StrategyEditorForm {
  name: string
  custom_prompt: string
  scan_interval_minutes: number
  btcEthMaxLeverage: number
  altcoinMaxLeverage: number
  btcEthPositionRatio: number
  altcoinPositionRatio: number
  isCrossMargin: boolean
  selectedTimeframes: string[]
  excludedCoins: string[]
  decisionContext: DecisionContextConfig
  scopeUnit: ScopeUnit | null
  enableAI500Data?: boolean
  enableOIData?: boolean
  enableNetflowData?: boolean
  enablePriceData?: boolean
  enableBinanceTechnicalData?: boolean
  enableBinanceSentimentData?: boolean
  binanceTechnicalIntervals?: ('1h' | '24h')[]
  dataDurations?: string[]
  enableEma?: boolean
  enableMacd?: boolean
  enableRsi?: boolean
  enableOi?: boolean
  enableFundingRate?: boolean
  enableAtr?: boolean
  enableBoll?: boolean
  enableVolume?: boolean
  emaPeriods?: number[]
  rsiPeriods?: number[]
  atrPeriods?: number[]
  bollPeriods?: number[]
  primaryCount?: number
  longerTimeframe?: string
  longerCount?: number
  enableOIRanking?: boolean
  oiRankingDuration?: string
  oiRankingLimit?: number
  enableNetFlowRanking?: boolean
  netFlowRankingDuration?: string
  netFlowRankingLimit?: number
  enablePriceRanking?: boolean
  priceRankingDuration?: string
  priceRankingLimit?: number
  tradingStyle?: 'scalp' | 'intraday' | 'swing' | 'default'
  maxPositions: number
  minPositionSize: number
  minRiskRewardRatio: number
  maxMarginUsage: number
  minConfidence: number
  enableOILiquidityFilter: boolean
  oiLiquidityFilterMinUSDT: number
  maxOpensPerHour: number
  maxOpensPerCycle: number
  minHoldDurationMin: number
  noiseCloseHoldDurationMin: number
  reentryCooldownMin: number
  earlyCloseStopLossBypassPct: number
  earlyCloseTakeProfitBypassPct: number
  noiseCloseLossFloorPct: number
  noiseCloseProfitCeilingPct: number
}

export type TradingStyle = 'scalp' | 'intraday' | 'swing' | 'default'

export const TRADING_STYLE_PRESETS: Record<TradingStyle, Partial<StrategyEditorForm>> = {
  scalp: { maxPositions: 5, maxOpensPerHour: 8, maxOpensPerCycle: 4, minHoldDurationMin: 10, noiseCloseHoldDurationMin: 30, reentryCooldownMin: 30, minRiskRewardRatio: 1.5, minPositionSize: 12 },
  intraday: { maxPositions: 4, maxOpensPerHour: 5, maxOpensPerCycle: 3, minHoldDurationMin: 45, noiseCloseHoldDurationMin: 90, reentryCooldownMin: 120, minRiskRewardRatio: 2.0, minPositionSize: 12 },
  swing: { maxPositions: 2, maxOpensPerHour: 2, maxOpensPerCycle: 1, minHoldDurationMin: 360, noiseCloseHoldDurationMin: 720, reentryCooldownMin: 480, minRiskRewardRatio: 3.0, minPositionSize: 12 },
  // `default` preset mirrors defaultRiskControl() defaults so applying the
  // "Default" style is a true no-op; keep these fields in sync to avoid drift.
  default: { maxPositions: 2, maxOpensPerHour: 3, maxOpensPerCycle: 2, minHoldDurationMin: 90, noiseCloseHoldDurationMin: 180, reentryCooldownMin: 240, minRiskRewardRatio: 3.0, minPositionSize: 12 },
}

export function applyTradingStyle(style: TradingStyle, form: StrategyEditorForm): StrategyEditorForm {
  const patch = TRADING_STYLE_PRESETS[style]
  return { ...form, tradingStyle: style, ...patch }
}

function clamp(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

export function buildCoinSource(unit: ScopeUnit | null): CoinSourceConfig {
  if (!unit) {
    return {
      source_type: 'static',
      static_coins: [],
      excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }
  if (unit.source_type === 'hyper_rank') {
    return {
      source_type: 'hyper_rank',
      hyper_rank_category: unit.category,
      hyper_rank_direction: unit.variant as 'gainers' | 'losers' | 'volume' | undefined || 'gainers',
      hyper_rank_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit.source_type === 'ai500') {
    return {
      source_type: 'ai500',
      use_ai500: true,
      ai500_limit: clamp(unit.limit, 1, 50),
      static_coins: [],
      excluded_coins: [],
      use_oi_top: false,
      oi_top_limit: 0,
      use_oi_low: false,
      oi_low_limit: 0,
      use_hyper_all: false,
      use_hyper_main: false,
      vergex_limit: 0,
    }
  }

  if (unit.source_type === 'nofxos_oi') {
    const isTop = unit.variant === 'top'
    return {
      source_type: isTop ? 'oi_top' : 'oi_low',
      use_oi_top: isTop,
      oi_top_limit: isTop ? clamp(unit.limit, 1, 50) : 0,
      use_oi_low: !isTop,
      oi_low_limit: !isTop ? clamp(unit.limit, 1, 50) : 0,
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit.source_type === 'nofxos_netflow') {
    const isInflow = unit.variant !== 'outflow'
    return {
      source_type: isInflow ? 'netflow_top' : 'netflow_low',
      netflow_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit.source_type === 'nofxos_price') {
    const isGainers = unit.variant !== 'losers'
    return {
      source_type: isGainers ? 'price_top' : 'price_low',
      price_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit.source_type === 'vergex') {
    const marketType = unit.category === 'stock' ? 'hip3_perp' : 'core_perp'
    return {
      source_type: 'vergex_signal',
      vergex_limit: clamp(unit.limit, 1, 50),
      vergex_market_type: marketType,
      vergex_direction: unit.variant,
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false,
    }
  }

  if (unit.source_type === 'binance_technical') {
    return {
      source_type: 'binance_technical',
      binance_technical_interval: unit.interval ?? '1h',
      binance_technical_direction: unit.direction ?? 'top',
      binance_technical_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }
  if (unit.source_type === 'binance_sentiment') {
    return {
      source_type: 'binance_sentiment',
      binance_sentiment_direction: unit.direction ?? 'top',
      binance_sentiment_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  // Any other paid/provider-pending source: fall back to a safe static/empty
  // pool so runtime never errors today.
  return {
    source_type: 'static',
    static_coins: [],
    excluded_coins: [],
    use_ai500: false,
    ai500_limit: 0,
    use_oi_top: false,
    oi_top_limit: 0,
    use_oi_low: false,
    oi_low_limit: 0,
    use_hyper_all: false,
    use_hyper_main: false,
    vergex_limit: 0,
  }
}

export function defaultRiskControl(input?: {
  btcEthMaxLeverage?: number
  altcoinMaxLeverage?: number
  btcEthPositionRatio?: number
  altcoinPositionRatio?: number
  maxPositions?: number
  minPositionSize?: number
  minRiskRewardRatio?: number
  maxMarginUsage?: number
  minConfidence?: number
  enableOILiquidityFilter?: boolean
  oiLiquidityFilterMinUSDT?: number
  maxOpensPerHour?: number
  maxOpensPerCycle?: number
  minHoldDurationMin?: number
  noiseCloseHoldDurationMin?: number
  reentryCooldownMin?: number
  earlyCloseStopLossBypassPct?: number
  earlyCloseTakeProfitBypassPct?: number
  noiseCloseLossFloorPct?: number
  noiseCloseProfitCeilingPct?: number
}): RiskControlConfig {
  return {
    max_positions: clamp(input?.maxPositions ?? 2, 1, 8),
    btc_eth_max_leverage: clamp(input?.btcEthMaxLeverage ?? 5, 1, 20),
    altcoin_max_leverage: clamp(input?.altcoinMaxLeverage ?? 5, 1, 20),
    btc_eth_max_position_value_ratio: clamp(
      input?.btcEthPositionRatio ?? 5,
      0.5,
      10
    ),
    altcoin_max_position_value_ratio: clamp(
      input?.altcoinPositionRatio ?? 5,
      0.5,
      10
    ),
    max_margin_usage: clamp(input?.maxMarginUsage ?? 1.0, 0.1, 1.0),
    min_position_size: clamp(input?.minPositionSize ?? 12, 1, 1000000),
    min_risk_reward_ratio: clamp(input?.minRiskRewardRatio ?? 3, 0.1, 100),
    min_confidence: clamp(input?.minConfidence ?? 78, 0, 100),
    enable_oi_liquidity_filter: input?.enableOILiquidityFilter ?? true,
    oi_liquidity_filter_min_usdt: clamp(input?.oiLiquidityFilterMinUSDT ?? 15000000, 0, Number.MAX_SAFE_INTEGER),
    throttling: {
      max_opens_per_hour: clamp(input?.maxOpensPerHour ?? 3, 1, 100),
      max_opens_per_cycle: clamp(input?.maxOpensPerCycle ?? 2, 1, 100),
      min_hold_duration_min: clamp(input?.minHoldDurationMin ?? 90, 0, Number.MAX_SAFE_INTEGER),
      noise_close_hold_duration_min: clamp(input?.noiseCloseHoldDurationMin ?? 180, 0, Number.MAX_SAFE_INTEGER),
      reentry_cooldown_min: clamp(input?.reentryCooldownMin ?? 240, 0, Number.MAX_SAFE_INTEGER),
      early_close_stop_loss_bypass_pct: clamp(input?.earlyCloseStopLossBypassPct ?? -3.0, -100, 100),
      early_close_take_profit_bypass_pct: clamp(input?.earlyCloseTakeProfitBypassPct ?? 8.0, -100, 100),
      noise_close_loss_floor_pct: clamp(input?.noiseCloseLossFloorPct ?? -2.0, -100, 100),
      noise_close_profit_ceiling_pct: clamp(input?.noiseCloseProfitCeilingPct ?? 3.0, -100, 100),
    },
  }
}

export function buildStrategyConfig(form: StrategyEditorForm): StrategyConfig {
  return {
    strategy_type: 'ai_trading',
    language: 'en',
    trading_style: form.tradingStyle,
    ai_config: {
      coin_source: buildCoinSource(form.scopeUnit),
      indicators: {
        klines: {
          primary_timeframe: form.selectedTimeframes[0] ?? '15m',
          primary_count: form.primaryCount ?? 30,
          enable_multi_timeframe: form.selectedTimeframes.length > 1,
          selected_timeframes: form.selectedTimeframes,
          longer_timeframe: form.longerTimeframe || undefined,
          longer_count: form.longerCount && form.longerCount > 0 ? form.longerCount : undefined,
        },
        enable_raw_klines: true,
        enable_ema: form.enableEma ?? false,
        enable_macd: form.enableMacd ?? false,
        enable_rsi: form.enableRsi ?? false,
        enable_atr: form.enableAtr ?? false,
        enable_boll: form.enableBoll ?? false,
        enable_volume: form.enableVolume ?? false,
        ema_periods: form.emaPeriods?.length ? form.emaPeriods : undefined,
        rsi_periods: form.rsiPeriods?.length ? form.rsiPeriods : undefined,
        atr_periods: form.atrPeriods?.length ? form.atrPeriods : undefined,
        boll_periods: form.bollPeriods?.length ? form.bollPeriods : undefined,
        enable_oi: form.enableOi ?? false,
        enable_funding_rate: form.enableFundingRate ?? false,
        enable_ai500_data: form.enableAI500Data ?? false,
        enable_oi_data: form.enableOIData ?? false,
        enable_netflow_data: form.enableNetflowData ?? false,
        enable_price_data: form.enablePriceData ?? false,
        enable_binance_technical_data: form.enableBinanceTechnicalData ?? false,
        enable_binance_sentiment_data: form.enableBinanceSentimentData ?? false,
        binance_technical_intervals: form.binanceTechnicalIntervals?.length ? form.binanceTechnicalIntervals : undefined,
        data_durations: form.dataDurations && form.dataDurations.length ? form.dataDurations : undefined,
        nofxos_api_key: '',
        enable_quant_data: false,
        enable_quant_oi: false,
        enable_quant_netflow: false,
        enable_oi_ranking: form.enableOIRanking ?? false,
        oi_ranking_duration: form.oiRankingDuration ?? '1h',
        oi_ranking_limit: form.oiRankingLimit ?? 10,
        enable_netflow_ranking: form.enableNetFlowRanking ?? false,
        netflow_ranking_duration: form.netFlowRankingDuration ?? '1h',
        netflow_ranking_limit: form.netFlowRankingLimit ?? 10,
        enable_price_ranking: form.enablePriceRanking ?? false,
        price_ranking_duration: form.priceRankingDuration ?? '1h',
        price_ranking_limit: form.priceRankingLimit ?? 10,
      },
      custom_prompt: form.custom_prompt,
      risk_control: defaultRiskControl(form),
      decision_context: form.decisionContext,
    },
    grid_config: null,
  }
}
