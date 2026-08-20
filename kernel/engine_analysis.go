package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	"nofx/provider/nofxos"
	"nofx/store"
	"regexp"
	"strings"
	"time"
)

// ============================================================================
// Pre-compiled regular expressions (performance optimization)
// ============================================================================

var (
	// Safe regex: precisely match ```json code blocks
	reJSONFence      = regexp.MustCompile(`(?is)` + "```json\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*```")
	reJSONArray      = regexp.MustCompile(`(?is)\[\s*\{.*?\}\s*\]`)
	reArrayHead      = regexp.MustCompile(`^\[\s*\{`)
	reArrayOpenSpace = regexp.MustCompile(`^\[\s+\{`)
	reInvisibleRunes = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF]")

	// XML tag extraction (supports any characters in reasoning chain)
	reReasoningTag = regexp.MustCompile(`(?s)<reasoning>(.*?)</reasoning>`)
	reDecisionTag  = regexp.MustCompile(`(?s)<decision>(.*?)</decision>`)
)

// ============================================================================
// Entry Functions - Main API
// ============================================================================

// GetFullDecision gets AI's complete trading decision (batch analysis of all coins and positions)
// Uses default strategy configuration - for production use GetFullDecisionWithStrategy with explicit config
func GetFullDecision(ctx *Context, mcpClient mcp.AIClient) (*FullDecision, error) {
	defaultConfig := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&defaultConfig)
	return GetFullDecisionWithStrategy(ctx, mcpClient, engine, "")
}

