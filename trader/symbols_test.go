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
