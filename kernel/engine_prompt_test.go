package kernel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nofx/market"
	"nofx/provider/binance"
	"nofx/provider/nofxos"
	"nofx/store"
)

func TestBuildSystemPromptUsesVergexClaw402Prompt(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.SourceType = "vergex_signal"
	cfg.CoinSource.VergexLimit = 5
	cfg.PromptSections.RoleDefinition = "# You are a professional Hyperliquid USDC multi-asset trading AI"
	cfg.CustomPrompt = "Long only, no shorts."

	engine := NewStrategyEngine(&cfg)
	prompt := engine.BuildSystemPrompt(30, "balanced")

	if !strings.Contains(prompt, "NOFX Claw402 auto-trader") {
		t.Fatalf("prompt did not use the Claw402/Vergex TradeFi role:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Claw402.ai Signal Ranking") || !strings.Contains(prompt, "Signal Lab") || !strings.Contains(prompt, "Cost/Liquidation Heatmap") {
		t.Fatalf("prompt is missing Claw402/Vergex detail data guidance:\n%s", prompt)
	}
	if !strings.Contains(prompt, "open_short") {
		t.Fatalf("prompt should explicitly allow short entries:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Direction must be data-driven") {
		t.Fatalf("prompt should explain that direction is data-driven, not long-only:\n%s", prompt)
	}
	if !strings.Contains(prompt, "every open position must use exactly 10x") {
		t.Fatalf("prompt should force 10x leverage for Claw402 opens:\n%s", prompt)
	}
	if !strings.Contains(prompt, "use the full max notional per position") {
		t.Fatalf("prompt should force full-size Claw402 opens:\n%s", prompt)
	}
	if containsCJK(prompt) {
		t.Fatalf("system prompt must be English-only, got CJK text:\n%s", prompt)
	}
	legacyPhrases := []string{
		"Hyperliquid USDC multi-asset trading AI",
		"Long only",
		"Altcoin",
		"BTC/ETH",
		"LONG-ONLY",
		"Do not short",
		"MUST open a long",
	}
	for _, phrase := range legacyPhrases {
		if strings.Contains(prompt, phrase) {
			t.Fatalf("prompt still contains legacy phrase %q:\n%s", phrase, prompt)
		}
	}
}

func TestBuildSystemPromptFallsBackToEnglishWhenConfiguredLanguageIsChinese(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.SourceType = "static"
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT"}
	cfg.CoinSource.VergexLimit = 0
	cfg.CoinSource.VergexMarketType = ""
	cfg.CoinSource.VergexChain = ""
	cfg.PromptSections.RoleDefinition = "# You are a Chinese system prompt"
	cfg.PromptSections.TradingFrequency = "# High-frequency trading\nTrade every minute."
	cfg.PromptSections.EntryStandards = "# Entry\nOpen positions freely."
	cfg.PromptSections.DecisionProcess = "# Decision\nOutput directly."
	cfg.CustomPrompt = "Chinese preference should not enter the system prompt."

	engine := NewStrategyEngine(&cfg)
	prompt := engine.BuildSystemPrompt(30, "balanced")

	required := []string{
		"Data Dictionary & Trading Rules",
		"You are a professional Hyperliquid USDC multi-asset trading AI",
		"Trading Frequency Awareness",
		"Entry Standards",
		"Decision Process",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("English fallback prompt missing %q:\n%s", phrase, prompt)
		}
	}
	if containsCJK(prompt) {
		t.Fatalf("system prompt must be English-only, got CJK text:\n%s", prompt)
	}
}

func TestBuildSystemPromptDoesNotForceLongOnlyForSingleXYZ(t *testing.T) {
	prompt := buildXYZStockCustomPrompt("XYZ:INTC")

	required := []string{
		"DIRECTIONAL, SIGNAL-DRIVEN",
		"You may open long or short",
		"open_short",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("single XYZ prompt missing %q:\n%s", phrase, prompt)
		}
	}

	forbidden := []string{
		"LONG-ONLY",
		"Do not short",
		"MUST open a long",
		"Probing > waiting",
	}
	for _, phrase := range forbidden {
		if strings.Contains(prompt, phrase) {
			t.Fatalf("single XYZ prompt still contains forced-long phrase %q:\n%s", phrase, prompt)
		}
	}
}

