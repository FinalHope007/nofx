# Codebase Structure

**Analysis Date:** 2026-08-08

## Directory Layout

```
nofx/                                  # Go module root (module "nofx"; binary "nofx")
├── main.go                            # Entry point: CLI dispatch, init, server start
├── cli.go                             # Local admin subcommands (reset-password, reset-account)
├── go.mod                             # Go 1.25.11; deps incl. gin, gorm, exchange SDKs, zerolog
├── Makefile                           # Build/run/test targets
├── start.sh / install.sh / install-stable.sh
├── Dockerfile.railway / railway.toml / docker-compose*.yml / nginx/ / railway/
├── api/                               # Gin REST handlers + Server (+ docs registry)
├── auth/                              # JWT issue/validate, password hash, blacklist
├── cmd/e2e_builder_fee/               # Standalone fee test binary
├── config/                            # Global env-driven config (MustInit)
├── crypto/                            # EncryptedString + encryption service
├── data/                              # Runtime data dir (data.db, logs) [not source]
├── documents/                         # (docs/)
├── docs/                              # Markdown docs incl. docs/architecture, docs/agent-skills
├── hook/                              # net/IP/trader observability hooks
├── kernel/                            # Strategy engine, prompts, grid engine, formatter
├── logger/                            # Zerolog bridge + MCPLogger
├── manager/                           # TraderManager (in-memory live traders)
├── market/                            # K-line/indicator/funding data aggregation
├── mcp/                               # LLM client abstraction + registry
│   ├── provider/                      #   One file per LLM provider
│   ├── payment/                       #   x402/claw402 payment layer
│   └── intro/                         #   Migration/usage docs only
├── nginx/, railway/, docker/, screenshots/, scripts/   # Ops/assets
├── provider/                          # Third-party market-data/API clients
│   ├── alpaca/  twelvedata/  coinank/ #   K-line/stock-forex data
│   ├── nofxos/                        #   AI500 quant data + claw402 routing
│   ├── vergex/                        #   Signal ranking / lab / heatmap
│   └── hyperliquid/                   #   Native HL coin/meta data
├── safe/                              # Panic-safe helpers
├── security/                          # SSRF-safe HTTP client + URL validation
├── store/                             # GORM data layer (sub-store per domain)
├── telegram/                          # Telegram bot server
│   ├── agent/                         #   Stateful per-chat AI agent (api_request tool)
│   └── session/                       #   Conversation memory
├── telemetry/                         # Anonymous usage/installation tracking
├── trader/                            # Autonomous trading loop + exchange adapters
│   ├── types/                         #   Trader / GridTrader plugin interfaces
│   ├── aster/  binance/  bitget/  bybit/  gate/  hyperliquid/  indodax/  kucoin/  lighter/  okx/
│   ├── syncloop/  testutil/
│   └── auto_trader*.go                #   Loop, decision, grid, orders, risk, position
├── wallet/                            # Claw402 USDC balance query + cache
└── web/                               # React+TS+Vite frontend (separate build)
    └── src/
        ├── pages/  components/  router/  stores/  contexts/  hooks/
        ├── lib/                        #   axios client + demo/launch helpers
        ├── types/  constants/  data/  utils/  i18n/
        └── test/                       #   Vitest tests
```

## Directory Purposes

**`api/`**
- Purpose: HTTP entry surface; all REST handlers, JWT middleware, rate limiting, preflight, route docs.
- Contains: `server.go`, `route_registry.go`, `errors.go`, `ratelimit.go`, `strategy.go`, `launch_preflight.go`, `exchange_account_state.go`, `crypto_handler.go`, and per-domain `handler_*.go`.
- Key files: `api/server.go` (server + route wiring), `api/route_registry.go` (`GetAPIDocs()` → agent prompt), `api/handler_trader.go` (largest handler, trader CRUD + actions).

**`manager/`**
- Purpose: In-memory lifecycle of all live traders; per-user lazy loading; competition data cache.
- Key files: `manager/trader_manager.go` (`TraderManager`, `addTraderFromStore`, `LoadUserTradersFromStore`, `AutoStartRunningTraders`).

