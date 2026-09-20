package trader

import (
	"testing"
	"time"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/types"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingTrader struct {
	types.Trader
	cancelledSL []string
	cancelledTP []string
	setSL       []float64
	setTP       []float64
}

func (r *recordingTrader) CancelStopLossOrders(symbol string) error {
	r.cancelledSL = append(r.cancelledSL, symbol)
	return nil
}

func (r *recordingTrader) CancelTakeProfitOrders(symbol string) error {
	r.cancelledTP = append(r.cancelledTP, symbol)
	return nil
}

func (r *recordingTrader) SetStopLoss(symbol, positionSide string, quantity, stopPrice float64) error {
	r.setSL = append(r.setSL, stopPrice)
	return nil
}

func (r *recordingTrader) SetTakeProfit(symbol, positionSide string, quantity, takeProfitPrice float64) error {
	r.setTP = append(r.setTP, takeProfitPrice)
	return nil
}

func newTrailingTestStore(t *testing.T) (*store.Store, *store.TraderPosition) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := st.Position().InitTables(); err != nil {
		t.Fatalf("init tables: %v", err)
	}
	pos := &store.TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 10, EntryPrice: 0.80, Leverage: 5, Status: "OPEN",
		EntryTime: time.Now().UTC().UnixMilli(), StopLoss: 0.75, TakeProfit: 0.95,
	}
	if err := st.Position().CreateOpenPosition(pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	return st, pos
}

func TestApplyTrailingSLTPCancelsBeforeReplacing(t *testing.T) {
	st, pos := newTrailingTestStore(t)
	rec := &recordingTrader{}
	at := &AutoTrader{id: "t1", exchange: "binance", store: st, trader: rec}
	ar := &store.DecisionAction{}

	dec := &kernel.Decision{Symbol: "BRUSDT", Action: "hold", StopLoss: 0.85, TakeProfit: 0.99}
	if err := at.applyTrailingSLTP(dec, ar); err != nil {
		t.Fatalf("applyTrailingSLTP: %v", err)
	}

	if len(rec.cancelledSL) != 1 || rec.cancelledSL[0] != "BRUSDT" {
		t.Fatalf("expected one SL cancel for BRUSDT, got %v", rec.cancelledSL)
	}
	if len(rec.cancelledTP) != 1 || rec.cancelledTP[0] != "BRUSDT" {
		t.Fatalf("expected one TP cancel for BRUSDT, got %v", rec.cancelledTP)
	}
	if len(rec.setSL) != 1 || rec.setSL[0] != 0.85 {
		t.Fatalf("expected SL set to 0.85, got %v", rec.setSL)
	}
	if len(rec.setTP) != 1 || rec.setTP[0] != 0.99 {
		t.Fatalf("expected TP set to 0.99, got %v", rec.setTP)
	}

	got, err := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "long")
	if err != nil || got == nil {
		t.Fatalf("reload position: %v", err)
	}
	if got.StopLoss != 0.85 || got.TakeProfit != 0.99 {
		t.Fatalf("persisted SL/TP = %v/%v, want 0.85/0.99", got.StopLoss, got.TakeProfit)
	}
	_ = pos
}

func TestApplyTrailingSLTPRejectsWideningStop(t *testing.T) {
	st, _ := newTrailingTestStore(t)
	rec := &recordingTrader{}
	at := &AutoTrader{id: "t1", exchange: "binance", store: st, trader: rec}
	ar := &store.DecisionAction{}

	dec := &kernel.Decision{Symbol: "BRUSDT", Action: "hold", StopLoss: 0.70, TakeProfit: 0.95}
	if err := at.applyTrailingSLTP(dec, ar); err != nil {
		t.Fatalf("applyTrailingSLTP: %v", err)
	}
	if len(rec.setSL) != 0 {
		t.Fatalf("widening stop must be ignored, got SetStopLoss calls %v", rec.setSL)
	}
	if len(rec.cancelledSL) != 0 {
		t.Fatalf("widening stop must not cancel the existing stop, got %v", rec.cancelledSL)
	}
	got, _ := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "long")
	if got.StopLoss != 0.75 {
		t.Fatalf("stop changed to %v, want unchanged 0.75", got.StopLoss)
	}
}

func TestApplyTrailingSLTPNoopWhenUnchanged(t *testing.T) {
	st, _ := newTrailingTestStore(t)
	rec := &recordingTrader{}
	at := &AutoTrader{id: "t1", exchange: "binance", store: st, trader: rec}
	ar := &store.DecisionAction{}

	dec := &kernel.Decision{Symbol: "BRUSDT", Action: "hold", StopLoss: 0.75, TakeProfit: 0.95}
	if err := at.applyTrailingSLTP(dec, ar); err != nil {
		t.Fatalf("applyTrailingSLTP: %v", err)
	}
	if len(rec.cancelledSL) != 0 || len(rec.cancelledTP) != 0 || len(rec.setSL) != 0 || len(rec.setTP) != 0 {
		t.Fatalf("no-op hold must not touch orders: %+v", rec)
	}
}