func TestEnginePerCoinSignals_StoreAndGet(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	e := NewStrategyEngine(&cfg)
	if s, ok := e.PerCoinSignalFor("BTCUSDT"); ok {
		t.Fatalf("unexpected existing signal for BTCUSDT: %+v", s)
	}
	sig := PerCoinSignal{AI500: &nofxos.CoinData{Pair: "CYSUSDT", Score: 78.3}}
	e.SetPerCoinSignals(map[string]PerCoinSignal{"CYSUSDT": sig})
	if s, ok := e.PerCoinSignalFor("CYSUSDT"); !ok || s.AI500 == nil || s.AI500.Score != 78.3 {
		t.Fatalf("PerCoinSignalFor: %+v", s)
	}
}

func TestAttachPerCoinSignals_filtersToCandidates(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAI500Data = true
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.EnableNetflowData = true
	cfg.Indicators.EnablePriceData = true
	cfg.Indicators.DataDurations = []string{"1h"}
	e := NewStrategyEngine(&cfg)
	tr := nofxos.NewFreeTrendingClient()
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	fsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/trending-category":
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BTC","pair":"BTCUSDT","score":78.3,"startPrice":0.5,"startTime":1785852000,"changePctValue":37.5,"signal":"Peak 87"},{"symbol":"ETH","pair":"ETHUSDT","score":55.1,"startPrice":18.0,"startTime":1785852000,"changePctValue":12.0,"signal":"Low 50"}]}}`))
		case r.URL.Query().Get("tab") == "oi":
			w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":1,"oi_delta":1,"oi_delta_percent":5.0,"oi_delta_value":1,"price_delta_percent":3.0,"net_long":1,"net_short":1},{"symbol":"ETH","rank":2,"price":500,"current_oi":1,"oi_delta":1,"oi_delta_percent":4.0,"oi_delta_value":1,"price_delta_percent":2.0,"net_long":1,"net_short":1}],"low":[]}`))
		case r.URL.Query().Get("tab") == "net_flow":
			w.Write([]byte(`{"top":[{"amount":29309691.42,"price":64119.7,"price_delta_percent":0.23,"rank":7,"symbol":"BTCUSDT"},{"amount":1000000.0,"price":500.0,"price_delta_percent":0.10,"rank":8,"symbol":"ETHUSDT"}],"low":[]}`))
		case r.URL.Query().Get("tab") == "price":
			w.Write([]byte(`{"top":[{"pair":"BTCUSDT","symbol":"BTC","price_delta":0.031,"price":63910,"future_flow":0.8e6,"spot_flow":0.9e6,"oi":100,"oi_delta":10,"oi_delta_value":5.3e6},{"pair":"ETHUSDT","symbol":"ETH","price_delta":0.021,"price":500,"future_flow":0.2e6,"spot_flow":0.3e6,"oi":50,"oi_delta":5,"oi_delta_value":1.2e6}],"low":[]}`))
		default:
			t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer fsrv.Close()
	tr.SetBaseURL(fsrv.URL)
	e.trending = tr

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT", Sources: []string{"oi_top"}}},
	}
	if err := attachPerCoinSignals(ctx, e); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	g, ok := e.PerCoinSignalFor("BTCUSDT")
	if !ok {
		t.Fatalf("BTCUSDT not attached")
	}
	if g.AI500 == nil || g.AI500.PeakScore != 87 {
		t.Fatalf("AI500 not attached/parsed: %+v", g.AI500)
	}
	if g.OI == nil || len(g.OI) == 0 {
		t.Fatalf("OI not attached: %+v", g.OI)
	}
	if g.Netflow == nil || len(g.Netflow) == 0 {
		t.Fatalf("netflow not attached: %+v", g.Netflow)
	}
	if g.Price == nil || len(g.Price) == 0 {
		t.Fatalf("price not attached: %+v", g.Price)
	}
	if _, ok := e.PerCoinSignalFor("ETHUSDT"); ok {
		t.Fatalf("non-candidate symbol ETHUSDT must be filtered out but was attached")
	}
}

