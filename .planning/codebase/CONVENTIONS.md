# Coding Conventions

**Analysis Date:** 2026-08-08

## Overview

`nofx` is a Go 1.25 crypto/futures trading system (`go.mod`, module `nofx`). The codebase follows idiomatic Go with standard library layout per-package, functional options for constructor configuration, a thin custom logging wrapper over `github.com/sirupsen/logrus`, and GORM for database access. Security-sensitive masks (schema/json tags use `snake_case`) and safe-error patterns are pervasive.

## Naming Patterns

**Files:**
- Small cohesive file per concern. The large packages split by responsibility, e.g. `trader/auto_trader.go`, `trader/auto_trader_grid.go`, `trader/auto_trader_risk.go`, `trader/auto_trader_loop.go`, `trader/auto_trader_orders.go`, `trader/auto_trader_throttle.go`.
- Test files always co-located: `foo_test.go` in the same directory as `foo.go`.
- Shared test helper file `mock_test.go` (e.g. `mcp/mock_test.go`) holds mocks/helpers used across that package's tests.
- API handlers use `handler_<domain>.go`, e.g. `api/handler_trader.go`, `api/handler_exchange.go`, `api/handler_klines.go`.

**Functions:**
- Exported functions and methods: `PascalCase` (e.g. `New`, `SetAPIKey`, `CallWithMessages`, `MustBuild`).
- Unexported functions: `camelCase` (e.g. `compilePrompt`, `convertSymbol`, `revertSymbol`).
- GORM query-scope helpers exported as sentence-like functions: `ForUser`, `ForTrader`, `OpenPositions`, `OrderByCreatedDesc`, `Paginate` (see `store/gorm.go`).
- Interface-compliance assertions named `<Name>_InterfaceCompliance` (e.g. `TestGateTrader_InterfaceCompliance` in `trader/gate/trader_test.go`).

**Variables:**
- Short, conventional Go naming: `db`, `cfg`, `s`, `t`, `c` (gin context), `req`, `resp`.
- Config global: `global` package-level var in `config/config.go`.
- Database global: `gormDB` in `store/gorm.go`; logger globals `Log`, `logFile` in `logger/logger.go`.

**Types:**
- Structs use `PascalCase`. Provider-specific clients suffix with `Client` (e.g. `DeepSeekClient`, `ClaudeClient`, `OpenAIClient` in `mcp/provider/`).
- Sub-stores in `store/` suffix with `Store`: `UserStore`, `PositionStore`, `StrategyStore`, `EquityStore`, `OrderStore`, `GridStore`.
- Request/response DTOs in the API use `Update<Resource>Request`, `Get<Resource>ConfigResponse` naming (e.g. `UpdateTraderRequest`, `GetTraderConfigResponse_*` in `api/server_test.go`).
- Interfaces declared small and purpose-specific (see `mcp/interface.go` examples `AIClient`, `ClientEmbedder`).

## Code Style

**Formatting:**
- `gofmt` is the formatting standard. CI enforces it in `.github/workflows/pr-checks-run.yml` (the `go-fmt` step runs `gofmt -l .`). `Makefile` provides `make fmt` → `go fmt ./...`.
- No `.editorconfig` or `.golangci.yml` is present in the repo — formatting relies on `gofmt`/`go vet` in CI, not a lint config file.
- Banners/section dividers in comments used heavily to delineate file sections, e.g. in `mcp/mock_test.go`, `logger/logger.go`, `store/gorm.go`:
  ```go
  // ============================================================
  // Section header comment
  // ============================================================
  ```

**Linting:**
- CI runs `go vet ./...` (advisory, in `pr-checks-run.yml`) and `gofmt -l`.
- `Makefile` provides `make lint` → `golangci-lint run` (requires manual install, not configured in CI).

**Import Organization:**
- Grouped into 3 blocks separated by blank lines:
  1. Standard library (`context`, `encoding/json`, `fmt`, `net/http`, `os`, `time`, `testing`).
  2. Internal module imports (`nofx/api`, `nofx/logger`, `nofx/store`).
  3. Third-party (external modules like `github.com/gin-gonic/gin`, `github.com/stretchr/testify`, `gorm.io/gorm`).
