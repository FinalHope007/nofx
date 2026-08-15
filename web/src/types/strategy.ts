// Strategy Studio Types
export interface Strategy {
  id: string;
  name: string;
  description: string;
  is_active: boolean;
  is_default: boolean;
  is_public: boolean;           // Whether published in the strategy marketplace
  config_visible: boolean;      // Whether the config parameters are publicly visible
  config: StrategyConfig;
  created_at: string;
  updated_at: string;
}

// Strategy usage statistics
export interface StrategyStats {
  clone_count: number;          // Number of times cloned
  active_users: number;         // Current number of users
  top_performers?: StrategyPerformer[];  // Performance ranking
}

// Strategy user performance ranking
export interface StrategyPerformer {
  user_id: string;
  user_name: string;            // Masked user name
  total_pnl_pct: number;        // Total return percentage
  total_pnl: number;            // Total PnL amount
  win_rate: number;             // Win rate
  trade_count: number;          // Number of trades
  using_since: string;          // Start time of usage
  rank: number;                 // Rank
}

export interface PromptSectionsConfig {
  role_definition?: string;
  trading_frequency?: string;
  entry_standards?: string;
  decision_process?: string;
}

export interface StrategyConfig {
  // Strategy type: "ai_trading" (default) or "grid_trading"
  strategy_type?: 'ai_trading' | 'grid_trading';
  // Language setting: "zh" for Chinese, "en" for English
  // Determines the language used for data formatting and prompt generation
  language?: 'zh' | 'en';
  trading_style?: 'scalp' | 'intraday' | 'swing' | 'default';
  // AI trading configuration. Legacy flat fields below are accepted only for
  // old data returned before the schema was split by strategy type.
  ai_config?: AIStrategyConfig;
  coin_source?: CoinSourceConfig;
  indicators?: IndicatorConfig;
  custom_prompt?: string;
  risk_control?: RiskControlConfig;
  prompt_sections?: PromptSectionsConfig;
  // Grid trading configuration (only used when strategy_type is 'grid_trading')
  grid_config?: GridStrategyConfig | null;
  publish_config?: PublishStrategyConfig;
}

export interface AIStrategyConfig {
  coin_source: CoinSourceConfig;
  indicators: IndicatorConfig;
  custom_prompt?: string;
  risk_control: RiskControlConfig;
  prompt_sections?: PromptSectionsConfig;
  decision_context?: DecisionContextConfig
}

export interface PublishStrategyConfig {
  is_public: boolean;
  config_visible: boolean;
}

// Grid trading specific configuration
export interface GridStrategyConfig {
  // Trading pair (e.g., "BTCUSDT")
  symbol: string;
  // Number of grid levels (5-50)
  grid_count: number;
  // Total investment in USDT
  total_investment: number;
  // Leverage (1-20)
  leverage: number;
  // Upper price boundary (0 = auto-calculate from ATR)
  upper_price: number;
  // Lower price boundary (0 = auto-calculate from ATR)
  lower_price: number;
  // Use ATR to auto-calculate bounds
  use_atr_bounds: boolean;
  // ATR multiplier for bound calculation (default 2.0)
  atr_multiplier: number;
  // Position distribution: "uniform" | "gaussian" | "pyramid"
  distribution: 'uniform' | 'gaussian' | 'pyramid';
  // Maximum drawdown percentage before emergency exit
  max_drawdown_pct: number;
  // Stop loss percentage per position
  stop_loss_pct: number;
  // Daily loss limit percentage
  daily_loss_limit_pct: number;
  // Use maker-only orders for lower fees
  use_maker_only: boolean;
  // Enable automatic grid direction adjustment based on box breakouts
  enable_direction_adjust?: boolean;
  // Direction bias ratio for long_bias/short_bias modes (default 0.7 = 70%/30%)
  direction_bias_ratio?: number;
}

export interface CoinSourceConfig {
  source_type:
    | 'static'
    | 'ai500'
    | 'oi_top'
    | 'oi_low'
    | 'netflow_top'
    | 'netflow_low'
    | 'price_top'
    | 'price_low'
    | 'hyper_all'
    | 'hyper_main'
    | 'hyper_rank'
    | 'vergex_signal'
    | 'custom'
  custom_scope?: CustomScopeConfig
  scope_mode?: 'overlap' | 'union'
  static_coins?: string[];
  excluded_coins?: string[];   // List of excluded coins
  use_ai500: boolean;
  ai500_limit?: number;
  use_oi_top: boolean;
  oi_top_limit?: number;
  use_oi_low: boolean;
  oi_low_limit?: number;
  use_hyper_all?: boolean;
  use_hyper_main?: boolean;
  hyper_main_limit?: number;
  hyper_rank_category?: 'stock' | 'commodity' | 'index' | 'forex' | 'pre_ipo' | 'crypto' | 'all';
  hyper_rank_direction?: 'gainers' | 'losers' | 'volume';
  hyper_rank_limit?: number;
  // Netflow / price ranking pool (candidate source) limits
  netflow_limit?: number;
  price_limit?: number;
  // Vergex sub-card selector: 'bull'|'bear'|'trending'|'gainers'|'losers'
  vergex_direction?: string;
  vergex_limit?: number;
  vergex_market_type?: string;
  vergex_chain?: string;
  vergex_liq_band?: string;
  // Note: API URLs are now built automatically using nofxos_api_key from IndicatorConfig
}

