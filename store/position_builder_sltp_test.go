package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProcessTradeAveragingPreservesSLTP(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}

	positions := NewPositionStore(db)
	if err := positions.InitTables(); err != nil {
		t.Fatalf("init position table: %v", err)
	}

	pos := &TraderPosition{
		TraderID:   "t1",
		Symbol:     "BRUSDT",
		Side:       "LONG",
		Quantity:   10,
		EntryPrice: 0.60,
		EntryTime:  time.Now().UTC().UnixMilli(),
		Leverage:   5,
		Status:     "OPEN",
		StopLoss:   0.50,
		TakeProfit: 0.80,
	}
	if err := positions.CreateOpenPosition(pos); err != nil {
		t.Fatalf("create: %v", err)
	}

	pb := NewPositionBuilder(positions)
	if err := pb.ProcessTrade("t1", "ex", "binance", "BRUSDT", "LONG", "open_long", 5, 0.70, 0, 0, time.Now().UTC().UnixMilli(), "o2"); err != nil {
		t.Fatalf("process: %v", err)
	}

	got, err := positions.GetOpenPositionBySymbol("t1", "BRUSDT", "LONG")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected open position")
	}
	if got.StopLoss != 0.50 || got.TakeProfit != 0.80 {
		t.Fatalf("SL/TP mutated to %v/%v, want 0.50/0.80", got.StopLoss, got.TakeProfit)
	}
	if got.Quantity <= 10 {
		t.Fatalf("quantity not averaged: %v", got.Quantity)
	}
}
