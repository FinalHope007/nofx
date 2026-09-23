package nofxos

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetAI500Cached_ServesStaleOnFetchFailure(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BR","pair":"BRUSDT","score":75}]}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html>Just a moment...</html>"))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	c.http = srv.Client()

	r1, err := c.GetAI500Cached()
	if err != nil || r1.Stale || len(r1.Value) != 1 {
		t.Fatalf("first: %+v err=%v", r1, err)
	}
	c.ForceStaleForTest(30 * time.Minute)

	r2, err := c.GetAI500Cached()
	if err != nil {
		t.Fatalf("expected stale serve, got %v", err)
	}
	if !r2.Stale || len(r2.Value) != 1 || r2.Value[0].Pair != "BRUSDT" {
		t.Fatalf("expected stale BRUSDT, got %+v", r2)
	}
}

func TestGetOIDataCached_DifferentDurationsDontCollide(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"top":[],"low":[]}`))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	c.http = srv.Client()

	if _, err := c.GetOIDataCached("15m", 10); err != nil {
		t.Fatalf("15m: %v", err)
	}
	if _, err := c.GetOIDataCached("1h", 10); err != nil {
		t.Fatalf("1h: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 underlying fetches for different durations, got %d", calls)
	}
	if _, err := c.GetOIDataCached("15m", 10); err != nil {
		t.Fatalf("15m repeat: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected fresh hit for repeated key, got calls=%d", calls)
	}
}
