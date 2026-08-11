package market

import "testing"

func TestResolveKlineExchange(t *testing.T) {
	cases := []struct {
		exchange, want string
	}{
		{"binance", "binance"},
		{"BINANCE", "binance"},
		{"hyperliquid", "hyperliquid"},
		{"okx", ""}, // unknown/other -> default (CoinAnk), represented as ""
		{"", ""},
	}
	for _, c := range cases {
		got := resolveKlineExchange(c.exchange)
		if got != c.want {
			t.Errorf("resolveKlineExchange(%q) = %q, want %q", c.exchange, got, c.want)
		}
	}
}
