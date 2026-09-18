package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestClassifyClose(t *testing.T) {
	const tol = 0.001
	baseExit := int64(1_700_000_000_000)

	llmClose := func(symbol, side string, ts int64) map[string][]int64 {
		return map[string][]int64{symbol + "|" + side: {ts}}
	}

	tests := []struct {
		name   string
		pos    TraderPosition
		closes map[string][]int64
		want   string
	}{
		{
			name:   "successful llm close within window",
			pos:    TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.63, ExitTime: baseExit, StopLoss: 0.50, TakeProfit: 0.80, RealizedPnL: 1.0},
			closes: llmClose("BRUSDT", "long", baseExit+60_000),
			want:   "llm",
		},
		{
			name:   "blocked close falls through to sl",
			pos:    TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.4999, ExitTime: baseExit, StopLoss: 0.50, TakeProfit: 0.80, RealizedPnL: -1.0},
			closes: map[string][]int64{},
			want:   "sl",
		},
		{
			name: "long exit at stop loss",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.50, ExitTime: baseExit, StopLoss: 0.50, TakeProfit: 0.80, RealizedPnL: -1.0},
			want: "sl",
		},
		{
			name: "long exit at take profit",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.80, ExitTime: baseExit, StopLoss: 0.50, TakeProfit: 0.80, RealizedPnL: 1.0},
			want: "tp",
		},
		{
			name: "no sltp positive pnl labeled exchange",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.63, ExitTime: baseExit, RealizedPnL: 1.0},
			want: "exchange",
		},
		{
			name: "no sltp negative pnl labeled exchange",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.59, ExitTime: baseExit, RealizedPnL: -1.0},
			want: "exchange",
		},
		{
			name: "no sltp zero pnl",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.60, ExitTime: baseExit, RealizedPnL: 0},
			want: "exchange",
		},
		{
			name: "manual close marker wins over price rules",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.59, ExitTime: baseExit, StopLoss: 0.50, TakeProfit: 0.80, RealizedPnL: -1.0, CloseReason: "manual"},
			want: "manual",
		},
		{
			name: "long trailing stop above entry locks profit",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", EntryPrice: 0.83129, ExitPrice: 0.86763, ExitTime: baseExit, StopLoss: 0.873, TakeProfit: 0.9178, RealizedPnL: 0.5},
			want: "trailing_sl",
		},
		{
			name: "long trailing stop below entry is a plain sl",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "LONG", EntryPrice: 0.80, ExitPrice: 0.7499, ExitTime: baseExit, StopLoss: 0.75, TakeProfit: 0.95, RealizedPnL: -1.0},
			want: "sl",
		},
		{
			name: "short trailing stop below entry locks profit",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "SHORT", EntryPrice: 0.80, ExitPrice: 0.7001, ExitTime: baseExit, StopLoss: 0.70, TakeProfit: 0.50, RealizedPnL: 0.5},
			want: "trailing_sl",
		},
		{
			name: "short trailing stop above entry is a plain sl",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "SHORT", EntryPrice: 0.70, ExitPrice: 0.8501, ExitTime: baseExit, StopLoss: 0.85, TakeProfit: 0.50, RealizedPnL: -1.0},
			want: "sl",
		},
		{
			name: "short exit at stop loss",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "SHORT", ExitPrice: 0.80, ExitTime: baseExit, StopLoss: 0.80, TakeProfit: 0.50, RealizedPnL: -1.0},
			want: "sl",
		},
		{
			name: "short exit at take profit",
			pos:  TraderPosition{Symbol: "BRUSDT", Side: "SHORT", ExitPrice: 0.50, ExitTime: baseExit, StopLoss: 0.80, TakeProfit: 0.50, RealizedPnL: 1.0},
			want: "tp",
		},
		{
			name:   "llm close outside window falls through",
			pos:    TraderPosition{Symbol: "BRUSDT", Side: "LONG", ExitPrice: 0.60, ExitTime: baseExit, RealizedPnL: 0},
			closes: llmClose("BRUSDT", "long", baseExit+16*60*1000),
			want:   "exchange",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyClose(tt.pos, tt.closes, tol)
			if got != tt.want {
				t.Fatalf("classifyClose = %q, want %q", got, tt.want)
			}
		})
	}
}

func newClassifyTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	st, err := NewFromGorm(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := st.Position().InitTables(); err != nil {
		t.Fatalf("init position table: %v", err)
	}
	if err := st.Decision().initTables(); err != nil {
		t.Fatalf("init decision table: %v", err)
	}
	return st
}

