# External Integrations

**Analysis Date:** 2026-08-08

Note: Document the services/endpoints this system calls. Secrets such as exchange API keys, JWT tokens, and API keys are stored via the browser-side encrypted-string pipeline in `crypto/crypto.go` (keys surfaced through `config/config.go` and per-account/DB-stored `EncryptedString`). Values are never reproduced here.

## APIs & External Services

**AI / LLM Providers (MCP):**
- OpenAI-compatible router — `mcp/client.go`, provider registry `mcp/providers.go`
  - OpenAI — `https://api.openai.com/v1`, default model `gpt-5.4` (`mcp/provider/openai.go`)
  - DeepSeek — `https://api.deepseek.com`, default `deepseek-chat` (`mcp/provider/deepseek.go`)
  - Anthropic Claude — `https://api.anthropic.com/v1`, default `claude-opus-4-6` (`mcp/provider/claude.go`)
  - Qwen (Alibaba DashScope) — `https://dashscope.aliyuncs.com/compatible-mode/v1`, default `qwen3-max` (`mcp/provider/qwen.go`)
  - Google Gemini — `https://generativelanguage.googleapis.com/v1beta/openai`, default `gemini-3.1-pro` (`mcp/provider/gemini.go`)
  - xAI Grok — `https://api.x.ai/v1`, default `grok` (`mcp/provider/grok.go`)
  - Moonshot Kimi — `https://api.moonshot.ai/v1`, default `moonshot-v1-auto` (`mcp/provider/kimi.go`)
  - MiniMax — `https://api.minimax.io/v1`, default `MiniMax-M2.7` (`mcp/provider/minimax.go`)
  - Auth: per-provider API key configured in web UI / DB, injected via `Client.SetAPIKey`; OpenAI/Gemini use Bearer auth (`mcp/client.go`)

**AI Payment Protocol:**
- x402 paid-LLM gateway — HTTP `402` retry loop, `X402MaxPaymentRetries = 5`, `X402Timeout = 5m` (`mcp/payment/x402.go`)
- Claw402 paid endpoint — `https://claw402.ai`, provider name `claw402` (`mcp/payment/claw402.go`, `provider/vergex/client.go`)

**Proprietary AI market-analysis API:**
- NofxOS — `https://nofxos.ai`, data endpoints for ai500, coin, netflow, oi, price, claw402 (`provider/nofxos/client.go`, `provider/nofxos/*.go`)

## Exchange Integrations (Futures trading)

All under `trader/<exchange>/`. Auth: per-trader API key/secret/passphrase stored in DB (encrypted), used to sign requests.

- **Binance Futures** — `trader/binance/futures*.go`, market base `https://fapi.binance.com` (`trader/binance/futures.go`, `market/api_client.go`)
- **Bybit** — `trader/bybit/*.go`, base `https://api.bybit.com` (v5 linear) (`trader/bybit/trader.go`)
- **OKX** — `trader/okx/*.go`, base `https://www.okx.com` (`trader/okx/trader.go`)
- **Gate.io** — `trader/gate/*.go`, SDK `github.com/gateio/gateapi-go/v6` (`trader/gate/trader.go`)
- **KuCoin Futures** — `trader/kucoin/*.go`, base `https://api-futures.kucoin.com` (`trader/kucoin/trader.go`)
- **Bitget** — `trader/bitget/*.go`, base `https://api.bitget.com` (`trader/bitget/trader.go`)
- **Hyperliquid** — `trader/hyperliquid/*.go`, REST `https://api.hyperliquid.xyz/info` + SDK `go-hyperliquid`; API wallets/nonce signing (`trader/hyperliquid/trader_account.go`)
- **Indodax** — `trader/indodax/*.go`, base `https://indodax.com` (public + private endpoints) (`trader/indodax/trader.go`)
- **Lighter (zk-rollup)** — `trader/lighter/*.go`, `https://mainnet.zklighter.elliot.ai` / `https://testnet.zklighter.elliot.ai` via `elliottech/lighter-go` (`trader/lighter/trader.go`)

Trading adapter contract defined in `trader/interface.go`; runtime health in `trader/runtime_health.go`; order state reconciliation in `trader/syncloop/syncloop.go`.

## Market Data Providers

- **CoinAnk** — primary market data source, REST `https://api.coinank.com` + WebSocket (`MainWsUrl`, `MainDepthWsUrl`) (`provider/coinank/coinank_api/kline.go`, `depth_ws.go`, `kline_ws.go`); OHLCV, open interest, liquidations, net positions, instrument aggregation
- **Binance Futures** — K-lines `https://fapi.binance.com/fapi/v1/klines`, open interest, premium index (`market/historical.go`, `market/data.go`)
- **Twitter/Stock Data via Alpaca** — `https://data.alpaca.markets/v2` (US stocks), auth `ALPACA_API_KEY` + `ALPACA_SECRET_KEY` (`provider/alpaca/kline.go`)
- **TwelveData** — `https://api.twelvedata.com` (forex & metals), auth `TWELVEDATA_API_KEY` (`provider/twelvedata/kline.go`)
- **Hyperliquid** — coin metadata + K-lines (`provider/hyperliquid/coins.go`, `kline.go`)
- **Vergex** — `https://claw402.ai` client (`provider/vergex/client.go`)