// GetFullDecisionWithStrategy uses StrategyEngine to get AI decision (unified prompt generation)
func GetFullDecisionWithStrategy(ctx *Context, mcpClient mcp.AIClient, engine *StrategyEngine, variant string) (*FullDecision, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	if engine == nil {
		defaultConfig := store.GetDefaultStrategyConfig("en")
		engine = NewStrategyEngine(&defaultConfig)
	}

	// Clamp strategy limits to prevent token overflow
	engineConfig := engine.GetConfig()
	engineConfig.ClampLimits()

	// Token estimation check — block if exceeding the specific model's context limit
	estimate := engineConfig.EstimateTokens()

	// Determine context limit for the specific model being used
	contextLimit := 131072 // safe default (strictest common limit)
	var providerName string
	if embedder, ok := mcpClient.(mcp.ClientEmbedder); ok {
		base := embedder.BaseClient()
		providerName = base.Provider
		contextLimit = store.GetContextLimitForClient(base.Provider, base.Model)
	}

	if estimate.Total > contextLimit {
		logger.Errorf("🚫 Token estimate %d exceeds %s context limit %d — blocking analysis",
			estimate.Total, providerName, contextLimit)
		return nil, fmt.Errorf("estimated %d tokens exceeds model context limit of %d; reduce coins, timeframes, or K-line count",
			estimate.Total, contextLimit)
	}
	if estimate.Total*100/contextLimit >= 80 {
		logger.Infof("⚠️  Token estimate %d — approaching %s context limit %d",
			estimate.Total, providerName, contextLimit)
	}

	// 1. Fetch market data using strategy config
	if len(ctx.MarketDataMap) == 0 {
		if err := fetchMarketDataWithStrategy(ctx, engine); err != nil {
			return nil, fmt.Errorf("failed to fetch market data: %w", err)
		}
	}
	pruneCandidateCoinsWithoutMarketData(ctx)
	enrichVergexDataWithStrategy(ctx, engine)

	// Ensure OITopDataMap is initialized
	if ctx.OITopDataMap == nil {
		ctx.OITopDataMap = make(map[string]*OITopData)
		oiPositions, err := engine.nofxosClient.GetOITopPositions()
		if err == nil {
			for _, pos := range oiPositions {
				ctx.OITopDataMap[pos.Symbol] = &OITopData{
					Rank:              pos.Rank,
					OIDeltaPercent:    pos.OIDeltaPercent,
					OIDeltaValue:      pos.OIDeltaValue,
					PriceDeltaPercent: pos.PriceDeltaPercent,
				}
			}
		}
	}

	// 2. Build System Prompt using strategy engine
	riskConfig := engine.GetRiskControlConfig()
	systemPrompt := engine.BuildSystemPrompt(ctx.Account.TotalEquity, variant)

	// 3. Build User Prompt using strategy engine
	userPrompt := engine.BuildUserPrompt(ctx)

	// 4. Call AI API
	aiCallStart := time.Now()
	aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	aiCallDuration := time.Since(aiCallStart)
	if err != nil {
		return nil, fmt.Errorf("AI API call failed: %w", err)
	}

	// 5. Parse AI response
	decision, err := parseFullDecisionResponse(
		aiResponse,
		ctx.Account.TotalEquity,
		riskConfig.BTCETHMaxLeverage,
		riskConfig.AltcoinMaxLeverage,
		riskConfig.BTCETHMaxPositionValueRatio,
		riskConfig.AltcoinMaxPositionValueRatio,
		riskConfig.MinPositionSize,
		riskConfig.MinRiskRewardRatio,
	)

	if decision != nil {
		decision.Timestamp = time.Now()
		decision.SystemPrompt = systemPrompt
		decision.UserPrompt = userPrompt
		decision.AIRequestDurationMs = aiCallDuration.Milliseconds()
		decision.RawResponse = aiResponse
	}

	if err != nil {
		return decision, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return decision, nil
}

func enrichVergexDataWithStrategy(ctx *Context, engine *StrategyEngine) {
	if ctx == nil || engine == nil || ctx.VergexDataMap != nil {
		return
	}
	symbolSet := make(map[string]bool)
	symbols := make([]string, 0, len(ctx.CandidateCoins)+len(ctx.Positions))
	for _, coin := range ctx.CandidateCoins {
		if !symbolSet[coin.Symbol] {
			symbolSet[coin.Symbol] = true
			symbols = append(symbols, coin.Symbol)
		}
	}
	for _, pos := range ctx.Positions {
		if !symbolSet[pos.Symbol] {
			symbolSet[pos.Symbol] = true
			symbols = append(symbols, pos.Symbol)
		}
	}
	ctx.VergexDataMap = engine.FetchVergexDataBatch(nil, symbols)
}

// ============================================================================
// Per-coin free data sources (candidates + positions)
// ============================================================================

// AttachPerCoinSignals is the exported wrapper around attachPerCoinSignals,
// used by the trader loop to fetch and attach per-coin free data sources.
func AttachPerCoinSignals(ctx *Context, engine *StrategyEngine) error {
	return attachPerCoinSignals(ctx, engine)
}

// attachPerCoinSignals fetches the enabled free per-coin data sources for the
// current candidate + position symbols and stores them on the engine for the
// prompt builder. Networks/sources with no data for a symbol are skipped.
func attachPerCoinSignals(ctx *Context, engine *StrategyEngine) error {
	if ctx == nil || engine == nil {
		return nil
	}
	cfg := engine.GetConfig()
	if !cfg.Indicators.EnableAI500Data && !cfg.Indicators.EnableOIData &&
		!cfg.Indicators.EnableNetflowData && !cfg.Indicators.EnablePriceData &&
		!cfg.Indicators.EnableBinanceTechnicalData && !cfg.Indicators.EnableBinanceSentimentData {
		return nil
	}

	symSet := make(map[string]bool)
	for _, c := range ctx.CandidateCoins {
		symSet[c.Symbol] = true
	}
	for _, p := range ctx.Positions {
		symSet[p.Symbol] = true
	}

	durations := cfg.Indicators.DataDurations
	if len(durations) == 0 {
		durations = []string{"24h"}
	}
	const limit = 50

	out := make(map[string]PerCoinSignal)

	if cfg.Indicators.EnableAI500Data {
		coins, err := engine.trending.GetAI500()
		if err == nil {
			for i := range coins {
				norm := market.Normalize(coins[i].Pair)
				if !symSet[norm] {
					continue
				}
				sig := out[norm]
				c := coins[i]
				sig.AI500 = &c
				out[norm] = sig
			}
		} else {
			logger.Warnf("⚠️ AI500 prompt data fetch failed: %v", err)
		}
	}

	oiByDur := make(map[string]map[string]nofxos.OIPosition)
	nfByDur := make(map[string]map[string]nofxos.NetFlowPosition)
	pxByDur := make(map[string]map[string]nofxos.PriceRankingItem)

	for _, dur := range durations {
		if cfg.Indicators.EnableOIData {
			env, err := engine.trending.GetOIData(dur, limit)
			if err == nil {
				oiByDur[dur] = oiListsToMap(env.Top, env.Low, symSet)
			} else {
				logger.Warnf("⚠️ OI prompt data fetch failed (%s): %v", dur, err)
			}
		}
		if cfg.Indicators.EnableNetflowData {
			env, err := engine.trending.GetNetflowData(dur, limit)
			if err == nil {
				nfByDur[dur] = netflowListsToMap(env.Top, env.Low, symSet)
			} else {
				logger.Warnf("⚠️ netflow prompt data fetch failed (%s): %v", dur, err)
			}
		}
		if cfg.Indicators.EnablePriceData {
			env, err := engine.trending.GetPriceData(dur, limit)
			if err == nil {
				pxByDur[dur] = priceListsToMap(env.Top, env.Low, symSet)
			} else {
				logger.Warnf("⚠️ price prompt data fetch failed (%s): %v", dur, err)
			}
		}
	}

	for sym := range symSet {
		sig := out[sym]
		sig.OI = oiByDur
		sig.Netflow = nfByDur
		sig.Price = pxByDur
		out[sym] = sig
	}

	// Binance Opportunity per-coin detail (free, per-symbol; read from TTL cache,
	// fall back to a synchronous fetch on miss).
	apiCtx := ctx.Ctx
	if apiCtx == nil {
		apiCtx = context.Background()
	}
	if cfg.Indicators.EnableBinanceTechnicalData {
		intervals := cfg.Indicators.BinanceTechnicalIntervals
		if len(intervals) == 0 {
			intervals = []string{"1h"}
		}
		for _, iv := range intervals {
			for sym := range symSet {
				key := "technical|" + sym + "|" + iv
				val, ok := engine.binanceDetail(key)
				if !ok {
					var fetchErr error
					val, fetchErr = engine.opportunity.GetAssetDetails(apiCtx, strings.TrimSuffix(sym, "USDT"), "technical", iv)
					if fetchErr != nil {
						logger.Warnf("⚠️ Binance technical detail fetch failed (%s %s): %v", sym, iv, fetchErr)
						continue
					}
					engine.cacheBinanceDetail(key, val)
				}
				sig := out[sym]
				if sig.BinanceTechnical == nil {
					sig.BinanceTechnical = make(map[string]string)
				}
				for k, v := range val {
					sig.BinanceTechnical[iv+"|"+k] = v
				}
				out[sym] = sig
			}
		}
	}
	if cfg.Indicators.EnableBinanceSentimentData {
		for sym := range symSet {
			key := "sentiment|" + sym
			val, ok := engine.binanceDetail(key)
			if !ok {
				var fetchErr error
				val, fetchErr = engine.opportunity.GetAssetDetails(apiCtx, strings.TrimSuffix(sym, "USDT"), "sentiment", "24h")
				if fetchErr != nil {
					logger.Warnf("⚠️ Binance sentiment detail fetch failed (%s): %v", sym, fetchErr)
					continue
				}
				engine.cacheBinanceDetail(key, val)
			}
			sig := out[sym]
			if sig.BinanceSentiment == nil {
				sig.BinanceSentiment = make(map[string]string)
			}
			for k, v := range val {
				sig.BinanceSentiment[k] = v
			}
			out[sym] = sig
		}
	}

	engine.SetPerCoinSignals(out)
	return nil
}

func oiListsToMap(top, low []nofxos.OIPosition, symSet map[string]bool) map[string]nofxos.OIPosition {
	m := make(map[string]nofxos.OIPosition)
	for _, p := range top {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["top:"+s] = p
		}
	}
	for _, p := range low {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["low:"+s] = p
		}
	}
	return m
}

