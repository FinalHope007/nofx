package kernel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nofx/provider/nofxos"
	"nofx/provider/vergex"
	"nofx/store"
)

func TestGetAI500CoinsMarksStale(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BR","pair":"BRUSDT","score":75}]}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	engine := &StrategyEngine{trending: tr}

	if coins, err := engine.getAI500Coins(10); err != nil || len(coins) != 1 {
		t.Fatalf("first: %v %d", err, len(coins))
	}
	if w := engine.ConsumePoolWarning(); w != "" {
		t.Fatalf("expected no warning on fresh fetch, got %q", w)
	}
	tr.ForceStaleForTest(30 * time.Minute)

	if coins, err := engine.getAI500Coins(10); err != nil || len(coins) != 1 {
		t.Fatalf("stale: %v %d", err, len(coins))
	}
	if w := engine.ConsumePoolWarning(); w == "" {
		t.Fatalf("expected stale warning")
	}
}

func TestGetAI500CoinsColdFailureReturnsEmptyNotError(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html>Just a moment...</html>"))
	}))
	defer srv.Close()

	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	engine := &StrategyEngine{trending: tr}

	coins, err := engine.getAI500Coins(10)
	if err != nil {
		t.Fatalf("expected empty pool, not error, got %v", err)
	}
	if len(coins) != 0 {
		t.Fatalf("expected no candidates, got %d", len(coins))
	}
	if w := engine.ConsumePoolWarning(); w == "" {
		t.Fatalf("expected pool-unavailable warning")
	}
}

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

func TestFreeTrending_GetOIData_duration(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("duration") != "1h" {
			t.Fatalf("duration=%q want 1h", r.URL.Query().Get("duration"))
		}
		w.Write([]byte(`{"top":[{"symbol":"BTC","rank":1,"current_oi":1,"oi_delta":1,"oi_delta_percent":1,"oi_delta_value":1,"price_delta_percent":1,"net_long":1,"net_short":1}],"low":[]}`))
	}))
	defer srv.Close()
	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	env, err := tr.GetOIData("1h", 50)
	if err != nil {
		t.Fatalf("GetOIData: %v", err)
	}
	if len(env.Top) != 1 || len(env.Low) != 0 {
		t.Fatalf("unexpected %+v", env)
	}
	if env.Top[0].OIDeltaPercent != 1 {
		t.Fatalf("oi_delta_percent not parsed: %+v", env.Top[0])
	}
}

func TestFreeTrending_GetNetflowData_duration(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("duration") != "1h" {
			t.Fatalf("duration=%q want 1h", r.URL.Query().Get("duration"))
		}
		w.Write([]byte(`{"top":[{"amount":29309691.4,"price":64119.7,"price_delta_percent":0.2316,"rank":1,"symbol":"BTCUSDT"}],"low":[]}`))
	}))
	defer srv.Close()
	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	env, err := tr.GetNetflowData("1h", 50)
	if err != nil {
		t.Fatalf("GetNetflowData: %v", err)
	}
	if len(env.Top) != 1 || len(env.Low) != 0 {
		t.Fatalf("unexpected %+v", env)
	}
	if env.Top[0].Symbol != "BTCUSDT" || env.Top[0].Amount != 29309691.4 {
		t.Fatalf("net_flow not parsed: %+v", env.Top[0])
	}
}

func TestFreeTrending_GetPriceData_duration(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("duration") != "1h" {
			t.Fatalf("duration=%q want 1h", r.URL.Query().Get("duration"))
		}
		w.Write([]byte(`{"top":[{"pair":"RAREUSDT","symbol":"RARE","price_delta":0.2465,"price":0.0153,"future_flow":2199.0,"spot_flow":139489.6,"oi":113026194,"oi_delta":37235516,"oi_delta_value":915909.5}],"low":[]}`))
	}))
	defer srv.Close()
	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	env, err := tr.GetPriceData("1h", 50)
	if err != nil {
		t.Fatalf("GetPriceData: %v", err)
	}
	if len(env.Top) != 1 || len(env.Low) != 0 {
		t.Fatalf("unexpected %+v", env)
	}
	if env.Top[0].Pair != "RAREUSDT" || env.Top[0].PriceDelta != 0.2465 {
		t.Fatalf("price not parsed: %+v", env.Top[0])
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
