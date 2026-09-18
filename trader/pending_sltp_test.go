package trader

import (
	"testing"

	"nofx/market"
	"nofx/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPendingSLTPTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := st.Position().InitTables(); err != nil {
		t.Fatalf("init position tables: %v", err)
	}
	return st
}

func TestReconcilePendingSLTP(t *testing.T) {
	st := newPendingSLTPTestStore(t)

	pos := &store.TraderPosition{
		TraderID:   "t1",
		Symbol:     "BRUSDT",
		Side:       "LONG",
		Quantity:   1,
		EntryPrice: 0.6,
		Status:     "OPEN",
	}
	if err := st.Position().Create(pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	at := &AutoTrader{
		id:          "t1",
		store:       st,
		pendingSLTP: map[string]pendingSLTP{"BRUSDT_long": {StopLoss: 0.5, TakeProfit: 0.8}},
	}

	dbPos, err := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "LONG")
	if err != nil || dbPos == nil {
		t.Fatalf("get open position: pos=%v err=%v", dbPos, err)
	}
	at.reconcilePendingSLTP(dbPos, "BRUSDT", "long")

	got, err := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "LONG")
	if err != nil || got == nil {
		t.Fatalf("re-fetch position: pos=%v err=%v", got, err)
	}
	if got.StopLoss != 0.5 || got.TakeProfit != 0.8 {
		t.Fatalf("SL/TP = %v/%v, want 0.5/0.8", got.StopLoss, got.TakeProfit)
	}
	if _, ok := at.pendingSLTP["BRUSDT_long"]; ok {
		t.Fatalf("pending entry was not removed")
	}
}

// TestPendingSLTPKeyMatchesPositionLoopKey verifies that the key used by the
// position loop (normalized DB symbol) agrees with the key written by the
// open path via recordPendingSLTP, so pending SL/TP is not discarded before
// OrderSync can apply it.
func TestPendingSLTPKeyMatchesPositionLoopKey(t *testing.T) {
	at := &AutoTrader{
		pendingSLTP: make(map[string]pendingSLTP),
	}

	rawSymbol := "BTC_USDT"
	side := "long"

	at.recordPendingSLTP(market.Normalize(rawSymbol), side, 0.5, 0.8)

	dbSymbol := market.Normalize(rawSymbol)
	posKey := dbSymbol + "_" + side

	at.pendingSLTPMutex.Lock()
	_, ok := at.pendingSLTP[posKey]
	at.pendingSLTPMutex.Unlock()
	if !ok {
		t.Fatalf("posKey %q did not match pending key %q (cleanup would delete it)", posKey, market.Normalize(rawSymbol)+"_"+side)
	}

	if posKey == rawSymbol+"_"+side {
		t.Fatalf("expected normalized key to differ from raw key")
	}
}

func TestReconcilePendingSLTPDoesNotOverwrite(t *testing.T) {
	st := newPendingSLTPTestStore(t)

	pos := &store.TraderPosition{
		TraderID:   "t1",
		Symbol:     "BRUSDT",
		Side:       "LONG",
		Quantity:   1,
		EntryPrice: 0.6,
		Status:     "OPEN",
		StopLoss:   0.4,
		TakeProfit: 0.9,
	}
	if err := st.Position().Create(pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	at := &AutoTrader{
		id:          "t1",
		store:       st,
		pendingSLTP: map[string]pendingSLTP{"BRUSDT_long": {StopLoss: 0.5, TakeProfit: 0.8}},
	}

	dbPos, err := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "LONG")
	if err != nil || dbPos == nil {
		t.Fatalf("get open position: pos=%v err=%v", dbPos, err)
	}
	at.reconcilePendingSLTP(dbPos, "BRUSDT", "long")

	got, err := st.Position().GetOpenPositionBySymbol("t1", "BRUSDT", "LONG")
	if err != nil || got == nil {
		t.Fatalf("re-fetch position: pos=%v err=%v", got, err)
	}
	if got.StopLoss != 0.4 || got.TakeProfit != 0.9 {
		t.Fatalf("SL/TP = %v/%v, want unchanged 0.4/0.9", got.StopLoss, got.TakeProfit)
	}
	if _, ok := at.pendingSLTP["BRUSDT_long"]; ok {
		t.Fatalf("pending entry was not removed")
	}
}
