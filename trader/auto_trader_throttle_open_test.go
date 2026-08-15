package trader

import (
	"testing"
	"time"

	"nofx/store"
)

func TestCountDistinctOpenEventsGroupsMultiFillOpens(t *testing.T) {
	now := time.Now().UTC().UnixMilli()
	// A single BTWUSDT long opened as 5 fills (5 order rows, same symbol+side).
	orders := []*store.TraderOrder{}
	for i := 0; i < 5; i++ {
		orders = append(orders, &store.TraderOrder{
			Symbol: "BTWUSDT", PositionSide: "LONG", Side: "BUY",
			OrderAction: "open_long", Status: "FILLED", CreatedAt: now,
		})
	}
	// A genuinely separate ETHUSDT open.
	orders = append(orders, &store.TraderOrder{
		Symbol: "ETHUSDT", PositionSide: "LONG", Side: "BUY",
		OrderAction: "open_long", Status: "FILLED", CreatedAt: now,
	})

	count := countDistinctOpenEvents(orders, time.Unix(0, 0).UnixMilli())
	if count != 2 {
		t.Fatalf("expected 2 distinct opens, got %d (5 fills must count as 1 open)", count)
	}
}

func TestCountDistinctOpenEventsSkipsCanceled(t *testing.T) {
	now := time.Now().UTC().UnixMilli()
	orders := []*store.TraderOrder{
		{Symbol: "BTCUSDT", PositionSide: "LONG", OrderAction: "open_long", Status: "CANCELED", CreatedAt: now},
		{Symbol: "ETHUSDT", PositionSide: "LONG", OrderAction: "open_long", Status: "FILLED", CreatedAt: now},
	}
	if got := countDistinctOpenEvents(orders, time.Unix(0, 0).UnixMilli()); got != 1 {
		t.Fatalf("expected 1 (canceled skipped), got %d", got)
	}
}