func TestFormatPerCoinSignals_rendersEnabledSources(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAI500Data = true
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.EnableNetflowData = true
	cfg.Indicators.EnablePriceData = true
	cfg.Indicators.DataDurations = []string{"1h"}
	e := NewStrategyEngine(&cfg)

	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"CYSUSDT": {
			AI500:   &nofxos.CoinData{Pair: "CYSUSDT", Score: 78.3, PeakScore: 87, StartPrice: 0.5117, IncreasePercent: 180.9},
			OI:      map[string]map[string]nofxos.OIPosition{"1h": {"top:CYSUSDT": {Symbol: "CYSUSDT", Rank: 4, OIDeltaValue: 1.2e6, OIDeltaPercent: 5.3, PriceDeltaPercent: 3.1, CurrentOI: 3.85e7, NetLong: 12e6, NetShort: 9e6}}},
			Netflow: map[string]map[string]nofxos.NetFlowPosition{"1h": {"top:CYSUSDT": {Symbol: "CYSUSDT", Rank: 7, Amount: 2.4e6, Price: 1.3821}}},
			Price:   map[string]map[string]nofxos.PriceRankingItem{"1h": {"top:CYSUSDT": {Symbol: "CYSUSDT", PriceDelta: 0.031, SpotFlow: 0.9e6, FutureFlow: 0.8e6, OIDeltaValue: 5.3e6, Price: 1.3821}}},
		},
	})

	out := e.formatPerCoinSignals("CYSUSDT", 1.3821)
	if out == "" {
		t.Fatalf("expected non-empty render")
	}
	for _, want := range []string{"AI500 Signal", "AI score 78.3", "peak score 87", "since starting alert", "Open Interest", "[1h \u00b7 Increase]", "Net Flow", "inflow", "Price Change", "price change +3.1%"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatPerCoinSignals_omitsMissing(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAI500Data = true
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.EnableNetflowData = true
	cfg.Indicators.EnablePriceData = true
	cfg.Indicators.DataDurations = []string{"1h"}
	e := NewStrategyEngine(&cfg)

	// The shared maps are populated for a DIFFERENT symbol ("AAPL"); the
	// queried symbol "CYSUSDT" has no OI/netflow/price rows of its own. Any
	// empty section header emitted for CYSUSDT would fail this test.
	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"AAPL": {
			OI:      map[string]map[string]nofxos.OIPosition{"1h": {"top:AAPL": {Symbol: "AAPL", Rank: 1}}},
			Netflow: map[string]map[string]nofxos.NetFlowPosition{"1h": {"top:AAPL": {Symbol: "AAPL", Rank: 1}}},
			Price:   map[string]map[string]nofxos.PriceRankingItem{"1h": {"top:AAPL": {Symbol: "AAPL"}}},
		},
	})

	out := e.formatPerCoinSignals("CYSUSDT", 1.3821)
	if out != "" {
		t.Fatalf("expected empty render for symbol with no rows, got:\n%s", out)
	}
}

func TestFormatUSDCompact_negative(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{-5.0e5, "-$500.0K"},
		{-5.0e6, "-$5.0M"},
		{5.0e6, "$5.0M"},
		{-123, "-$123.00"},
		{0, "$0.00"},
	}
	for _, c := range cases {
		if got := formatUSDCompact(c.in); got != c.want {
			t.Errorf("formatUSDCompact(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatPerCoinSignals_negativeOIDeltaRendersDecrease(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableOIData = true
	cfg.Indicators.DataDurations = []string{"1h"}
	e := NewStrategyEngine(&cfg)

	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"CYSUSDT": {
			OI: map[string]map[string]nofxos.OIPosition{"1h": {"low:CYSUSDT": {Symbol: "CYSUSDT", Rank: 2, OIDeltaPercent: -4.2, OIDeltaValue: -5.0e5, PriceDeltaPercent: -1.0, CurrentOI: 1.0e7, NetLong: 1.0e6, NetShort: 2.0e6}}},
		},
	})

	out := e.formatPerCoinSignals("CYSUSDT", 1.3821)
	if !strings.Contains(out, "[1h \u00b7 Decrease]") {
		t.Fatalf("expected decrease-marked OI line with negative delta:\n%s", out)
	}
	if !strings.Contains(out, "-$500.0K") {
		t.Fatalf("expected compact negative OI delta rendered as -$500.0K:\n%s", out)
	}
}

