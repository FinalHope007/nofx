package vergex

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestFreeDetailSymbol(t *testing.T) {
	cases := []struct {
		marketType string
		symbol     string
		want       string
	}{
		// crypto symbols -> core_perp form (no address), regardless of marketType
		{"core_perp", "BTC", "core_perp:BTC"},
		{"core_perp", "core_perp:BTC", "core_perp:BTC"},
		{"perp", "BTC", "core_perp:BTC"},
		{"perp", "ETH", "core_perp:ETH"},
		{"perp", "PERP:LIT", "core_perp:LIT"},
		{"hip3_perp", "PUMP", "core_perp:PUMP"},
		{"all", "PUMP", "core_perp:PUMP"},
		// stock/xyz symbols -> hip3_perp form (address-qualified)
		{"hip3_perp", "xyz:SP500", "hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:SP500"},
		{"hip3_perp", "SP500", "hip3_perp:0x88806a71d74ad0a510b350545c9ae490912f0888:xyz:SP500"},
	}
	for _, c := range cases {
		got := FreeDetailSymbol(c.marketType, c.symbol)
		if got != c.want {
			t.Errorf("FreeDetailSymbol(%q, %q) = %q, want %q", c.marketType, c.symbol, got, c.want)
		}
	}
}

func TestResolveDetailMarket(t *testing.T) {
	cases := []struct{ symbol, want string }{
		{"BTC", "core_perp"},
		{"core_perp:BTC", "core_perp"},
		{"perp:ETH", "core_perp"},
		{"PUMP", "core_perp"},
		{"xyz:SP500", "hip3_perp"},
		{"SP500", "hip3_perp"},
	}
	for _, c := range cases {
		got := resolveDetailMarket(c.symbol)
		if got != c.want {
			t.Errorf("resolveDetailMarket(%q) = %q, want %q", c.symbol, got, c.want)
		}
	}
}

func TestDetailPathURLEncodesColons(t *testing.T) {
	// The symbol segment must render colons as %3A so the request matches the
	// verified working curl: .../hip3_perp/hip3_perp%3A...%3Axyz%3ASP500/riskbins
	sym := FreeDetailSymbol("hip3_perp", "SP500")
	enc := strings.ReplaceAll(sym, ":", "%3A")
	want := "hip3_perp%3A0x88806a71d74ad0a510b350545c9ae490912f0888%3Axyz%3ASP500"
	if enc != want {
		t.Errorf("encoded = %q, want %q", enc, want)
	}
}
