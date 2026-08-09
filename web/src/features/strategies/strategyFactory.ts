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
      hyper_rank_direction: unit.direction || 'gainers',
      hyper_rank_limit: clamp(unit.limit, 1, 50),
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

  if (unit?.source_type === 'vergex') {
    return {
      source_type: 'vergex_signal',
      scope_mode: mode,
      vergex_limit: clamp(unit.limit, 1, 50),
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
        enable_ema: false,
        enable_macd: false,
        enable_rsi: false,
        enable_atr: false,
        enable_boll: false,
        enable_volume: false,
        enable_oi: false,
        enable_funding_rate: false,
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