func containsCJK(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func TestRenderRecentDecisionsStructured(t *testing.T) {
	cfg := store.DecisionContextConfig{Enabled: true, RecentCount: 2, Mode: "structured"}
	records := []*store.DecisionRecord{
		{Timestamp: time.Now().Add(-2 * time.Hour), RawResponse: "decision A"},
		{Timestamp: time.Now().Add(-1 * time.Hour), RawResponse: "decision B"},
	}
	out := renderRecentDecisions(&cfg, records)
	if out == "" {
		t.Fatal("expected non-empty recent-decisions section")
	}
	if !strings.Contains(out, "decision A") || !strings.Contains(out, "decision B") {
		t.Fatalf("missing prior responses: %q", out)
	}
}

func TestFormatPerCoinSignalsBinance(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h"}
	cfg.Indicators.EnableBinanceSentimentData = true
	e := NewStrategyEngine(cfg)
	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"BTCUSDT": {
			BinanceTechnical: map[string]*binance.BinanceAssetDetail{
				"1h": {
					Metrics: map[string]binance.BinanceMetric{
						"technical_summary_1h": {ValueLabel: "Bullish overall for BTC."},
						"technical_score_1h":   {Value: "8.73", ValueLabel: "Strong Positive"},
					},
				},
			},
			BinanceSentiment: map[string]string{"sentiment_summary": "In the past 24h BTC was bullish."},
		},
	})
	out := e.formatPerCoinSignals("BTCUSDT", 60000)
	if !strings.Contains(out, "Binance Technical") || !strings.Contains(out, "Bullish overall") {
		t.Fatalf("missing technical section: %s", out)
	}
	if !strings.Contains(out, "Sentiment") || !strings.Contains(out, "In the past 24h") {
		t.Fatalf("missing sentiment section: %s", out)
	}
}

func TestFormatPerCoinSignalsBinanceRichDetail(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h"}
	e := NewStrategyEngine(cfg)
	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"BTCUSDT": {
			BinanceTechnical: map[string]*binance.BinanceAssetDetail{
				"1h": {
					Metrics: map[string]binance.BinanceMetric{
						"technical_summary_1h":     {ValueLabel: "Bullish overall for BTC."},
						"technical_score_1h":       {Value: "8.73", ValueLabel: "Strong Positive"},
						"technical_score_trend_1h": {Value: "9.52", ValueLabel: "Bullish"},
					},
					Categories: []binance.BinanceCategory{
						{
							Category: "Trend Indicators",
							SubIndicators: []binance.BinanceSubIndicator{
								{Title: "MACD (Moving Average Convergence Divergence)", Signal: "Weak Bullish", Score: "7.00", Summary: "MACD remains weakly bullish."},
							},
						},
					},
				},
			},
		},
	})
	out := e.formatPerCoinSignals("BTCUSDT", 60000)
	for _, want := range []string{
		"Binance Technical",
		"Bullish overall",
		"Overall Score: Strong Positive (8.73/10.00)",
		"--- Trend Indicators (Score: Bullish 9.52/10.00) ---",
		"MACD (Moving Average Convergence Divergence): Weak Bullish (score: 7.00/10.00) - MACD remains weakly bullish."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatPerCoinSignalsBinance24h(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"1h", "24h"}
	e := NewStrategyEngine(cfg)
	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"BTCUSDT": {
			BinanceTechnical: map[string]*binance.BinanceAssetDetail{
				"1h": {
					Metrics: map[string]binance.BinanceMetric{
						"technical_summary_1h": {ValueLabel: "Bullish 1h."},
					},
				},
				"24h": {
					Metrics: map[string]binance.BinanceMetric{
						// The API uses the _1d suffix for the 24h interval.
						"technical_summary_1d": {ValueLabel: "Bullish 24h."},
						"technical_score_1d":   {Value: "7.50", ValueLabel: "Positive"},
					},
				},
			},
		},
	})
	out := e.formatPerCoinSignals("BTCUSDT", 60000)
	if !strings.Contains(out, "[1h]") || !strings.Contains(out, "Bullish 1h.") {
		t.Fatalf("missing 1h section:\n%s", out)
	}
	if !strings.Contains(out, "[24h]") || !strings.Contains(out, "Bullish 24h.") {
		t.Fatalf("missing 24h section (24h uses _1d suffix):\n%s", out)
	}
	if !strings.Contains(out, "Overall Score: Positive (7.50/10.00)") {
		t.Fatalf("missing 24h overall score:\n%s", out)
	}
}

