package kernel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BTC","pair":"BTCUSDT","score":78.3,"startPrice":0.5,"startTime":1785852000,"changePctValue":37.5,"signal":"Peak 87"}]}}`))
		case r.URL.Query().Get("tab") == "oi":
			w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":1,"oi_delta":1,"oi_delta_percent":5.0,"oi_delta_value":1,"price_delta_percent":3.0,"net_long":1,"net_short":1}],"low":[]}`))
		case r.URL.Query().Get("tab") == "net_flow":
			w.Write([]byte(`{"top":[{"amount":29309691.42,"price":64119.7,"price_delta_percent":0.23,"rank":7,"symbol":"BTCUSDT"}],"low":[]}`))
		case r.URL.Query().Get("tab") == "price":
			w.Write([]byte(`{"top":[{"pair":"BTCUSDT","symbol":"BTC","price_delta":0.031,"price":63910,"future_flow":0.8e6,"spot_flow":0.9e6,"oi":100,"oi_delta":10,"oi_delta_value":5.3e6}],"low":[]}`))
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
}

func containsCJK(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
