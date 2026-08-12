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
  scopeUnits: ScopeUnit[]
  scopeMode: 'overlap' | 'union'
  enableAI500Data?: boolean
  enableOIData?: boolean
  enableNetflowData?: boolean
  enablePriceData?: boolean
  dataDurations?: string[]
  enableEma?: boolean
  enableMacd?: boolean
  enableRsi?: boolean
  enableOi?: boolean
  enableFundingRate?: boolean
}

function clamp(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

export function buildCoinSource(
  units: ScopeUnit[],
  mode: 'overlap' | 'union' = 'union'
): CoinSourceConfig {
  if (units.length > 1) {
    return {
      source_type: 'custom',
      scope_mode: mode,
      custom_scope: { scope_units: units, mode },
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
      hyper_main_limit: 0,
      vergex_limit: 0,
    }
  }

  const unit = units[0]
  if (unit?.source_type === 'hyper_rank') {
    return {
      source_type: 'hyper_rank',
      scope_mode: mode,
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

  if (unit?.source_type === 'ai500') {
    return {
      source_type: 'ai500',
      scope_mode: mode,
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

  if (unit?.source_type === 'nofxos_oi') {
    const isTop = unit.variant === 'top'
    return {
      source_type: isTop ? 'oi_top' : 'oi_low',
      scope_mode: mode,
      use_oi_top: isTop,
      oi_top_limit: isTop ? clamp(unit.limit, 1, 50) : 0,
      use_oi_low: !isTop,
      oi_low_limit: !isTop ? clamp(unit.limit, 1, 50) : 0,
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'nofxos_netflow') {
    const isInflow = unit.variant !== 'outflow'
    return {
      source_type: isInflow ? 'netflow_top' : 'netflow_low',
      scope_mode: mode,
      netflow_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'nofxos_price') {
    const isGainers = unit.variant !== 'losers'
    return {
      source_type: isGainers ? 'price_top' : 'price_low',
      scope_mode: mode,
      price_limit: clamp(unit.limit, 1, 50),
      static_coins: [], excluded_coins: [],
      use_ai500: false, ai500_limit: 0,
      use_oi_top: false, oi_top_limit: 0,
      use_oi_low: false, oi_low_limit: 0,
      use_hyper_all: false, use_hyper_main: false, vergex_limit: 0,
    }
  }

  if (unit?.source_type === 'vergex') {
    const marketType = unit.category === 'stock' ? 'hip3_perp' : 'core_perp'
    return {
      source_type: 'vergex_signal',
      scope_mode: mode,
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

  // Any other paid/provider-pending source: mark custom with the units but
  // fall back to a safe static/empty pool so runtime never errors today.
  return {
    source_type: 'custom',
    scope_mode: mode,
    custom_scope: { scope_units: units, mode },
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
}): RiskControlConfig {
  return {
    max_positions: 2,
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
    max_margin_usage: 1.0,
    min_position_size: 12,
    min_risk_reward_ratio: 3,
    min_confidence: 78,
  }
}

export function buildStrategyConfig(form: StrategyEditorForm): StrategyConfig {
  return {
    strategy_type: 'ai_trading',
    language: 'en',
    ai_config: {
      coin_source: buildCoinSource(form.scopeUnits, form.scopeMode),
      indicators: {
        klines: {
          primary_timeframe: form.selectedTimeframes[0] ?? '15m',
          primary_count: 30,
          enable_multi_timeframe: form.selectedTimeframes.length > 1,
          selected_timeframes: form.selectedTimeframes,
        },
        enable_raw_klines: true,
        enable_ema: form.enableEma ?? false,
        enable_macd: form.enableMacd ?? false,
        enable_rsi: form.enableRsi ?? false,
        enable_atr: false,
        enable_boll: false,
        enable_volume: false,
        enable_oi: form.enableOi ?? false,
        enable_funding_rate: form.enableFundingRate ?? false,
        enable_ai500_data: form.enableAI500Data ?? false,
        enable_oi_data: form.enableOIData ?? false,
        enable_netflow_data: form.enableNetflowData ?? false,
        enable_price_data: form.enablePriceData ?? false,
        data_durations: form.dataDurations && form.dataDurations.length ? form.dataDurations : undefined,
        nofxos_api_key: '',
        enable_quant_data: false,
        enable_quant_oi: false,
        enable_quant_netflow: false,
        enable_oi_ranking: false,
        enable_netflow_ranking: false,
        enable_price_ranking: false,
      },
      custom_prompt: form.custom_prompt,
      risk_control: defaultRiskControl(form),
      decision_context: form.decisionContext,
    },
    grid_config: null,
  }
}