func TestGetRecentTradesWithReasonClassifiesLLMClose(t *testing.T) {
	st := newClassifyTestStore(t)
	now := time.Now().UTC()
	closeRec := &DecisionRecord{
		TraderID: "t1", CycleNumber: 2, Timestamp: now,
		Success:   true,
		Decisions: []DecisionAction{{Action: "close_long", Symbol: "BRUSDT", Success: true}},
	}
	if err := st.Decision().LogDecision(closeRec); err != nil {
		t.Fatalf("log: %v", err)
	}
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 0, EntryPrice: 0.60, ExitPrice: 0.63, Leverage: 5,
		EntryTime: now.Add(-30 * time.Minute).UnixMilli(),
		ExitTime:  now.UnixMilli(), RealizedPnL: 1.0, Status: "CLOSED",
		StopLoss: 0.50, TakeProfit: 0.80,
	}
	seedClosedPosition(t, st, pos)
	trades, err := st.GetRecentTradesWithReason("t1", 15)
	if err != nil || len(trades) != 1 {
		t.Fatalf("trades=%v err=%v", trades, err)
	}
	if trades[0].CloseReason != "llm" {
		t.Fatalf("reason=%q want llm", trades[0].CloseReason)
	}
}

func seedClosedPosition(t *testing.T, st *Store, pos *TraderPosition) {
	t.Helper()
	if err := st.Position().Create(pos); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.gdb.Model(&TraderPosition{}).Where("id = ?", pos.ID).
		Update("status", "CLOSED").Error; err != nil {
		t.Fatalf("mark closed: %v", err)
	}
}

func TestGetRecentTradesWithReasonBlockedCloseFallsToSL(t *testing.T) {
	st := newClassifyTestStore(t)
	now := time.Now().UTC()
	blockedRec := &DecisionRecord{
		TraderID: "t1", CycleNumber: 143, Timestamp: now,
		Success:   true,
		Decisions: []DecisionAction{{Action: "close_long", Symbol: "BRUSDT", Success: false}},
	}
	if err := st.Decision().LogDecision(blockedRec); err != nil {
		t.Fatalf("log: %v", err)
	}
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 0, EntryPrice: 0.60, ExitPrice: 0.4999, Leverage: 5,
		EntryTime: now.Add(-30 * time.Minute).UnixMilli(),
		ExitTime:  now.UnixMilli(), RealizedPnL: -1.0, Status: "CLOSED",
		StopLoss: 0.50, TakeProfit: 0.80,
	}
	seedClosedPosition(t, st, pos)
	trades, err := st.GetRecentTradesWithReason("t1", 15)
	if err != nil || len(trades) != 1 {
		t.Fatalf("trades=%v err=%v", trades, err)
	}
	if trades[0].CloseReason != "sl" {
		t.Fatalf("reason=%q want sl", trades[0].CloseReason)
	}
}

func TestGetRecentTradesWithReasonFailedCycleFallsToSL(t *testing.T) {
	st := newClassifyTestStore(t)
	now := time.Now().UTC()
	failedRec := &DecisionRecord{
		TraderID: "t1", CycleNumber: 143, Timestamp: now,
		Success:   false,
		Decisions: []DecisionAction{{Action: "close_long", Symbol: "BRUSDT", Success: true}},
	}
	if err := st.Decision().LogDecision(failedRec); err != nil {
		t.Fatalf("log: %v", err)
	}
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 0, EntryPrice: 0.60, ExitPrice: 0.4999, Leverage: 5,
		EntryTime: now.Add(-30 * time.Minute).UnixMilli(),
		ExitTime:  now.UnixMilli(), RealizedPnL: -1.0, Status: "CLOSED",
		StopLoss: 0.50, TakeProfit: 0.80,
	}
	seedClosedPosition(t, st, pos)
	trades, err := st.GetRecentTradesWithReason("t1", 15)
	if err != nil || len(trades) != 1 {
		t.Fatalf("trades=%v err=%v", trades, err)
	}
	if trades[0].CloseReason != "sl" {
		t.Fatalf("reason=%q want sl", trades[0].CloseReason)
	}
}

