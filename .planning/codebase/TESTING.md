# Testing Patterns

**Analysis Date:** 2026-08-08

## Test Framework

**Runner:** Go's standard `testing` package (table-driven subtests via `t.Run`).

**Assertion Library:** `github.com/stretchr/testify` — primarily the `assert` package (`assert/assert.go`). `assert.NoError`, `assert.Error`, `assert.Equal`, `assert.NotNil`, `assert.Contains`, `assert.Greater`, `assert.NotEmpty` are the common calls. `testify/require` appears in only one file; plain `t.Fatal`/`t.Fatalf`/`t.Errorf` are used widely in cleaner, lower-dependency tests.

**Mocking:** Custom in-repo handcrafted mocks (not `testify/mock`). `github.com/agiledragon/gomonkey/v2` is declared in `go.mod` and used only by `trader/testutil/test_suite.go` (as the patch mechanism for the shared trader test suite). No other `gomonkey` usage is present.

**Network/fake servers:** Standard `net/http/httptest` (7 test files) for mock exchange/HTTP servers, plus shared `MockHTTPClient` (implements `http.RoundTripper`) in `mcp/mock_test.go`.

**Config** `go.mod` (`go 1.25.11`).

**Run Commands:**
```bash
go test -v ./...                 # All backend tests (verbose)
go test -race -coverprofile=coverage.out -covermode=atomic ./...
make test-backend                # go test -v ./...
make test-coverage               # coverage.out + html report (Makefile)
go test ./...                    # CI advisory run (pr-checks-run.yml, test.yml)
```
- CI (`pr-go-test-coverage.yml`) runs with `-race -coverprofile=coverage.out -covermode=atomic` and a required env var `DATA_ENCRYPTION_KEY` set to a test value.
- `Makefile` `test` also runs frontend tests (`cd web && npm run test`).

## Test File Organization

**Location:** Co-located — `_test.go` lives beside the source file it tests (e.g. `api/handler_trader_test.go` next to `api/handler_trader.go`).

**Naming:**
- File: `<source>_test.go`.
- Shared mock helper file named `mock_test.go` per package (e.g. `mcp/mock_test.go`).
- Tests: `Test<FunctionOrBehavior>` either camelCase (`TestGetOpenPositionBySymbol...`) or with a `_` separator for behavior (`TestGateTrader_SymbolConversion`).
- Example tests: `Example_downcase_style()` in `mcp/examples_test.go` (uses `package mcp_test` for black-box examples).

**Package clause:** Tests use the same package (white-box, e.g. `package api`, `package store`, `package gate`, `package mcp`) to reach unexported symbols. `mcp/examples_test.go` is the exception (`package mcp_test`).

**Structure:**
```
<package>/
├── <source>.go
├── <source>_test.go       # unit tests
└── mock_test.go           # package-shared mocks (e.g. mcp/)
```

## Test Structure

**Suite Organization:** Standard Go testing with `t.Run` subtests. No external BDD/testify-suite wrappers are used.

Two dominant styles co-exist:

1. **Table-driven** with anonymous struct slice + `t.Run`, the most common pattern. Example from `api/server_test.go`:
```go
tests := []struct {
    name                   string
    requestJSON            string
    expectedPromptTemplate string
}{
    {name: "Should accept system_prompt_template=nof1 ...", ...},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // arrange, act, assert using tt.*
    })
}
```

2. **Shared interface test suite** — `trader/testutil/test_suite.go` defines `TraderTestSuite` (black-box test suite for any `types.Trader` implementation). Each exchange consumer embeds it and supplies its own mocks:
```go
type GateTraderTestSuite struct {
    *testutil.TraderTestSuite
    mockServer *httptest.Server
}
func NewGateTraderTestSuite(t *testing.T) *GateTraderTestSuite { ... }
s.T.Run("GetBalance", func(t *testing.T) { s.TestGetBalance() })
```
Consumers: `trader/aster/trader_test.go`, `trader/binance/futures_test.go`, `trader/gate/trader_test.go`.

**Patterns:**
- **Setup:** construct SUT inline at top of each test; `gin.SetMode(gin.TestMode)` set once in tests that build a real router (`api/server_test.go`).
- **Teardown/cleanup:** explicit `defer` to `Close()`/`Cleanup()`. `GateTraderTestSuite.Cleanup()` calls mock server `Close()` then `TraderTestSuite.Cleanup()` (which resets gomonkey patches).
- **Assertion:** prefer `assert.*`; for failure-critical checks use `t.Fatalf("...: %v", err)`.

## Mocking

**Framework:** Handcrafted mocks; `httptest` servers; gomonkey only inside the shared `TraderTestSuite`.

**Patterns:**

1. **Mock HTTP server** for exchange API calls (`trader/gate/trader_test.go`):
```go
mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Path
    var respBody interface{}
    switch {
    case strings.Contains(path, "/futures/usdt/accounts"):
        respBody = map[string]interface{}{"total": "10000.00", ...}
    ...
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(respBody)
}))
```

2. **Mock HTTP client via RoundTripper** for LLM/HTTP callers (`mcp/mock_test.go`):
```go
type MockHTTPClient struct {
    Response     string
    StatusCode   int
    Error        error
    ResponseFunc func(req *http.Request) (*http.Response, error)
    Requests     []*http.Request
}
func (m *MockHTTPClient) ToHTTPClient() *http.Client {
    return &http.Client{Transport: m}
}
```
Helpers like `SetSuccessResponse`, `SetErrorResponse`, `SetNetworkError` configure behavior per test.

