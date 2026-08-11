# Exchange-Form Symbol Mapping at the Order Layer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Binance `code=-1121 Invalid symbol` (and equivalent CEX errors) when the `vergex_signal` strategy opens/closes a crypto position, by resolving the canonical bare symbol (`XRP`) to the exchange's required trading symbol (`XRPUSDT`) at the order layer.

**Architecture:** Add a small `ExchangeSymbol(symbol, exchange)` helper that returns the `USDT`-suffixed symbol for USD-M futures CEX adapters (leaving bare/`xyz:`/Hyperliquid unchanged). Apply it consistently inside the four order-entry/exit functions in `trader/auto_trader_orders.go` (open long/short, close long/short) — both for the live adapter calls and for the position matching/lookup that compares against exchange-form positions. The prompt, candidate pool, `decision.Symbol`, and `recordAndConfirmOrder` keep the canonical form; only the order-layer adapter calls and position matches switch to exchange form.

**Tech Stack:** Go 1.25. Reuses `market.Normalize`. CEX symbol normalization is purely at the order layer.

## Global Constraints

- `go vet ./...` + `gofmt` clean; backend errors use safe-error conventions (`fmt.Errorf("...: %w", err)`).
- HIGH-RISK surface (live orders): the only runtime behavior change is the **symbol string passed into CEX adapters + position matching within the four order functions**. Hyperliquid/`xyz:`/empty exchange behavior is byte-identical (`ExchangeSymbol` returns input unchanged for those). No prompt, pool, canonical-decision, or DB-record format changes in this plan.
- CEX list for `ExchangeSymbol` = `binance, bybit, okx, bitget, gate, kucoin, aster`.
- `lighter` is intentionally excluded (it normalizes symbols internally). `hyperliquid`/`indodax`/unknown/empty → unchanged.
- `decision.Symbol` stays canonical (`XRP`) for logging, `posKey`, and `recordAndConfirmOrder`. Exchange adapter calls + position matching use `exchangeSymbol`.
- Risk/stop-loss close path (`auto_trader_risk.go:143,149`) needs no change — verified it already passes exchange-form symbols from `GetPositions()`.
- Grid paths out of scope (use `gridConfig.Symbol`, not the decision path).

---

### Task 1: `ExchangeSymbol` helper + unit tests

**Files:**
- Create: `trader/symbols.go`
- Test: `trader/symbols_test.go`

**Interfaces:**
- Consumes: `market.Normalize` (from `nofx/market`), `strings`.
- Produces: `func ExchangeSymbol(symbol, exchange string) string` in `package trader`.

- [ ] **Step 1: Write the failing test**

Create `trader/symbols_test.go`:

```go
package trader

import "testing"

func TestExchangeSymbol(t *testing.T) {
	cases := []struct{ symbol, exchange, want string }{
		{"XRP", "binance", "XRPUSDT"},
		{"XRP", "bybit", "XRPUSDT"},
		{"XRP", "okx", "XRPUSDT"},
		{"XRP", "bitget", "XRPUSDT"},
		{"XRP", "gate", "XRPUSDT"},
		{"XRP", "kucoin", "XRPUSDT"},
		{"XRP", "aster", "XRPUSDT"},
		// already-quoted is a no-op
		{"BTCUSDT", "binance", "BTCUSDT"},
		// xyz instruments are unchanged (Hyperliquid-only)
		{"xyz:SP500", "binance", "xyz:SP500"},
		// non-CEX / Hyperliquid / empty stay bare
		{"XRP", "hyperliquid", "XRP"},
		{"XRP", "", "XRP"},
		{"XRP", "indodax", "XRP"},
		{"BTCUSDT", "hyperliquid", "BTCUSDT"},
	}
	for _, c := range cases {
		got := ExchangeSymbol(c.symbol, c.exchange)
		if got != c.want {
			t.Errorf("ExchangeSymbol(%q, %q) = %q, want %q", c.symbol, c.exchange, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./trader/ -run TestExchangeSymbol -v`
Expected: FAIL — `undefined: ExchangeSymbol`.

- [ ] **Step 3: Implement `ExchangeSymbol`**

Create `trader/symbols.go`:

```go
package trader

import (
	"strings"

	"nofx/market"
)

// ExchangeSymbol returns the trading symbol an exchange API expects for a
// canonical strategy symbol. Canonical crypto is the bare base ("XRP");
// USD-M futures CEX adapters require "<BASE>USDT". xyz: instruments and
// Hyperliquid/non-CEX symbols are left unchanged.
//
// lighter is intentionally excluded: it normalizes symbols internally.
// hyperliquid / indodax / empty / unknown -> unchanged (Hyperliquid accepts bare).
func ExchangeSymbol(symbol, exchange string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" || strings.HasPrefix(s, "XYZ:") {
		return strings.TrimSpace(symbol)
	}
	switch strings.ToLower(strings.TrimSpace(exchange)) {
	case "binance", "bybit", "okx", "bitget", "gate", "kucoin", "aster":
		return market.Normalize(s)
	default:
		return strings.TrimSpace(symbol)
	}
}
```

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./trader/ -run TestExchangeSymbol -v` → PASS.
Then: `go build ./... && go vet ./...` → PASS. `gofmt -l trader/symbols.go trader/symbols_test.go` → clean.

- [ ] **Step 5: Commit**

```bash
git add trader/symbols.go trader/symbols_test.go
git commit -m "feat(trader): ExchangeSymbol helper maps canonical base to CEX trading symbol"
```

---

### Task 2: Wire `ExchangeSymbol` into the four order-entry/exit functions

**Files:**
- Modify: `trader/auto_trader_orders.go`

**Interfaces:**
- Consumes: `ExchangeSymbol(symbol, at.exchange)` from Task 1.
- Produces: the open/close functions use the exchange-form symbol for live adapter calls AND position matching/lookup.

- [ ] **Step 1: `executeOpenLongWithRecord`**

At the top of `executeOpenLongWithRecord` (after the positions fetch), add:
```go
	exchangeSymbol := ExchangeSymbol(decision.Symbol, at.exchange)
```
Use `exchangeSymbol` for:
- the position-matching loop that checks for existing same-direction position (`pos["symbol"] == exchangeSymbol`).
- `at.trader.SetMarginMode(exchangeSymbol, at.config.IsCrossMargin)`
- `at.trader.OpenLong(exchangeSymbol, quantity, decision.Leverage)`
- `at.trader.SetStopLoss(exchangeSymbol, "LONG", quantity, decision.StopLoss)`
- `at.trader.SetTakeProfit(exchangeSymbol, "LONG", quantity, decision.TakeProfit)`

Keep `decision.Symbol` for logging, `recordAndConfirmOrder(order, decision.Symbol, ...)`, `posKey` (`decision.Symbol + "_long"`).

- [ ] **Step 2: `executeOpenShortWithRecord`**

Mirror Step 1 for the short function:
```go
	exchangeSymbol := ExchangeSymbol(decision.Symbol, at.exchange)
```
Use `exchangeSymbol` for the existing-short position check (`pos["symbol"] == exchangeSymbol`), `SetMarginMode`, `OpenShort`, `SetStopLoss`/`SetTakeProfit` ("SHORT"). Keep `decision.Symbol` for logging/records.

- [ ] **Step 3: `executeCloseLongWithRecord`**

Add `exchangeSymbol := ExchangeSymbol(decision.Symbol, at.exchange)` near the top (after market data). Use `exchangeSymbol` for:
- the exchange position fallback loop (`pos["symbol"] == exchangeSymbol && pos["side"] == "long"`)
- `at.trader.CloseLong(exchangeSymbol, 0)`
- the DB open-position lookup: `at.store.Position().GetOpenPositionBySymbol(at.id, market.Normalize(exchangeSymbol), "LONG")` (replaces the current `normalizedSymbol := market.Normalize(decision.Symbol)` — equivalent but keep it consistent with `exchangeSymbol`).

Keep `decision.Symbol` for logging / `recordAndConfirmOrder`.

- [ ] **Step 4: `executeCloseShortWithRecord`**

Mirror Step 3 for the short close: `exchangeSymbol`, position loop (`pos["symbol"] == exchangeSymbol && pos["side"] == "short"`), `CloseShort(exchangeSymbol, 0)`, DB lookup with `market.Normalize(exchangeSymbol)`.

- [ ] **Step 5: Build + vet + trader/kernel tests**

Run:
```bash
go build ./... && go vet ./... && go test ./trader/ ./kernel/ ./market/
```
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add trader/auto_trader_orders.go
git commit -m "fix(trader): pass exchange-form symbol to CEX order/close/matching"
```

---

## Self-Review Notes

- **Spec coverage:** Task 1 (helper + tests) → Design §1; Task 2 (order-layer wiring) → Design §2. Full verification + live retest handled by the user (Binance crypto-bias strategy open/close).
- **Type consistency:** `ExchangeSymbol(symbol, exchange string) string` (Task 1) consumed by Task 2 with `at.exchange`. `market.Normalize` reused (already idempotent for quoted/xyz).
- **Placeholder check:** all steps carry literal Go. Task 2's steps reference actual current call sites (`auto_trader_orders.go` lines 125-154, 241-260, 289-335, 353-399) — implementer will locate exact lines.
- **Risk note:** HIGH-RISK (live orders). The change is scoped strictly to the symbol passed to CEX adapters + position matching inside the four order functions; Hyperliquid/xyz/unknown-exchange paths are byte-identical. Verified risk.go close path and grid paths need no change.
