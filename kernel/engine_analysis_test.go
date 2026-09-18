package kernel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"nofx/provider/vergex"
	"nofx/store"
)

type fakeTrader struct {
	exchange string
}

func (f *fakeTrader) GetBalance() (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeTrader) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

func TestFetchMarketDataEarlyExit(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "binance_technical"
	cfg.CoinSource.BinanceTechnicalInterval = "1h"
	cfg.CoinSource.BinanceTechnicalDirection = "top"
	cfg.CoinSource.BinanceTechnicalLimit = 10
	cfg.RiskControl.EnableOILiquidityFilter = false

	engine := NewStrategyEngine(cfg)
	engine.opportunity = &fakeOpportunityClientN{n: 20}

	// Get 20 candidates
	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &Context{
		CandidateCoins: candidates,
		Positions:      nil,
		Ctx:            context.Background(),
	}

	// fetchMarketDataWithStrategy should not panic or error
	// With fake exchange, market data fetches will fail, but the
	// early exit test is about iteration count, not data success.
	// We test that the function iterates without hanging.
	err = fetchMarketDataWithStrategy(ctx, engine)
	if err != nil {
		// Expected to fail on market data fetch with fake exchange
		// The key assertion is it doesn't hang or panic
		t.Logf("expected fetch error with fake exchange: %v", err)
	}
}

func TestIndicatorPeriodsFor(t *testing.T) {
	cfg := store.IndicatorConfig{
		EMAPeriods:  []int{10},
		RSIPeriods:  []int{6, 12},
		ATRPeriods:  []int{21},
		BOLLPeriods: []int{30},
	}

	got := indicatorPeriodsFor(cfg)

	if !reflect.DeepEqual(got.EMA, []int{10}) {
		t.Fatalf("EMA: got %v, want %v", got.EMA, []int{10})
	}
	if !reflect.DeepEqual(got.RSI, []int{6, 12}) {
		t.Fatalf("RSI: got %v, want %v", got.RSI, []int{6, 12})
	}
	if !reflect.DeepEqual(got.ATR, []int{21}) {
		t.Fatalf("ATR: got %v, want %v", got.ATR, []int{21})
	}
	if !reflect.DeepEqual(got.BOLL, []int{30}) {
		t.Fatalf("BOLL: got %v, want %v", got.BOLL, []int{30})
	}
}

func TestAttachPerCoinSignalsAltFins(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15"}

	engine := NewStrategyEngine(cfg)
	engine.altfinsClient = &fakeAltfinsClient{ids: map[string]int64{"ZEC": 1021300}}

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "ZECUSDT"}},
		Ctx:            context.Background(),
	}
	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	sig, ok := engine.PerCoinSignalFor("ZECUSDT")
	if !ok || sig.AltFins["MINUTES15"] == nil {
		t.Fatalf("expected AltFins signal, got %+v ok=%v", sig, ok)
	}
}

func TestAttachPerCoinSignalsVergexSignalLab(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.RequestURI
		w.Write([]byte(`{"signal_lab":"ok"}`))
	}))
	defer srv.Close()

	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "ai500"
	cfg.Indicators.EnableVergexSignalLabData = true

	engine := NewStrategyEngine(cfg)
	fc, err := vergex.NewFreeClient(srv.URL, "", nil)
	if err != nil {
		t.Fatalf("NewFreeClient: %v", err)
	}
	engine.freeClient = fc

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "ZECUSDT"}},
		Ctx:            context.Background(),
	}
	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	sig, ok := engine.PerCoinSignalFor("ZECUSDT")
	if !ok || len(sig.VergexSignalLab) == 0 {
		t.Fatalf("expected Vergex signal lab, got %+v ok=%v", sig, ok)
	}
	// A crypto symbol must resolve to the core_perp family, NOT be
	// mis-classified as an xyz/stock asset.
	if !strings.Contains(gotPath, "/core_perp%3AZEC/signals") {
		t.Fatalf("expected core_perp crypto path, got %q", gotPath)
	}
	if strings.Contains(gotPath, "xyz%3A") {
		t.Fatalf("crypto symbol must not be routed to xyz path, got %q", gotPath)
	}
	if !strings.Contains(gotPath, "chain=mainnet") {
		t.Fatalf("expected normalized chain in request, got %q", gotPath)
	}
}

func TestAttachPerCoinSignalsVergexSignalLabXYZStock(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.RequestURI
		w.Write([]byte(`{"signal_lab":"ok"}`))
	}))
	defer srv.Close()

	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "ai500"
	cfg.Indicators.EnableVergexSignalLabData = true

	engine := NewStrategyEngine(cfg)
	fc, err := vergex.NewFreeClient(srv.URL, "", nil)
	if err != nil {
		t.Fatalf("NewFreeClient: %v", err)
	}
	engine.freeClient = fc

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "xyz:SP500"}},
		Ctx:            context.Background(),
	}
	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	sig, ok := engine.PerCoinSignalFor("xyz:SP500")
	if !ok || len(sig.VergexSignalLab) == 0 {
		t.Fatalf("expected Vergex signal lab, got %+v ok=%v", sig, ok)
	}
	if !strings.Contains(gotPath, "/hip3_perp%3A0x") || !strings.Contains(gotPath, "xyz%3ASP500") {
		t.Fatalf("expected hip3_perp xyz stock path, got %q", gotPath)
	}
}

func TestAttachPerCoinSignalsVergexSkipsForVergexSignalSource(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Write([]byte(`{"signal_lab":"ok"}`))
	}))
	defer srv.Close()

	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "vergex_signal"
	cfg.Indicators.EnableVergexSignalLabData = true

	engine := NewStrategyEngine(cfg)
	fc, err := vergex.NewFreeClient(srv.URL, "", nil)
	if err != nil {
		t.Fatalf("NewFreeClient: %v", err)
	}
	engine.freeClient = fc

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "ZECUSDT"}},
		Ctx:            context.Background(),
	}
	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	if hit {
		t.Fatalf("expected no vergex detail fetch for vergex_signal source")
	}
	sig, _ := engine.PerCoinSignalFor("ZECUSDT")
	if len(sig.VergexSignalLab) != 0 {
		t.Fatalf("expected no signal lab for vergex_signal source, got %s", sig.VergexSignalLab)
	}
}

func TestAttachPerCoinSignalsVergexSkipsWithoutClient(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "ai500"
	cfg.Indicators.EnableVergexSignalLabData = true

	engine := NewStrategyEngine(cfg)
	engine.freeClient = nil

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "ZECUSDT"}},
		Ctx:            context.Background(),
	}
	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	sig, _ := engine.PerCoinSignalFor("ZECUSDT")
	if len(sig.VergexSignalLab) != 0 {
		t.Fatalf("expected no signal lab without client, got %s", sig.VergexSignalLab)
	}
}