## Blockchain / On-Chain

- **Base (EVM) chain** — RPC `https://mainnet.base.org`, USDC balance queries on contract `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` (`wallet/usdc.go`); powered by `go-ethereum`
- **zkVM / ZK proof toolchain** — `zkvm_runtime`, `poseidon_crypto`, `blst`, `gnark-crypto` indirect deps from `elliottech/lighter-go` (`go.mod`)

## Data Storage

**Databases:**
- SQLite (default) — `modernc.org/sqlite`, file `data/data.db` (`DB_PATH`); pure-Go driver (`store/driver.go`)
- PostgreSQL — `github.com/lib/pq` + GORM `gorm.io/driver/postgres`, connection via `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME/DB_SSLMODE` (`store/driver.go`)
- Selection via `DB_TYPE` enum (`DBTypeSQLite` / `DBTypePostgres`); the driver API is ORM-agnostic (native `database/sql` + GORM, `store/gorm.go`)
- Migration/auto-migrate handled in `store/store.go`

**File Storage:**
- Local filesystem only (SQLite file, `data/` volume in docker-compose)

**Caching:**
- In-memory only, per-module (`provider/nofxos/ai500_cache.go`, `wallet/balance_cache.go`); no Redis/external cache detected

## Authentication & Identity

**Auth Provider:** Custom, self-hosted
- Stateless JWT (HS256) via `github.com/golang-jwt/jwt/v5`, secret enforced >= 32 bytes (`config/config.go`)
- `bcrypt` password hashing (`auth/auth.go`)
- In-memory token blacklist with periodic sweep (`auth/auth.go`)
- Per-exchange/market API keys encrypted at rest via AES + optional RSA end-to-end browser encryption (`crypto/crypto.go`, `TRANSPORT_ENCRYPTION`)
- Telegram bot token stored in DB, supports hot-reload on change (`telegram/bot.go`, `store/telegram_config.go`)

## External Endpoints (Telegram)

- **Telegram Bot API** — long-polling via `telegram-bot-api/v5`; bot token resolved from DB (`store/telegram_config.go`) then env fallback; user/monitor commands (`telegram/bot.go`, `telegram/agent/*`)

## Monitoring & Observability

**Error Tracking:** None detected

**Logs:**
- `github.com/sirupsen/logrus` + custom formatter in `logger/logger.go` (log file + console)
- `github.com/rs/zerolog` and `go.elastic.co/apm/v2` (APM) present as dependencies
- `telemetry/experience.go` — anonymous usage analytics. NOTE: uses the **Google Analytics 4 legacy endpoint** `https://www.google-analytics.com/mp/collect` (Measurement Protocol). This is a deprecated/will-be-removed endpoint (see CONCERNS if mapped); toggle via `EXPERIENCE_IMPROVEMENT`

**Health probes:**
- Railway healthcheck `/health` (`railway.toml` → `Dockerfile.railway`); docker-compose `/api/health` (`docker-compose.yml`)

## CI/CD & Deployment

**Hosting:**
- Railway (`railway.toml`, `Dockerfile.railway`) — all-in-one container with nginx + SQLite
- Self-host via `docker-compose.yml` (backend :8080, frontend via nginx :3000)

**CI Pipeline:**
- GitHub Actions (`.github/workflows/`)
  - `docker-build.yml` — build/push backend & frontend images to GHCR (ghcr.io/nofxaios/nofx/*)
  - `pr-checks.yml` / `pr-checks-run.yml` / `pr-checks-comment.yml` — Go fmt/vet/test + frontend lint/test with artifact aggregation
- Go 1.21 CI, Node for frontend checks
- `code-review`: `github-actions` + CodeOwner-based reviewers

**Registry:**
- GitHub Container Registry (GHCR)

## Environment Configuration

**Required env vars (critical):**
- `JWT_SECRET` — required at boot, min 32 bytes (`config/config.go`)

**Optional / provider:**
- `DB_TYPE`, `DB_PATH`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`
- `TRANSPORT_ENCRYPTION`, `EXPERIENCE_IMPROVEMENT`
- `ALPACA_API_KEY`, `ALPACA_SECRET_KEY`, `TWELVEDATA_API_KEY`
- `NOFX_BACKEND_PORT`, `NOFX_FRONTEND_PORT`

**Secrets location:**
- `.env` file (bind-mounted into containers per `docker-compose.yml`); values referenced in `config/config.go`
- Exchange/AI API keys persisted in the database via `crypto.CryptoService` (encrypted `ENC:v1:` values)
- Telegram token in `store/telegram_config.go`

## Webhooks & Callbacks

**Incoming:**
- No external service webhooks detected. The web UI polls the REST API (`api/server.go`) and Telegram long-polls the Bot API
- (Market updates fetched via WebSocket/REST pull from CoinAnk/Binance, not push webhooks)

**Outgoing:**
- Telegram Bot API sends messages/updates (`telegram/bot.go`)
- GA4 Measurement Protocol POST `https://www.google-analytics.com/mp/collect` (`telemetry/experience.go`)
- NofxOS, Vergex, x402/Claw402 paid-LLM endpoints (`provider/nofxos/`, `provider/vergex/`, `mcp/payment/`)
- All configured exchange REST/WebSocket endpoints under `trader/*`

---

*Integration audit: 2026-08-08*
