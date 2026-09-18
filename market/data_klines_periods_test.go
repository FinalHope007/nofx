package market

import (
	"math"
	"testing"
)

func TestCalculateTimeframeSeriesPeriods(t *testing.T) {
	klines := generateTestKlines(120)
	periods := IndicatorPeriods{
		EMA:  []int{10},
		RSI:  []int{21},
		ATR:  []int{7},
		BOLL: []int{50},
	}

	data := calculateTimeframeSeries(klines, "5m", 120, periods)

	if len(data.EMA10Values) == 0 {
		t.Fatalf("expected EMA10Values non-empty")
	}
	wantEMA10 := ExportCalculateEMA(klines, 10)
	if got := data.EMA10Values[len(data.EMA10Values)-1]; math.Abs(got-wantEMA10) > 1e-6 {
		t.Fatalf("EMA10Values last = %v, want %v", got, wantEMA10)
	}

	if len(data.RSI21Values) == 0 {
		t.Fatalf("expected RSI21Values non-empty")
	}
	wantRSI21 := ExportCalculateRSI(klines, 21)
	if got := data.RSI21Values[len(data.RSI21Values)-1]; math.Abs(got-wantRSI21) > 1e-6 {
		t.Fatalf("RSI21Values last = %v, want %v", got, wantRSI21)
	}

	wantATR7 := ExportCalculateATR(klines, 7)
	if math.Abs(data.ATR7-wantATR7) > 1e-6 {
		t.Fatalf("ATR7 = %v, want %v", data.ATR7, wantATR7)
	}

	if len(data.BOLL50Upper) == 0 {
		t.Fatalf("expected BOLL50Upper non-empty")
	}
	wantBOLL50Upper, _, _ := ExportCalculateBOLL(klines, 50, 2.0)
	if got := data.BOLL50Upper[len(data.BOLL50Upper)-1]; math.Abs(got-wantBOLL50Upper) > 1e-6 {
		t.Fatalf("BOLL50Upper last = %v, want %v", got, wantBOLL50Upper)
	}

	if len(data.EMA20Values) == 0 {
		t.Fatalf("expected legacy EMA20Values non-empty")
	}
}
