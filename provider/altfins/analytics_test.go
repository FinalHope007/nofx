package altfins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nofx/security"
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

func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0 min"},
		{15 * time.Minute, "15 min"},
		{59 * time.Minute, "59 min"},
		{time.Hour, "1 hour"},
		{8 * time.Hour, "8 hours"},
		{47 * time.Hour, "47 hours"},
		{48 * time.Hour, "2 days"},
		{72 * time.Hour, "3 days"},
	}
	for _, tc := range cases {
		if got := humanizeDuration(tc.in); got != tc.want {
			t.Fatalf("humanizeDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAgeFromRaw(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		rawMs    int64
		interval string
		wantBars int
		wantAge  string
	}{
		{"six bars of 15m", now.Add(-103 * time.Minute).UnixMilli(), IntervalMinutes15, 6, "~90 min ago"},
		{"one hour", now.Add(-61 * time.Minute).UnixMilli(), IntervalHourly, 1, "~1 hour ago"},
		{"eight hours", now.Add(-8 * time.Hour).UnixMilli(), IntervalHours4, 2, "~8 hours ago"},
		{"one day", now.Add(-25 * time.Hour).UnixMilli(), IntervalDaily, 1, "~1 day ago"},
		{"two days", now.Add(-((2*24 + 1) * time.Hour)).UnixMilli(), IntervalDaily, 2, "~2 days ago"},
		{"zero timestamp", 0, IntervalMinutes15, 0, ""},
		{"unknown interval", now.UnixMilli(), "HOURS1", 0, ""},
	}
	for _, tc := range cases {
		bars, age := ageFromRaw(tc.rawMs, tc.interval)
		if bars != tc.wantBars || age != tc.wantAge {
			t.Fatalf("%s: ageFromRaw = (%d, %q), want (%d, %q)", tc.name, bars, age, tc.wantBars, tc.wantAge)
		}
	}
}

func TestResolveIdentifierRetrySendsPOSTBody(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	attempts := 0
	var secondBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		secondBody = body
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[{"securityIdentifierId":1021300}]}`))
	}))
	defer srv.Close()

	c := &AnalyticsClient{http: security.SafeHTTPClient(5 * time.Second), baseURL: srv.URL}
	id, ok, err := c.ResolveIdentifier(context.Background(), "ZECUSDT")
	if err != nil {
		t.Fatalf("ResolveIdentifier: %v", err)
	}
	if !ok || id != 1021300 {
		t.Fatalf("expected resolved id 1021300, got id=%d ok=%v", id, ok)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts (429 then 200), got %d", attempts)
	}
	var sent map[string]string
	if err := json.Unmarshal(secondBody, &sent); err != nil {
		t.Fatalf("retry body is not valid JSON (%q): %v", string(secondBody), err)
	}
	if sent["coinFilter"] != "ZEC" {
		t.Fatalf("retry body coinFilter = %q, want ZEC (body was not resent)", sent["coinFilter"])
	}
}
