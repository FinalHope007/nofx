# Vergex.trade Cloudflare Resilience & Position Management on Failure

**Date:** 2026-09-23
**Status:** Design — approved for implementation planning
**Related:** `docs/superpowers/specs/2026-09-19-altfins-and-vergex-per-coin-data-sources-design.md` (access caveat), `data-alt-endpoints.md`

## Problem

Since a few hours before this report, AI500 traders emit `No candidate coins available, cycle skipped`. Verified against the live backend logs (local and server):

```
⚠️ Failed to get candidate coins: trending GET: HTTP 403: <!DOCTYPE html>...Just a moment...
⚠️ AI500 prompt data fetch failed: trending GET: HTTP 403: ...
ℹ️ No candidate coins available, skipping this cycle
```

The vergex.trade `trending-category?key=ai500` endpoint returns a Cloudflare **managed challenge** (HTTP 403, `cType: 'managed'`) to the Go backend's `net/http` client. The same server running `curl` with the identical URL and `User-Agent` receives **HTTP 200 JSON** (7 assets). The difference is the **TLS/HTTP2 fingerprint**, not the headers or path. Cloudflare's managed-challenge tuning changed; Go's default transport fingerprint now falls on the blocked side.

Because every free vergex.trade call shares the same weak transport, the blast radius is **all** vergex.trade endpoints: AI500, OI increase/decrease, netflow, price rankings, and per-coin detail (SignalLab/Heatmap). None have a request cache or fallback.

Secondary defect: when `GetCandidateCoins()` returns empty, `trader/auto_trader_loop.go` aborts the cycle **before calling the LLM**, so open positions go unmanaged.

## Goals

1. Make free vergex.trade calls survive Cloudflare's managed challenge.
2. Serve a last-good candidate pool during a fetch outage, with an explicit warning, for up to 2 hours.
3. Ensure the LLM manages existing positions whenever the candidate pool is unavailable — the cycle must not silently skip.
4. Preserve existing safe-mode semantics when the LLM call itself fails.

## Non-Goals

- A deterministic fallback position manager when the LLM itself fails. (Deferred; out of scope.)
- Server-side Cloudflare solving or hosting a browser session to mint `cf_clearance`.
- Frontend component changes beyond consuming the existing `execution_log` field.

## Design

### Layer 1 — Resilient transport (`security/`)

New file `security/freehttp.go`.

```go
type FreeHTTPMode string
const (
    FreeHTTPFingerprint FreeHTTPMode = "fingerprint" // Chrome TLS/HTTP2 profile
    FreeHTTPCurl        FreeHTTPMode = "curl"        // curl subprocess
    FreeHTTPStdlib      FreeHTTPMode = "stdlib"      // current net/http
)

// HTTPDoer abstracts *http.Client so curl mode can be substituted.
type HTTPDoer interface {
    Do(req *http.Request) (*http.Response, error)
}

// NewFreeHTTPClient returns a client honoring VERGEX_HTTP_MODE
// (default "fingerprint"), with automatic fallback to "curl".
func NewFreeHTTPClient(timeout time.Duration) (HTTPDoer, error)
```

- **Fingerprint mode:** `github.com/bogdanfinn/tls-client` with a Chrome profile.
- **Curl mode:** adapter whose `Do()` executes `curl` (the exact invocation verified to return 200) and synthesizes an `*http.Response`.
- **Fallback chain:** start in fingerprint mode. On the first response that is a 403 Cloudflare challenge, log once and demote to curl for the process lifetime. `VERGEX_HTTP_MODE` pins the mode.
- **SSRF preservation:** fingerprint dialer retains the private-IP `DialContext` check from `security.SafeHTTPClient` (`security/url_validator.go:177`). Curl mode validates via `security.ValidateURL` before exec and on each redirect.
- **Browser headers:** centralize `security.SetBrowserHeaders(req)` — Chrome `User-Agent`, `Accept`, `Accept-Language: en-US,en;q=0.9`, `Referer: https://vergex.trade/`, `sec-fetch-dest/mode/site`.
- **Optional `cf_clearance`:** env `VERGEX_CF_CLEARANCE` → `Cookie: cf_clearance=<v>; cookieConsent=true`. Documented as IP/UA-bound; never committed. Warn at startup if unset while a vergex-backed source is active.

**Provider wiring:**
- `provider/nofxos/free.go`: `FreeTrendingClient` takes an `HTTPDoer` from `NewFreeHTTPClient`; `get()` calls `SetBrowserHeaders`.
- `provider/vergex/client.go`: `NewFreeClient` takes the client; `doFreeGET` calls `SetBrowserHeaders` + optional cookie.
- `NewFreeTrendingClient()` and `NewFreeClient()` signatures may gain no args (construct internally) to minimize call-site churn.

### Layer 2 — TTL cache with 2-hour stale-serve

New file `provider/nofxos/free_cache.go`. A small, concurrency-safe, generic TTL cache embedded in `FreeTrendingClient`.

```go
const (
    freePoolFreshTTL  = 15 * time.Minute // within this age, serve without refetch
    freePoolMaxStale  = 2 * time.Hour    // after this age, do not serve stale
)

type staleCache[T any] struct { /* mutex, value, fetchedAt, hasValue */ }
```

Per-getter behavior (`GetAI500`, `GetOITop/Low`, `GetNetflowTop/Low`, `GetPriceTop/Low`, and the duration-envelope variants):

1. **Fresh** (`age < 15m`): return cached value.
2. **Stale, fetch succeeds:** update cache; return fresh value.
3. **Stale, fetch fails, cache age < 2h:** return cached value and report `stale=true, age`.
4. **Stale, fetch fails, cache age >= 2h (or cold):** return error/empty.

