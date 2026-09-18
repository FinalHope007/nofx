package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nofx/logger"
	"nofx/market"
	"nofx/provider/altfins"
	"nofx/provider/binance"
	"nofx/provider/hyperliquid"
	"nofx/provider/nofxos"
	"nofx/provider/vergex"
	"nofx/security"
	"nofx/store"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Type Definitions
// ============================================================================

// PositionInfo position information
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	PeakPnLPct       float64 `json:"peak_pnl_pct"` // Historical peak profit percentage
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // Position update timestamp (milliseconds)
	EntryTime        int64   `json:"entry_time"`
	StopLoss         float64 `json:"stop_loss"`
	TakeProfit       float64 `json:"take_profit"`
}

// AccountInfo account information
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // Account equity
	AvailableBalance float64 `json:"available_balance"` // Available balance
	UnrealizedPnL    float64 `json:"unrealized_pnl"`    // Unrealized profit/loss
	TotalPnL         float64 `json:"total_pnl"`         // Total profit/loss
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // Total profit/loss percentage
	MarginUsed       float64 `json:"margin_used"`       // Used margin
	MarginUsedPct    float64 `json:"margin_used_pct"`   // Margin usage rate
	PositionCount    int     `json:"position_count"`    // Number of positions
}

// CandidateCoin candidate coin (from coin pool)
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // Sources: "ai500" and/or "oi_top"
}

// OITopData open interest growth top data (for AI decision reference)
type OITopData struct {
	Rank              int     // OI Top ranking
	OIDeltaPercent    float64 // Open interest change percentage (1 hour)
	OIDeltaValue      float64 // Open interest change value
	PriceDeltaPercent float64 // Price change percentage
}

// TradingStats trading statistics (for AI input)
type TradingStats struct {
	TotalTrades    int     `json:"total_trades"`     // Total number of trades (closed)
	WinRate        float64 `json:"win_rate"`         // Win rate (%)
	ProfitFactor   float64 `json:"profit_factor"`    // Profit factor
	SharpeRatio    float64 `json:"sharpe_ratio"`     // Sharpe ratio
	TotalPnL       float64 `json:"total_pnl"`        // Total profit/loss
	AvgWin         float64 `json:"avg_win"`          // Average win
	AvgLoss        float64 `json:"avg_loss"`         // Average loss
	MaxDrawdownPct float64 `json:"max_drawdown_pct"` // Maximum drawdown (%)
}

// RecentOrder recently completed order (for AI input)
type RecentOrder struct {
	Symbol       string  `json:"symbol"`        // Trading pair
	Side         string  `json:"side"`          // long/short
	EntryPrice   float64 `json:"entry_price"`   // Entry price
	ExitPrice    float64 `json:"exit_price"`    // Exit price
	RealizedPnL  float64 `json:"realized_pnl"`  // Realized profit/loss
	PnLPct       float64 `json:"pnl_pct"`       // Profit/loss percentage
	EntryTime    string  `json:"entry_time"`    // Entry time
	ExitTime     string  `json:"exit_time"`     // Exit time
	HoldDuration string  `json:"hold_duration"` // Hold duration, e.g. "2h30m"
	CloseReason  string  `json:"close_reason"`  // Close reason: llm/tp/sl/exchange
}

// Context trading context (complete information passed to AI)
type Context struct {
	CurrentTime        string                             `json:"current_time"`
	RuntimeMinutes     int                                `json:"runtime_minutes"`
	CallCount          int                                `json:"call_count"`
	Account            AccountInfo                        `json:"account"`
	Positions          []PositionInfo                     `json:"positions"`
	CandidateCoins     []CandidateCoin                    `json:"candidate_coins"`
	PromptVariant      string                             `json:"prompt_variant,omitempty"`
	TradingStats       *TradingStats                      `json:"trading_stats,omitempty"`
	RecentOrders       []RecentOrder                      `json:"recent_orders,omitempty"`
	MarketDataMap      map[string]*market.Data            `json:"-"`
	MultiTFMarket      map[string]map[string]*market.Data `json:"-"`
	OITopDataMap       map[string]*OITopData              `json:"-"`
	QuantDataMap       map[string]*QuantData              `json:"-"`
	VergexDataMap      map[string]*vergex.MarketAnalysis  `json:"-"`
	OIRankingData      *nofxos.OIRankingData              `json:"-"` // Market-wide OI ranking data
	NetFlowRankingData *nofxos.NetFlowRankingData         `json:"-"` // Market-wide fund flow ranking data
	PriceRankingData   *nofxos.PriceRankingData           `json:"-"` // Market-wide price gainers/losers
	BTCETHLeverage     int                                `json:"-"`
	AltcoinLeverage    int                                `json:"-"`
	Timeframes         []string                           `json:"-"`
	Ctx                context.Context                    `json:"-"` // Go context for cancellable API calls
}