- No import aliases except the blank import `_ "nofx/mcp/payment"` for side effects in `main.go`.

**Path aliases:** None — imports use full module paths.

## Constructor & Configuration Patterns

**Global config:**
- `config/config.go` exposes `MustInit()` (panics on failure, for `main`) and `Init()` (fail-soft for tests/tools), plus `Get()`. Loaded from environment via `os.Getenv`; sensitive/boot-critical values (JWT secret) reject insecure defaults (`minJWTSecretLength = 32`).

**Client constructors:**
- Provider clients use functional options: `NewDeepSeekClient()` and `NewDeepSeekClientWithOptions(opts ...mcp.ClientOption)` in `mcp/provider/deepseek.go` (same pattern in `claude.go`, `openai.go`).

**Builder pattern:**
- `mcp/request_builder.go` uses a fluent builder with `WithSystemPrompt(...)`, `WithUserPrompt(...)`, `AddUserMessage(...)`, and a terminal `Build()` (error-returning) plus `MustBuild()` (panic-on-error).

**Store construction:**
- `store/store.go` exposes `New(dbPath)` returning a `*Store` aggregating lazy sub-stores; GORM init via `store/gorm.go` (`InitGorm`, `InitGormPostgres`, `InitGormWithConfig`). All timestamps via UTC `NowFunc`.

## Error Handling

**Wrapper functions** — backend always returns errors as `(T, error)` or `error`.

**API layer** — errors returned to clients are never raw. `api/errors.go` defines a single safe-error convention:
- `SafeError`, `SafeErrorWithDetails`, `SafeInternalError`, `SafeBadRequest`, `SafeBadRequestWithDetails`, `SafeNotFound`, `SafeUnauthorized`, `SafeForbidden`.
- These take a public message (shown to client) plus an internal `error` that is only logged via `logger.Errorf`.
- `IsSensitiveError` and `SanitizeError` redact internal details (DB host/user, paths, credentials, IPs) from client-visible messages.
- Structured error response shape: `APIErrorResponse{Error, ErrorKey, ErrorParams}` where `ErrorKey` is a stable snake_case string like `trader.create.invalid_btc_eth_leverage` (asserted in `api/handler_trader_test.go`).

**Error wrapping:**
- `fmt.Errorf("...: %w", err)` is the dominant pattern for error wrapping (within `store/gorm.go`, `store/store.go`, `config/config.go`). `errors.New` is rarely used (single constant errors, e.g. `mcp/request_builder_test.go` "at least one message is required").

**Sentinel configurations:**
- Fail-soft vs fail-hard split: `config.MustInit` panics on boot-critical misconfig; store/test callers use the non-panicking `Init`.

**Returning nil slices/maps:**
- Functions commonly return typed `nil` result with a non-nil error (e.g. `GetOpenPositionBySymbol` returns `nil, err`); callers check `err` first, then nil-ness.

## Logging

**Framework:** custom wrapper over `github.com/sirupsen/logrus` — `logger/logger.go`. A single package-level global `Log` initialized in `logger.Init(cfg)` / `logger.InitWithSimpleConfig(level)`.

**Patterns:**
- Package-level functions: `logger.Info`, `logger.Infof`, `logger.Error`, `logger.Errorf`, `logger.Warn`, `logger.Warnf`, `logger.Debugf`, `logger.Fatal(f)`, `logger.Panic(f)`.
- Contextual logging via `logger.WithFields(logrus.Fields{...})` or `logger.WithField(key, value)`.
- Structured-API error logging uses a bracketed prefix: `logger.Errorf("[API Error] %s: %v", publicMsg, internalErr)` and `logger.Errorf("[Internal Error] %s: %v", ...)`.
- Trading-flow warnings are tagged by domain: `logger.Warnf("[Grid] Failed to set leverage %dx: %v", ...)` in `trader/interface.go`.
- Custom `compactFormatter` renders `MM-DD HH:MM:SS [LEVEL] pkg/file.go:line message` using `runtime.Caller`.
- MCP adapter `logger.MCPLogger` implements `mcp.Logger` to bridge the MCP package to the global logger.
- `data/nofx_YYYY-MM-DD.log` file output plus stdout via `io.MultiWriter` (see `logger.Init`); `logger.Shutdown()` closes the file.