export interface IndicatorConfig {
  klines: KlineConfig;
  // Raw OHLCV kline data - required for AI analysis
  enable_raw_klines: boolean;
  // Technical indicators (optional)
  enable_ema: boolean;
  enable_macd: boolean;
  enable_rsi: boolean;
  enable_atr: boolean;
  enable_boll: boolean;
  enable_volume: boolean;
  enable_oi: boolean;
  enable_funding_rate: boolean;
  ema_periods?: number[];
  rsi_periods?: number[];
  atr_periods?: number[];
  boll_periods?: number[];
  external_data_sources?: ExternalDataSource[];

  // ========== Unified NofxOS data source configuration ==========
  // Unified NofxOS API Key - used for all NofxOS data sources
  nofxos_api_key?: string;

  // Quant data sources (fund flow, open interest changes, price changes)
  enable_quant_data?: boolean;
  enable_quant_oi?: boolean;
  enable_quant_netflow?: boolean;

  // OI ranking data (market open interest increase/decrease ranking)
  enable_oi_ranking?: boolean;
  oi_ranking_duration?: string;  // "1h", "4h", "24h"
  oi_ranking_limit?: number;

  // NetFlow ranking data (institutional/retail fund flow ranking)
  enable_netflow_ranking?: boolean;
  netflow_ranking_duration?: string;  // "1h", "4h", "24h"
  netflow_ranking_limit?: number;

  // Price ranking data (gainers/losers ranking)
  enable_price_ranking?: boolean;
  price_ranking_duration?: string;  // "1h", "4h", "24h" or "1h,4h,24h"
  price_ranking_limit?: number;

  // Free per-coin data sources exposed to the LLM prompt (optional toggles)
  enable_ai500_data?: boolean;
  enable_oi_data?: boolean;
  enable_netflow_data?: boolean;
  enable_price_data?: boolean;
  data_durations?: string[];
}

export interface KlineConfig {
  primary_timeframe: string;
  primary_count: number;
  longer_timeframe?: string;
  longer_count?: number;
  enable_multi_timeframe: boolean;
  // Added: support selecting multiple timeframes
  selected_timeframes?: string[];
}

export interface ExternalDataSource {
  name: string;
  type: 'api' | 'webhook';
  url: string;
  method: string;
  headers?: Record<string, string>;
  data_path?: string;
  refresh_secs?: number;
}

export interface RiskControlConfig {
  // Max number of coins held simultaneously (CODE ENFORCED)
  max_positions: number;

  // Trading Leverage - exchange leverage for opening positions (AI guided)
  btc_eth_max_leverage: number;    // BTC/ETH max exchange leverage
  altcoin_max_leverage: number;    // Altcoin max exchange leverage

  // Position Value Ratio - single position notional value / account equity (CODE ENFORCED)
  // Max position value = equity × this ratio
  btc_eth_max_position_value_ratio?: number;     // default: 5 (BTC/ETH max position = 5x equity)
  altcoin_max_position_value_ratio?: number;     // default: 1 (Altcoin max position = 1x equity)

  // Risk Parameters
  max_margin_usage: number;        // Max margin utilization, e.g. 0.9 = 90% (CODE ENFORCED)
  min_position_size: number;       // Min position size in USDT (CODE ENFORCED)
  min_risk_reward_ratio: number;   // Min take_profit / stop_loss ratio (AI guided)
  min_confidence: number;          // Min AI confidence to open position (AI guided)
  enable_oi_liquidity_filter: boolean;
  oi_liquidity_filter_min_usdt: number;
  throttling: ThrottlingConfig;
}

export interface ThrottlingConfig {
  max_opens_per_hour: number;
  max_opens_per_cycle: number;
  min_hold_duration_min: number;
  noise_close_hold_duration_min: number;
  reentry_cooldown_min: number;
  early_close_stop_loss_bypass_pct: number;
  early_close_take_profit_bypass_pct: number;
  noise_close_loss_floor_pct: number;
  noise_close_profit_ceiling_pct: number;
}

// A single selected trading-scope card (from the scope wizard step).
export type ScopeVariant =
  | 'bull' | 'bear' | 'trending'   // vergex bias + stock trending
  | 'gainers' | 'losers'           // hyper_rank / vergex movers / price
  | 'top' | 'low'                  // oi top/low, netflow, price (rank array)
  | 'inflow' | 'outflow'           // netflow direction
  | 'volume'                       // hyper_rank volume

export interface ScopeUnit {
  id: string
  category: 'crypto' | 'stock'
  source_type:
    | 'hyper_rank'
    | 'ai500'
    | 'oi_top'
    | 'oi_low'
    | 'vergex'
    | 'nofxos_netflow'
    | 'nofxos_oi'
    | 'nofxos_price'
    | 'other'
  // Per-source rank-direction qualifier so the backend can pick the exact
  // free endpoint (and top/low array) for the chosen card.
  variant?: ScopeVariant
  limit: number
  label: string
  provider: 'free' | 'paid'
}

// The full multi-scope selection plus Overlap/Union mode. Stored on the
// strategy so the future backend can resolve candidate pools in AND/OR form.
export interface CustomScopeConfig {
  scope_units: ScopeUnit[]
  mode: 'overlap' | 'union'
}

// Runtime prompt-context knobs. Backend prompt-builder wiring is pending;
// the frontend persists these so no data is lost.
export interface DecisionContextConfig {
  enabled: boolean
  recent_count: number
  mode: 'structured' | 'digest'
}
