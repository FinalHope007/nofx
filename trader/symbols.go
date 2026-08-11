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
