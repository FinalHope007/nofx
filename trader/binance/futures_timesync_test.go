package binance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// newTimesyncTestTrader returns a FuturesTrader wired to a mock Binance server
// whose /fapi/v1/time returns serverTime. The caller can mutate *serverTime to
// make a re-sync observable via a changed TimeOffset.
func newTimesyncTestTrader(t *testing.T, serverTime *int64) (*FuturesTrader, *httptest.Server) {
	t.Helper()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var respBody interface{}
		switch r.URL.Path {
		case "/fapi/v1/time":
			respBody = map[string]interface{}{"serverTime": *serverTime}
		case "/fapi/v1/positionSide/dual":
			respBody = map[string]interface{}{"code": 200, "msg": "success"}
		default:
			respBody = map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody)
	}))

	ft := NewFuturesTrader("test_api_key", "test_secret_key", "test_user")
	ft.client.BaseURL = mockServer.URL
	ft.client.HTTPClient = mockServer.Client()
	return ft, mockServer
}

// TestEnsureTimeSyncedReSyncsWhenStale verifies that a stale offset triggers a
// fresh server-time re-sync (the -1021 "timestamp ahead" clock-skew fix).
func TestEnsureTimeSyncedReSyncsWhenStale(t *testing.T) {
	serverTime := int64(1234567890000)
	ft, mockServer := newTimesyncTestTrader(t, &serverTime)
	defer mockServer.Close()

	// The constructor already synced against serverTime; capture that offset.
	offsetBefore := ft.client.TimeOffset

	// Move the mock server time forward so a re-sync yields a different offset,
	// and force the offset stale.
	serverTime += 60000
	ft.timeSyncInterval = time.Minute
	ft.lastTimeSync = time.Now().Add(-2 * time.Minute)

	ft.ensureTimeSynced()

	if ft.client.TimeOffset == offsetBefore {
		t.Fatalf("expected TimeOffset to change after stale re-sync (before %d, after %d)", offsetBefore, ft.client.TimeOffset)
	}
	if time.Since(ft.lastTimeSync) > time.Second {
		t.Fatalf("expected lastTimeSync advanced to now, got %s ago", time.Since(ft.lastTimeSync))
	}
}

// TestEnsureTimeSyncedSkipsWhenFresh verifies a recent sync does not re-hit the
// server: TimeOffset stays at the value from the constructor sync.
func TestEnsureTimeSyncedSkipsWhenFresh(t *testing.T) {
	serverTime := int64(1234567890000)
	ft, mockServer := newTimesyncTestTrader(t, &serverTime)
	defer mockServer.Close()

	// Fresh sync (lastTimeSync = now from the constructor). Move the server time
	// ahead; if ensureTimeSynced wrongly re-syncs, the offset would change.
	serverTime += 60000
	offsetBefore := ft.client.TimeOffset
	ft.timeSyncInterval = time.Minute
	ft.lastTimeSync = time.Now()

	ft.ensureTimeSynced()

	if ft.client.TimeOffset != offsetBefore {
		t.Fatalf("expected TimeOffset unchanged when fresh (before %d, after %d)", offsetBefore, ft.client.TimeOffset)
	}
}

// TestEnsureTimeSyncedConcurrent verifies the mutex-guarded re-sync is safe
// under concurrent access (no race, no double-sync thrash beyond one).
func TestEnsureTimeSyncedConcurrent(t *testing.T) {
	serverTime := int64(1234567890000)
	ft, mockServer := newTimesyncTestTrader(t, &serverTime)
	defer mockServer.Close()

	// Force staleness so every goroutine would attempt a re-sync.
	ft.timeSyncInterval = time.Minute
	ft.lastTimeSync = time.Now().Add(-2 * time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ft.ensureTimeSynced()
		}()
	}
	wg.Wait()

	// lastTimeSync must have advanced (the sync ran at least once).
	if time.Since(ft.lastTimeSync) > time.Second {
		t.Fatalf("expected lastTimeSync advanced after concurrent ensureTimeSynced")
	}
}
