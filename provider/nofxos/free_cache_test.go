package nofxos

import (
	"errors"
	"testing"
	"time"
)

func TestPoolCacheFreshHitSkipsFetch(t *testing.T) {
	var c poolCache[int]
	fetches := 0
	fetch := func() (int, error) { fetches++; return 1, nil }

	if r, err := c.get(fetch); err != nil || r.Value != 1 || r.Stale {
		t.Fatalf("first get: %+v err=%v", r, err)
	}
	if r, err := c.get(fetch); err != nil || r.Stale {
		t.Fatalf("second get: %+v err=%v", r, err)
	}
	if fetches != 1 {
		t.Fatalf("expected 1 fetch within fresh window, got %d", fetches)
	}
}

func TestPoolCacheStaleServeOnErrorWithinMaxStale(t *testing.T) {
	var c poolCache[int]
	if _, err := c.get(func() (int, error) { return 7, nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c.fetchedAt = time.Now().Add(-30 * time.Minute)

	r, err := c.get(func() (int, error) { return 0, errors.New("403") })
	if err != nil {
		t.Fatalf("expected stale serve, got err %v", err)
	}
	if !r.Stale || r.Value != 7 {
		t.Fatalf("expected stale value 7, got %+v", r)
	}
}

func TestPoolCacheExpiresAfterMaxStale(t *testing.T) {
	var c poolCache[int]
	if _, err := c.get(func() (int, error) { return 7, nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c.fetchedAt = time.Now().Add(-3 * time.Hour)

	if _, err := c.get(func() (int, error) { return 0, errors.New("403") }); err == nil {
		t.Fatalf("expected error after max stale")
	}
}

func TestPoolCacheColdFetchError(t *testing.T) {
	var c poolCache[int]
	if _, err := c.get(func() (int, error) { return 0, errors.New("403") }); err == nil {
		t.Fatalf("expected error on cold failure")
	}
}

func TestKeyedPoolCache_DifferentKeysDontCollide(t *testing.T) {
	c := newKeyedPoolCache[string]()
	fetches := 0

	r1, err := c.get("15m", func() (string, error) { fetches++; return "a", nil })
	if err != nil || r1.Value != "a" {
		t.Fatalf("15m: %+v err=%v", r1, err)
	}
	r2, err := c.get("1h", func() (string, error) { fetches++; return "b", nil })
	if err != nil || r2.Value != "b" {
		t.Fatalf("1h: %+v err=%v", r2, err)
	}
	if fetches != 2 {
		t.Fatalf("expected one fetch per key, got %d", fetches)
	}

	if _, err := c.get("15m", func() (string, error) { fetches++; return "x", nil }); err != nil {
		t.Fatalf("15m repeat: %v", err)
	}
	if _, err := c.get("1h", func() (string, error) { fetches++; return "x", nil }); err != nil {
		t.Fatalf("1h repeat: %v", err)
	}
	if fetches != 2 {
		t.Fatalf("expected fresh hits within each key, got fetches=%d", fetches)
	}
}