// Decision AI trading decision
type Decision struct {
	Symbol string `json:"symbol"`
	Action string `json:"action"` // Standard: "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	// Grid actions: "place_buy_limit", "place_sell_limit", "cancel_order", "cancel_all_orders", "pause_grid", "resume_grid", "adjust_grid"

	// Opening position parameters
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`

	// Grid trading parameters
	Price      float64 `json:"price,omitempty"`       // Limit order price (for grid)
	Quantity   float64 `json:"quantity,omitempty"`    // Order quantity (for grid)
	LevelIndex int     `json:"level_index,omitempty"` // Grid level index
	OrderID    string  `json:"order_id,omitempty"`    // Order ID (for cancel)

	// Common parameters
	Confidence int     `json:"confidence,omitempty"` // Confidence level (0-100)
	RiskUSD    float64 `json:"risk_usd,omitempty"`   // Maximum USD risk
	Reasoning  string  `json:"reasoning"`
}

// FullDecision AI's complete decision (including chain of thought)
type FullDecision struct {
	SystemPrompt        string     `json:"system_prompt"`
	UserPrompt          string     `json:"user_prompt"`
	CoTTrace            string     `json:"cot_trace"`
	Decisions           []Decision `json:"decisions"`
	RawResponse         string     `json:"raw_response"`
	Timestamp           time.Time  `json:"timestamp"`
	AIRequestDurationMs int64      `json:"ai_request_duration_ms,omitempty"`
}

// QuantData quantitative data structure (fund flow, position changes, price changes)
type QuantData struct {
	Symbol      string             `json:"symbol"`
	Price       float64            `json:"price"`
	Netflow     *NetflowData       `json:"netflow,omitempty"`
	OI          map[string]*OIData `json:"oi,omitempty"`
	PriceChange map[string]float64 `json:"price_change,omitempty"`
}

type NetflowData struct {
	Institution *FlowTypeData `json:"institution,omitempty"`
	Personal    *FlowTypeData `json:"personal,omitempty"`
}

type FlowTypeData struct {
	Future map[string]float64 `json:"future,omitempty"`
	Spot   map[string]float64 `json:"spot,omitempty"`
}

type OIData struct {
	CurrentOI float64                 `json:"current_oi"`
	Delta     map[string]*OIDeltaData `json:"delta,omitempty"`
}

type OIDeltaData struct {
	OIDelta        float64 `json:"oi_delta"`
	OIDeltaValue   float64 `json:"oi_delta_value"`
	OIDeltaPercent float64 `json:"oi_delta_percent"`
}

// ============================================================================
// StrategyEngine - Core Strategy Execution Engine
// ============================================================================

// PerCoinSignal carries the free per-coin enrichment data for one symbol
// (AI500 score, OI / netflow / price change per selected duration).
type PerCoinSignal struct {
	AI500            *nofxos.CoinData                        // may be nil
	OI               map[string]map[string]nofxos.OIPosition // duration -> list(top/low) -> data
	Netflow          map[string]map[string]nofxos.NetFlowPosition
	Price            map[string]map[string]nofxos.PriceRankingItem
	BinanceTechnical map[string]*binance.BinanceAssetDetail // interval -> parsed detail (may be nil)
	BinanceSentiment map[string]string                      // flattened sentiment detail labels (may be nil)
	AltFins          map[string]*altfins.Analytics          // interval -> parsed analytics
	VergexSignalLab  json.RawMessage                        // vergex.trade SignalLab (non-vergex sources)
	VergexHeatmap    json.RawMessage                        // vergex.trade liquidation heatmap
}

// binanceOpportunityGetter abstracts the Binance Opportunity client for
// testability. *binance.OpportunityClient satisfies this interface.
type binanceOpportunityGetter interface {
	GetOpportunityAssets(ctx context.Context, interval, scene string) ([]binance.OpportunityAsset, error)
	GetAssetDetails(ctx context.Context, symbol, scene, interval string) (*binance.BinanceAssetDetail, error)
}

// altfinsAnalyticsGetter abstracts the AltFins client for testability.
type altfinsAnalyticsGetter interface {
	ResolveIdentifier(ctx context.Context, symbol string) (int64, bool, error)
	GetAnalytics(ctx context.Context, id int64, interval string) (*altfins.Analytics, error)
}

var newAltfinsClient = func() *altfins.AnalyticsClient { return altfins.NewAnalyticsClient() }

// StrategyEngine strategy execution engine
type StrategyEngine struct {
	config             *store.StrategyConfig
	nofxosClient       *nofxos.Client
	vergexClient       *vergex.Client
	vergexRankingCache map[string]*vergex.SignalRankItem
	perCoinSignals     map[string]PerCoinSignal

	// Free vergex.trade client (free-mode) + trending client (no Claw402 needed)
	freeClient *vergex.Client
	trending   *nofxos.FreeTrendingClient

	// Binance Opportunity client (technical + sentiment scopes)
	opportunity    binanceOpportunityGetter
	binanceDetails *binanceDetailCache // free Binance per-coin detail TTL cache

	// AltFins per-coin analytics client + TTL cache.
	altfinsClient  altfinsAnalyticsGetter
	altfinsDetails *altfinsDetailCache

	recentDecisions []*store.DecisionRecord // prior-cycle assistant responses (per-trader)

	exchange string // trader exchange used to pick the kline source
}

// newOpportunityClient is the factory used by NewStrategyEngine to create the
// Binance Opportunity client. Tests can override this to inject fakes.
var newOpportunityClient = func() *binance.OpportunityClient { return binance.NewOpportunityClient() }

// NewStrategyEngine creates strategy execution engine.
// claw402WalletKey is optional — if provided, nofxos data requests are routed through claw402.
func NewStrategyEngine(config *store.StrategyConfig, claw402WalletKey ...string) *StrategyEngine {
	if config == nil {
		defaultCfg := store.GetDefaultStrategyConfig("en")
		config = &defaultCfg
	}
	// Create NofxOS client with API key from config
	apiKey := config.Indicators.NofxOSAPIKey
	if apiKey == "" {
		apiKey = nofxos.DefaultAuthKey
	}
	client := nofxos.NewClient(nofxos.DefaultBaseURL, apiKey)

	freeVergex, err := vergex.NewFreeClient(vergex.DefaultFreeBaseURL, os.Getenv("VERGEX_API_TOKEN"), &logger.MCPLogger{})
	if err != nil {
		logger.Warnf("⚠️ Failed to init free Vergex client: %v (using paid path only)", err)
	}
	trendingClient := nofxos.NewFreeTrendingClient()
	opportunityClient := newOpportunityClient()

	// If claw402 wallet key is provided (from trader's AI config), route through claw402
	walletKey := ""
	if len(claw402WalletKey) > 0 {
		walletKey = claw402WalletKey[0]
	}
	if walletKey == "" {
		walletKey = os.Getenv("CLAW402_WALLET_KEY")
	}
	if walletKey != "" {
		claw402URL := os.Getenv("CLAW402_URL")
		if claw402URL == "" {
			claw402URL = "https://claw402.ai"
		}
		claw402Client, err := nofxos.NewClaw402DataClient(claw402URL, walletKey, &logger.MCPLogger{})
		if err == nil {
			client.SetClaw402(claw402Client)
			logger.Infof("🔗 NofxOS data routed through claw402 (%s)", claw402URL)
		} else {
			logger.Warnf("⚠️ Failed to init claw402 data client: %v (using direct nofxos.ai)", err)
		}

		vergexClient, err := vergex.NewClient(claw402URL, walletKey, &logger.MCPLogger{})
		if err == nil {
			logger.Infof("🔗 Vergex signals routed through claw402 (%s)", claw402URL)
		} else {
			logger.Warnf("⚠️ Failed to init Vergex claw402 client: %v", err)
		}
		return &StrategyEngine{
			config:             config,
			nofxosClient:       client,
			vergexClient:       vergexClient,
			vergexRankingCache: make(map[string]*vergex.SignalRankItem),
			perCoinSignals:     make(map[string]PerCoinSignal),
			freeClient:         freeVergex,
			trending:           trendingClient,
			opportunity:        opportunityClient,
			binanceDetails:     newBinanceDetailCache(),
			altfinsClient:      newAltfinsClient(),
			altfinsDetails:     newAltfinsDetailCache(),
		}
	}

	return &StrategyEngine{
		config:             config,
		nofxosClient:       client,
		vergexRankingCache: make(map[string]*vergex.SignalRankItem),
		perCoinSignals:     make(map[string]PerCoinSignal),
		freeClient:         freeVergex,
		trending:           trendingClient,
		opportunity:        opportunityClient,
		binanceDetails:     newBinanceDetailCache(),
		altfinsClient:      newAltfinsClient(),
		altfinsDetails:     newAltfinsDetailCache(),
	}
}

// SetRecentDecisions sets prior-cycle decision records (this trader only) so
// the prompt builder can feed them into the system prompt when configured.
func (e *StrategyEngine) SetRecentDecisions(records []*store.DecisionRecord) {
	e.recentDecisions = records
}

// DecisionContextConfig returns the strategy's decision_context config, or nil.
func (e *StrategyEngine) DecisionContextConfig() *store.DecisionContextConfig {
	if e == nil || e.config == nil {
		return nil
	}
	return e.config.DecisionContext
}

func (e *StrategyEngine) usesHyperliquidNativeUniverse() bool {
	if e == nil || e.config == nil {
		return false
	}
	source := e.config.CoinSource
	if source.SourceType == "hyper_all" || source.SourceType == "hyper_main" || source.SourceType == "hyper_rank" || source.SourceType == "vergex_signal" || source.UseHyperAll || source.UseHyperMain {
		return true
	}
	for _, symbol := range source.StaticCoins {
		if market.IsXyzDexAsset(symbol) {
			return true
		}
	}
	return false
}

// GetRiskControlConfig gets risk control configuration
func (e *StrategyEngine) GetRiskControlConfig() store.RiskControlConfig {
	return e.config.RiskControl
}

// GetLanguage returns the language from config or falls back to auto-detection
func (e *StrategyEngine) GetLanguage() Language {
	switch e.config.Language {
	case "zh":
		return LangChinese
	case "en":
		return LangEnglish
	default:
		// Fall back to auto-detection from prompt content for backward compatibility
		return detectLanguage(e.config.PromptSections.RoleDefinition)
	}
}

// SetExchange sets the trader exchange used to pick the kline source.
func (e *StrategyEngine) SetExchange(exchange string) {
	if e == nil {
		return
	}
	e.exchange = exchange
}

// GetConfig gets complete strategy configuration
func (e *StrategyEngine) GetConfig() *store.StrategyConfig {
	return e.config
}

// SetPerCoinSignals stores the per-coin enrichment signals keyed by symbol.
func (e *StrategyEngine) SetPerCoinSignals(signals map[string]PerCoinSignal) {
	e.perCoinSignals = signals
}

// PerCoinSignalFor returns the retained per-coin signal for a symbol.
func (e *StrategyEngine) PerCoinSignalFor(symbol string) (PerCoinSignal, bool) {
	if e == nil || e.perCoinSignals == nil {
		return PerCoinSignal{}, false
	}
	s, ok := e.perCoinSignals[symbol]
	return s, ok
}

func (e *StrategyEngine) binanceDetail(key string) (*binance.BinanceAssetDetail, bool) {
	return e.binanceDetails.get(key)
}

func (e *StrategyEngine) cacheBinanceDetail(key string, v *binance.BinanceAssetDetail) {
	e.binanceDetails.set(key, v)
}

func (e *StrategyEngine) altfinsDetail(key string) (*altfins.Analytics, bool) {
	return e.altfinsDetails.get(key)
}

func (e *StrategyEngine) cacheAltfinsDetail(key string, v *altfins.Analytics) {
	e.altfinsDetails.set(key, v)
}

// PrefetchAltFinsDetails warms the AltFins analytics TTL cache for the given
// symbols, gated by EnableAltFinsData. Best-effort; failures are logged.
func (e *StrategyEngine) PrefetchAltFinsDetails(ctx context.Context, symbols []string) {
	if e == nil || e.altfinsDetails == nil || e.altfinsClient == nil || e.config == nil {
		return
	}
	if !e.config.Indicators.EnableAltFinsData {
		return
	}
	intervals := e.config.Indicators.AltFinsIntervals
	if len(intervals) == 0 {
		intervals = []string{"MINUTES15", "DAILY"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, sym := range symbols {
		id, ok, err := e.altfinsClient.ResolveIdentifier(ctx, sym)
		if err != nil {
			logger.Warnf("⚠️ AltFins resolve failed (%s): %v", sym, err)
			continue
		}
		if !ok {
			continue
		}
		for _, iv := range intervals {
			key := sym + "|" + iv
			if _, cached := e.altfinsDetail(key); cached {
				continue
			}
			val, err := e.altfinsClient.GetAnalytics(ctx, id, iv)
			if err != nil {
				logger.Warnf("⚠️ AltFins analytics failed (%s %s): %v", sym, iv, err)
				continue
			}
			e.cacheAltfinsDetail(key, val)
		}
	}
}

// PrefetchBinanceDetails populates the binance detail cache for the given symbols,
// gated by the EnableBinanceTechnicalData / EnableBinanceSentimentData flags.
// This is safe to call from the trader loop before a cycle starts.
//
// Deprecated: this targets the discontinued Binance Opportunity feed and is
// retained as a reference implementation only; it will not return usable data.
func (e *StrategyEngine) PrefetchBinanceDetails(ctx context.Context, symbols []string) error {
	if e == nil || e.binanceDetails == nil || e.opportunity == nil {
		return nil
	}
	cfg := e.GetConfig()
	if cfg == nil {
		return nil
	}
	if !cfg.Indicators.EnableBinanceTechnicalData && !cfg.Indicators.EnableBinanceSentimentData {
		return nil
	}
	logger.Warnf("⚠️ Binance Opportunity feed is discontinued; PrefetchBinanceDetails will not return usable data")
	for _, sym := range symbols {
		base := strings.TrimSuffix(sym, "USDT")
		if cfg.Indicators.EnableBinanceTechnicalData {
			intervals := cfg.Indicators.BinanceTechnicalIntervals
			if len(intervals) == 0 {
				intervals = []string{"1h"}
			}
			for _, iv := range intervals {
				key := "technical|" + sym + "|" + iv
				if _, ok := e.binanceDetail(key); ok {
					continue
				}
				val, err := e.opportunity.GetAssetDetails(ctx, base, "technical", iv)
				if err != nil {
					logger.Warnf("⚠️ Prefetch technical detail failed (%s %s): %v", sym, iv, err)
					continue
				}
				e.cacheBinanceDetail(key, val)
			}
		}
		if cfg.Indicators.EnableBinanceSentimentData {
			key := "sentiment|" + sym
			if _, ok := e.binanceDetail(key); ok {
				continue
			}
			val, err := e.opportunity.GetAssetDetails(ctx, base, "sentiment", "24h")
			if err != nil {
				logger.Warnf("⚠️ Prefetch sentiment detail failed (%s): %v", sym, err)
				continue
			}
			e.cacheBinanceDetail(key, val)
		}
	}
	return nil
}

// ============================================================================
// Candidate Coins
// ============================================================================

// GetCandidateCoins gets candidate coins based on strategy configuration
func (e *StrategyEngine) GetCandidateCoins() ([]CandidateCoin, error) {
	var candidates []CandidateCoin
	symbolSources := make(map[string][]string)

	coinSource := e.config.CoinSource

	switch coinSource.SourceType {
	case "static":
		for _, symbol := range coinSource.StaticCoins {
			symbol = market.Normalize(symbol)
			candidates = append(candidates, CandidateCoin{
				Symbol:  symbol,
				Sources: []string{"static"},
			})
		}

		return e.filterExcludedCoins(candidates), nil

	case "ai500":
		// Check use_ai500 flag; if false, fall back to static coins
		if !coinSource.UseAI500 {
			logger.Infof("⚠️  source_type is 'ai500' but use_ai500 is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getAI500Coins(coinSource.AI500Limit)
		if err != nil {
			return nil, err
		}
		// Empty list is a normal condition, return directly
		return e.filterExcludedCoins(coins), nil

	case "oi_top":
		// Check use_oi_top flag; if false, fall back to static coins
		if !coinSource.UseOITop {
			logger.Infof("⚠️  source_type is 'oi_top' but use_oi_top is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getOITopCoins(coinSource.OITopLimit)
		if err != nil {
			return nil, err
		}
		// Empty list is a normal condition, return directly
		return e.filterExcludedCoins(coins), nil

	case "oi_low":
		// OI decrease ranking, suitable for short positions
		if !coinSource.UseOILow {
			logger.Infof("⚠️  source_type is 'oi_low' but use_oi_low is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getOILowCoins(coinSource.OILowLimit)
		if err != nil {
			return nil, err
		}
		// Empty list is a normal condition, return directly
		return e.filterExcludedCoins(coins), nil

	case "netflow_top":
		coins, err := e.getNetflowTopCoins(coinSource.NetflowLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "netflow_low":
		coins, err := e.getNetflowLowCoins(coinSource.NetflowLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "price_top":
		coins, err := e.getPriceTopCoins(coinSource.PriceLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "price_low":
		coins, err := e.getPriceLowCoins(coinSource.PriceLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "hyper_all":
		// All Hyperliquid perp coins
		if !coinSource.UseHyperAll {
			logger.Infof("⚠️  source_type is 'hyper_all' but use_hyper_all is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getHyperAllCoins()
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "hyper_main":
		// Top N Hyperliquid coins by 24h volume
		if !coinSource.UseHyperMain {
			logger.Infof("⚠️  source_type is 'hyper_main' but use_hyper_main is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getHyperMainCoins(coinSource.HyperMainLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "hyper_rank":
		coins, err := e.getHyperRankCoins(coinSource.HyperRankCategory, coinSource.HyperRankDirection, coinSource.HyperRankLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "binance_technical":
		coins, err := e.getBinanceOpportunityCoins("technical", coinSource.BinanceTechnicalInterval, coinSource.BinanceTechnicalDirection, coinSource.BinanceTechnicalLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "binance_sentiment":
		coins, err := e.getBinanceOpportunityCoins("sentiment", "", coinSource.BinanceSentimentDirection, coinSource.BinanceSentimentLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "vergex_signal":
		coins, err := e.getVergexSignalCoins(
			coinSource.VergexLimit,
			coinSource.VergexMarketType,
			coinSource.VergexChain,
			coinSource.VergexLiqBand,
			coinSource.HyperRankCategory,
			coinSource.StaticCoins,
			coinSource.VergexDirection,
		)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "mixed":
		if coinSource.UseAI500 {
			poolCoins, err := e.getAI500Coins(coinSource.AI500Limit)
			if err != nil {
				logger.Infof("⚠️  Failed to get AI500 coins: %v", err)
			} else {
				for _, coin := range poolCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "ai500")
				}
			}
		}

		if coinSource.UseOITop {
			oiCoins, err := e.getOITopCoins(coinSource.OITopLimit)
			if err != nil {
				logger.Infof("⚠️  Failed to get OI Top: %v", err)
			} else {
				for _, coin := range oiCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "oi_top")
				}
			}
		}

		if coinSource.UseOILow {
			oiLowCoins, err := e.getOILowCoins(coinSource.OILowLimit)
			if err != nil {
				logger.Infof("⚠️  Failed to get OI Low: %v", err)
			} else {
				for _, coin := range oiLowCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "oi_low")
				}
			}
		}

		if coinSource.UseHyperAll {
			hyperCoins, err := e.getHyperAllCoins()
			if err != nil {
				logger.Infof("⚠️  Failed to get Hyperliquid All coins: %v", err)
			} else {
				for _, coin := range hyperCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "hyper_all")
				}
			}
		}

		if coinSource.UseHyperMain {
			hyperMainCoins, err := e.getHyperMainCoins(coinSource.HyperMainLimit)
			if err != nil {
				logger.Infof("⚠️  Failed to get Hyperliquid Main coins: %v", err)
			} else {
				for _, coin := range hyperMainCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "hyper_main")
				}
			}
		}

		for _, symbol := range coinSource.StaticCoins {
			symbol = market.Normalize(symbol)
			if _, exists := symbolSources[symbol]; !exists {
				symbolSources[symbol] = []string{"static"}
			} else {
				symbolSources[symbol] = append(symbolSources[symbol], "static")
			}
		}

		for symbol, sources := range symbolSources {
			candidates = append(candidates, CandidateCoin{
				Symbol:  symbol,
				Sources: sources,
			})
		}
		return e.filterExcludedCoins(candidates), nil

	default:
		return nil, fmt.Errorf("unknown coin source type: %s", coinSource.SourceType)
	}
}

// filterExcludedCoins removes excluded coins from the candidates list
func (e *StrategyEngine) filterExcludedCoins(candidates []CandidateCoin) []CandidateCoin {
	if len(e.config.CoinSource.ExcludedCoins) == 0 {
		return candidates
	}

	// Build excluded set for O(1) lookup
	excluded := make(map[string]bool)
	for _, coin := range e.config.CoinSource.ExcludedCoins {
		normalized := market.Normalize(coin)
		excluded[normalized] = true
	}

	// Filter out excluded coins
	filtered := make([]CandidateCoin, 0, len(candidates))
	for _, c := range candidates {
		if !excluded[c.Symbol] {
			filtered = append(filtered, c)
		} else {
			logger.Infof("🚫 Excluded coin: %s", c.Symbol)
		}
	}

	return filtered
}

func (e *StrategyEngine) getAI500Coins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 30
	}

	coins, err := e.trending.GetAI500()
	if err != nil {
		return nil, err
	}
	var candidates []CandidateCoin
	for _, c := range coins {
		symbol := market.Normalize(c.Pair)
		candidates = append(candidates, CandidateCoin{Symbol: symbol, Sources: []string{"ai500"}})
	}
	return candidates, nil
}

func (e *StrategyEngine) getOITopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetOITop(limit)
	if err != nil {
		return nil, err
	}
	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(pos.Symbol), Sources: []string{"oi_top"}})
	}
	return candidates, nil
}

func (e *StrategyEngine) getOILowCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetOILow(limit)
	if err != nil {
		return nil, err
	}
	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(pos.Symbol), Sources: []string{"oi_low"}})
	}
	return candidates, nil
}

func (e *StrategyEngine) getNetflowTopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetNetFlowTop(limit)
	if err != nil {
		return nil, err
	}
	return netflowPositionsToCandidates(positions, "netflow_top", limit)
}

func (e *StrategyEngine) getNetflowLowCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	positions, err := e.trending.GetNetFlowLow(limit)
	if err != nil {
		return nil, err
	}
	return netflowPositionsToCandidates(positions, "netflow_low", limit)
}

func netflowPositionsToCandidates(positions []nofxos.NetFlowPosition, source string, limit int) ([]CandidateCoin, error) {
	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(pos.Symbol), Sources: []string{source}})
	}
	return candidates, nil
}

func (e *StrategyEngine) getPriceTopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	items, err := e.trending.GetPriceTop(limit)
	if err != nil {
		return nil, err
	}
	return priceItemsToCandidates(items, "price_top", limit)
}

func (e *StrategyEngine) getPriceLowCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	items, err := e.trending.GetPriceLow(limit)
	if err != nil {
		return nil, err
	}
	return priceItemsToCandidates(items, "price_low", limit)
}

func priceItemsToCandidates(items []nofxos.PriceRankingItem, source string, limit int) ([]CandidateCoin, error) {
	var candidates []CandidateCoin
	for i, it := range items {
		if i >= limit {
			break
		}
		candidates = append(candidates, CandidateCoin{Symbol: market.Normalize(it.Symbol), Sources: []string{source}})
	}
	return candidates, nil
}

// getHyperAllCoins returns all available Hyperliquid perpetual coins
func (e *StrategyEngine) getHyperAllCoins() ([]CandidateCoin, error) {
	ctx := context.Background()
	symbols, err := hyperliquid.GetAllCoinSymbols(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Hyperliquid coins: %w", err)
	}

	var candidates []CandidateCoin
	for _, symbol := range symbols {
		// Add USDT suffix for compatibility
		normalizedSymbol := market.Normalize(symbol + "USDT")
		candidates = append(candidates, CandidateCoin{
			Symbol:  normalizedSymbol,
			Sources: []string{"hyper_all"},
		})
	}
	logger.Infof("✅ Loaded %d Hyperliquid coins (hyper_all)", len(candidates))
	return candidates, nil
}

// getHyperMainCoins returns top N Hyperliquid coins by 24h volume
func (e *StrategyEngine) getHyperMainCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 20
	}

	ctx := context.Background()
	symbols, err := hyperliquid.GetMainCoinSymbols(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get Hyperliquid main coins: %w", err)
	}

	var candidates []CandidateCoin
	for _, symbol := range symbols {
		// Add USDT suffix for compatibility
		normalizedSymbol := market.Normalize(symbol + "USDT")
		candidates = append(candidates, CandidateCoin{
			Symbol:  normalizedSymbol,
			Sources: []string{"hyper_main"},
		})
	}
	logger.Infof("✅ Loaded %d Hyperliquid main coins (hyper_main) by 24h volume", len(candidates))
	return candidates, nil
}

func clampHyperRankLimit(limit int) int {
	if limit <= 0 {
		return 5
	}
	if limit > 10 {
		return 10
	}
	return limit
}

func (e *StrategyEngine) getHyperRankCoins(category, direction string, limit int) ([]CandidateCoin, error) {
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" {
		category = "stock"
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "" {
		direction = "gainers"
	}
	limit = clampHyperRankLimit(limit)

	ctx := context.Background()
	var ranked []struct {
		symbol string
		info   hyperliquid.CoinInfo
		cat    string
	}

	if category == "crypto" || category == "all" {
		coins, err := hyperliquid.GetPerpDexCoins(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get Hyperliquid crypto ranking: %w", err)
		}
		for _, coin := range coins {
			ranked = append(ranked, struct {
				symbol string
				info   hyperliquid.CoinInfo
				cat    string
			}{symbol: market.Normalize(coin.Symbol + "USDT"), info: coin, cat: "crypto"})
		}
	}

	if category != "crypto" {
		coins, err := hyperliquid.GetPerpDexCoins(ctx, "xyz")
		if err != nil {
			return nil, fmt.Errorf("failed to get Hyperliquid XYZ ranking: %w", err)
		}
		for _, coin := range coins {
			base := strings.TrimPrefix(coin.Symbol, "xyz:")
			cat := hyperliquid.XYZCategory(base)
			if category != "all" && cat != category {
				continue
			}
			ranked = append(ranked, struct {
				symbol string
				info   hyperliquid.CoinInfo
				cat    string
			}{symbol: hyperliquid.FormatCoinForAPI("xyz:" + base), info: coin, cat: cat})
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		switch direction {
		case "losers":
			return ranked[i].info.Change24hPct < ranked[j].info.Change24hPct
		case "volume":
			return ranked[i].info.Volume24h > ranked[j].info.Volume24h
		default:
			return ranked[i].info.Change24hPct > ranked[j].info.Change24hPct
		}
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	candidates := make([]CandidateCoin, 0, len(ranked))
	source := fmt.Sprintf("hyper_rank_%s_%s", category, direction)
	for _, item := range ranked {
		candidates = append(candidates, CandidateCoin{Symbol: item.symbol, Sources: []string{source}})
	}
	logger.Infof("✅ Loaded %d Hyperliquid rank coins (%s/%s, capped at %d)", len(candidates), category, direction, limit)
	return candidates, nil
}

// getBinanceOpportunityCoins returns candidate coins from the Binance
// Opportunity feed.
//
// Deprecated: the Binance Opportunity feed is discontinued and is retained as a
// reference implementation only; this will return no candidates.
func (e *StrategyEngine) getBinanceOpportunityCoins(scene, interval, direction string, limit int) ([]CandidateCoin, error) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "" {
		direction = "top"
	}
	logger.Warnf("⚠️ Binance Opportunity feed is discontinued; coin source %q will return no candidates", "binance_"+scene)
	assets, err := e.opportunity.GetOpportunityAssets(context.Background(), interval, scene)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Binance %s opportunity: %w", scene, err)
	}
	// Map all spot symbols to perp FIRST, then filter and sort.
	// This avoids losing assets due to early truncation before the mapping check.
	type scoredPerp struct {
		symbol string
		score  float64
	}
	mapped := make([]scoredPerp, 0, len(assets))
	for _, a := range assets {
		perp := mapSpotToPerp(a.Symbol)
		if perp == "" {
			continue
		}
		mapped = append(mapped, scoredPerp{symbol: perp, score: a.Score})
	}
	sort.Slice(mapped, func(i, j int) bool {
		if direction == "bottom" {
			return mapped[i].score < mapped[j].score
		}
		return mapped[i].score > mapped[j].score
	})
	// Safety soft cap: avoid returning an absurdly large candidate list to the
	// prompt builder. This is NOT the final prompt cap — the OI liquidity
	// filter in fetchMarketDataWithStrategy applies the real MaxCandidateCoins
	// cap AFTER filtering low-liquidity coins. The user's `limit` parameter is
	// intentionally unused here; it no longer truncates the pool.
	const softCap = 50
	if len(mapped) > softCap {
		mapped = mapped[:softCap]
	}
	candidates := make([]CandidateCoin, 0, len(mapped))
	for _, m := range mapped {
		candidates = append(candidates, CandidateCoin{
			Symbol:  m.symbol,
			Sources: []string{"binance_" + scene},
		})
	}
	logger.Infof("✅ Loaded %d Binance %s opportunity coins (dir=%s, soft-capped at %d)", len(candidates), scene, direction, softCap)
	return candidates, nil
}

func mapSpotToPerp(spot string) string {
	s := strings.ToUpper(strings.TrimSpace(spot))
	if s == "" {
		return ""
	}
	return market.Normalize(s)
}

func (e *StrategyEngine) getVergexSignalCoins(limit int, marketType, chain, liqBand, category string, selectedSymbols []string, direction string) ([]CandidateCoin, error) {
	if marketType == "" {
		marketType = vergex.DefaultMarketType
	}
	chain = vergex.QueryChain(chain)
	if limit <= 0 {
		limit = 5
	}
	if limit > store.MaxCandidateCoins {
		limit = store.MaxCandidateCoins
	}
	category = strings.ToLower(strings.TrimSpace(category))

	direction = strings.ToLower(strings.TrimSpace(direction))
	var (
		ranking *vergex.SignalRankingData
		err     error
	)
	switch direction {
	case "trending":
		ranking, err = e.freeClient.GetStockTrending(limit)
	case "gainers", "losers":
		ranking, err = e.freeClient.GetStockMovers(direction, limit)
	case "", "bull", "bear", "all":
		ranking, err = e.freeClient.GetSignalRanking(context.Background(), vergex.Query{})
	default:
		ranking, err = e.freeClient.GetSignalRanking(context.Background(), vergex.Query{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Vergex data: %w", err)
	}

	rankedItems := vergex.FilterSignalRankingItems(ranking.Items, marketType, store.MaxCandidateCoins)
	if len(rankedItems) == 0 && strings.TrimSpace(chain) != "" {
		fallbackRanking, fallbackErr := e.freeClient.GetSignalRanking(context.Background(), vergex.Query{LiqBand: liqBand})
		if fallbackErr == nil {
			fallbackItems := vergex.FilterSignalRankingItems(fallbackRanking.Items, marketType, store.MaxCandidateCoins)
			if len(fallbackItems) > 0 {
				logger.Infof("✅ Vergex signal ranking returned TradeFi items after retrying without chain filter (chain=%s)", chain)
				ranking = fallbackRanking
				rankedItems = fallbackItems
			}
		} else {
			logger.Warnf("⚠️ Vergex signal ranking retry without chain failed: %v", fallbackErr)
		}
	}
	e.vergexRankingCache = make(map[string]*vergex.SignalRankItem, len(rankedItems))
	for _, item := range rankedItems {
		itemCopy := item
		if symbol := vergex.TradableSymbolForMarket(item.MarketType, item.Symbol); symbol != "" {
			e.vergexRankingCache[symbol] = &itemCopy
		}
	}

	if len(selectedSymbols) > 0 {
		candidates := make([]CandidateCoin, 0, minInt(len(selectedSymbols), limit))
		seen := make(map[string]bool)
		for _, raw := range selectedSymbols {
			symbol := vergex.TradableSymbolForMarket(marketType, raw)
			if symbol == "" || seen[symbol] {
				continue
			}
			candidates = append(candidates, CandidateCoin{
				Symbol:  symbol,
				Sources: []string{"vergex_signal"},
			})
			seen[symbol] = true
			if len(candidates) >= limit {
				break
			}
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("selected Claw402 symbols are not tradable %s items", marketType)
		}
		logger.Infof("✅ Loaded %d selected Vergex candidates (%s)", len(candidates), marketType)
		return candidates, nil
	}

	// Direction-balanced selection: interleave the top bullish and top bearish
	// signals so the candidate universe carries BOTH long and short ideas every
	// cycle (instead of filling up with whichever bias ranks highest). This is
	// what lets the AI actually judge — and trade — both directions.
	var bullItems, bearItems, otherItems []vergex.SignalRankItem
	for _, item := range rankedItems {
		if category != "" && category != "all" && item.Category != category {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Bias)) {
		case "bearish", "short", "sell":
			bearItems = append(bearItems, item)
		case "bullish", "long", "buy":
			bullItems = append(bullItems, item)
		default:
			otherItems = append(otherItems, item)
		}
	}
	items := make([]vergex.SignalRankItem, 0, limit)
	bi, ri, oi := 0, 0, 0
	for len(items) < limit {
		progressed := false
		if bi < len(bullItems) {
			items = append(items, bullItems[bi])
			bi++
			progressed = true
			if len(items) >= limit {
				break
			}
		}
		if ri < len(bearItems) {
			items = append(items, bearItems[ri])
			ri++
			progressed = true
			if len(items) >= limit {
				break
			}
		}
		if !progressed {
			if oi < len(otherItems) {
				items = append(items, otherItems[oi])
				oi++
			} else {
				break
			}
		}
	}
	if len(items) == 0 {
		if category != "" && category != "all" {
			return nil, fmt.Errorf("vergex signal ranking returned no tradable %s items in category %s", marketType, category)
		}
		return nil, fmt.Errorf("vergex signal ranking returned no tradable %s items", marketType)
	}

	candidates := make([]CandidateCoin, 0, len(items))
	for _, item := range items {
		itemCopy := item
		symbol := vergex.TradableSymbolForMarket(item.MarketType, item.Symbol)
		if symbol == "" {
			continue
		}
		e.vergexRankingCache[symbol] = &itemCopy
		candidates = append(candidates, CandidateCoin{
			Symbol:  symbol,
			Sources: []string{"vergex_signal"},
		})
	}
	logger.Infof("✅ Loaded %d Vergex signal candidates (%s/%s, capped at %d)", len(candidates), marketType, withDefaultText(category, "all"), limit)
	return candidates, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DirectionalCandidate is a Vergex board candidate with its directional
// signal strength (the board z-score; sign follows the bias direction).
type DirectionalCandidate struct {
	Symbol string
	Score  float64
}

// DirectionalCandidates returns bullish (long) and bearish (short) candidates
// from the most recent Vergex signal ranking, each ordered by upstream rank
// (strongest first) and carrying the signal score so callers can require a
// minimum strength. Only populated for vergex_signal coin sources, since that
// is the only source carrying a per-symbol directional bias.
func (e *StrategyEngine) DirectionalCandidates() (bullish []DirectionalCandidate, bearish []DirectionalCandidate) {
	if e == nil || len(e.vergexRankingCache) == 0 {
		return nil, nil
	}
	type ranked struct {
		cand DirectionalCandidate
		rank int
	}
	rankKey := func(r int) int {
		if r > 0 {
			return r
		}
		return 1 << 30
	}
	var bl, br []ranked
	for sym, item := range e.vergexRankingCache {
		if item == nil {
			continue
		}
		entry := ranked{DirectionalCandidate{Symbol: sym, Score: item.Score}, item.Rank}
		switch strings.ToLower(strings.TrimSpace(item.Bias)) {
		case "bearish", "short", "sell":
			br = append(br, entry)
		case "bullish", "long", "buy":
			bl = append(bl, entry)
		}
	}
	sort.SliceStable(bl, func(i, j int) bool { return rankKey(bl[i].rank) < rankKey(bl[j].rank) })
	sort.SliceStable(br, func(i, j int) bool { return rankKey(br[i].rank) < rankKey(br[j].rank) })
	for _, r := range bl {
		bullish = append(bullish, r.cand)
	}
	for _, r := range br {
		bearish = append(bearish, r.cand)
	}
	return bullish, bearish
}

func withDefaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ============================================================================
// External & Quant Data
// ============================================================================

// FetchMarketData fetches market data based on strategy configuration
func (e *StrategyEngine) FetchMarketData(symbol string) (*market.Data, error) {
	return market.Get(symbol)
}

// FetchExternalData fetches external data sources
func (e *StrategyEngine) FetchExternalData() (map[string]interface{}, error) {
	externalData := make(map[string]interface{})

	for _, source := range e.config.Indicators.ExternalDataSources {
		data, err := e.fetchSingleExternalSource(source)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch external data source [%s]: %v", source.Name, err)
			continue
		}
		externalData[source.Name] = data
	}

	return externalData, nil
}

func (e *StrategyEngine) fetchSingleExternalSource(source store.ExternalDataSource) (interface{}, error) {
	// SSRF Protection: Validate URL before making request
	if err := security.ValidateURL(source.URL); err != nil {
		return nil, fmt.Errorf("external source URL validation failed: %w", err)
	}

	timeout := time.Duration(source.RefreshSecs) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Use SSRF-safe HTTP client
	client := security.SafeHTTPClient(timeout)

	req, err := http.NewRequest(source.Method, source.URL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range source.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if source.DataPath != "" {
		result = extractJSONPath(result, source.DataPath)
	}

	return result, nil
}

func extractJSONPath(data interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// FetchQuantData fetches quantitative data for a single coin
func (e *StrategyEngine) FetchQuantData(symbol string) (*QuantData, error) {
	if !e.config.Indicators.EnableQuantData {
		return nil, nil
	}
	if e.usesHyperliquidNativeUniverse() || market.IsXyzDexAsset(symbol) {
		logger.Infof("⏭️  Skipping NofxOS quant data for Hyperliquid symbol %s; using native Hyperliquid klines/mark data only", symbol)
		return nil, nil
	}

	// Use nofxos client with unified API key
	include := "oi,price"
	if e.config.Indicators.EnableQuantNetflow {
		include = "netflow,oi,price"
	}

	nofxosData, err := e.nofxosClient.GetCoinData(symbol, include)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch quant data: %w", err)
	}

	if nofxosData == nil {
		return nil, nil
	}

	// Convert nofxos.QuantData to kernel.QuantData
	quantData := &QuantData{
		Symbol:      nofxosData.Symbol,
		Price:       nofxosData.Price,
		PriceChange: nofxosData.PriceChange,
	}

	// Convert OI data
	if nofxosData.OI != nil {
		quantData.OI = make(map[string]*OIData)
		for exchange, oiData := range nofxosData.OI {
			if oiData != nil {
				kData := &OIData{
					CurrentOI: oiData.CurrentOI,
				}
				if oiData.Delta != nil {
					kData.Delta = make(map[string]*OIDeltaData)
					for dur, delta := range oiData.Delta {
						if delta != nil {
							kData.Delta[dur] = &OIDeltaData{
								OIDelta:        delta.OIDelta,
								OIDeltaValue:   delta.OIDeltaValue,
								OIDeltaPercent: delta.OIDeltaPercent,
							}
						}
					}
				}
				quantData.OI[exchange] = kData
			}
		}
	}

	// Convert Netflow data
	if nofxosData.Netflow != nil {
		quantData.Netflow = &NetflowData{}
		if nofxosData.Netflow.Institution != nil {
			quantData.Netflow.Institution = &FlowTypeData{
				Future: nofxosData.Netflow.Institution.Future,
				Spot:   nofxosData.Netflow.Institution.Spot,
			}
		}
		if nofxosData.Netflow.Personal != nil {
			quantData.Netflow.Personal = &FlowTypeData{
				Future: nofxosData.Netflow.Personal.Future,
				Spot:   nofxosData.Netflow.Personal.Spot,
			}
		}
	}

	return quantData, nil
}

// FetchQuantDataBatch batch fetches quantitative data
func (e *StrategyEngine) FetchQuantDataBatch(symbols []string) map[string]*QuantData {
	result := make(map[string]*QuantData)

	if !e.config.Indicators.EnableQuantData {
		return result
	}

	for _, symbol := range symbols {
		data, err := e.FetchQuantData(symbol)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch quantitative data for %s: %v", symbol, err)
			continue
		}
		if data != nil {
			result[symbol] = data
		}
	}

	return result
}

func (e *StrategyEngine) FetchVergexDataBatch(ctx context.Context, symbols []string) map[string]*vergex.MarketAnalysis {
	result := make(map[string]*vergex.MarketAnalysis)
	if e == nil || e.config == nil {
		return result
	}
	if e.freeClient == nil {
		logger.Warnf("⚠️ Vergex signal data skipped: free Vergex client is not configured")
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}

	source := e.config.CoinSource
	marketType := source.VergexMarketType
	if marketType == "" {
		marketType = vergex.DefaultMarketType
	}
	chain := source.VergexChain
	chain = vergex.QueryChain(chain)

	seen := make(map[string]bool)
	limited := make([]string, 0, store.MaxCandidateCoins)
	for _, symbol := range symbols {
		symbol = vergexDetailSymbolForLookup(marketType, symbol)
		if symbol == "" {
			continue
		}
		if seen[symbol] {
			continue
		}
		seen[symbol] = true
		limited = append(limited, symbol)
		if len(limited) >= store.MaxCandidateCoins+store.MaxPositions {
			break
		}
	}

	type vergexAnalysisResult struct {
		symbol   string
		analysis *vergex.MarketAnalysis
	}

	resultCh := make(chan vergexAnalysisResult, len(limited))
	var wg sync.WaitGroup
	sem := make(chan struct{}, vergexDetailSymbolConcurrency)
	for _, symbol := range limited {
		symbol := symbol
		querySymbol := vergex.QuerySymbol(symbol)
		if querySymbol == "" {
			continue
		}
		itemMarketType := marketType
		itemCategory := ""
		var ranking *vergex.SignalRankItem
		if cached, ok := e.vergexRankingCache[symbol]; ok && cached != nil {
			ranking = cached
			if cached.MarketType != "" {
				itemMarketType = cached.MarketType
			}
			itemCategory = cached.Category
		}

		analysis := &vergex.MarketAnalysis{
			Symbol:      symbol,
			QuerySymbol: querySymbol,
			MarketType:  itemMarketType,
			Ranking:     ranking,
		}
		query := vergex.Query{
			MarketType: itemMarketType,
			Symbol:     symbol,
			Chain:      chain,
			LiqBand:    source.VergexLiqBand,
			Category:   itemCategory,
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				analysis.SignalLabError = ctx.Err().Error()
				analysis.HeatmapError = ctx.Err().Error()
				resultCh <- vergexAnalysisResult{symbol: symbol, analysis: analysis}
				return
			}
			e.populateVergexDetailData(ctx, analysis, query)
			if len(analysis.SignalLab) > 0 || len(analysis.Heatmap) > 0 ||
				analysis.SignalLabError != "" || analysis.HeatmapError != "" || analysis.Ranking != nil {
				resultCh <- vergexAnalysisResult{symbol: symbol, analysis: analysis}
			}
		}()
	}

	wg.Wait()
	close(resultCh)
	for item := range resultCh {
		result[item.symbol] = item.analysis
	}

	logger.Infof("📊 Vergex detail data ready for %d symbols", len(result))
	return result
}

func vergexDetailSymbolForLookup(marketType, symbol string) string {
	return vergex.TradableSymbolForMarket(marketType, symbol)
}

const (
	vergexDetailRequestTimeout    = 45 * time.Second
	vergexDetailSymbolConcurrency = 2
)

func (e *StrategyEngine) populateVergexDetailData(ctx context.Context, analysis *vergex.MarketAnalysis, query vergex.Query) {
	type endpointResult struct {
		name string
		body json.RawMessage
		err  error
	}

	run := func(name string, fetch func(context.Context, vergex.Query) (json.RawMessage, error), out chan<- endpointResult) {
		requestCtx, cancel := context.WithTimeout(ctx, vergexDetailRequestTimeout)
		defer cancel()
		body, err := fetch(requestCtx, query)
		out <- endpointResult{name: name, body: body, err: err}
	}

	out := make(chan endpointResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		run("signal-lab", e.fetchVergexSignalLabWithFallback, out)
	}()
	go func() {
		defer wg.Done()
		run("heatmap", e.fetchVergexHeatmapWithFallback, out)
	}()
	wg.Wait()
	close(out)

	for item := range out {
		switch item.name {
		case "signal-lab":
			if item.err != nil {
				logger.Warnf("⚠️ Failed to fetch Vergex signal-lab for %s: %v", analysis.Symbol, item.err)
				analysis.SignalLabError = item.err.Error()
			} else {
				analysis.SignalLab = item.body
			}
		case "heatmap":
			if item.err != nil {
				logger.Warnf("⚠️ Failed to fetch Vergex heatmap for %s: %v", analysis.Symbol, item.err)
				analysis.HeatmapError = item.err.Error()
			} else {
				analysis.Heatmap = item.body
			}
		}
	}
}

func (e *StrategyEngine) fetchVergexSignalLabWithFallback(ctx context.Context, query vergex.Query) (json.RawMessage, error) {
	var lastErr error
	for idx, candidate := range vergexDetailQueryCandidates(query) {
		body, err := e.freeClient.GetSignalLab(ctx, candidate)
		if err == nil {
			if idx > 0 {
				logger.Infof("✅ Vergex signal-lab succeeded with fallback marketType=%s chain=%s", candidate.MarketType, withDefaultText(candidate.Chain, "default"))
			}
			return body, nil
		}
		lastErr = err
		if !isRetryableVergexDetailError(err) {
			break
		}
	}
	return nil, lastErr
}

func (e *StrategyEngine) fetchVergexHeatmapWithFallback(ctx context.Context, query vergex.Query) (json.RawMessage, error) {
	var lastErr error
	for idx, candidate := range vergexDetailQueryCandidates(query) {
		body, err := e.freeClient.GetCostLiquidationHeatmap(ctx, candidate)
		if err == nil {
			if idx > 0 {
				logger.Infof("✅ Vergex heatmap succeeded with fallback marketType=%s chain=%s", candidate.MarketType, withDefaultText(candidate.Chain, "default"))
			}
			return body, nil
		}
		lastErr = err
		if !isRetryableVergexDetailError(err) {
			break
		}
	}
	return nil, lastErr
}

func vergexDetailQueryCandidates(query vergex.Query) []vergex.Query {
	marketTypes := vergexDetailMarketTypeCandidates(query)
	chains := uniqueValues(query.Chain, "mainnet", "")

	candidates := make([]vergex.Query, 0, len(marketTypes)*len(chains))
	for _, marketType := range marketTypes {
		for _, chain := range chains {
			candidate := query
			candidate.MarketType = marketType
			candidate.Chain = chain
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func vergexDetailMarketTypeCandidates(query vergex.Query) []string {
	if isVergexAllMarketType(query.MarketType) {
		if market.IsXyzDexAsset(query.Symbol) {
			return uniqueNonEmpty(vergex.DefaultMarketType, "hip3-perp", "hip3Perp", "core_perp")
		}
		return uniqueNonEmpty("core_perp", vergex.DefaultMarketType, "hip3-perp", "hip3Perp")
	}
	values := []string{query.MarketType, vergex.DefaultMarketType, "hip3-perp", "hip3Perp", "core_perp"}
	return uniqueNonEmpty(values...)
}

func isVergexAllMarketType(marketType string) bool {
	switch strings.ToLower(strings.TrimSpace(marketType)) {
	case "", "all", "any", "ranking", "signal-ranking", "signal_ranking", "claw402", "vergex":
		return true
	default:
		return false
	}
}

func isRetryableVergexDetailError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid markettype") ||
		strings.Contains(msg, "invalid_request") ||
		strings.Contains(msg, "invalid chain") ||
		strings.Contains(msg, "market not found") ||
		strings.Contains(msg, "not_found")
}

func uniqueNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func uniqueValues(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// FetchOIRankingData fetches market-wide OI ranking data
func (e *StrategyEngine) FetchOIRankingData() *nofxos.OIRankingData {
	indicators := e.config.Indicators
	if !indicators.EnableOIRanking {
		return nil
	}
	if e.usesHyperliquidNativeUniverse() {
		logger.Infof("⏭️  Skipping NofxOS OI ranking for Hyperliquid strategy; native Hyperliquid universe is the source of truth")
		return nil
	}

	duration := indicators.OIRankingDuration
	if duration == "" {
		duration = "1h"
	}

	limit := indicators.OIRankingLimit
	if limit <= 0 {
		limit = 10
	}

	logger.Infof("📊 Fetching OI ranking data (duration: %s, limit: %d)", duration, limit)

	data, err := e.nofxosClient.GetOIRanking(duration, limit)
	if err != nil {
		logger.Warnf("⚠️  Failed to fetch OI ranking data: %v", err)
		return nil
	}

	logger.Infof("✓ OI ranking data ready: %d top, %d low positions",
		len(data.TopPositions), len(data.LowPositions))

	return data
}

// FetchNetFlowRankingData fetches market-wide NetFlow ranking data
func (e *StrategyEngine) FetchNetFlowRankingData() *nofxos.NetFlowRankingData {
	indicators := e.config.Indicators
	if !indicators.EnableNetFlowRanking {
		return nil
	}
	if e.usesHyperliquidNativeUniverse() {
		logger.Infof("⏭️  Skipping NofxOS netflow ranking for Hyperliquid strategy; native Hyperliquid universe is the source of truth")
		return nil
	}

	duration := indicators.NetFlowRankingDuration
	if duration == "" {
		duration = "1h"
	}

	limit := indicators.NetFlowRankingLimit
	if limit <= 0 {
		limit = 10
	}

	logger.Infof("💰 Fetching NetFlow ranking data (duration: %s, limit: %d)", duration, limit)

	data, err := e.nofxosClient.GetNetFlowRanking(duration, limit)
	if err != nil {
		logger.Warnf("⚠️  Failed to fetch NetFlow ranking data: %v", err)
		return nil
	}

	logger.Infof("✓ NetFlow ranking data ready: inst_in=%d, inst_out=%d, retail_in=%d, retail_out=%d",
		len(data.InstitutionFutureTop), len(data.InstitutionFutureLow),
		len(data.PersonalFutureTop), len(data.PersonalFutureLow))

	return data
}

// FetchPriceRankingData fetches market-wide price ranking data (gainers/losers)
func (e *StrategyEngine) FetchPriceRankingData() *nofxos.PriceRankingData {
	indicators := e.config.Indicators
	if !indicators.EnablePriceRanking {
		return nil
	}
	if e.usesHyperliquidNativeUniverse() {
		logger.Infof("⏭️  Skipping NofxOS price ranking for Hyperliquid strategy; native Hyperliquid universe is the source of truth")
		return nil
	}

	durations := indicators.PriceRankingDuration
	if durations == "" {
		durations = "1h"
	}

	limit := indicators.PriceRankingLimit
	if limit <= 0 {
		limit = 10
	}

	logger.Infof("📈 Fetching Price ranking data (durations: %s, limit: %d)", durations, limit)

	data, err := e.nofxosClient.GetPriceRanking(durations, limit)
	if err != nil {
		logger.Warnf("⚠️  Failed to fetch Price ranking data: %v", err)
		return nil
	}

	logger.Infof("✓ Price ranking data ready for %d durations", len(data.Durations))

	return data
}

// ============================================================================
// Helper Functions
// ============================================================================

// detectLanguage detects language from text content
// Returns LangChinese if text contains Chinese characters, otherwise LangEnglish
func detectLanguage(text string) Language {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return LangChinese
		}
	}
	return LangEnglish
}