func netflowListsToMap(top, low []nofxos.NetFlowPosition, symSet map[string]bool) map[string]nofxos.NetFlowPosition {
	m := make(map[string]nofxos.NetFlowPosition)
	for _, p := range top {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["top:"+s] = p
		}
	}
	for _, p := range low {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["low:"+s] = p
		}
	}
	return m
}

func priceListsToMap(top, low []nofxos.PriceRankingItem, symSet map[string]bool) map[string]nofxos.PriceRankingItem {
	m := make(map[string]nofxos.PriceRankingItem)
	for _, p := range top {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["top:"+s] = p
		}
	}
	for _, p := range low {
		s := market.Normalize(p.Symbol)
		if symSet[s] {
			m["low:"+s] = p
		}
	}
	return m
}

// keepCoinByOILiquidity decides whether a candidate coin passes the OI-liquidity
// filter. Existing positions and XYZ (non-perp) assets are always kept, matching
// prior behavior. When the filter is disabled the coin is kept.
func keepCoinByOILiquidity(isExistingPosition, isXyzAsset bool, oiValue float64, enable bool, minThresholdUSDT float64) bool {
	if isExistingPosition || isXyzAsset || !enable {
		return true
	}
	if oiValue <= 0 {
		return false
	}
	return oiValue >= minThresholdUSDT
}