**`trader/`**
- Purpose: The autonomous trading loop plus per-exchange adapters.
- Contains core: `auto_trader.go` (constructor/config + `Run`), `auto_trader_loop.go` (`runCycle`), `auto_trader_decision.go`, `auto_trader_orders.go`, `auto_trader_risk.go`, `auto_trader_grid*.go`, `auto_trader_force.go`, `auto_trader_throttle.go`, `position_*.go`, `grid_regime.go`, `runtime_health.go`, `interface.go`, `helpers.go`.
- Contains adapters: `trader/<exchange>/` per supported exchange.
- Key file: `trader/types/interface.go` (the `Trader` contract).

**`kernel/`**
- Purpose: Strategy execution engine and AI prompt/decision pipelines.
- Key files: `kernel/engine.go` (`StrategyEngine`), `kernel/engine_analysis.go` (`GetFullDecisionWithStrategy`), `kernel/engine_prompt.go` (+ test), `kernel/prompt_builder.go`, `kernel/formatter.go`, `kernel/grid_engine.go`, `kernel/schema.go`.

**`mcp/`**
- Purpose: LLM gateway abstraction, provider registry, request builder, payment.
- Key files: `mcp/client.go`, `mcp/interface.go` (`AIClient`), `mcp/registry.go`, `mcp/providers.go`, `mcp/request_builder.go`, `mcp/context_guard.go`, `mcp/payment/x402.go`/`claw402.go`, `mcp/provider/*.go`.
- Blank imports used for registration: `_ "nofx/mcp/payment"`, `_ "nofx/mcp/provider"`.

**`market/`**
- Purpose: Market-data aggregation and indicator math.
- Key files: `market/data.go` (`Get`, `GetWithTimeframes`, `Normalize`, `IsXyzDexAsset`), `market/data_klines.go`, `market/data_indicators.go`, `market/types.go`, `market/timeframe.go`.

**`provider/`**
- Purpose: Third-party clients feeding data/signals into `kernel`.
- Key files: `provider/nofxos/` (`client.go`, `ai500.go`, `claw402.go`, `oi.go`, `netflow.go`, `price.go`), `provider/vergex/` (`client.go`), `provider/hyperliquid/`, `provider/coinank/`, `provider/twelvedata/kline.go`, `provider/alpaca/kline.go`.

**`store/`**
- Purpose: Canonical persistence. One sub-store per domain, all GORM.
- Contains: `store.go` (facade + init), `user.go`, `ai_model.go`, `exchange.go`, `trader.go`, `strategy.go`, `strategy_schema.go`, `decision.go`, `position*.go`, `equity.go`, `order.go`, `grid.go`, `ai_charge.go`, `telegram_config.go`, `visibility.go`, `gorm.go`, `driver.go`.
- Key file: `store/store.go`.

**`telegram/`**
- Purpose: Bot server + stateful AI trading assistant agent.
- Key files: `telegram/bot.go`, `telegram/agent/agent.go` (the agent loop), `telegram/agent/manager.go`, `telegram/agent/prompt.go`, `telegram/session/memory.go`.

**`web/`**
- Purpose: React + TypeScript + Vite UI.
- Key files: `web/src/main.tsx`, `App.tsx`, `web/src/router/`, `web/src/pages/*.tsx`, `web/src/lib/api/` (axios client), `web/src/stores/` (Zustand).

## Key File Locations

**Entry Points:**
- `main.go`: process entry; wiring + boot order.
- `cli.go`: `reset-password` / `reset-account` admin recovery.
- `api/server.go` `Server.Start()`: HTTP server entry.
- `telegram/bot.go`: Telegram event loop.
- `web/src/main.tsx` / `web/src/router/`: frontend entry.

**Configuration:**
- `config/config.go`: global config (`.env`-driven).
- `go.mod`, `Makefile`, `.env.example`, `docker-compose.yml`, `nginx/`, `railway/`, `web/vite.config.*`.

**Core Logic:**
- Trading loop: `trader/auto_trader_loop.go`, `trader/auto_trader.go`, `trader/auto_trader_decision.go`.
- Strategy engine: `kernel/engine.go`, `kernel/engine_prompt.go`, `kernel/engine_analysis.go`.
- HTTP handlers: `api/handler_*.go`.
- Persistence: `store/*.go`.

