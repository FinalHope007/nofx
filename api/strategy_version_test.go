package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

func newStrategyVersionTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New(), store: st}
	s.setupRoutes()
	return s
}

// callHandler invokes a gin handler directly with a hermetic context that has
// user_id set (avoids JWT middleware in tests).
func callHandler(t *testing.T, h gin.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Params = gin.Params{{Key: "id", Value: "strat-1"}}
	c.Set("user_id", "user-1")
	h(c)
	return w
}

// callHandlerVersion is like callHandler but also sets the :version path param
// for handlers that read it (GET /strategies/:id/versions/:version).
func callHandlerVersion(t *testing.T, h gin.HandlerFunc, method, path, body, version string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Params = gin.Params{{Key: "id", Value: "strat-1"}, {Key: "version", Value: version}}
	c.Set("user_id", "user-1")
	h(c)
	return w
}

func TestStrategyVersionEndpoints(t *testing.T) {
	s := newStrategyVersionTestServer(t)

	// Create a strategy via the store directly to seed v1.
	strategy := &store.Strategy{ID: "strat-1", UserID: "user-1", Name: "Test", Config: `{"strategy_type":"ai_trading"}`}
	if err := s.store.Strategy().Create(strategy); err != nil {
		t.Fatalf("create strategy: %v", err)
	}

	// GET /versions (direct handler call — hermetic, no JWT).
	w := callHandler(t, s.handleListStrategyVersions, http.MethodGet, "/api/strategies/strat-1/versions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET versions status = %d, body=%s", w.Code, w.Body.String())
	}
	var listResp struct {
		Versions []strategyVersionDTO `json:"versions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResp.Versions) != 1 || listResp.Versions[0].Version != 1 {
		t.Fatalf("versions = %+v", listResp.Versions)
	}

	// GET /versions/:version
	w = callHandlerVersion(t, s.handleGetStrategyVersion, http.MethodGet, "/api/strategies/strat-1/versions/1", "", "1")
	if w.Code != http.StatusOK {
		t.Fatalf("GET version status = %d, body=%s", w.Code, w.Body.String())
	}

	// POST /restore
	w = callHandler(t, s.handleRestoreStrategyVersion, http.MethodPost, "/api/strategies/strat-1/restore", `{"version":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST restore status = %d, body=%s", w.Code, w.Body.String())
	}
}