func TestFormatPerCoinSignalsBinance24hCategoryScoreAndSummary(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableBinanceTechnicalData = true
	cfg.Indicators.BinanceTechnicalIntervals = []string{"24h"}
	e := NewStrategyEngine(cfg)
	e.SetPerCoinSignals(map[string]PerCoinSignal{
		"TIAUSDT": {
			BinanceTechnical: map[string]*binance.BinanceAssetDetail{
				"24h": {
					Metrics: map[string]binance.BinanceMetric{
						"technical_summary_1d":          {ValueLabel: "Bullish near term."},
						"technical_score_1d":            {Value: "7.23", ValueLabel: "Positive"},
						"technical_score_volatility_1d": {Value: "7.40", ValueLabel: "Volatility Expansion"},
						"technical_ind_rsi_signal_1d":   {Value: "7.00", ValueLabel: "Neutral to Bullish", Label: "RSI (Relative Strength Index)"},
						"technical_ind_rsi_summary_1d":  {Value: "RSI at 58 shows momentum.", ValueLabel: "RSI at 58 shows momentum.", Label: "technical_ind_rsi_summary_1d_name"},
						"technical_ind_atr_signal_1d":   {Value: "10.00", ValueLabel: "Extreme Volatility", Label: "ATR (Average True Range)"},
						"technical_ind_atr_summary_1d":  {Value: "ATR signals extreme volatility.", ValueLabel: "ATR signals extreme volatility.", Label: "technical_ind_atr_summary_1d_name"},
					},
					Categories: []binance.BinanceCategory{
						{Category: "Volatility Indicators", SubIndicators: []binance.BinanceSubIndicator{
							{Title: "ATR (Average True Range)", Signal: "Extreme Volatility", Score: "10.00", Summary: "ATR signals extreme volatility."},
						}},
						{Category: "Momentum Indicators", SubIndicators: []binance.BinanceSubIndicator{
							{Title: "RSI (Relative Strength Index)", Signal: "Neutral to Bullish", Score: "7.00", Summary: "RSI at 58 shows momentum."},
						}},
					},
				},
			},
		},
	})
	out := e.formatPerCoinSignals("TIAUSDT", 3.0)

	// Issue 2: the category header must include the category score.
	if !strings.Contains(out, "--- Volatility Indicators (Score: Volatility Expansion 7.40/10.00) ---") {
		t.Fatalf("missing category score in header:\n%s", out)
	}
	// Issue 1: 24h subindicator summaries must be included (not just the signal label).
	if !strings.Contains(out, "ATR (Average True Range): Extreme Volatility (score: 10.00/10.00) - ATR signals extreme volatility.") {
		t.Fatalf("missing 24h subindicator summary:\n%s", out)
	}
	if !strings.Contains(out, "RSI (Relative Strength Index): Neutral to Bullish (score: 7.00/10.00) - RSI at 58 shows momentum.") {
		t.Fatalf("missing 24h RSI summary:\n%s", out)
	}
}

func TestRenderRecentDecisionsDigestAndDisabled(t *testing.T) {
	disabled := store.DecisionContextConfig{Enabled: false, RecentCount: 2, Mode: "structured"}
	if out := renderRecentDecisions(&disabled, nil); out != "" {
		t.Fatalf("expected empty when disabled, got %q", out)
	}
	digest := store.DecisionContextConfig{Enabled: true, RecentCount: 1, Mode: "digest"}
	records := []*store.DecisionRecord{{Timestamp: time.Now(), RawResponse: "a very long response " + strings.Repeat("x", 200)}}
	out := renderRecentDecisions(&digest, records)
	if !strings.Contains(out, "a very long response") {
		t.Fatalf("digest missing snippet: %q", out)
	}
}