**Sensitive-data masking before logging:**
- `api/utils.go` provides `MaskSensitiveString`, `SanitizeModelConfigForLog`, `SanitizeExchangeConfigForLog`, `MaskEmail`. These are mandatory for any handler that logs request payloads containing API keys/secrets. `SanitizeExchangeConfigForLog` deliberately takes the same `ExchangeConfigUpdate` type as the handler so the field list can never drift out of sync.

## Comments

**When to Comment:**
- The codebase is comment-heavy. Every exported symbol has a doc comment starting with its name (`// SanitizeError returns...`, `// Init initializes...`).
- Section banners `// ===...===` organize multi-part files.
- Block comments explain *why* for security/non-obvious decisions, e.g. the decryption-oracle warning in `api/server_test.go`, the JWT-default refusal in `config/config.go`, and field-drift rationale in `api/utils.go`.
- Inline comments document exchange-specific mock mappings in trader tests (e.g. `// Mock GetBalance - /api/v4/futures/usdt/accounts` in `trader/gate/trader_test.go`).

**JSDoc/TSDoc:**
- No JSDoc — Go doc comments are the standard (`// Name ...` form).

## Function Design

**Size:** Functions are generally focused. Large orchestration logic is split across many files in a package (e.g. `trader/` splits loop/orders/risk/throttle/grid). The `api` handlers are single-responsibility (`handleXxx`). Some engine files (e.g. `kernel/engine_prompt.go` at ~63k) are large by policy of separating prompt-building concerns into their own file.

**Parameters:** Positional with explicit names. Some functions accept many closely-related scalars (e.g. `InitGormPostgres(host string, port int, user, password, dbname, sslmode string)` in `store/gorm.go`) rather than a config struct — this is a known tension but consistent in the store package.

**Return values:** Convention is `(result, error)`. For optional lookups the pattern is a pointer result that may be `nil` + error. `MustBuild()` variants panic for invariants that "cannot fail" in correct usage.

**Option pattern:**
- Functional options (`...mcp.ClientOption`) for provider client customization.
- Builder pattern for request construction (`mcp/request_builder.go`).

## Module Design

**Exports:** Minimal public surface per package — unexported helpers/state, exported constructors and operations. Provider packages re-export from `nofx/trader/types` via type aliases for backward compatibility (see `trader/interface.go` — `ClosedPnLRecord = types.ClosedPnLRecord`, etc.).

**Barrel files:** Re-export wrapper files act as barrel-style aggregation: `trader/interface.go` blocks `type ( ... = types.X ... )`. `api/route_registry.go` collects route definitions.

**Package responsibilities:**
- `store` is the single DB access layer ("All database operations should go through this package" — `store/store.go` package comment).
- `logger` is the single logging surface.
- `api` owns HTTP handlers + Gin router; `mcp` owns LLM provider clients.
- `trader` owns exchange adapters; `kernel` owns prompt/decision engines.

## Cross-Cutting Security & Data Handling

- All sensitive strings (API keys, secrets, private keys, email) are masked before logging (`api/utils.go`).
- Client-facing API errors never expose internal error text (`api/errors.go` `SafeInternalError`, `SanitizeError`).
- The API error contract carries a stable machine-readable `error_key` alongside a human message.
- JWT/DB config validation is strict and fails the process boot (`config/config.go`).
- Tests for security regressions exist, e.g. `TestPublicDecryptRouteNotRegistered` asserts a decryption-oracle route is never registered (`api/server_test.go`).

---

*Convention analysis: 2026-08-08*