**Testing:**
- Go tests co-located as `*_test.go` next to sources (e.g. `api/handler_exchange_test.go`, `store/position_reconcile_test.go`, `market/data_test.go`, `trader/types/parse_test.go`).
- Frontend tests in `web/src/test/`.

## Naming Conventions

**Files:**
- Go: `snake_case.go` for support/schema files (`auto_trader_loop.go`, `exchange_account_state.go`, `launch_preflight.go`); `handler_<domain>.go` for API handlers; `engine_<focus>.go` in `kernel/`; `position_<focus>.go` in `store/`.
- Frontend: PascalCase for pages/components (`TraderDashboardPage.tsx`, `LandingPage.tsx`); `kebab-case` inside component directories.

**Directories:**
- Package dirs are lowercase single words (`api`, `store`, `kernel`, `manager`, `trader`, `mcp`, `provider`, `market`). Exchange adapters are lowercase named dirs under `trader/` (`binance`, `okx`, `hyperliquid`).

**Go packages:**
- Package name matches the trailing directory name (standard Go).
- Prefixed helper packages: `mcp/provider/*`, `provider/<name>/*`, `trader/<exchange>/*`.

## Where to Add New Code

**New Feature (new REST endpoint):**
- Handler: `api/handler_<domain>.go` (new file) or add method to an existing `handler_*.go`.
- Route: register in `api/server.go#setupRoutes` via `s.route(...)` / `s.routeWithSchema(...)` (schema text is surfaced to the Telegram agent and web docs).
- Shared docs tooling: nothing extra — `route_registry.go` collects automatically.
- Tests: `api/handler_<domain>_test.go`.

**New Exchange Integration:**
- Contract is fixed (`Trader`/`GridTrader`): implement it in a new `trader/<exchange>/` package.
- Register constructor + sync in `trader/auto_trader.go#NewAutoTrader` switch and `Run()`; also add store mapping in `manager/trader_manager.go#addTraderFromStore`.
- Add `exchange_type` value where supported-exchanges are enumerated (API + frontend).

**New LLM Provider:**
- Implement `mcp.AIClient` in `mcp/provider/<provider>.go`.
- Register in the provider registry and provider-name constants (`mcp/providers.go`, `mcp/client.go#NewAIClientByProvider`).

**New Strategy Feature / Coin Source:**
- Strategy config schema + validation: `store/strategy.go`, `store/strategy_schema.go`.
- Candidate sourcing logic: `kernel/engine.go#GetCandidateCoins`.
- Prompt / decision parsing: `kernel/prompt_builder.go`, `kernel/engine_analysis.go`, `kernel/formatter.go`.

**New Data Source (K-line / indicator / external):**
- Market/kline provider: `market/data_klines.go` + `provider/<name>/`.
- Quant/signal client: `provider/<name>/` client, wired into `kernel/engine.go`.

**New Persisted Entity:**
- Add a sub-store in `store/<domain>.go`, register it in `store/store.go` (lazy getter + `initTables()`/AutoMigrate).

**Utilities / shared-safe code:**
- `safe/` for panic-safe helpers, `hook/` for observability hooks, `logger/` for logging.

## Special Directories

**`data/`:**
- Purpose: Runtime state (`data.db` SQLite file, logs). Not source code.
- Generated: Yes.
- Committed: No (`gitignore`-ed).

**`web/`:**
- Purpose: Frontend build (Node workspace with its own `node_modules`, `package.json`, `dist/`).
- Generated: `dist/` build artifact + `node_modules/`.
- Committed: source is committed; `node_modules` is not; `dist/` may be committed per `web/.gitignore` config.

**`kernel/mcp/intro/`, `docs/`, `screenshots/`:**
- Purpose: Documentation and asset content only — no Go code.

**`cmd/e2e_builder_fee/`:**
- Purpose: Standalone one-off test binary (fee/`e2e_builder_fee`), not part of the main server.

**`mcp/payment/`:**
- Purpose: x402/claw402 payment/streaming layer — imported for side-effect registration (`_ "nofx/mcp/payment"`).

---

*Structure analysis: 2026-08-08*
