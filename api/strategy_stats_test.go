package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

func TestStrategyStatsMergesLinkedTraders(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	gin.SetMode(gin.TestMode)
	s := &Server{store: st}

	if err := st.Strategy().Create(&store.Strategy{ID: "strat-1", UserID: "user-1", Name: "S", Config: `{"strategy_type":"ai_trading"}`}); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	mk := func(id string) {
		if err := st.Trader().Create(&store.Trader{ID: id, UserID: "user-1", Name: id, AIModelID: "m", ExchangeID: "e", StrategyID: "strat-1"}); err != nil {
			t.Fatalf("create trader %s: %v", id, err)
		}
	}
	mk("t1")
	mk("t2")

	t0 := time.Now().UTC().Add(-6 * 24 * time.Hour)
	t1 := time.Now().UTC()
	// Both traders share the same two timestamps so the merge sums them.
	for _, snap := range []store.EquitySnapshot{
		{TraderID: "t1", Timestamp: t0, TotalEquity: 100},
		{TraderID: "t2", Timestamp: t0, TotalEquity: 200},
		{TraderID: "t1", Timestamp: t1, TotalEquity: 110},
		{TraderID: "t2", Timestamp: t1, TotalEquity: 220},
	} {
		if err := st.Equity().Save(&snap); err != nil {
			t.Fatalf("save equity: %v", err)
		}
	}

	w := callHandler(t, s.handleGetStrategyStats, http.MethodGet, "/api/strategies/strat-1/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body=%s", w.Code, w.Body.String())
	}
	var dto strategyStatsDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if len(dto.NavPoints) != 2 {
		t.Fatalf("nav_points length = %d, want 2", len(dto.NavPoints))
	}
	// Merged first point = 100 + 200 = 300; last = 110 + 220 = 330.
	if dto.NavPoints[0].TotalEquity != 300 || dto.NavPoints[1].TotalEquity != 330 {
		t.Fatalf("nav_points = %+v", dto.NavPoints)
	}
	if dto.SevenDayYield == nil {
		t.Fatalf("expected non-nil seven_day_yield, got %+v", dto)
	}
	if dto.AUM != 330 {
		t.Fatalf("aum = %v, want 330", dto.AUM)
	}
}
