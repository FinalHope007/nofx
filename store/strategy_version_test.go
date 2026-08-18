package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newStrategyVersionTestStore(t *testing.T) *StrategyVersionStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	s := NewStrategyVersionStore(db)
	if err := s.initTables(); err != nil {
		t.Fatalf("init strategy_versions table: %v", err)
	}
	return s
}

func TestStrategyVersionStoreLifecycle(t *testing.T) {
	s := newStrategyVersionTestStore(t)

	v1, err := s.CreateSnapshot("strat-1", "user-1", `{"a":1}`, "v1")
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if v1.Version != 1 {
		t.Fatalf("first version = %d, want 1", v1.Version)
	}
	if err := s.SetCurrent("strat-1", "user-1", v1.Version); err != nil {
		t.Fatalf("set current v1: %v", err)
	}

	v2, err := s.CreateSnapshot("strat-1", "user-1", `{"a":2}`, "Snapshot before edit")
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("second version = %d, want 2", v2.Version)
	}

	list, err := s.List("strat-1", "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2", len(list))
	}

	got, err := s.Get("strat-1", "user-1", 1)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if got.Config != `{"a":1}` {
		t.Fatalf("v1 config = %q", got.Config)
	}

	if err := s.DeleteForStrategy("strat-1", "user-1"); err != nil {
		t.Fatalf("delete for strategy: %v", err)
	}
	if remaining, err := s.List("strat-1", "user-1"); err != nil || len(remaining) != 0 {
		t.Fatalf("expected no versions after delete, got %d (err=%v)", len(remaining), err)
	}
}
