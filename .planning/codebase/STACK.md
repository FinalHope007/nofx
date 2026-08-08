# Technology Stack

**Analysis Date:** 2026-08-08

## Languages

**Primary:**
- Go 1.25.11 — backend trading engine, HTTP API, CLI (`go.mod`), all of the `trader`, `provider`, `kernel`, `store`, `mcp` packages
- TypeScript + React — web frontend (`web/package.json`, `web/src/`)

**Secondary:**
- SQL — SQLite/PostgreSQL SQL embedded in `store/driver.go` and GORM models
- Shell — `start.sh`, `install.sh`, `railway/start.sh`, Docker entrypoints
- Dockerfile — container builds under `docker/` and `Dockerfile.railway`

## Runtime

**Environment:**
- Backend: Go binary (`nofx`), `main.go`
- Frontend: Node.js + Vite dev server; static build served by nginx in containers

**Package Manager:**
- Backend: Go modules (`go.mod` / `go.sum`, lockfile present)
- Frontend: npm (`web/package-lock.json`, lockfile present)

## Frameworks

**Core:**
- Gin v1.11.0 — HTTP REST API server (`api/server.go`, `api/handler_*.go`)
- GORM v1.31.1 — ORM for SQLite/PostgreSQL (`store/gorm.go`, `store/store.go`)
- gRPC/protobuf — indirect dependency via geth/zkVM toolchain (`go.mod`)

**Trading / WebSocket:**
- adshao/go-binance/v2 — Binance Futures SDK (`trader/binance/`)
- bybit-exchange/bybit.go.api — Bybit SDK (`trader/bybit/`)
- gateio/gateapi-go/v6 — Gate.io SDK (`trader/gate/`)
- sonirico/go-hyperliquid v0.36.0 — Hyperliquid CLI/Wallet SDK (`trader/hyperliquid/`)
- elliottech/lighter-go — Lighter zk-rollup trading SDK (`trader/lighter/`)
- ethereum/go-ethereum v1.17.3 — EVM (Base chain) wallet / USDC / zkVM (`wallet/usdc.go`)
- gorilla/websocket — WebSocket clients (CoinAnk ws, market data)

**AI / MCP (Model Context Protocol):**
- Custom MCP client core (`mcp/client.go`, `mcp/registry.go`, `mcp/providers.go`)
- lite-llm style router pattern; provider adapters in `mcp/provider/`
- x402 payment protocol (`mcp/payment/x402.go`)

**Web frontend:**
- React 18 + Vite (`web/vite.config.ts`)
- React Router v7, zustand state, swr data fetch, recharts/lightweight-charts charts, tailwind CSS, framer-motion

**Testing:**
- stretchr/testify — assertions (`trader/`, `store/` tests)
- agiledragon/gomonkey — function monkey-patching (`store/`, tests)
- Vitest + Testing Library (frontend, `web/vitest.config.ts`)

## Key Dependencies

**Critical:**
- `github.com/gin-gonic/gin` — all REST endpoints (`api/`)
- `gorm.io/gorm` + `store/driver.go` — persistence layer shared by every subsystem
- `github.com/adshao/go-binance/v2` — core Binance Futures execution (`trader/binance/`)
- `github.com/sonirico/go-hyperliquid` — Hyperliquid trading + wallet (`trader/hyperliquid/`, `wallet/`)
- `github.com/joho/godotenv` — `.env` loading (`config/config.go`, `main.go`)
- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram bot integration (`telegram/bot.go`)

**Infrastructure:**
- `modernc.org/sqlite` — pure-Go SQLite driver (default DB, `store/driver.go`)
- `github.com/lib/pq` — PostgreSQL driver (`store/driver.go`)
- `github.com/rs/zerolog` + `github.com/sirupsen/logrus` — logging (`logger/`); APM via `go.elastic.co/apm/v2`
- `github.com/golang-jwt/jwt/v5` — stateless auth (`auth/auth.go`)
- `golang.org/x/crypto/bcrypt` — password hashing (`auth/auth.go`)

## Configuration

**Environment:**
- `.env` loaded via `github.com/joho/godotenv` in `config/config.go` (`initConfig`) and `main.go`
- `.env.example` documents all variables
- Refuses to boot without a `JWT_SECRET` >= 32 bytes (`config/config.go:90-101`)

**Key env vars (non-secret names):**
- `JWT_SECRET` (required), `API_SERVER_PORT` (default 8080)
- `DB_TYPE` (sqlite|postgres, default sqlite), `DB_PATH`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`
- `TRANSPORT_ENCRYPTION` (browser-side encryption toggle, default false)
- `EXPERIENCE_IMPROVEMENT` (anonymous telemetry, default true)
- `ALPACA_API_KEY`, `ALPACA_SECRET_KEY`, `TWELVEDATA_API_KEY` (market data)
- `NOFX_BACKEND_PORT`, `NOFX_FRONTEND_PORT` (docker-compose)

**Build:**
- Backend: `Makefile`, `Dockerfile.backend` under `docker/`
- Frontend: `web/vite.config.ts`, `web/tsconfig.json`, `web/tailwind.config.js`, `web/eslint.config.js`
- Docker Compose: `docker-compose.yml`, `docker-compose.prod.yml`, `docker-compose.stable.yml`

## Platform Requirements

**Development:**
- Go 1.25+, Node.js (for frontend), TA-Lib native lib (`libta_lib*` copied in Dockerfile.railway)
- `.env` file with `JWT_SECRET` set

**Production:**
- Railway (primary) via `railway.toml` + `Dockerfile.railway` — embeds nginx + SQLite
- Docker Compose (self-host) — separate backend/frontend containers
- GitHub Container Registry images (ghcr.io/nofxaios/nofx/*) built by `.github/workflows/docker-build.yml`

---

*Stack analysis: 2026-08-08*
