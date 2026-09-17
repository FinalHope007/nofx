package market

import "testing"

func TestFormatPriceSigFigs(t *testing.T) {
	cases := []struct {
		name  string
		price float64
		want  string
	}{
		// High-priced: the floor of 2 decimal places keeps the cents.
		{"btc keeps cents", 76708.9123, "76708.91"},
		// 5 significant figures dominate once the integer part is short.
		{"eth two signals then floor", 2456.785, "2456.78"},
		{"three-digit price", 123.4567, "123.46"},
		{"two-digit price", 23.45678, "23.457"},
		{"one-digit price", 5.43219, "5.4322"},
		{"one-digit 5 sig figs", 5.4, "5.4000"},
		// Sub-dollar coins keep 5 significant figures.
		{"sub-dollar coin", 0.995432, "0.99543"},
		{"sub-cent coin", 0.005568, "0.0055680"},
		{"sub-cent coin trailing", 0.00015060, "0.00015060"},
		{"ultra-low coin", 0.00002070, "0.000020700"},
		{"zero", 0, "0.00"},
		{"negative", -0.005568, "-0.0055680"},
		{"exact two-decimal", 100, "100.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatPriceSigFigs(tc.price); got != tc.want {
				t.Errorf("FormatPriceSigFigs(%v) = %q, want %q", tc.price, got, tc.want)
			}
		})
	}
}

// Lower-precision exchange values must pass through unchanged (the formatter
// must not invent precision that changes the value, and must not reject it).
func TestFormatPriceSigFigs_LowerPrecisionExchangeValue(t *testing.T) {
	cases := []struct {
		price float64
		want  string
	}{
		{97000, "97000.00"},
		{1234, "1234.00"},
		{12.5, "12.500"},
		{0.5, "0.50000"},
	}
	for _, tc := range cases {
		if got := FormatPriceSigFigs(tc.price); got != tc.want {
			t.Errorf("FormatPriceSigFigs(%v) = %q, want %q", tc.price, got, tc.want)
		}
	}
}
