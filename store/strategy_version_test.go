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

func newStrategyVersionLifecycleStore(t *testing.T) (*StrategyStore, *StrategyVersionStore) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	vs := NewStrategyVersionStore(db)
	if err := vs.initTables(); err != nil {
		t.Fatalf("init strategy_versions table: %v", err)
	}
	ss := NewStrategyStore(db)
	if err := ss.initTables(); err != nil {
		t.Fatalf("init strategies table: %v", err)
	}
	return ss, vs
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

func TestStrategyLifecycleSnapshots(t *testing.T) {
	ss, vs := newStrategyVersionLifecycleStore(t)

	// Create -> v1 snapshot exists.
	strat := &Strategy{
		ID:       "strat-life",
		UserID:   "user-life",
		Name:     "Lifecycle",
		Config:   `{"a":1}`,
		IsActive: false,
	}
	if err := ss.Create(strat); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	list, err := vs.List("strat-life", "user-life")
	if err != nil {
		t.Fatalf("list versions after create: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("after create, version count = %d, want 1", len(list))
	}
	if list[0].Version != 1 || list[0].Config != `{"a":1}` || list[0].Note != "v1" {
		t.Fatalf("v1 snapshot mismatch: %+v", list[0])
	}
	if !list[0].IsCurrent {
		t.Fatalf("v1 snapshot should be current")
	}

	// Update -> pre-edit snapshot (v2) exists, version bumped.
	updated := &Strategy{
		ID:       "strat-life",
		UserID:   "user-life",
		Name:     "Lifecycle edited",
		Config:   `{"a":2}`,
		IsActive: false,
	}
	if err := ss.Update(updated); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	list, err = vs.List("strat-life", "user-life")
	if err != nil {
		t.Fatalf("list versions after update: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("after update, version count = %d, want 2", len(list))
	}
	if list[1].Version != 2 || list[1].Config != `{"a":1}` || list[1].Note != "Snapshot before edit" {
		t.Fatalf("pre-edit snapshot mismatch: %+v", list[1])
	}

	// Delete -> versions removed.
	if err := ss.Delete("user-life", "strat-life"); err != nil {
		t.Fatalf("delete strategy: %v", err)
	}
	remaining, err := vs.List("strat-life", "user-life")
	if err != nil {
		t.Fatalf("list versions after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("after delete, version count = %d, want 0", len(remaining))
	}
}
