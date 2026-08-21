package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetOpportunityAssetsTechnical(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "technical" || r.URL.Query().Get("interval") != "1h" {
			t.Errorf("unexpected query: %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"items":[
			{"asset":"TREE","rank":1,"metrics":{"technical_score_1h":{"value":"9.45"}}},
			{"asset":"BTC","rank":2,"metrics":{"technical_score_1h":{"value":"7.85"}}}
		]},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	assets, err := c.GetOpportunityAssets(context.Background(), "1h", "technical")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets[0].Symbol != "TREE" || assets[0].Score != 9.45 {
		t.Fatalf("unexpected first asset: %+v", assets[0])
	}
}

func TestGetAssetDetails(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asset-details" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("asset"); got != "BTC" {
			t.Errorf("expected asset=BTC, got %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"metrics":{
			"technical_score_1h":{"value":"7.85","valueLabel":"Positive","label":"Technical Score"},
			"technical_summary_1h":{"value":"Bullish overall for BTC.","valueLabel":"Bullish overall for BTC.","label":"Summary"}
		}},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	labels := got.LabelMap()
	if labels["technical_score_1h"] != "Positive" {
		t.Fatalf("expected score label, got %q", labels["technical_score_1h"])
	}
	if !strings.Contains(labels["technical_summary_1h"], "Bullish overall") {
		t.Fatalf("expected summary, got %q", labels["technical_summary_1h"])
	}
	// Verify the numeric value and label are retained in the structured metric.
	if got.Metrics["technical_score_1h"].Value != "7.85" {
		t.Fatalf("expected numeric value 7.85, got %q", got.Metrics["technical_score_1h"].Value)
	}
}

func TestGetAssetDetailsParsesUiModulesCategories(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{
			"metrics":{
				"technical_ind_rsi_signal_1h":{"value":"10.00","valueLabel":"Overbought","label":"RSI (Relative Strength Index)"},
				"technical_ind_rsi_summary_1h":{"value":"RSI at 92 signals overbought.","valueLabel":"RSI at 92 signals overbought.","label":"RSI (Relative Strength Index)"},
				"technical_ind_volume_signal_1h":{"value":"7.00","valueLabel":"1.2-1.5x average volume","label":"Volume"},
				"technical_ind_volume_summary_1h":{"value":"Volume is healthy.","valueLabel":"Volume is healthy.","label":"Volume"}
			},
			"uiModules":{"technicalIndicatorSummariesModule":[
				{"title":"Momentum Indicators","items":[
					{"title":"RSI (Relative Strength Index)","summary":"uiModules RSI summary","signal":"Overbought"}
				]},
				{"title":"Volume & Price Indicators","items":[
					{"title":"Volume","summary":"uiModules Volume summary","signal":"1.2-1.5x average volume"}
				]}
			]}
		},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(got.Categories))
	}
	// First category: Momentum Indicators -> RSI
	cat := got.Categories[0]
	if cat.Category != "Momentum Indicators" {
		t.Fatalf("expected Momentum Indicators first, got %q", cat.Category)
	}
	if len(cat.SubIndicators) != 1 {
		t.Fatalf("expected 1 subindicator in momentum, got %d", len(cat.SubIndicators))
	}
	rsi := cat.SubIndicators[0]
	if rsi.Score != "10.00" {
		t.Fatalf("expected RSI score 10.00, got %q", rsi.Score)
	}
	// Summary should come from the correlated *_summary_* metric (not uiModules)
	if rsi.Summary != "RSI at 92 signals overbought." {
		t.Fatalf("expected correlated RSI summary, got %q", rsi.Summary)
	}
	if rsi.Signal != "Overbought" {
		t.Fatalf("expected RSI signal Overbought, got %q", rsi.Signal)
	}
}

func TestGetAssetDetails24hFallsBackToStaticCategories(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 24h detail: metrics present but uiModules.technicalIndicatorSummariesModule is empty.
		w.Write([]byte(`{"code":"000000","data":{
			"metrics":{
				"technical_ind_rsi_signal_1d":{"value":"10.00","valueLabel":"Overbought","label":"RSI (Relative Strength Index)"},
				"technical_ind_rsi_summary_1d":{"value":"RSI at 92 signals overbought.","valueLabel":"RSI at 92 signals overbought.","label":"RSI (Relative Strength Index)"},
				"technical_ind_macd_signal_1d":{"value":"9.00","valueLabel":"Golden Cross","label":"MACD (Moving Average Convergence Divergence)"},
				"technical_ind_macd_summary_1d":{"value":"MACD golden cross.","valueLabel":"MACD golden cross.","label":"MACD (Moving Average Convergence Divergence)"}
			},
			"uiModules":{"technicalIndicatorSummariesModule":[]}
		},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "24h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Even though the 24h endpoint returns no uiModules, the static category
	// mapping should group the present subindicators by category.
	if len(got.Categories) != 2 {
		t.Fatalf("expected 2 categories from static fallback, got %d", len(got.Categories))
	}
	// Category order follows staticTechnicalCategories: Trend first, then Momentum.
	if got.Categories[0].Category != "Trend Indicators" {
		t.Fatalf("expected Trend Indicators first, got %q", got.Categories[0].Category)
	}
	if got.Categories[1].Category != "Momentum Indicators" {
		t.Fatalf("expected Momentum Indicators second, got %q", got.Categories[1].Category)
	}
	// Trend category should contain MACD.
	macd := got.Categories[0].SubIndicators[0]
	if macd.Title != "MACD (Moving Average Convergence Divergence)" || macd.Score != "9.00" || macd.Signal != "Golden Cross" {
		t.Fatalf("unexpected MACD subindicator: %+v", macd)
	}
	// Momentum category should contain RSI.
	rsi := got.Categories[1].SubIndicators[0]
	if rsi.Title != "RSI (Relative Strength Index)" || rsi.Score != "10.00" || rsi.Signal != "Overbought" {
		t.Fatalf("unexpected RSI subindicator: %+v", rsi)
	}
	if rsi.Summary != "RSI at 92 signals overbought." {
		t.Fatalf("expected RSI summary from correlated metric, got %q", rsi.Summary)
	}
}

func TestGetAssetDetailsRetryOn429(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"code":"429","message":"rate limit exceeded"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"metrics":{
			"technical_score_1h":{"value":"7.85","valueLabel":"Positive","label":"Technical Score"}
		}},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LabelMap()["technical_score_1h"] != "Positive" {
		t.Fatalf("expected score label after retry, got %q", got.LabelMap()["technical_score_1h"])
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts (1 fail + 1 retry), got %d", attempts)
	}
}
