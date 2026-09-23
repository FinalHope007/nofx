package nofxos

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFreeTrendingClient_GetOITop(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	body := `{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":100,"oi_delta":5,"oi_delta_percent":15.5,"oi_delta_value":150000,"price_delta_percent":2.1,"net_long":10,"net_short":5}],"low":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tab") != "oi" {
			t.Fatalf("tab = %q, want oi", r.URL.Query().Get("tab"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	pos, err := c.GetOITop(10)
	if err != nil {
		t.Fatalf("GetOITop: %v", err)
	}
	if len(pos) != 1 || pos[0].Symbol != "BTC" || pos[0].OIDeltaPercent != 15.5 {
		t.Fatalf("unexpected %+v", pos)
	}
}

func TestFreeTrendingClient_GetPriceLow(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	body := `{"top":[],"low":[{"pair":"ETHUSDT","symbol":"ETH","price_delta":-0.12,"price":3000,"future_flow":0,"spot_flow":0,"oi":0,"oi_delta":0,"oi_delta_value":0}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tab") != "price" {
			t.Fatalf("tab = %q, want price", r.URL.Query().Get("tab"))
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	items, err := c.GetPriceLow(10)
	if err != nil {
		t.Fatalf("GetPriceLow: %v", err)
	}
	if len(items) != 1 || items[0].Symbol != "ETH" || items[0].PriceDelta != -0.12 {
		t.Fatalf("unexpected %+v", items)
	}
}

func TestFreeTrendingClient_GetAI500(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	body := `{"category":{"assets":[{"symbol":"CYS","pair":"CYSUSDT","score":75.0,"startTime":1785852000,"startPrice":0.51,"changePctValue":143.7}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	coins, err := c.GetAI500()
	if err != nil {
		t.Fatalf("GetAI500: %v", err)
	}
	if len(coins) != 1 || coins[0].Pair != "CYSUSDT" || coins[0].Score != 75.0 {
		t.Fatalf("unexpected %+v", coins)
	}
}

func TestFreeTrendingClient_SendsBrowserHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go test http server runs on 127.0.0.1; the SSRF-guarded clients would block it,
		// so assert headers via a direct call path with a stdlib-backed client.
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Mozilla") {
			t.Errorf("missing browser UA, got %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Referer") != "https://vergex.trade/" {
			t.Errorf("missing Referer, got %q", r.Header.Get("Referer"))
		}
		w.Write([]byte(`{"category":{"assets":[]}}`))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	c.http = srv.Client() // stdlib client bypasses fingerprint/curl for this assertion
	if _, err := c.GetAI500(); err != nil {
		t.Fatalf("GetAI500: %v", err)
	}
}