// ============================================================================
// Market Data Fetching
// ============================================================================

// fetchMarketDataWithStrategy fetches market data using strategy config (multiple timeframes)
func fetchMarketDataWithStrategy(ctx *Context, engine *StrategyEngine) error {
	config := engine.GetConfig()
	ctx.MarketDataMap = make(map[string]*market.Data)

	timeframes := config.Indicators.Klines.SelectedTimeframes
	primaryTimeframe := config.Indicators.Klines.PrimaryTimeframe
	klineCount := config.Indicators.Klines.PrimaryCount

	// Compatible with old configuration
	if len(timeframes) == 0 {
		if primaryTimeframe != "" {
			timeframes = append(timeframes, primaryTimeframe)
		} else {
			timeframes = append(timeframes, "3m")
		}
		if config.Indicators.Klines.LongerTimeframe != "" {
			timeframes = append(timeframes, config.Indicators.Klines.LongerTimeframe)
		}
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}
	if klineCount <= 0 {
		klineCount = 30
	}

	logger.Infof("📊 Strategy timeframes: %v, Primary: %s, Kline count: %d", timeframes, primaryTimeframe, klineCount)

	// 1. First fetch data for position coins (must fetch)
	for _, pos := range ctx.Positions {
		data, err := market.GetWithTimeframesWithExchange(pos.Symbol, timeframes, primaryTimeframe, klineCount, engine.exchange)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch market data for position %s: %v", pos.Symbol, err)
			continue
		}
		ctx.MarketDataMap[pos.Symbol] = data
	}

	// 2. Fetch data for all candidate coins
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	riskCfg := engine.GetRiskControlConfig()
	enableOIFilter := riskCfg.EnableOILiquidityFilter
	minOITHRESHOLDUSDT := riskCfg.OILiquidityFilterMinUSDT
	if minOITHRESHOLDUSDT <= 0 {
		minOITHRESHOLDUSDT = 15_000_000 // preserve historical default
	}

	for _, coin := range ctx.CandidateCoins {
		if _, exists := ctx.MarketDataMap[coin.Symbol]; exists {
			continue
		}

		data, err := market.GetWithTimeframesWithExchange(coin.Symbol, timeframes, primaryTimeframe, klineCount, engine.exchange)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch market data for %s: %v", coin.Symbol, err)
			continue
		}

		// Liquidity filter (skip for xyz dex assets - they don't have OI data from Binance)
		isExistingPosition := positionSymbols[coin.Symbol]
		isXyzAsset := market.IsXyzDexAsset(coin.Symbol)
		if data.OpenInterest != nil && data.CurrentPrice > 0 {
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			if !keepCoinByOILiquidity(isExistingPosition, isXyzAsset, oiValue, enableOIFilter, minOITHRESHOLDUSDT) {
				logger.Infof("⚠️  %s OI value too low (%.2fM USD), skipping coin",
					coin.Symbol, oiValue/1_000_000)
				continue
			}
		}

		ctx.MarketDataMap[coin.Symbol] = data
	}

	logger.Infof("📊 Successfully fetched multi-timeframe market data for %d coins", len(ctx.MarketDataMap))
	return nil
}

