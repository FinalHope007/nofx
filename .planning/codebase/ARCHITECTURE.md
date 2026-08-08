<!-- refreshed: 2026-08-08 -->
# Architecture

**Analysis Date:** 2026-08-08

## System Overview

NOFX is a single-binary Go monolith (module `nofx`, 281 Go files, ~62k LOC) for AI-driven crypto/futures trading. It serves a REST API (`api/`) consumed by a React frontend (`web/`) and a Telegram bot, runs autonomous AI trading loops (`trader/` + `kernel/`), and persists state via GORM to SQLite or PostgreSQL (`store/`).

```text
┌─────────────────────────────────────────────────────────────────────┐
│                  CLIENTS                                             │
├──────────────────┬───────────────────────────┬───────────────────────┤
│  Web Frontend    │  Telegram Bot             │  REST / MCP Clients   │
│  `web/src/`      │  `telegram/bot.go`        │                       │
└────────┬─────────┴────────────┬──────────────┴───────────┬───────────┘
         │  REST /api/* (JWT)   │  api_request tool        │
         ▼                      ▼                          │
┌─────────────────────────────────────────────────────────────────────┐
│  api/  — Gin REST handlers (`Server` in `api/server.go`)            │
│  route_registry.go → GetAPIDocs() feeds the Telegram agent prompt   │
└────────┬────────────────────────────┬───────────────────────────────┘
         │ create/query traders       │ start/stop/status
         ▼                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│  manager/TraderManager   holds live `*trader.AutoTrader` in memory  │
│  `manager/trader_manager.go`   (map[traderID]*AutoTrader)           │
└────────┬────────────────────────────────────────────────────────────┘
         │  per-trader lifecycle
         ▼
┌─────────────────────────────────────────────────────────────────────┐
│  trader/AutoTrader  — autonomous trading loop                       │
│  `trader/auto_trader.go` `auto_trader_loop.go`                      │
│   └─ kernel/StrategyEngine  — data sourcing + AI prompt             │
│       `kernel/engine.go` `engine_prompt.go` `prompt_builder.go`     │
│        ├─ kernel/formatter.go  (Context → AI-readable text)         │
│        └─ kernel/grid_engine.go (grid strategy mode)                │
│   └─ mcp/AIClient  — LLM gateway  (`mcp/` + `mcp/provider/*`)       │
│   └─ trader/types.Trader  — exchange adapter plugin interface       │
│       `trader/types/interface.go`  (impls in `trader/<exchange>`/)  │
└──────────────┬──────────────┬──────────────┬───────────────────────┘
               │              │              │
               ▼              ▼              ▼
┌───────────────────────────┐ ┌───────────┐ ┌────────────────────────┐
│  market/  market data     │ │  provider/│ │  store/ data layer      │
│  `market/data.go` (K-line │ │  nofxos,  │ │  `store/store.go` sub-  │
│  + indicators + funding)  │ │  vergex,  │ │  stores (GORM)          │
│  provider/coinank · twel- │ │  hyper.,  │ │  SQLite/Postgres        │
│  vedata · alpaca          │ │  coinank  │ │                          │
└───────────────────────────┘ └───────────┘ └────────────────────────┘
                                                          │
                                                          ▼
                                               crypto/ (EncryptedString)
                                               auth/ (JWT) · config/
                                               security/ · logger/ · telemetry/
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| `api` | HTTP REST surface; JWT auth; enterprise-rate limits; rich route docs for the LLM | `api/server.go`, `api/route_registry.go`, all `handler_*.go` |
| `manager` | Owns live `AutoTrader` instances per user; loads traders from store; competition cache | `manager/trader_manager.go` |
| `trader` | The autonomous trading loop: build context → call AI → execute decisions; also exchange adapter orchestration | `trader/auto_trader*.go`, `trader/interface.go` |
| `trader/types` | The `Trader` / `GridTrader` plugin interfaces every exchange implements | `trader/types/interface.go` |
| `trader/<exchange>` | Concrete per-exchange adapters (binance, bybit, okx, gate, kucoin, bitget, indodax, hyperliquid, aster, lighter) | `trader/binance/`, `trader/hyperliquid/`, … |
| `kernel` | Strategy engine: candidate selection, market/quant/vergex data fetch, prompt building, AI decision parsing, grid engine, token-estimation guard | `kernel/engine.go`, `kernel/engine_analysis.go`, `kernel/prompt_builder.go`, `kernel/formatter.go`, `kernel/grid_engine.go` |
| `mcp` | LLM client abstraction; base HTTP client, provider subpackages, streaming, tool-calling, x402/claw402 payment layer | `mcp/client.go`, `mcp/interface.go`, `mcp/provider/*`, `mcp/payment/` |
| `market` | Market data aggregation: K-lines (CoinAnk/Hyperliquid), technical indicators, funding rate, symbol normalization | `market/data.go`, `market/data_klines.go`, `market/data_indicators.go` |
| `provider` | Third-party data/API clients: NofxOS (ai500 quant), Vergex (signals), Hyperliquid, CoinAnk, TwelveData, Alpaca | `provider/<name>/` |
| `store` | Persistent storage (GORM); sub-store per domain (user, trader, exchange, strategy, position, decision, equity, order, grid, ai_charge) | `store/store.go`, `store/*.go` |
| `telegram` | Bot server; stateful AI agent per chat that calls the REST API via a single `api_request` tool | `telegram/bot.go`, `telegram/agent/agent.go`, `telegram/session/` |
| `config` | Global env-driven configuration; refuses insecure defaults (e.g. JWT secret) | `config/config.go` |
| `auth` | JWT issue/validation, password hashing, token blacklist | `auth/auth.go` |
| `crypto` | `EncryptedString` field-level encryption for stored secrets | `crypto/crypto.go` |
| `security` | SSRF-safe HTTP client + URL validation | `security/url_validator.go` |
| `logger` | Zerolog bridge with structured tags + MCPLogger  | `logger/logger.go` |
| `telemetry` | Anonymous usage/installation telemetry | `telemetry/experience.go` |
| `wallet` | Claw402 USDC balance query/cache (EVM) | `wallet/usdc.go` |
| `hook` | Global net/IP/trader hooks (observability) | `hook/hooks.go` |
| `safe` | Panic-safe Go/dev helpers | `safe/go.go` |
| `web` | React + Vite + TypeScript frontend (served separately; consumes `/api`) | `web/src/` |

## Pattern Overview

**Overall:** Layered monolith with **plugin/adapter interfaces** at the exchange boundary and a **registry-based provider** model for LLMs. The two largest extension points are both interfaces:
- `trader/types.Trader` / `trader/types.GridTrader` — each exchange implements it (`trader/<exchange>/`).
- `mcp.AIClient` — each LLM provider implements it, registered via `mcp` (`mcp/provider/*`), created through `mcp.NewAIClientByProvider` / the `mcp` registry.

**Key Characteristics:**
- **Single deployable binary** (`main.go` → `api.NewServer`); the React app in `web/` is a separate build.
- **In-memory live state** in `manager.TraderManager` held in `map[string]*trader.AutoTrader`, hydrating from `store` on boot (`LoadTradersFromStore`) and per-request (`LoadUserTradersFromStore`).
- **Owner-scoped reads**: API resolves traders strictly from the caller's own list to prevent cross-tenant leaks (IDOR) — see `server.go#getTraderFromQuery`.
- **Route docs as first-class**: every route is registered via `routeRegistry` (`api/route_registry.go`) and `GetAPIDocs()` is injected into the Telegram agent's system prompt, making the bot self-describing.
- **Single-purpose HTTP tool for the agent**: the Telegram agent (`telegram/agent/agent.go`) exposes exactly one `api_request` tool; it is not allowed to pick among many.
- **Per-exchange background sync**: `AutoTrader.Run` starts a dedicated order/position sync goroutine per exchange (`*StartOrderSync(..., 30s, stopMonitorCh)`).

## Layers

**Client / Presentation Layer:**
- Purpose: Human and bot interaction surfaces.
- Location: `web/src/`, `telegram/bot.go`, `telegram/agent/`
- Contains: React pages/components; Telegram updates; stateful per-chat AI agent.
- Depends on: `api` REST endpoints; `auth` for bot JWT.
- Used by: End users.

**API Layer:**
- Purpose: All HTTP entry points; authentication; request shaping; read-model assembly.
- Location: `api/`
- Contains: `Server`, route registry, `handler_*.go`, `strategy.go`, `launch_preflight.go`, `ratelimit.go`, `exchange_account_state.go`, `crypto_handler.go`.
- Depends on: `manager`, `store`, `auth`, `crypto`, `config`, `logger`.
- Used by: `web/`, `telegram/agent`, external clients.

**Orchestration Layer:**
- Purpose: Own live trader instances and their lifecycle; start/stop; competition aggregation.
- Location: `manager/`
- Contains: `TraderManager`, `CompetitionCache`.
- Depends on: `trader`, `store`, `logger`.
- Used by: `api`, `main.go`.

**Trader / Strategy Engine Layer:**
- Purpose: The autonomous decision loop and strategy-driven data sourcing + AI prompting.
- Location: `trader/`, `kernel/`, `mcp/`
- Contains: `AutoTrader`, `StrategyEngine`, prompt builder/formatter, grid engine, `mcp.AIClient` gateway.
- Depends on: `market`, `provider/*`, `mcp`, `store`, `trader/types`, `trader/<exchange>` impls.
- Used by: `manager`.

**Exchange Adapter Layer:**
- Purpose: Translate the unified `Trader` interface into exchange-specific REST/WS calls.
- Location: `trader/types/` (contract) + `trader/<exchange>/` (implementations).
- Depends on: exchange SDKs (binance, bybit, okx, gate, kucoin, hyperliquid, lighter, etc.) and `store` for sync.
- Used by: `AutoTrader` via `trader/types.Trader`.

**Data & Provider Layer:**
- Purpose: Persistence and third-party data ingestion.
- Location: `store/`, `market/`, `provider/`
- Contains: GORM sub-stores; K-line/indicator aggregation; NofxOS/Vergex/Hyperliquid/CoinAnk/TwelveData/Alpaca clients; `wallet/` for USDC.
- Depends on: GORM, `<exchange> SDKs`, `config`.
- Used by: every upper layer.

**Infrastructure Layer:**
- Purpose: Cross-cutting concerns.
- Location: `config/`, `auth/`, `crypto/`, `logger/`, `security/`, `telemetry/`, `hook/`, `safe/`
- Depends on: nothing above it.
- Used by: all layers.

## Data Flow

### Primary Request Path (User reads trader state via API)

1. Client calls `GET /api/positions?trader_id=…` → Gin router in `api/server.go#setupRoutes`.
2. `authMiddleware` validates Bearer JWT (`api/server.go#authMiddleware`) → sets `user_id`.
3. `getTraderFromQuery` loads caller's traders and strict-resolves the trader ID (`api/server.go`).
4. Handler calls into the live `AutoTrader` via `s.traderManager.GetTrader(id)` → `at.GetPositions()` / store queries (e.g. `api/handler_trader.go`).
5. Response serialized as JSON to the client.

### Autonomous Trading Cycle (Core loop)

1. `manager.TraderManager.LoadTradersFromStore` / `LoadUserTradersFromStore` build `AutoTrader` instances (`manager/trader_manager.go#addTraderFromStore`).
2. `at.Run()` (`trader/auto_trader.go`) starts per-exchange sync goroutines, then loops every `ScanInterval`.
3. `runCycle()` (`trader/auto_trader_loop.go`) builds a `kernel.Context` from account balance, open positions, candidate coins, quant/ranking data.
4. `kernel.GetFullDecisionWithStrategy(ctx, mcpClient, engine, variant)` (`kernel/engine_analysis.go`) runs token-estimation guard, builds prompts, calls the LLM via `mcp.AIClient`, parses the `Decision[]`.
5. Decisions are sorted (close-first), filtered to the strategy universe, and executed through `at.executeDecisionWithRecord` → `at.trader.OpenLong/OpenShort/…`.
6. Result persisted as a `store.DecisionRecord`; AI cost recorded via `at.store.AICharge()`.

### Telegram Agent Flow

1. `telegram/bot.go` receives a message → builds per-chat `agent.Agent` (`telegram/agent/agent.go`).
2. On first turn, `buildAccountContext()` snapshots account state via live REST calls.
3. Agent runs a function-calling loop with a single `api_request` tool until the LLM returns a plain-text reply (max 10 iterations, `maxIterations`).
4. Tool executes `newAPICallTool` → REST calls authenticated by a bot JWT (`auth.GenerateJWT`).

**State Management:**
- Persistent: `store.Store` (GORM). Tables initialized via `initTables()` (`store/store.go`) using `AutoMigrate` + raw SQL for `system_config`.
- Live/in-memory: `TraderManager.traders map[string]*AutoTrader`; per-trader caches (`peakPnLCache`, `positionFirstSeenTime`, `CompetitionCache`, market `fundingRateMap`).
- Ephemeral per chat: `telegram/session.Memory`.

## Key Abstractions

**`store.Store` + Sub-Stores:**
- Purpose: Single database facade; each domain has a dedicated sub-store (lazy-init).
- Examples: `store/store.go#User()`, `#Trader()`, `#Position()`, `#Strategy()`, `#Decision()`, `#Equity()`, `#Order()`, `#Grid()`, `#AICharge()`, `#TelegramConfig()`.
- Pattern: Facade + Repository.

**`trader/types.Trader` / `GridTrader`:**
- Purpose: Unified exchange operations (balance, positions, open/close long/short, leverage, SL/TP, closed PnL, open orders); `GridTrader` adds limit orders + order book.
- Examples: `trader/types/interface.go`; implementations `trader/binance`, `trader/okx`, `trader/hyperliquid`, `trader/aster`, `trader/lighter`, `trader/indodax`, …
- Pattern: Strategy/Plugin interface + `GridTraderAdapter` fallback.

**`mcp.AIClient`:**
- Purpose: LLM gateway (CallWithMessages, CallWithRequest, streaming, tool-calling `CallWithRequestFull`).
- Examples: `mcp/interface.go`; providers `mcp/provider/{openai,deepseek,claude,qwen,gemini,grok,kimi,minimax}.go`; payment layer `mcp/payment/{x402,claw402}.go`.
- Pattern: Interface + registry (`mcp/client.go#NewAIClientByProvider`), extensible via `_ "nofx/mcp/provider"` / `_ "nofx/mcp/payment"` blank imports.

**`kernel.StrategyEngine`:**
- Purpose: Holds config, sources candidate coins (static/ai500/oi_top/hyper_rank/vergex_signal/mixed), fetches quant/vergex/ranking data, owns NofxOS + Vergex clients.
- Examples: `kernel/engine.go`.
- Pattern: Strategy engine / service.

**`kernel.GetFullDecisionWithStrategy`:**
- Purpose: End-to-end AI decision pipeline (token guard → prompt → LLM → parse).
- Examples: `kernel/engine_analysis.go`.
- Pattern: Pipeline/orchestrator entry.

**`api.Server` + `routeRegistry`:**
- Purpose: HTTP server with self-documenting routes; `GetAPIDocs()` builds a schema describing every endpoint for the agent.
- Examples: `api/server.go`, `api/route_registry.go`.
- Pattern: Layered HTTP handler with registry.

## Entry Points

**`main.go` → `main()`:**
- Location: `/mnt/e/Users/limli/Documents/GitHub/nofx/main.go`
- Triggers: process start.
- Responsibilities: CLI subcommand dispatch (`cli.go`), `.env` load, logger init, `config.MustInit()`, crypto init, store init, `manager.NewTraderManager()` + `LoadTradersFromStore`, `api.NewServer(...).Start()`, graceful shutdown, `traderManager.StopAll()`.

**`cli.go` → `runCLISubcommand`:**
- Location: `/mnt/e/Users/limli/Documents/GitHub/nofx/cli.go`
- Triggers: `nofx reset-password|reset-account` (local admin recovery, never over HTTP).
- Responsibilities: Open store directly, reset password / wipe accounts.

**`api/server.go` → `Server.Start()`:**
- Location: `/mnt/e/Users/limli/Documents/GitHub/nofx/api/server.go`
- Triggers: `main.go`.
- Responsibilities: Start Gin router on port, serve all `/api/*` routes, graceful `Shutdown()`.

**`telegram/bot.go` → Telegram bot server:**
- Location: `/mnt/e/Users/limli/Documents/GitHub/nofx/telegram/bot.go`
- Triggers: Telegram message events (`go-telegram-bot-api`).
- Responsibilities: Route updates to per-chat `agent.Agent`, reload on config change, unsubscribe.

## Architectural Constraints

- **Threading:** Go: one main goroutine + one goroutine per running `AutoTrader` (started in `manager.trader_manager.go#StartAll` and per `at.Run`) + per-exchange order-sync goroutines + HTTP handlers. Shared state guarded by `sync.RWMutex` (`isRunningMutex`, `peakPnLCacheMutex`, `stopMonitorCh`, `monitorWg`, `TraderManager.mu`, `CompetitionCache.mu`, `Store.mu`).
- **Global state:** `manager.TraderManager.traders` (module-level singleton per server); `config.global` (`config/config.go`); `crypto.SetGlobalCryptoService`; `market.fundingRateMap` (package-level `sync.Map`); `routeRegistry` (`api/route_registry.go`); `auth` token blacklist. These are intentionally process-global but mutation is mutex-guarded.
- **Multi-tenancy:** Every trader/exchange/strategy/model is scoped to a `user_id`. API must never fall back to the global in-memory map when serving a `trader_id` — ownership is enforced by store lookup (`api/server.go#getTraderFromQuery`).
- **Secrets:** Stored as `crypto.EncryptedString`, decrypted at read time; never logged in full (`AGENTS.md` security conventions).
- **SSRF:** External-data URLs pass through `security.ValidateURL` + `security.SafeHTTPClient` before fetching (`kernel/engine.go#fetchSingleExternalSource`).
- **Recovery isolation:** Password/account recovery is deliberately off the HTTP surface (CLI-only, `cli.go`).
- **Insecure config = no boot:** `config.MustInit()` panics on missing/short/default JWT secret.

## Anti-Patterns

### Legacy `sql.DB` path kept for back-compat

**What happens:** `store.Store` retains a `db *sql.DB` and `DBDriver` alongside the GORM `gdb`, with `Deprecated:` markers on `Driver()`, `DB()`, `TransactionSQL()` (`store/store.go`).
**Why it's wrong:** Two DB abstractions mean new code may accidentally reach for the legacy path; `initTables()` still mixes raw `Exec` with `AutoMigrate`.
**Do this instead:** Prefer `store.GormDB()` / `store.Transaction()`; remove `sql.DB`-only accessors once no callers remain.

### Field-name duck-typing for account/position data

**What happens:** `trader/auto_trader_loop.go#buildTradingContext` maps untyped `map[string]interface{}` from `trader.GetBalance()`/`GetPositions()` by hand (`"totalWalletBalance"`, `"positionAmt"`, `"unRealizedProfit"`, `"leverage"`), plus `extractInitialBalance` probing multiple field names in `auto_trader.go`.
**Why it's wrong:** Exchanges disagree on field names; a rename or new exchange silently yields zero values.
**Do this instead:** Have each `trader/<exchange>` adapter normalize into typed structs (as `trader/types.Trader` intends) rather than leaving normalization to the loop.

### Giant `AutoTrader.Run` switch for per-exchange sync/init

**What happens:** `trader/auto_trader.go#Run` and the constructor `NewAutoTrader` contain large `switch config.Exchange { ... }` blocks doing `StartOrderSync` per exchange.
**Why it's wrong:** Adding an exchange means editing the central switch in two places instead of extending the adapter.
**Do this instead:** Define how sync is started on the `Trader`/`GridTrader` interface (or a small optional interface) so each adapter self-registers its sync loop.

## Error Handling

**Strategy:** Consistent wrapped-sentinel-error style with contextual logging. Handlers map errors to HTTP status + `{"error": ...}`; the trading loop logs and persists failures into `store.DecisionRecord`.

**Patterns:**
- Wrapping: `fmt.Errorf("...failed to X: %w", err)`.
- Structured transport errors: `api/errors.go`, `error_key` codes (e.g. `trader.start.preflight_failed`) surfaced to clients.
- Safe-mode degradation: 3 consecutive AI failures activate `setSafeMode` — no new positions, protect existing SL/TP (`auto_trader_loop.go`).
- Payment-layer typed error: `payment.ErrInsufficientFunds` (via `errors.As`) marks AI wallet as empty (`auto_trader_loop.go`).
- Graceful shutdown: SIGINT/SIGTERM → `server.Shutdown()` + `traderManager.StopAll()`.

## Cross-Cutting Concerns

**Logging:** `logger` package (Zerolog); verbose emoji-prefixed `Info/Warn/Error` with `logTag()` (`[trader_id=… trader_name=…]`); `MCPLogger` bridges LLM provider logs; `logger/logger.go`, `logger/config.go`.
**Validation:** Strategy configs validated/clamped via `StrategyConfig.ClampLimits()`; token-estimation guard before LLM calls (`kernel/engine_analysis.go`); `security.ValidateURL` for external URLs.
**Authentication:** JWT HS256 for web/API, bot JWT for Telegram agent, CLI recovery bypasses HTTP entirely; login/register rate-limited per-IP (`api/ratelimit.go`).
**Encryption:** `crypto.CryptoService` + `EncryptedString` for exchange credentials and wallet keys; browser-side transport encryption toggle (`api/crypto_handler.go`).

---

*Architecture analysis: 2026-08-08*
