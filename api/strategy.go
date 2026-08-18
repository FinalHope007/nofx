package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	_ "nofx/mcp/payment"
	_ "nofx/mcp/provider"
	"nofx/store"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// validateStrategyConfig validates strategy configuration and returns warnings
func validateStrategyConfig(config *store.StrategyConfig) []string {
	var warnings []string
	if config.StrategyType == "grid_trading" {
		return warnings
	}

	// Validate NofxOS API key if any NofxOS feature is enabled
	if (config.Indicators.EnableQuantData || config.Indicators.EnableOIRanking ||
		config.Indicators.EnableNetFlowRanking || config.Indicators.EnablePriceRanking) &&
		config.Indicators.NofxOSAPIKey == "" {
		warnings = append(warnings, "NofxOS API key is not configured. NofxOS data sources may not work properly.")
	}

	return warnings
}

func attachPublishConfig(config *store.StrategyConfig, strategy *store.Strategy) {
	if config == nil || strategy == nil {
		return
	}
	config.ClampLimits()
	config.PublishConfig = &store.PublishStrategyConfig{
		IsPublic:      strategy.IsPublic,
		ConfigVisible: strategy.ConfigVisible,
	}
}

// handleEstimateTokens estimates token usage for a strategy config (no auth required, pure computation)
func (s *Server) handleEstimateTokens(c *gin.Context) {
	var req struct {
		Config store.StrategyConfig `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	estimate := req.Config.EstimateTokens()
	c.JSON(http.StatusOK, estimate)
}

// handlePublicStrategies Get public strategies for strategy market (no auth required)
func (s *Server) handlePublicStrategies(c *gin.Context) {
	strategies, err := s.store.Strategy().ListPublic()
	if err != nil {
		SafeInternalError(c, "Failed to get public strategies", err)
		return
	}

	// Convert to frontend format with visibility control
	result := make([]gin.H, 0, len(strategies))
	for _, st := range strategies {
		item := gin.H{
			"id":             st.ID,
			"name":           st.Name,
			"description":    st.Description,
			"author_email":   "", // Will be filled if we have user info
			"is_public":      st.IsPublic,
			"config_visible": st.ConfigVisible,
			"created_at":     st.CreatedAt,
			"updated_at":     st.UpdatedAt,
		}

		// Only include config if config_visible is true
		if st.ConfigVisible {
			var config store.StrategyConfig
			json.Unmarshal([]byte(st.Config), &config)
			attachPublishConfig(&config, st)
			item["config"] = config
		}

		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": result,
	})
}

// handleGetStrategies Get strategy list
func (s *Server) handleGetStrategies(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	lang := c.Query("lang")
	if lang == "" {
		lang = "zh"
	}
	if err := s.createDefaultStrategies(userID, lang); err != nil {
		logger.Warnf("Failed to sync default strategy presets for user %s: %v", userID, err)
	}

	strategies, err := s.store.Strategy().List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get strategy list", err)
		return
	}

	// Convert to frontend format
	result := make([]gin.H, 0, len(strategies))
	for _, st := range strategies {
		var config store.StrategyConfig
		json.Unmarshal([]byte(st.Config), &config)
		attachPublishConfig(&config, st)

		result = append(result, gin.H{
			"id":             st.ID,
			"name":           st.Name,
			"description":    st.Description,
			"is_active":      st.IsActive,
			"is_default":     st.IsDefault,
			"is_public":      st.IsPublic,
			"config_visible": st.ConfigVisible,
			"config":         config,
			"created_at":     st.CreatedAt,
			"updated_at":     st.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": result,
	})
}

// handleGetStrategy Get single strategy
func (s *Server) handleGetStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategy, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}

	var config store.StrategyConfig
	json.Unmarshal([]byte(strategy.Config), &config)
	attachPublishConfig(&config, strategy)

	c.JSON(http.StatusOK, gin.H{
		"id":          strategy.ID,
		"name":        strategy.Name,
		"description": strategy.Description,
		"is_active":   strategy.IsActive,
		"is_default":  strategy.IsDefault,
		"config":      config,
		"created_at":  strategy.CreatedAt,
		"updated_at":  strategy.UpdatedAt,
	})
}

// handleCreateStrategy Create strategy.
// If "config" is omitted from the request body, the system default config is used automatically.
func (s *Server) handleCreateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name          string                `json:"name" binding:"required"`
		Description   string                `json:"description"`
		Lang          string                `json:"lang"`   // "zh" or "en", used when config is omitted
		Config        *store.StrategyConfig `json:"config"` // optional — uses default if omitted
		IsPublic      bool                  `json:"is_public"`
		ConfigVisible bool                  `json:"config_visible"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Use default config when none provided
	if req.Config == nil {
		lang := req.Lang
		if lang == "" {
			lang = "zh"
		}
		defaultCfg := store.GetDefaultStrategyConfig(lang)
		req.Config = &defaultCfg
	}
	beforeClamp := *req.Config
	req.Config.ClampLimits()
	hadPublishConfig := req.Config.PublishConfig != nil
	isPublic := req.IsPublic
	configVisible := req.ConfigVisible
	if hadPublishConfig {
		isPublic = req.Config.PublishConfig.IsPublic
		configVisible = req.Config.PublishConfig.ConfigVisible
	}
	req.Config.PublishConfig = &store.PublishStrategyConfig{
		IsPublic:      isPublic,
		ConfigVisible: configVisible,
	}

	// Serialize configuration
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}

	strategy := &store.Strategy{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    false,
		IsDefault:   false,
		IsPublic:    isPublic,
		// Existing default is true; keep that behavior when no explicit publish config is sent.
		ConfigVisible: configVisible || !hadPublishConfig,
		Config:        string(configJSON),
	}

	if err := s.store.Strategy().Create(strategy); err != nil {
		SafeInternalError(c, "Failed to create strategy", err)
		return
	}

	// Validate configuration and collect warnings
	warnings := validateStrategyConfig(req.Config)
	warnings = append(warnings, store.StrategyClampWarnings(beforeClamp, *req.Config, req.Config.Language)...)

	response := gin.H{
		"id":      strategy.ID,
		"message": "Strategy created successfully",
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	c.JSON(http.StatusOK, response)
}

// handleUpdateStrategy Update strategy.
// The incoming config is merged with the existing one: top-level sections present in the
// request overwrite the corresponding existing sections; absent sections are preserved.
// This prevents partial updates from zeroing out unmentioned fields.
func (s *Server) handleUpdateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Check if it's a system default strategy
	existing, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	if existing.IsDefault {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify system default strategy"})
		return
	}

	var req struct {
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		Config        json.RawMessage `json:"config"` // raw JSON so we can merge
		IsPublic      bool            `json:"is_public"`
		ConfigVisible bool            `json:"config_visible"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Start with the existing config as base — preserves all unmentioned fields.
	var mergedConfig store.StrategyConfig
	if err := json.Unmarshal([]byte(existing.Config), &mergedConfig); err != nil {
		// If existing config is corrupt, start from zero
		mergedConfig = store.StrategyConfig{}
	}

	// Apply incoming config on top while preserving nested fields that were not sent.
	if len(req.Config) > 0 && string(req.Config) != "null" {
		var patch map[string]any
		if err := json.Unmarshal(req.Config, &patch); err != nil {
			SafeBadRequest(c, "Invalid config JSON")
			return
		}
		mergedConfig, err = store.MergeStrategyConfig(mergedConfig, patch)
		if err != nil {
			SafeBadRequest(c, "Invalid config JSON")
			return
		}
	}
	beforeClamp := mergedConfig
	mergedConfig.ClampLimits()

	// Preserve existing name/description when not supplied
	name := req.Name
	if name == "" {
		name = existing.Name
	}
	description := req.Description
	if description == "" {
		description = existing.Description
	}

	configJSON, err := json.Marshal(mergedConfig)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}

	strategy := &store.Strategy{
		ID:            strategyID,
		UserID:        userID,
		Name:          name,
		Description:   description,
		Config:        string(configJSON),
		IsPublic:      req.IsPublic,
		ConfigVisible: req.ConfigVisible,
	}

	if err := s.store.Strategy().Update(strategy); err != nil {
		SafeInternalError(c, "Failed to update strategy", err)
		return
	}

	// Token overflow check — block save if all models exceed context limits
	if mergedConfig.StrategyType == "" || mergedConfig.StrategyType == "ai_trading" {
		estimate := mergedConfig.EstimateTokens()
		allExceed := true
		for _, ml := range estimate.ModelLimits {
			if ml.UsagePct <= 100 {
				allExceed = false
				break
			}
		}
		if allExceed && len(estimate.ModelLimits) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count.", estimate.Total),
				"token_estimate": estimate,
			})
			return
		}
	}

	// Validate merged configuration and collect warnings
	warnings := validateStrategyConfig(&mergedConfig)
	warnings = append(warnings, store.StrategyClampWarnings(beforeClamp, mergedConfig, mergedConfig.Language)...)

	response := gin.H{"message": "Strategy updated successfully"}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	c.JSON(http.StatusOK, response)
}

// handleDeleteStrategy Delete strategy
func (s *Server) handleDeleteStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := s.store.Strategy().Delete(userID, strategyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": SanitizeError(err, "Failed to delete strategy")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy deleted successfully"})
}

// handleActivateStrategy Activate strategy
func (s *Server) handleActivateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := s.store.Strategy().SetActive(userID, strategyID); err != nil {
		SafeInternalError(c, "Failed to activate strategy", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy activated successfully"})
}

// handleDuplicateStrategy Duplicate strategy
func (s *Server) handleDuplicateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	sourceID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	newID := uuid.New().String()
	if err := s.store.Strategy().Duplicate(userID, sourceID, newID, req.Name); err != nil {
		SafeInternalError(c, "Failed to duplicate strategy", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      newID,
		"message": "Strategy duplicated successfully",
	})
}

// handleGetActiveStrategy Get currently active strategy
func (s *Server) handleGetActiveStrategy(c *gin.Context) {
	userID := c.GetString("user_id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategy, err := s.store.Strategy().GetActive(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active strategy"})
		return
	}

	var config store.StrategyConfig
	json.Unmarshal([]byte(strategy.Config), &config)
	attachPublishConfig(&config, strategy)

	c.JSON(http.StatusOK, gin.H{
		"id":          strategy.ID,
		"name":        strategy.Name,
		"description": strategy.Description,
		"is_active":   strategy.IsActive,
		"is_default":  strategy.IsDefault,
		"config":      config,
		"created_at":  strategy.CreatedAt,
		"updated_at":  strategy.UpdatedAt,
	})
}

// handleGetDefaultStrategyConfig Get default strategy configuration template
func (s *Server) handleGetDefaultStrategyConfig(c *gin.Context) {
	// Get language from query parameter, default to "en"
	lang := c.Query("lang")
	if lang != "zh" {
		lang = "en"
	}

	// Return default configuration with i18n support
	defaultConfig := store.GetDefaultStrategyConfig(lang)
	c.JSON(http.StatusOK, defaultConfig)
}

// handlePreviewPrompt Preview prompt generated by strategy
func (s *Server) handlePreviewPrompt(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Config        store.StrategyConfig `json:"config" binding:"required"`
		AccountEquity float64              `json:"account_equity"`
		PromptVariant string               `json:"prompt_variant"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Use default values
	if req.AccountEquity <= 0 {
		req.AccountEquity = 1000.0 // Default simulated account equity
	}
	if req.PromptVariant == "" {
		req.PromptVariant = "balanced"
	}

	// Create strategy engine to build prompt
	engine := kernel.NewStrategyEngine(&req.Config)

	// Build system prompt (using built-in method from strategy engine)
	systemPrompt := engine.BuildSystemPrompt(
		req.AccountEquity,
		req.PromptVariant,
	)

	c.JSON(http.StatusOK, gin.H{
		"system_prompt":  systemPrompt,
		"prompt_variant": req.PromptVariant,
		"config_summary": gin.H{
			"coin_source":      req.Config.CoinSource.SourceType,
			"primary_tf":       req.Config.Indicators.Klines.PrimaryTimeframe,
			"btc_eth_leverage": req.Config.RiskControl.BTCETHMaxLeverage,
			"altcoin_leverage": req.Config.RiskControl.AltcoinMaxLeverage,
			"max_positions":    req.Config.RiskControl.MaxPositions,
		},
	})
}

// handleStrategyTestRun AI test run (does not execute trades, only returns AI analysis results)
func (s *Server) handleStrategyTestRun(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Config        store.StrategyConfig `json:"config" binding:"required"`
		PromptVariant string               `json:"prompt_variant"`
		AIModelID     string               `json:"ai_model_id"`
		RunRealAI     bool                 `json:"run_real_ai"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	if req.PromptVariant == "" {
		req.PromptVariant = "balanced"
	}

	claw402WalletKey, err := s.resolveStrategyDataWalletKey(userID, req.AIModelID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       err.Error(),
			"ai_response": "",
		})
		return
	}

	// Create strategy engine to build prompt
	engine := kernel.NewStrategyEngine(&req.Config, claw402WalletKey)

	// Get candidate coins
	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		logger.Errorf("[API Error] Failed to get candidate coins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       "Failed to get candidate coins",
			"ai_response": "",
		})
		return
	}

	// Get timeframe configuration
	timeframes := req.Config.Indicators.Klines.SelectedTimeframes
	primaryTimeframe := req.Config.Indicators.Klines.PrimaryTimeframe
	klineCount := req.Config.Indicators.Klines.PrimaryCount

	// If no timeframes selected, use default values
	if len(timeframes) == 0 {
		// Backward compatibility: use primary and longer timeframes
		if primaryTimeframe != "" {
			timeframes = append(timeframes, primaryTimeframe)
		} else {
			timeframes = append(timeframes, "3m")
		}
		if req.Config.Indicators.Klines.LongerTimeframe != "" {
			timeframes = append(timeframes, req.Config.Indicators.Klines.LongerTimeframe)
		}
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}
	if klineCount <= 0 {
		klineCount = 30
	}

	fmt.Printf("📊 Using timeframes: %v, primary: %s, kline count: %d\n", timeframes, primaryTimeframe, klineCount)

	// Get real market data (using multiple timeframes)
	marketDataMap := make(map[string]*market.Data)
	for _, coin := range candidates {
		data, err := market.GetWithTimeframes(coin.Symbol, timeframes, primaryTimeframe, klineCount)
		if err != nil {
			// If getting data for a coin fails, log but continue
			fmt.Printf("⚠️  Failed to get market data for %s: %v\n", coin.Symbol, err)
			continue
		}
		marketDataMap[coin.Symbol] = data
	}

	// Fetch quantitative data for each candidate coin
	symbols := make([]string, 0, len(candidates))
	for _, c := range candidates {
		symbols = append(symbols, c.Symbol)
	}
	quantDataMap := engine.FetchQuantDataBatch(symbols)
	vergexDataMap := engine.FetchVergexDataBatch(context.Background(), symbols)

	// Fetch OI ranking data (market-wide position changes)
	oiRankingData := engine.FetchOIRankingData()

	// Fetch NetFlow ranking data (market-wide fund flow)
	netFlowRankingData := engine.FetchNetFlowRankingData()

	// Fetch Price ranking data (market-wide gainers/losers)
	priceRankingData := engine.FetchPriceRankingData()

	// Build real context (for generating User Prompt)
	testContext := &kernel.Context{
		CurrentTime:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes: 0,
		CallCount:      1,
		Account: kernel.AccountInfo{
			TotalEquity:      1000.0,
			AvailableBalance: 1000.0,
			UnrealizedPnL:    0,
			TotalPnL:         0,
			TotalPnLPct:      0,
			MarginUsed:       0,
			MarginUsedPct:    0,
			PositionCount:    0,
		},
		Positions:          []kernel.PositionInfo{},
		CandidateCoins:     candidates,
		PromptVariant:      req.PromptVariant,
		MarketDataMap:      marketDataMap,
		QuantDataMap:       quantDataMap,
		VergexDataMap:      vergexDataMap,
		OIRankingData:      oiRankingData,
		NetFlowRankingData: netFlowRankingData,
		PriceRankingData:   priceRankingData,
	}

	// Build System Prompt
	systemPrompt := engine.BuildSystemPrompt(1000.0, req.PromptVariant)

	// Build User Prompt (using real market data)
	userPrompt := engine.BuildUserPrompt(testContext)

	// If requesting real AI call
	if req.RunRealAI && req.AIModelID != "" {
		aiResponse, aiErr := s.runRealAITest(userID, req.AIModelID, systemPrompt, userPrompt)
		if aiErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"system_prompt":   systemPrompt,
				"user_prompt":     userPrompt,
				"candidate_count": len(candidates),
				"candidates":      candidates,
				"prompt_variant":  req.PromptVariant,
				"ai_response":     fmt.Sprintf("❌ AI call failed: %s", aiErr.Error()),
				"ai_error":        aiErr.Error(),
				"note":            "AI call error",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"system_prompt":   systemPrompt,
			"user_prompt":     userPrompt,
			"candidate_count": len(candidates),
			"candidates":      candidates,
			"prompt_variant":  req.PromptVariant,
			"ai_response":     aiResponse,
			"note":            "✅ Real AI test run successful",
		})
		return
	}

	// Return result (without actually calling AI, only return built prompt)
	c.JSON(http.StatusOK, gin.H{
		"system_prompt":   systemPrompt,
		"user_prompt":     userPrompt,
		"candidate_count": len(candidates),
		"candidates":      candidates,
		"prompt_variant":  req.PromptVariant,
		"ai_response":     "Please select an AI model and click 'Run Test' to perform real AI analysis.",
		"note":            "AI model not selected or real AI call not enabled",
	})
}

// runRealAITest Execute real AI test call
func (s *Server) runRealAITest(userID, modelID, systemPrompt, userPrompt string) (string, error) {
	// Get AI model configuration
	model, err := s.store.AIModel().Get(userID, modelID)
	if err != nil {
		return "", fmt.Errorf("failed to get AI model: %w", err)
	}

	if !model.Enabled {
		return "", fmt.Errorf("AI model %s is not enabled", model.Name)
	}

	if model.APIKey == "" {
		return "", fmt.Errorf("AI model %s is missing API Key", model.Name)
	}

	// Create AI client via registry
	provider := model.Provider
	apiKey := string(model.APIKey)

	aiClient := mcp.NewAIClientByProvider(provider)
	if aiClient == nil {
		aiClient = mcp.NewClient()
	}

	// Payment providers ignore custom URL
	switch provider {
	case "claw402":
		aiClient.SetAPIKey(apiKey, "", model.CustomModelName)
	default:
		aiClient.SetAPIKey(apiKey, model.CustomAPIURL, model.CustomModelName)
	}

	// Call AI API
	response, err := aiClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("AI API call failed: %w", err)
	}

	return response, nil
}

func (s *Server) resolveStrategyDataWalletKey(userID, selectedModelID string) (string, error) {
	return s.store.AIModel().ResolveClaw402WalletKey(userID, selectedModelID)
}

type strategyVersionDTO struct {
	Version    int             `json:"version"`
	StrategyID string          `json:"strategy_id"`
	Label      string          `json:"label"`
	Note       string          `json:"note"`
	Config     json.RawMessage `json:"config"`
	CreatedAt  time.Time       `json:"created_at"`
	IsCurrent  bool            `json:"is_current"`
}

func toStrategyVersionDTO(v *store.StrategyVersion) strategyVersionDTO {
	return strategyVersionDTO{
		Version:    v.Version,
		StrategyID: v.StrategyID,
		Label:      fmt.Sprintf("v%d", v.Version),
		Note:       v.Note,
		Config:     json.RawMessage(v.Config),
		CreatedAt:  v.CreatedAt,
		IsCurrent:  v.IsCurrent,
	}
}

// runningTradersForStrategy returns the names of running traders linked to a strategy.
func (s *Server) runningTradersForStrategy(userID, strategyID string) ([]string, error) {
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		return nil, err
	}
	var running []string
	for _, t := range traders {
		if t.StrategyID == strategyID && t.IsRunning {
			running = append(running, t.Name)
		}
	}
	return running, nil
}

// handleListStrategyVersions List strategy version snapshots
func (s *Server) handleListStrategyVersions(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if _, err := s.store.Strategy().Get(userID, strategyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	versions, err := s.store.StrategyVersion().List(strategyID, userID)
	if err != nil {
		SafeInternalError(c, "Failed to get strategy versions", err)
		return
	}
	out := make([]strategyVersionDTO, 0, len(versions))
	for _, v := range versions {
		out = append(out, toStrategyVersionDTO(v))
	}
	c.JSON(http.StatusOK, gin.H{"versions": out})
}

// handleGetStrategyVersion Get a single strategy version snapshot
func (s *Server) handleGetStrategyVersion(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	versionStr := c.Param("version")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var version int
	if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
		SafeBadRequest(c, "Invalid version")
		return
	}
	v, err := s.store.StrategyVersion().Get(strategyID, userID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	c.JSON(http.StatusOK, toStrategyVersionDTO(v))
}

// handleRestoreStrategyVersion Restore a strategy to a saved version
func (s *Server) handleRestoreStrategyVersion(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var req struct {
		Version int `json:"version" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	// Blocked while a running trader uses the strategy (same guard as the editor).
	running, err := s.runningTradersForStrategy(userID, strategyID)
	if err != nil {
		SafeInternalError(c, "Check running traders", err)
		return
	}
	if len(running) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot restore a strategy while a trader is running on it. Stop the trader first."})
		return
	}
	v, err := s.store.StrategyVersion().Get(strategyID, userID, req.Version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	strategy, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	strategy.Config = v.Config
	if err := s.store.Strategy().Update(strategy); err != nil {
		SafeInternalError(c, "Failed to restore strategy", err)
		return
	}
	if err := s.store.StrategyVersion().SetCurrent(strategyID, userID, v.Version); err != nil {
		SafeInternalError(c, "Failed to mark version current", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Strategy restored to v%d", v.Version), "version": v.Version})
}

// navPointDTO is a single point on the merged NAV curve.
type navPointDTO struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalEquity float64   `json:"total_equity"`
}

// strategyStatsDTO is the aggregate stats payload for a strategy.
type strategyStatsDTO struct {
	AUM           float64       `json:"aum"`
	Symbols       []string      `json:"symbols"`
	NavPoints     []navPointDTO `json:"nav_points"`
	SevenDayYield *float64      `json:"seven_day_yield"`
	Sharpe        *float64      `json:"sharpe"`
	MaxDrawdown   *float64      `json:"max_drawdown"`
}

// handleGetStrategyStats returns aggregate stats for a strategy by merging the
// equity curves of all linked traders over the last 7 days.
func (s *Server) handleGetStrategyStats(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if _, err := s.store.Strategy().Get(userID, strategyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		SafeInternalError(c, "List traders", err)
		return
	}
	linked := make([]*store.Trader, 0)
	for _, t := range traders {
		if t.StrategyID == strategyID {
			linked = append(linked, t)
		}
	}

	// Merged equity curve over the last 7 days.
	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)
	series := make(map[int64]float64)
	order := make([]int64, 0)
	for _, t := range linked {
		snaps, err := s.store.Equity().GetByTimeRange(t.ID, start, now)
		if err != nil {
			continue
		}
		for _, snap := range snaps {
			key := snap.Timestamp.UTC().Unix()
			if _, ok := series[key]; !ok {
				order = append(order, key)
			}
			series[key] += snap.TotalEquity
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	nav := make([]navPointDTO, 0, len(order))
	for _, key := range order {
		nav = append(nav, navPointDTO{Timestamp: time.Unix(key, 0).UTC(), TotalEquity: series[key]})
	}

	dto := strategyStatsDTO{NavPoints: nav, Symbols: []string{}}
	// AUM = sum of latest equity per linked trader.
	var aum float64
	for _, t := range linked {
		if latest, err := s.store.Equity().GetLatest(t.ID, 1); err == nil && len(latest) > 0 {
			aum += latest[len(latest)-1].TotalEquity
		}
	}
	dto.AUM = aum

	// Open-position symbols across linked traders (union).
	symbolSet := make(map[string]bool)
	for _, t := range linked {
		if positions, err := s.store.Position().GetOpenPositions(t.ID); err == nil {
			for _, p := range positions {
				symbolSet[p.Symbol] = true
			}
		}
	}
	for sym := range symbolSet {
		dto.Symbols = append(dto.Symbols, sym)
	}

	// Metrics.
	dto.SevenDayYield = sevenDayYield(nav)
	dto.MaxDrawdown = maxDrawdown(nav)
	dto.Sharpe = sharpeFromCurve(nav)

	c.JSON(http.StatusOK, dto)
}

// sevenDayYield returns the percentage change from the first to the last NAV
// point, or nil when there are fewer than 2 points or the first point is zero.
func sevenDayYield(nav []navPointDTO) *float64 {
	if len(nav) < 2 || nav[0].TotalEquity == 0 {
		return nil
	}
	y := (nav[len(nav)-1].TotalEquity - nav[0].TotalEquity) / nav[0].TotalEquity * 100
	return &y
}

// maxDrawdown returns the maximum drawdown percentage from the running peak,
// or nil when there are fewer than 2 points.
func maxDrawdown(nav []navPointDTO) *float64 {
	if len(nav) < 2 {
		return nil
	}
	peak := nav[0].TotalEquity
	var maxDD float64
	for _, p := range nav {
		if p.TotalEquity > peak {
			peak = p.TotalEquity
		}
		if peak > 0 {
			dd := (peak - p.TotalEquity) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return &maxDD
}

// sharpeFromCurve computes annualized Sharpe from per-interval returns.
func sharpeFromCurve(nav []navPointDTO) *float64 {
	if len(nav) < 3 {
		return nil
	}
	returns := make([]float64, 0, len(nav)-1)
	for i := 1; i < len(nav); i++ {
		if nav[i-1].TotalEquity == 0 {
			return nil
		}
		returns = append(returns, (nav[i].TotalEquity-nav[i-1].TotalEquity)/nav[i-1].TotalEquity)
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns))
	if variance == 0 {
		return nil
	}
	std := math.Sqrt(variance)
	annual := math.Sqrt(float64(len(returns))) // per-sample annualization factor
	out := mean / std * annual
	return &out
}
