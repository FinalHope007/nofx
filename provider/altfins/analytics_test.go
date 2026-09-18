package altfins

import (
	"testing"
	"time"
)

func TestValidInterval(t *testing.T) {
	valid := []string{IntervalMinutes15, IntervalHourly, IntervalHours4, IntervalHours12, IntervalDaily}
	for _, iv := range valid {
		if !ValidInterval(iv) {
			t.Fatalf("expected %s valid", iv)
		}
	}
	for _, iv := range []string{"HOURS1", "MINUTES5", "", "1h"} {
		if ValidInterval(iv) {
			t.Fatalf("expected %s invalid", iv)
		}
	}
}

func TestIntervalLabelAndDuration(t *testing.T) {
	cases := map[string]struct {
		label string
		dur   time.Duration
	}{
		IntervalMinutes15: {"15m", 15 * time.Minute},
		IntervalHourly:    {"1h", time.Hour},
		IntervalHours4:    {"4h", 4 * time.Hour},
		IntervalHours12:   {"12h", 12 * time.Hour},
		IntervalDaily:     {"1d", 24 * time.Hour},
	}
	for iv, want := range cases {
		if got := IntervalLabel(iv); got != want.label {
			t.Fatalf("label %s: got %q want %q", iv, got, want.label)
		}
		if got := IntervalDuration(iv); got != want.dur {
			t.Fatalf("dur %s: got %v want %v", iv, got, want.dur)
		}
	}
}

func TestTranslateTrend(t *testing.T) {
	cases := map[string]string{
		"Strong Up (10/10)":  "Strongly Bullish (10/10)",
		"Up (8/10)":          "Bullish (8/10)",
		"Neutral (5/10)":     "Neutral (5/10)",
		"Down (3/10)":        "Bearish (3/10)",
		"Strong Down (2/10)": "Strongly Bearish (2/10)",
	}
	for in, want := range cases {
		if got := translateTrend(in); got != want {
			t.Fatalf("translateTrend(%q) = %q want %q", in, got, want)
		}
	}
}

func TestTranslateTrendChange(t *testing.T) {
	cases := map[string]string{
		"UP_TO_STRONG_UP":      "Bullish to Strongly Bullish",
		"NEUTRAL_TO_DOWN":      "Neutral to Bearish",
		"STRONG_DOWN_TO_DOWN":  "Strongly Bearish to Bearish",
		"UP_TO_NEUTRAL":        "Bullish to Neutral",
		"NEUTRAL_TO_STRONG_UP": "Neutral to Strongly Bullish",
		"DOWN_TO_STRONG_DOWN":  "Bearish to Strongly Bearish",
		"STRONG_UP_TO_UP":      "Strongly Bullish to Bullish",
		"NEUTRAL_TO_UP":        "Neutral to Bullish",
		"DOWN_TO_NEUTRAL":      "Bearish to Neutral",
	}
	for in, want := range cases {
		if got := translateTrendChange(in); got != want {
			t.Fatalf("translateTrendChange(%q) = %q want %q", in, got, want)
		}
	}
}

func TestTranslateMACDSignal(t *testing.T) {
	// The raw MACD_SIGNAL values are "Buy"/"Sell".
	if got := translateMACDSignal("Buy"); got != "Bullish" {
		t.Fatalf("Buy -> %q", got)
	}
	if got := translateMACDSignal("Sell"); got != "Bearish" {
		t.Fatalf("Sell -> %q", got)
	}
}

func TestTranslateHistogram(t *testing.T) {
	if got := translateHistogram("UP"); got != "Bullish" {
		t.Fatalf("UP -> %q", got)
	}
	if got := translateHistogram("DOWN"); got != "Bearish" {
		t.Fatalf("DOWN -> %q", got)
	}
	if got := translateHistogram("null"); got != "" {
		t.Fatalf("null -> %q want empty", got)
	}
	if got := translateHistogram("-"); got != "" {
		t.Fatalf("- -> %q want empty", got)
	}
}