func TestGetRecentTradesWithReasonClassifiesVariantSymbolLLMClose(t *testing.T) {
	st := newClassifyTestStore(t)
	now := time.Now().UTC()
	closeRec := &DecisionRecord{
		TraderID: "t1", CycleNumber: 2, Timestamp: now,
		Success:   true,
		Decisions: []DecisionAction{{Action: "close_long", Symbol: "QNT-USDT-SWAP", Success: true}},
	}
	if err := st.Decision().LogDecision(closeRec); err != nil {
		t.Fatalf("log: %v", err)
	}
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "QNTUSDT", Side: "LONG",
		Quantity: 0, EntryPrice: 10, ExitPrice: 10.5, Leverage: 5,
		EntryTime: now.Add(-30 * time.Minute).UnixMilli(),
		ExitTime:  now.UnixMilli(), RealizedPnL: 0, Status: "CLOSED",
	}
	seedClosedPosition(t, st, pos)
	trades, err := st.GetRecentTradesWithReason("t1", 15)
	if err != nil || len(trades) != 1 {
		t.Fatalf("trades=%v err=%v", trades, err)
	}
	if trades[0].CloseReason != "llm" {
		t.Fatalf("reason=%q want llm", trades[0].CloseReason)
	}
}

func TestGetRecordsInRange(t *testing.T) {
	st := newClassifyTestStore(t)
	now := time.Now().UTC()
	rec := &DecisionRecord{
		TraderID: "t1", CycleNumber: 1, Timestamp: now,
		Success: true,
	}
	if err := st.Decision().LogDecision(rec); err != nil {
		t.Fatalf("log: %v", err)
	}
	got, err := st.Decision().GetRecordsInRange("t1", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetRecordsInRange: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("records=%d want 1", len(got))
	}
	out, err := st.Decision().GetRecordsInRange("t1", now.Add(time.Hour), now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("GetRecordsInRange out of range: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("out-of-range records=%d want 0", len(out))
	}
}

func TestManualCloseMarkerLabelsSyncedClose(t *testing.T) {
	st := newClassifyTestStore(t)
	now := time.Now().UTC()
	pos := &TraderPosition{
		TraderID: "t1", Symbol: "BRUSDT", Side: "LONG",
		Quantity: 10, EntryPrice: 0.67, Leverage: 5,
		EntryTime: now.Add(-10 * time.Minute).UnixMilli(), Status: "OPEN",
		StopLoss: 0.60, TakeProfit: 0.80,
	}
	if err := st.Position().CreateOpenPosition(pos); err != nil {
		t.Fatalf("create: %v", err)
	}

	st.Position().MarkManualClose("t1", "BRUSDT", "long")
	if !st.Position().consumeManualClose("t1", "BRUSDT", "long") {
		t.Fatal("marker not consumable")
	}
	if st.Position().consumeManualClose("t1", "BRUSDT", "long") {
		t.Fatal("marker consumed twice")
	}

	st.Position().MarkManualClose("t1", "BRUSDT", "long")
	pb := NewPositionBuilder(st.Position())
	if err := pb.ProcessTrade("t1", "ex", "binance", "BRUSDT", "LONG", "close_long", 10, 0.6693, 0, 0, time.Now().UTC().UnixMilli(), "o1"); err != nil {
		t.Fatalf("process: %v", err)
	}
	closedList, err := st.Position().GetClosedPositions("t1", 5)
	if err != nil || len(closedList) != 1 {
		t.Fatalf("closed=%v err=%v", closedList, err)
	}
	if closedList[0].CloseReason != "manual" {
		t.Fatalf("close_reason=%q want manual", closedList[0].CloseReason)
	}

	trades, err := st.GetRecentTradesWithReason("t1", 15)
	if err != nil || len(trades) != 1 {
		t.Fatalf("trades=%v err=%v", trades, err)
	}
	if trades[0].CloseReason != "manual" {
		t.Fatalf("recent reason=%q want manual", trades[0].CloseReason)
	}
}

func TestManualCloseMarkerExpires(t *testing.T) {
	st := newClassifyTestStore(t)
	st.Position().MarkManualClose("t1", "BRUSDT", "long")
	st.Position().markMu.Lock()
	st.Position().manual[manualCloseKey("t1", "BRUSDT", "long")] = time.Now().UTC().Add(-manualCloseTTL - time.Minute).UnixMilli()
	st.Position().markMu.Unlock()
	if st.Position().consumeManualClose("t1", "BRUSDT", "long") {
		t.Fatal("expired marker should not be consumed as manual")
	}
}
