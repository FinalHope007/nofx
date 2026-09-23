package trader

import (
	"nofx/kernel"
	"testing"
)

func TestShouldSkipForNoCandidates(t *testing.T) {
	cases := []struct {
		candidates, positions int
		want                  bool
	}{
		{0, 0, true},
		{0, 1, false},
		{3, 0, false},
		{3, 2, false},
	}
	for _, c := range cases {
		if got := shouldSkipForNoCandidates(c.candidates, c.positions); got != c.want {
			t.Fatalf("candidates=%d positions=%d: got %v want %v", c.candidates, c.positions, got, c.want)
		}
	}
}

func TestFilterDecisionsPositionsOnlyBlocksOpens(t *testing.T) {
	heldCtx := func(candidates []kernel.CandidateCoin) *kernel.Context {
		return &kernel.Context{
			CandidateCoins: candidates,
			Positions: []kernel.PositionInfo{
				{Symbol: "xyz:INTC", Side: "long"},
			},
		}
	}

	cases := []struct {
		name       string
		candidates []kernel.CandidateCoin
		decisions  []kernel.Decision
		wantKept   int
	}{
		{
			name:       "empty candidates drops open_long on held symbol",
			candidates: nil,
			decisions:  []kernel.Decision{{Symbol: "xyz:INTC", Action: "open_long"}},
			wantKept:   0,
		},
		{
			name:       "empty candidates keeps close on held symbol",
			candidates: nil,
			decisions:  []kernel.Decision{{Symbol: "xyz:INTC", Action: "close_long"}},
			wantKept:   1,
		},
		{
			name:       "non-empty candidates keeps open on held symbol",
			candidates: []kernel.CandidateCoin{{Symbol: "xyz:BTC"}},
			decisions:  []kernel.Decision{{Symbol: "xyz:INTC", Action: "open_long"}},
			wantKept:   1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at := &AutoTrader{}
			got := at.filterDecisionsToStrategyUniverse(c.decisions, heldCtx(c.candidates))
			if len(got) != c.wantKept {
				t.Fatalf("kept %d decisions, want %d: %+v", len(got), c.wantKept, got)
			}
		})
	}
}
