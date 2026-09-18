package market

import "testing"

func TestIndicatorPeriodsFields(t *testing.T) {
	d := TimeframeSeriesData{
		Periods:     IndicatorPeriods{EMA: []int{10}, RSI: []int{21}, ATR: []int{7}, BOLL: []int{50}},
		EMA10Values: []float64{1.5},
		RSI21Values: []float64{55.0},
		ATR7:        2.5,
		BOLL50Upper: []float64{9.9},
	}
	if d.Periods.EMA[0] != 10 || d.EMA10Values[0] != 1.5 || d.ATR7 != 2.5 || d.BOLL50Upper[0] != 9.9 {
		t.Fatalf("new indicator fields not stored: %+v", d)
	}
}
