package binance

import (
	"testing"
	"time"
)

func TestSplitTimeWindowsStaleCursorChunksIntoSevenDayWindows(t *testing.T) {
	start := time.Date(2026, 8, 23, 14, 42, 50, 0, time.UTC)
	end := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)

	windows := splitTimeWindows(start, end, binanceUserTradesMaxWindow)
	if len(windows) < 4 {
		t.Fatalf("stale 26-day cursor split into %d windows, want >=4", len(windows))
	}
	if !windows[0][0].Equal(start) {
		t.Fatalf("first window start = %v, want %v", windows[0][0], start)
	}
	if !windows[len(windows)-1][1].Equal(end) {
		t.Fatalf("last window end = %v, want %v", windows[len(windows)-1][1], end)
	}

	for i, w := range windows {
		if span := w[1].Sub(w[0]); span > binanceUserTradesMaxWindow {
			t.Fatalf("window %d spans %v, exceeds max %v", i, span, binanceUserTradesMaxWindow)
		}
		if i > 0 && !w[0].Equal(windows[i-1][1]) {
			t.Fatalf("window %d start %v does not follow previous end %v", i, w[0], windows[i-1][1])
		}
	}

	sep18 := time.Date(2026, 9, 18, 19, 53, 26, 0, time.UTC)
	covered := false
	for _, w := range windows {
		if !sep18.Before(w[0]) && !sep18.After(w[1]) {
			covered = true
			break
		}
	}
	if !covered {
		t.Fatalf("Sep 18 open time not covered by any window: %v", windows)
	}
}

func TestSplitTimeWindowsShortSpanSingleWindow(t *testing.T) {
	start := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)

	windows := splitTimeWindows(start, end, binanceUserTradesMaxWindow)
	if len(windows) != 1 {
		t.Fatalf("8-hour span split into %d windows, want 1", len(windows))
	}
	if !windows[0][0].Equal(start) || !windows[0][1].Equal(end) {
		t.Fatalf("window = %v..%v, want %v..%v", windows[0][0], windows[0][1], start, end)
	}
}

func TestSplitTimeWindowsExactBoundary(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(binanceUserTradesMaxWindow)

	windows := splitTimeWindows(start, end, binanceUserTradesMaxWindow)
	if len(windows) != 1 {
		t.Fatalf("exactly-7-day span split into %d windows, want 1", len(windows))
	}
}

func TestSplitTimeWindowsInvalidRange(t *testing.T) {
	start := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	if windows := splitTimeWindows(start, start, binanceUserTradesMaxWindow); windows != nil {
		t.Fatalf("equal start/end returned %v, want nil", windows)
	}
	if windows := splitTimeWindows(start, start.Add(-time.Hour), binanceUserTradesMaxWindow); windows != nil {
		t.Fatalf("reversed range returned %v, want nil", windows)
	}
}