**Crucial rule (confirmed):** the cache is populated **only from a successful fetch**. A successful fetch that legitimately contains no entry for a given coin (e.g. a coin absent from the OI leaderboard) does **not** trigger stale-serve; per-coin detail remains omitted as it is today.

**Stale reporting:** each cached getter returns `(value, stale bool, age time.Duration, err)`. `getAI500Coins`/`getOITopCoins`/etc. in `kernel/engine.go` propagate the stale flag so the loop can log a warning. The prompt-path fetchers in `kernel/engine_analysis.go` also route through the cache: on fetch failure with a < 2h entry they use the stale value and log a warning; otherwise they warn-and-omit exactly as today. (Existing non-cached getters keep their current signatures; only the `FreeTrendingClient` methods change shape, and their call sites are updated.)

`provider/vergex` per-coin details already have a 10-minute TTL cache (`kernel/vergex_detail_cache.go`) and continue to warn-and-omit on failure; only their transport changes.

### Layer 3 — Loop manages positions when the pool is unavailable

Modify `trader/auto_trader_loop.go:86-102`.

```
if len(ctx.CandidateCoins) == 0 && len(ctx.Positions) == 0:
    skip cycle as today (equity snapshot preserved; no LLM call)

if len(ctx.CandidateCoins) == 0 && len(ctx.Positions) > 0:
    DO NOT abort.
    append execution-log warning:
      "⚠️ Candidate pool unavailable (live fetch failed, cache expired); managing existing positions only"
    call the LLM with positions-only context
```

- When candidates come **from cache**, they are present and opens are allowed on the cached pool. No gating change is needed; `filterDecisionsToStrategyUniverse` (`auto_trader_loop.go:385`) already blocks any open outside the candidate+position universe.
- When candidates are empty (cache expired or cold) but positions exist, the LLM still runs with a positions-only context; opens are impossible (empty universe) and close/hold proceed.
- The prompt already renders `Current Positions` independently and omits candidates when empty (`kernel/formatter.go:75-89`, `kernel/engine_prompt.go:896-915`). Add an explicit instruction when candidates are empty: no candidate pool this cycle; manage existing positions only.
- Campaign-context warnings (cached pool used, live fetch failed) are appended to `record.ExecutionLog`, rendered by `web/src/components/terminal/ExecutionLog.tsx` with no frontend changes.

**LLM-call failure:** unchanged. Existing safe mode activates after 3 consecutive failures; positions retain SL/TP and are retried next cycle. No deterministic fallback manager in this change.

## Data Flow

```
trader cycle
  → buildTradingContext
      → GetCandidateCoins()
          → getAI500Coins → FreeTrendingClient.GetAI500
              → cache hit (fresh)            → return
              → cache stale + fetch ok        → update, return
              → cache stale + fetch fail <2h  → return stale + warn
              → cache stale + fetch fail ≥2h  → error/empty
      → attach positions (always)
  → if candidates==0 && positions>0: call LLM (positions-only)
  → if candidates==0 && positions==0: skip
  → record.ExecutionLog gets fetch-failure / stale-cache warnings
```

## Error Handling

| Failure | Behavior |
|---|---|
| Cloudflare 403 | Fingerprint transport; demote to curl on first challenge; if still 403, serve cache |
| Candidate fetch fail, cache < 2h | Serve cached pool, warn in execution log, trade on cached pool |
| Candidate fetch fail, cache ≥ 2h / cold | Empty pool; if positions exist, LLM manages them; else skip |
| Per-coin detail fetch fail | Existing warn-and-omit (unchanged); stale-serve only when a prior fetch populated the cache |
| Per-coin detail present but empty for a coin | Omit (unchanged); no cache |
| LLM call fail | Existing safe mode (unchanged) |

## Testing

- `security/freehttp_test.go`: mode selection from env; fingerprint→curl demotion on 403 challenge; `SetBrowserHeaders` sets expected headers; `cf_clearance` cookie attached when env set; SSRF validation in curl mode.
- `provider/nofxos/free_cache_test.go`: fresh hit; stale-refetch success; stale-serve on failure (< 2h); expiry at ≥ 2h; cache populated only on successful fetch; empty-per-coin does not trigger stale-serve.
- `kernel` tests: stale flag propagation into candidate getters.
- `trader` tests: (empty candidates, no positions) → skipped; (empty candidates, positions) → LLM called; (cached candidates, positions) → normal decision with warning log.
- **Server verification (mandatory):** this dev box is 403-blocked. On the server, build and run a Go probe / the trader and confirm the Go client returns 200 JSON and cycles log candidates. Exact command provided in the implementation plan.

## Rollout

- Default `VERGEX_HTTP_MODE=fingerprint` with automatic curl fallback; no config change required.
- Existing deployments continue to work; behavior degrades to stdlib only if both fingerprint and curl fail.
- Document `VERGEX_CF_CLEARANCE` in `data-alt-endpoints.md` and `.env.example`.

## Files Touched

- `security/freehttp.go` (new), `security/freehttp_test.go` (new)
- `go.mod` / `go.sum` — add `github.com/bogdanfinn/tls-client`
- `provider/nofxos/free.go` — transport + headers
- `provider/nofxos/free_cache.go` (new), `provider/nofxos/free_cache_test.go` (new)
- `provider/vergex/client.go` — free client transport + headers
- `kernel/engine.go` — stale-flag propagation in candidate getters
- `kernel/engine_analysis.go` — route prompt fetchers through cache
- `trader/auto_trader_loop.go` — position-management path + warnings
- `.env.example`, `data-alt-endpoints.md`, the 2026-09-19 spec's access caveat