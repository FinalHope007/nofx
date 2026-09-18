package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUpdatePositionSLTP(t *testing.T) {
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
		Symbol:     "HBARUSDT",
		Side:       "LONG",
		Quantity:   100,
		EntryPrice: 0.0752,
		EntryTime:  time.Now().UTC().UnixMilli(),
		Leverage:   5,
		Status:     "OPEN",
	}
	if err := positions.CreateOpenPosition(pos); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := positions.UpdatePositionSLTP(pos.ID, 0.0714, 0.0827); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := positions.GetOpenPositionBySymbol("t1", "HBARUSDT", "LONG")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StopLoss != 0.0714 || got.TakeProfit != 0.0827 {
		t.Fatalf("SL/TP = %v/%v, want 0.0714/0.0827", got.StopLoss, got.TakeProfit)
	}
	if got.Quantity != pos.Quantity {
		t.Fatalf("Quantity = %v, want %v", got.Quantity, pos.Quantity)
	}
	if got.EntryPrice != pos.EntryPrice {
		t.Fatalf("EntryPrice = %v, want %v", got.EntryPrice, pos.EntryPrice)
	}
	if got.Status != pos.Status {
		t.Fatalf("Status = %v, want %v", got.Status, pos.Status)
	}
	if got.EntryTime != pos.EntryTime {
		t.Fatalf("EntryTime = %v, want %v", got.EntryTime, pos.EntryTime)
	}
}
