package kernel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nofx/provider/nofxos"
	"nofx/provider/vergex"
	"nofx/store"
)

func TestEngine_getOITopCoins_usesFree(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srvOI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tab") != "oi" {
			t.Fatalf("tab=%q want oi", r.URL.Query().Get("tab"))
		}
		w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"price":63910,"current_oi":1,"oi_delta":1,"oi_delta_percent":1,"oi_delta_value":1,"price_delta_percent":1,"net_long":1,"net_short":1}],"low":[]}`))
	}))
	defer srvOI.Close()

	e := &StrategyEngine{}
	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srvOI.URL)
	e.trending = tr

	coins, err := e.getOITopCoins(5)
	if err != nil {
		t.Fatalf("getOITopCoins: %v", err)
	}
	if len(coins) != 1 || coins[0].Symbol != "BTCUSDT" || coins[0].Sources[0] != "oi_top" {
		t.Fatalf("unexpected %+v", coins)
	}
}

func TestEngine_getVergexSignalCoins_usesFreeLeaderboard(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"symbol":"PUMP","bias":"bullish","directionScore":4,"rank":1,"market":{"marketType":"core_perp"}}]}`))
	}))
	defer srv.Close()

	e := &StrategyEngine{}
	fc, cerr := vergex.NewFreeClient(srv.URL, "", nil)
	if cerr != nil {
		t.Fatalf("NewFreeClient: %v", cerr)
	}
	e.freeClient = fc

	coins, err := e.getVergexSignalCoins(5, "core_perp", "", "", "all", nil, "")
	if err != nil {
		t.Fatalf("getVergexSignalCoins: %v", err)
	}
	if len(coins) != 1 || coins[0].Symbol != "PUMP" {
		t.Fatalf("unexpected %+v", coins)
	}
}

func TestFormatVergexData_omitsUnavailableForNonVergex(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "ai500"
	e := NewStrategyEngine(&cfg)

	analysis := &vergex.MarketAnalysis{
		Symbol:         "CYS",
		QuerySymbol:    "CYS",
		MarketType:     "core_perp",
		SignalLabError: "vergex endpoint has no data for this market",
		HeatmapError:   "vergex endpoint has no data for this market",
	}
	// ai500 => omitUnavailable = true; neither section has data => block fully omitted
	out := e.formatVergexData(analysis, true)
	if out != "" {
		t.Errorf("generic user prompt should fully omit the signals block for no-data symbols, got: %q", out)
	}

	// When detail IS present, the signals are rendered (with the error-cleared copy).
	analysis.SignalLab = json.RawMessage(`{"data":{"bias":"bullish"}}`)
	out = e.formatVergexData(analysis, true)
	if !strings.Contains(out, "Signal Lab") {
		t.Errorf("generic user prompt should render Signal Lab when present, got: %q", out)
	}
	if strings.Contains(out, "unavailable") {
		t.Errorf("generic user prompt should not contain 'unavailable', got: %q", out)
	}
}