3. **Interface mock structs with function fields** (functional mocks) — `mcp/mock_test.go` `MockLogger` and `MockClientHooks` use `*Func` fields that can be swapped per test, e.g. `BuildUrlFunc func() string`.

4. **In-memory SQLite via GORM** for store/DB tests (`store/position_test.go`, `trader/exchange_sync_test.go`, `trader/hyperliquid/sync_test.go`):
```go
db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
positions := NewPositionStore(db)
if err := positions.InitTables(); err != nil { t.Fatalf(...) }
```

**What to Mock:**
- All external side-effects: HTTP exchanges, LLM providers, network errors.
- Database persistence via in-memory SQLite rather than the Postgres/SQLite file driver.

**What NOT to Mock:**
- Pure computation / parsing helpers are tested against real values (e.g. `calculateMaxDrawdownFromPnls` in `store/position_test.go`).
- JSON field mapping is asserted against real marshal/unmarshal round-trips (`api/server_test.go`).
- Route registration is verified against the real Gin router (`TestPublicDecryptRouteNotRegistered`).

## Fixtures and Factories

**Test Data:** Inline literal construction — there are few shared fixtures. Test data is defined as anonymous struct fields or inline `map[string]interface{}`/JSON string literals within each test case. JSON payloads are embeded as raw strings and fed to `json.Unmarshal`.

**Location:**
- Shared test-suite data lives in the suite methods themselves (`trader/testutil/test_suite.go`).
- Mock helpers (`MockLogger`, `MockHTTPClient`, `MockClientHooks`) live in `mcp/mock_test.go`.
- No centralized `testdata/` or external YAML/JSON fixture files were found for Go tests.

## Coverage

**Requirements:** No hard threshold is enforced in CI. `.github/workflows/pr-go-test-coverage.yml` computes coverage and posts an advisory PR comment with a coverage percentage + emoji/badge status via `.github/workflows/scripts/calculate_coverage.py`. Failures do not block the PR (advisory only). `test.yml` explicitly uses `continue-on-error: true`.

**View Coverage:**
```bash
make test-coverage            # coverage.out + coverage.html (Makefile)
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out -o coverage.html
go tool cover -func coverage.out
```

## Test Types

**Unit Tests:** The dominant type. Table-driven tests of pure functions, struct field mapping, symbol conversion, leverage validation, position calculations. Examples: `api/handler_exchange_test.go`, `api/traderid_test.go`, `store/position_test.go`, `kernel/validate_test.go`.

**Integration Tests:** Exchange traders tested against in-process `httptest` mock servers (`trader/gate/trader_test.go`, `trader/binance/futures_test.go`) exercising the full HTTP call → parse → map path. DB access tested against real in-memory SQLite via GORM (`store/position_test.go`, `trader/exchange_sync_test.go`, `trader/hyperliquid/sync_test.go`).

**E2E Tests:** Not used in the Go backend. The frontend (Vite) has its own test run (`cd web && npm run test` in `Makefile` / `test.yml`).

## Common Patterns

**Async/race testing:** CI runs with `-race`. Tests exercising concurrent state (e.g. sync loops) rely on the race detector; the `MockLogger`/`MockHTTPClient` use `sync.Mutex` to make assertions on shared recorded state safe.

**Error Testing:**
```go
if tt.wantError {
    assert.Error(t, err)
} else {
    assert.NoError(t, err)
    if tt.validate != nil { tt.validate(t, result) }
}
```
And for pure functions, range assertions rather than exact floats:
```go
if got < 18.1 || got > 18.3 {
    t.Fatalf("expected ~18.18%% drawdown on a 500 baseline, got %.2f", got)
}
```

**Security regression tests:** Tests encode historical vulnerabilities as assertions, e.g. `TestPublicDecryptRouteNotRegistered` (`api/server_test.go`) fails loudly with `t.Fatalf("SECURITY REGRESSION: ...")` if a decryption-oracle route is ever re-added.

**Interface compliance tests:**
```go
var _ types.Trader = (*GateTrader)(nil)   // compile-time guard
```
in each exchange's `trader_test.go`.

**JSON round-trip tests:** Struct field presence/coverage is verified by unmarshalling full JSON payloads and asserting each DTO field (`api/server_test.go` `TestUpdateTraderRequest_CompleteFields`).

## Testing Conventions to Follow

- Write table-driven tests with `t.Run` subtests for multi-case functionality (`api/server_test.go`, `trader/gate/trader_test.go`).
- Use `testify/assert` for non-fatal assertions; use `t.Fatal/t.Fatalf` for setup/precondition failures and critical regressions.
- For DB/query logic in `store/`, connect to in-memory SQLite (`gorm.Open(sqlite.Open(":memory:"), ...)`) and call `InitTables()` before seeding data.
- For exchange clients, build an `httptest.Server` responding per-path/`r.Method` and point the client at its URL; assert on recorded requests and decoded results.
- Reuse the shared `TraderTestSuite` for new `types.Trader` implementations instead of duplicating generic balance/position/order tests (`trader/testutil/test_suite.go`).
- Place package-local mocks/helpers in `mock_test.go`.
- Reference `DATA_ENCRYPTION_KEY`-dependent tests must set a dummy value (CI does this in `pr-go-test-coverage.yml`).

---

*Testing analysis: 2026-08-08*