// TestFormatMarketDataSigFigs verifies that price-like fields in the prompt are
// rendered with exchange-standard significant figures (5 sig figs, floor 2dp)
// rather than a fixed 4 decimal places.
func TestFormatMarketDataSigFigs(t *testing.T) {
	cfg := &store.StrategyConfig{}
	e := NewStrategyEngine(cfg)

	data := &market.Data{
		Symbol:       "BTCUSDT",
		CurrentPrice: 76708.9123,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"1h": {
				Timeframe: "1h",
				Klines: []market.KlineBar{
					{Time: 1700000000000, Open: 76700.1234, High: 76800.9876, Low: 76650.5432, Close: 76708.9123, Volume: 12.5},
				},
			},
		},
	}
	out := e.formatMarketData(data)

	if !strings.Contains(out, "current_price = 76708.91") {
		t.Fatalf("current_price not at exchange precision:\n%s", out)
	}
	// The OHLC table must keep 5 sig figs (BTC keeps its cents).
	if !strings.Contains(out, "76700.12") || !strings.Contains(out, "76800.99") ||
		!strings.Contains(out, "76650.54") || !strings.Contains(out, "76708.91") {
		t.Fatalf("OHLC row not at exchange precision:\n%s", out)
	}
	// Must not fall back to the old fixed 4-decimal rendering.
	if strings.Contains(out, "76708.9123") {
		t.Fatalf("unexpected full-precision leakage:\n%s", out)
	}
}

func TestFormatTimeframeSeriesDataHonorsSelectedPeriods(t *testing.T) {
	cfg := &store.StrategyConfig{
		Indicators: store.IndicatorConfig{
			EnableEMA:  true,
			EnableRSI:  true,
			EnableATR:  true,
			EnableBOLL: true,
		},
	}
	e := NewStrategyEngine(cfg)

	data := &market.Data{
		Symbol: "BTCUSDT",
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"1h": {
				Timeframe: "1h",
				Periods: market.IndicatorPeriods{
					EMA:  []int{10},
					RSI:  []int{21},
					ATR:  []int{7},
					BOLL: []int{50},
				},
				EMA10Values:  []float64{1.5},
				RSI21Values:  []float64{55},
				ATR7:         2.5,
				MidPrices:    []float64{100},
				BOLL50Upper:  []float64{110},
				BOLL50Middle: []float64{100},
				BOLL50Lower:  []float64{90},
			},
		},
	}
	out := e.formatMarketData(data)

	for _, want := range []string{"EMA10", "RSI21", "ATR7", "BOLL(50)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"EMA20", "EMA50", "RSI7", "RSI14", "ATR14"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("output unexpectedly contains %q:\n%s", unwanted, out)
		}
	}
}

func TestFormatTimeframeSeriesDataVolumeGating(t *testing.T) {
	base := market.TimeframeSeriesData{
		Timeframe: "1h",
		MidPrices: []float64{100},
		Volume:    []float64{12.5},
	}
	data := &market.Data{
		Symbol:        "BTCUSDT",
		TimeframeData: map[string]*market.TimeframeSeriesData{"1h": &base},
	}

	disabled := NewStrategyEngine(&store.StrategyConfig{
		Indicators: store.IndicatorConfig{EnableVolume: false},
	})
	out := disabled.formatMarketData(data)
	if strings.Contains(out, "Volume:") {
		t.Fatalf("volume line present when EnableVolume=false:\n%s", out)
	}

	enabled := NewStrategyEngine(&store.StrategyConfig{
		Indicators: store.IndicatorConfig{EnableVolume: true},
	})
	out = enabled.formatMarketData(data)
	if !strings.Contains(out, "Volume:") {
		t.Fatalf("volume line missing when EnableVolume=true:\n%s", out)
	}
}

// TestFormatMarketDataSigFigsLowPriceCoin verifies sub-cent coins keep their
// magnitude instead of being flattened to 4 decimals.
func TestFormatMarketDataSigFigsLowPriceCoin(t *testing.T) {
	cfg := &store.StrategyConfig{}
	e := NewStrategyEngine(cfg)

	data := &market.Data{
		Symbol:       "PEPEUSDT",
		CurrentPrice: 0.005568,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"1h": {
				Timeframe: "1h",
				Klines: []market.KlineBar{
					{Time: 1700000000000, Open: 0.0055, High: 0.0056, Low: 0.0054, Close: 0.005568, Volume: 100},
				},
			},
		},
	}
	out := e.formatMarketData(data)

	if !strings.Contains(out, "current_price = 0.0055680") {
		t.Fatalf("sub-cent current_price lost precision:\n%s", out)
	}
	if !strings.Contains(out, "0.0055680") {
		t.Fatalf("sub-cent OHLC close lost precision:\n%s", out)
	}
}
