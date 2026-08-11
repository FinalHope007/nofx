# Exchange-Form Symbol Mapping at the Order Layer — Design

> Design for fixing the **`Invalid symbol` (Binance -1121)** error when the `vergex_signal` strategy tries to open/close a position. Root cause is a symbol-form inconsistency between the prompt/decision (bare base, e.g. `XRP`) and what CEX futures APIs require (`XRPUSDT`). Nothing is implemented until this design is reviewed and approved.

## Problem

- The `vergex_signal` crypto candidate pool emits **bare base symbols** (`XRP`) because `TradableSymbolForMarket("core_perp", ...)` returns `QuerySymbol(...)` with the `USDT`/`USD` suffix stripped (`provider/vergex/client.go:444-456`). Verified by executing the code: `XRP`, `XRPUSDT`, `xyz:XRP` all yield `XRP`.
- The LLM correctly echoes the candidate it is shown (`XRP`), per the vergex prompt contract ("do not add USDT to core crypto symbols").
- On a **Binance** (USDM futures) trader, the order layer passes `XRP` straight through `OpenShort`/`OpenLong` → `SetLeverage` → Binance SDK, which requires `XRPUSDT` → `code=-1121, msg=Invalid symbol`.
- Hyperliquid traders accept **both** bare and `USDT` forms (its adapter normalizes via `FormatCoinForAPI`), so the bug only manifests on CEX adapters.

## Symbol-form inventory (verified)

| Layer | Form | Example |
|---|---|---|
| `vergex_signal` crypto candidate pool | bare base | `XRP` |
| `hyper_main`/`hyper_rank`/`ai500`/oi/netflow/price pools | `market.Normalize(base)` → `USDT`-suffixed | `XRPUSDT`, `CYSUSDT` |
| LLM decision / prompt (vergex_signal) | bare base (contract) | `XRP` |
| Order layer (live API calls) today | passes `decision.Symbol` raw | `XRP` → Binance rejects |
| DB book (Binance via OrderSync) | exchange form | `XRPUSDT` |
| DB lookup on close | `market.Normalize(decision.Symbol)` | `XRPUSDT` |

So the only problematic seam is **the order layer passing the canonical (bare) symbol into CEX adapters**; the DB book is already exchange-form for Binance (written by OrderSync from Binance fill data).

## Design (option 2 — exchange-form symbol at the order layer, canonical book)

Decision: keep the prompt/decision/pool in **canonical form** (bare `XRP`), and resolve the symbol to the **exchange's required form** at the order-entry/exit layer. Use that exchange-form symbol consistently for the live adapter calls AND position matching/lookups in those order functions, so open/close/match agree.

### 1. New helper `ExchangeSymbol` (in `trader/`)

```go
// ExchangeSymbol returns the trading symbol an exchange API expects for a
// canonical strategy symbol. Canonical crypto is the bare base ("XRP");
// USD-M futures CEX adapters require "<BASE>USDT". xyz: instruments and
// Hyperliquid/non-CEX symbols are left unchanged.
func ExchangeSymbol(symbol, exchange string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" || strings.HasPrefix(s, "XYZ:") {
		return strings.TrimSpace(symbol)
	}
	switch strings.ToLower(strings.TrimSpace(exchange)) {
	case "binance", "bybit", "okx", "bitget", "gate", "kucoin", "aster":
		return market.Normalize(s) // "XRP" -> "XRPUSDT" (no-op if already quoted, xyz unchanged)
	default:
		return strings.TrimSpace(symbol)
	}
}
```

Notes:
- `market.Normalize` idempotent: `Normalize("XRPUSDT")` returns `XRPUSDT`; `Normalize("XRP")` returns `XRPUSDT`; `Normalize("xyz:SP500")` returns `xyz:SP500` (xyz preserved). So the helper is safe for already-quoted and xyz symbols.
- CEX list = the futures CEX adapters that strictly require a `USDT`-suffixed symbol, verified to pass the symbol unmodified to their API: **binance, bybit, okx, bitget, gate, kucoin, aster**.
- `lighter` is intentionally NOT in the list — it normalizes symbols internally (handles `USDT` vs bare forms itself).
- `hyperliquid` / `indodax` / empty / unknown → unchanged (bare `XRP`); Hyperliquid accepts bare.

### 2. Wire into the order-entry/exit functions (`trader/auto_trader_orders.go`)

In each of the four functions, compute once near the top:
```go
exchangeSymbol := ExchangeSymbol(decision.Symbol, at.exchange)
```

Use `exchangeSymbol` for:
- `at.trader.SetMarginMode(exchangeSymbol, ...)`
- `at.trader.OpenLong/OpenShort(exchangeSymbol, qty, lev)`
- `at.trader.SetStopLoss/SetTakeProfit(exchangeSymbol, ...)`
- `at.trader.CloseLong/CloseShort(exchangeSymbol, 0)`
- the **position-matching loops** that compare against `at.trader.GetPositions()` (whose symbols are exchange-form, e.g. `XRPUSDT`): `pos["symbol"] == exchangeSymbol`
- the DB open-position lookup for close: `GetOpenPositionBySymbol(at.id, market.Normalize(exchangeSymbol), side)` (already effectively `XRPUSDT`; keep `market.Normalize` for robustness, or `exchangeSymbol`).

Do NOT change:
- `decision.Symbol` (stays canonical) — used for logging, `posKey`, `recordAndConfirmOrder(order, decision.Symbol, ...)`, etc., to keep the canonical/decision layer stable.

### 3. Risk / stop-loss close path (`trader/auto_trader_risk.go`)

The forced-close path (`CloseLong(symbol, 0)` / `CloseShort(symbol, 0)`) at `auto_trader_risk.go:143,149` uses a `symbol` that comes from open positions (exchange form already). Verify it passes the exchange-form symbol; if it derives from `GetPositions()`, it is already `XRPUSDT` and needs no change. Confirm only.

### 4. Grid paths

Out of scope — grid uses `gridConfig.Symbol` (already the config symbol, not the LLM decision path).

## Verification

- New unit test for `ExchangeSymbol`:
  - `("XRP", "binance")` → `XRPUSDT`
  - `("BTCUSDT", "binance")` → `BTCUSDT` (no-op)
  - `("xyz:SP500", "binance")` → `xyz:SP500` (unchanged)
  - `("XRP", "hyperliquid")` → `XRP` (unchanged)
  - `("XRP", "")` → `XRP` (unchanged)
  - `("XRP", "bybit")` → `XRPUSDT` (all CEX)
- Backend `go build ./... && go vet ./... && go test ./trader/ ./market/ ./kernel/`.
- Live (user retest, Binance trader + crypto-bias strategy): open/close a crypto position succeeds (`XRPUSDT`), no `-1121`.

## Risk

- HIGH-RISK surface (live orders) per HANDOFF. The only runtime behavior change is the **symbol string passed into CEX adapters + position matching within the four order functions**. Hyperliquid/xyz behavior is byte-identical (`ExchangeSymbol` returns the input unchanged). No prompt, pool, or canonical-decision changes.
