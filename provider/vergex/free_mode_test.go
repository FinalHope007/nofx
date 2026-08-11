package vergex

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewFreeClient_NoKey_PlainGET(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/direction-change/leaderboard" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"symbol":"PUMP","bias":"bullish","rank":1,"market":{"marketType":"core_perp"}}]}`))
	}))
	defer srv.Close()

	c, err := NewFreeClient(srv.URL, "tok", nil)
	if err != nil {
		t.Fatalf("NewFreeClient: %v", err)
	}
	if c.freeMode != true {
		t.Fatalf("freeMode = false, want true")
	}
	data, err := c.GetSignalRanking(t.Context(), Query{})
	if err != nil {
		t.Fatalf("GetSignalRanking free: %v", err)
	}
	if len(data.Items) != 1 || data.Items[0].Symbol != "PUMP" {
		t.Fatalf("items = %+v", data.Items)
	}
}

func TestClient_GetStockMovers_freePath(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/market-data/hl-stocks-movers" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("direction") != "gainers" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"direction":"gainers","entries":[{"symbol":"SMSN-USDC","rank":1,"change24hPct":5.2}]}`))
	}))
	defer srv.Close()

	c, err := NewFreeClient(srv.URL, "", nil)
	if err != nil {
		t.Fatalf("NewFreeClient: %v", err)
	}
	data, err := c.GetStockMovers("gainers", 10)
	if err != nil {
		t.Fatalf("GetStockMovers: %v", err)
	}
	if len(data.Items) != 1 || data.Items[0].Symbol != "SMSN" {
		t.Fatalf("items = %+v", data.Items)
	}
}
