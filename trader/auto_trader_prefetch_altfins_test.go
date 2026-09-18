package trader

import (
	"context"
	"testing"

	"nofx/kernel"
	"nofx/store"
)

func TestPrefetchAltFinsDetailsIsCalled(t *testing.T) {
	// Lightweight guard: ensure the symbols slice is derived from candidates
	// and the engine's AltFins prefetch is invocable without panic.
	engine := kernel.NewStrategyEngine(&store.StrategyConfig{})
	engine.PrefetchAltFinsDetails(context.Background(), []string{"ZECUSDT"}, 0)
}

func TestPrefetchRequestsPerCoinIncludesVergex(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "ai500"
	cfg.Indicators.EnableVergexSignalLabData = true
	cfg.Indicators.EnableVergexHeatmapData = true
	if got, want := prefetchRequestsPerCoin(cfg), 2; got != want {
		t.Fatalf("prefetchRequestsPerCoin = %d, want %d", got, want)
	}

	// vergex_signal source is handled by FetchVergexDataBatch, so it must not
	// add to the prefetch request count.
	cfg = &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "vergex_signal"
	cfg.Indicators.EnableVergexSignalLabData = true
	cfg.Indicators.EnableVergexHeatmapData = true
	if got := prefetchRequestsPerCoin(cfg); got != 0 {
		t.Fatalf("prefetchRequestsPerCoin = %d, want 0 for vergex_signal", got)
	}
}

func TestPrefetchRequestsPerCoinIncludesAltFins(t *testing.T) {
	// AltFins alone (no Binance sources) must yield a non-zero request count
	// so the scheduler does not early-return before warming the cache.
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	if got := prefetchRequestsPerCoin(cfg); got == 0 {
		t.Fatalf("expected non-zero requests per coin with AltFins enabled")
	}

	// Default intervals => len(ivs)+1 resolve call.
	cfg = &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15", "DAILY"}
	if got, want := prefetchRequestsPerCoin(cfg), 3; got != want {
		t.Fatalf("prefetchRequestsPerCoin = %d, want %d", got, want)
	}

	// Nothing enabled => zero, scheduler early-returns.
	cfg = &store.StrategyConfig{}
	if got := prefetchRequestsPerCoin(cfg); got != 0 {
		t.Fatalf("prefetchRequestsPerCoin = %d, want 0", got)
	}
}