func pruneCandidateCoinsWithoutMarketData(ctx *Context) {
	if ctx == nil || len(ctx.CandidateCoins) == 0 || len(ctx.MarketDataMap) == 0 {
		return
	}
	kept := make([]CandidateCoin, 0, len(ctx.CandidateCoins))
	for _, coin := range ctx.CandidateCoins {
		if _, ok := ctx.MarketDataMap[coin.Symbol]; ok {
			kept = append(kept, coin)
			continue
		}
		logger.Infof("⚠️  Skipping candidate %s in AI prompt: no valid market/K-line data", coin.Symbol)
	}
	ctx.CandidateCoins = kept
}

// ============================================================================
// AI Response Parsing
// ============================================================================

func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio, minPositionSize, minRiskRewardRatio float64) (*FullDecision, error) {
	cotTrace := extractCoTTrace(aiResponse)

	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("failed to extract decisions: %w", err)
	}

	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio, minPositionSize, minRiskRewardRatio); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("decision validation failed: %w", err)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

func extractCoTTrace(response string) string {
	if match := reReasoningTag.FindStringSubmatch(response); match != nil && len(match) > 1 {
		logger.Infof("✓ Extracted reasoning chain using <reasoning> tag")
		return strings.TrimSpace(match[1])
	}

	if decisionIdx := strings.Index(response, "<decision>"); decisionIdx > 0 {
		logger.Infof("✓ Extracted content before <decision> tag as reasoning chain")
		return strings.TrimSpace(response[:decisionIdx])
	}

	jsonStart := strings.Index(response, "[")
	if jsonStart > 0 {
		logger.Infof("⚠️  Extracted reasoning chain using old format ([ character separator)")
		return strings.TrimSpace(response[:jsonStart])
	}

	return strings.TrimSpace(response)
}

