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
			"technical_score_1h":{"value":"7.85","valueLabel":"Positive"},
			"technical_summary_1h":{"value":"Bullish overall for BTC.","valueLabel":"Bullish overall for BTC."}
		}},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["technical_score_1h"] != "Positive" {
		t.Fatalf("expected score label, got %q", got["technical_score_1h"])
	}
	if !strings.Contains(got["technical_summary_1h"], "Bullish overall") {
		t.Fatalf("expected summary, got %q", got["technical_summary_1h"])
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
			"technical_score_1h":{"value":"7.85","valueLabel":"Positive"}
		}},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	got, err := c.GetAssetDetails(context.Background(), "BTC", "technical", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["technical_score_1h"] != "Positive" {
		t.Fatalf("expected score label after retry, got %q", got["technical_score_1h"])
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts (1 fail + 1 retry), got %d", attempts)
	}
}