func extractDecisions(response string) ([]Decision, error) {
	s := removeInvisibleRunes(response)
	s = strings.TrimSpace(s)
	s = fixMissingQuotes(s)

	var jsonPart string
	if match := reDecisionTag.FindStringSubmatch(s); match != nil && len(match) > 1 {
		jsonPart = strings.TrimSpace(match[1])
		logger.Infof("✓ Extracted JSON using <decision> tag")
	} else {
		jsonPart = s
		logger.Infof("⚠️  <decision> tag not found, searching JSON in full text")
	}

	jsonPart = fixMissingQuotes(jsonPart)

	if m := reJSONFence.FindStringSubmatch(jsonPart); m != nil && len(m) > 1 {
		jsonContent := strings.TrimSpace(m[1])
		jsonContent = compactArrayOpen(jsonContent)
		jsonContent = fixMissingQuotes(jsonContent)
		if err := validateJSONFormat(jsonContent); err != nil {
			return nil, fmt.Errorf("JSON format validation failed: %w\nJSON content: %s\nFull response:\n%s", err, jsonContent, response)
		}
		var decisions []Decision
		if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
			return nil, fmt.Errorf("JSON parsing failed: %w\nJSON content: %s", err, jsonContent)
		}
		return decisions, nil
	}

	jsonContent := strings.TrimSpace(reJSONArray.FindString(jsonPart))
	if jsonContent == "" {
		logger.Infof("⚠️  [SafeFallback] AI didn't output JSON decision, entering safe wait mode")

		cotSummary := jsonPart
		if len(cotSummary) > 240 {
			cotSummary = cotSummary[:240] + "..."
		}

		fallbackDecision := Decision{
			Symbol:    "ALL",
			Action:    "wait",
			Reasoning: fmt.Sprintf("Model didn't output structured JSON decision, entering safe wait; summary: %s", cotSummary),
		}

		return []Decision{fallbackDecision}, nil
	}

	jsonContent = compactArrayOpen(jsonContent)
	jsonContent = fixMissingQuotes(jsonContent)

	if err := validateJSONFormat(jsonContent); err != nil {
		return nil, fmt.Errorf("JSON format validation failed: %w\nJSON content: %s\nFull response:\n%s", err, jsonContent, response)
	}

	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON parsing failed: %w\nJSON content: %s", err, jsonContent)
	}

	return decisions, nil
}

func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"")
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"")
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")

	jsonStr = strings.ReplaceAll(jsonStr, "［", "[")
	jsonStr = strings.ReplaceAll(jsonStr, "］", "]")
	jsonStr = strings.ReplaceAll(jsonStr, "｛", "{")
	jsonStr = strings.ReplaceAll(jsonStr, "｝", "}")
	jsonStr = strings.ReplaceAll(jsonStr, "：", ":")
	jsonStr = strings.ReplaceAll(jsonStr, "，", ",")

	jsonStr = strings.ReplaceAll(jsonStr, "【", "[")
	jsonStr = strings.ReplaceAll(jsonStr, "】", "]")
	jsonStr = strings.ReplaceAll(jsonStr, "〔", "[")
	jsonStr = strings.ReplaceAll(jsonStr, "〕", "]")
	jsonStr = strings.ReplaceAll(jsonStr, "、", ",")

	jsonStr = strings.ReplaceAll(jsonStr, "　", " ")

	return jsonStr
}

func validateJSONFormat(jsonStr string) error {
	trimmed := strings.TrimSpace(jsonStr)

	if !reArrayHead.MatchString(trimmed) {
		if strings.HasPrefix(trimmed, "[") && !strings.Contains(trimmed[:min(20, len(trimmed))], "{") {
			return fmt.Errorf("not a valid decision array (must contain objects {}), actual content: %s", trimmed[:min(50, len(trimmed))])
		}
		return fmt.Errorf("JSON must start with [{ (whitespace allowed), actual: %s", trimmed[:min(20, len(trimmed))])
	}

	if strings.Contains(jsonStr, "~") {
		return fmt.Errorf("JSON cannot contain range symbol ~, all numbers must be precise single values")
	}

	for i := 0; i < len(jsonStr)-4; i++ {
		if jsonStr[i] >= '0' && jsonStr[i] <= '9' &&
			jsonStr[i+1] == ',' &&
			jsonStr[i+2] >= '0' && jsonStr[i+2] <= '9' &&
			jsonStr[i+3] >= '0' && jsonStr[i+3] <= '9' &&
			jsonStr[i+4] >= '0' && jsonStr[i+4] <= '9' {
			return fmt.Errorf("JSON numbers cannot contain thousand separator comma, found: %s", jsonStr[i:min(i+10, len(jsonStr))])
		}
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func removeInvisibleRunes(s string) string {
	return reInvisibleRunes.ReplaceAllString(s, "")
}

func compactArrayOpen(s string) string {
	return reArrayOpenSpace.ReplaceAllString(strings.TrimSpace(s), "[{")
}
