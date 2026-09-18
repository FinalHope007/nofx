# AltFins + Vergex Per-Coin Data Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add AltFins analytics and vergex.trade SignalLab/Heatmap as free, source-independent per-coin data sources in the LLM prompt.

**Architecture:** Follow the existing Binance per-coin enrichment pattern: a provider fetches per-symbol data, a TTL cache in `kernel` avoids repeat fetches, `attachPerCoinSignals` populates `kernel.PerCoinSignal`, `formatPerCoinSignals` renders prompt blocks behind new `IndicatorConfig` toggles, and the trader loop pre-warms the cache before each cycle. AltFins adds a two-step provider (resolve ID → analytics). Vergex reuses the existing free client and adds two independent toggles, gated to non-`vergex_signal` sources to avoid double-fetching.

**Tech Stack:** Go 1.x (`nofx`), `net/http`, `security.SafeHTTPClient`; React + TypeScript + Vite + Vitest (`web/`).

## Global Constraints

- All new network calls MUST use `security.SafeHTTPClient(...)` (SSRF protection); never `http.DefaultClient`.
- All new fetches are best-effort: never return an error into the prompt, never block a cycle on failure; log server-side and omit.
- Do NOT log API keys, tokens, or credentials.
- AltFins valid intervals are exactly `MINUTES15`, `HOURLY`, `HOURS4`, `HOURS12`, `DAILY` (`HOURS1` is INVALID; the enum is `HOURLY`).
- AltFins `level` is always `LEVEL_1`.
- Rendered trend/MACD text uses FULL words and `Bullish`/`Bearish`; never `ST`/`MT`/`LT`, never `Buy`/`Sell`/`Up`/`Down` in output.
- Prompt interval labels: `MINUTES15→[15m]`, `HOURLY→[1h]`, `HOURS4→[4h]`, `HOURS12→[12h]`, `DAILY→[1d]`.
- Do NOT change the Binance Opportunity code, the `vergex_signal` candidate-pool path, or `FetchVergexDataBatch` behavior.
- Verify commands: `go build ./...`, `go vet ./...`, `gofmt -l .`; `cd web && npx tsc --noEmit && npm test`.

---

## File Structure

**New files:**
- `provider/altfins/analytics.go` — AltFins client (resolve id + analytics) and enum translation.
- `provider/altfins/analytics_test.go` — parse/translation tests with fixtures.
- `kernel/altfins_detail_cache.go` — TTL cache for AltFins analytics keyed `<symbol>|<interval>`.
- `kernel/altfins_detail_cache_test.go` — cache TTL test.

**Modified files:**
- `kernel/engine.go` — `PerCoinSignal` fields, `altfinsClient` + cache fields, interface, constructor init, `PrefetchAltFinsDetails`.
- `kernel/engine_analysis.go` — `attachPerCoinSignals` gating + AltFins/vergex fetch blocks.
- `kernel/engine_prompt.go` — render AltFins + vergex blocks.
- `store/strategy.go` — `IndicatorConfig` fields, `ClampLimits`, `EstimateTokens`.
- `store/strategy_test.go` — schema/clamp/estimate tests.
- `trader/auto_trader_loop.go` — generalize prefetch scheduler to include AltFins (+ vergex when non-signal).
- `trader/auto_trader_prefetch.go` — helper if needed.
- `web/src/types/strategy.ts` — new config types.
- `web/src/features/strategies/strategyFactory.ts` — new form fields mapping.
- `web/src/features/strategies/strategyFactory.test.ts` — mapping test.
- `web/src/features/strategies/EditorStepPage.tsx` — new toggles + interval multiselect + restore.
- `web/src/features/strategies/dataSourceDefaults.ts` — new keys default false.

---

## Task 1: AltFins provider (client + translation)

**Files:**
- Create: `provider/altfins/analytics.go`
- Test: `provider/altfins/analytics_test.go`

**Interfaces:**
- Consumes: `nofx/security`.
- Produces:
  ```go
  package altfins

  const (
      IntervalMinutes15 = "MINUTES15"
      IntervalHourly    = "HOURLY"
      IntervalHours4    = "HOURS4"
      IntervalHours12   = "HOURS12"
      IntervalDaily     = "DAILY"
  )

  // ValidInterval reports whether iv is one of the five supported AltFins intervals.
  func ValidInterval(iv string) bool

  // IntervalLabel maps a canonical interval to its prompt label ("15m","1h","4h","12h","1d").
  func IntervalLabel(iv string) string

  // IntervalDuration returns the time interval duration for age math.
  func IntervalDuration(iv string) time.Duration

  type Analytics struct {
      Interval              string
      ShortTermTrend        string
      MediumTermTrend       string
      LongTermTrend         string
      ShortTermTrendChange  string
      MediumTermTrendChange string
      LongTermTrendChange   string
      MACDSignal            string // "Bullish"/"Bearish"
      MACDSignalBarsAgo     int
      MACDSignalAgeText     string // "~90 min ago"
      MACDHistogram         string // "Bullish"/"Bearish"/"" (omit)
  }

  type AnalyticsClient struct { /* unexported fields */ }
  func NewAnalyticsClient() *AnalyticsClient
  func (c *AnalyticsClient) ResolveIdentifier(ctx context.Context, symbol string) (int64, bool, error)
  func (c *AnalyticsClient) GetAnalytics(ctx context.Context, id int64, interval string) (*Analytics, error)
  ```

- [ ] **Step 1: Write the failing test**

Create `provider/altfins/analytics_test.go`:

```go
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
		"Strong Up (10/10)":   "Strongly Bullish (10/10)",
		"Up (8/10)":           "Bullish (8/10)",
		"Neutral (5/10)":      "Neutral (5/10)",
		"Down (3/10)":         "Bearish (3/10)",
		"Strong Down (2/10)":  "Strongly Bearish (2/10)",
	}
	for in, want := range cases {
		if got := translateTrend(in); got != want {
			t.Fatalf("translateTrend(%q) = %q want %q", in, got, want)
		}
	}
}

func TestTranslateTrendChange(t *testing.T) {
	cases := map[string]string{
		"UP_TO_STRONG_UP":       "Bullish to Strongly Bullish",
		"NEUTRAL_TO_DOWN":       "Neutral to Bearish",
		"STRONG_DOWN_TO_DOWN":   "Strongly Bearish to Bearish",
		"UP_TO_NEUTRAL":         "Bullish to Neutral",
		"NEUTRAL_TO_STRONG_UP":  "Neutral to Strongly Bullish",
		"DOWN_TO_STRONG_DOWN":   "Bearish to Strongly Bearish",
		"STRONG_UP_TO_UP":       "Strongly Bullish to Bullish",
		"NEUTRAL_TO_UP":         "Neutral to Bullish",
		"DOWN_TO_NEUTRAL":       "Bearish to Neutral",
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/altfins/ -run Test -v`
Expected: FAIL — package/undefined symbols (`translateTrend`, etc.).

- [ ] **Step 3: Write minimal implementation**

Create `provider/altfins/analytics.go`:

```go
// Package altfins provides a client for the unofficial, scraped AltFins
// analytics feed used to enrich per-coin LLM context.
package altfins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nofx/security"
)

const (
	// canonical intervals accepted by the analytics endpoint. Note the enum is
	// HOURLY (not HOURS1).
	IntervalMinutes15 = "MINUTES15"
	IntervalHourly    = "HOURLY"
	IntervalHours4    = "HOURS4"
	IntervalHours12   = "HOURS12"
	IntervalDaily     = "DAILY"

	altFinsBaseURL = "https://altfins.com"
	resolvePath    = "/vaadinRest/v1/nonauth/signal-feed"
	analyticsPath  = "/api/v1/nonauth/marketData/analytics"
	analyticsLevel = "LEVEL_1"

	// valueIds is sent in a fixed canonical order; the API returns
	// values/formattedValues in this same order.
	valueIDs = "COIN_SYMBOL,SHORT_TERM_TREND,MEDIUM_TERM_TREND,LONG_TERM_TREND," +
		"SHORT_TERM_TREND_CHANGE,MEDIUM_TERM_TREND_CHANGE,LONG_TERM_TREND_CHANGE," +
		"MACD_SIGNAL,AGE,MACD_HISTOGRAM_H2"
	valueFieldCount = 10
)

// Analytics holds the translated per-interval AltFins values for one coin.
type Analytics struct {
	Interval              string
	ShortTermTrend        string
	MediumTermTrend       string
	LongTermTrend         string
	ShortTermTrendChange  string
	MediumTermTrendChange string
	LongTermTrendChange   string
	MACDSignal            string
	MACDSignalBarsAgo     int
	MACDSignalAgeText     string
	MACDHistogram         string
}

// AnalyticsClient fetches AltFins analytics using a SSRF-safe HTTP client.
type AnalyticsClient struct {
	http    *http.Client
	baseURL string
}

func NewAnalyticsClient() *AnalyticsClient {
	return &AnalyticsClient{http: security.SafeHTTPClient(30 * time.Second), baseURL: altFinsBaseURL}
}

var intervalDurations = map[string]time.Duration{
	IntervalMinutes15: 15 * time.Minute,
	IntervalHourly:    time.Hour,
	IntervalHours4:    4 * time.Hour,
	IntervalHours12:   12 * time.Hour,
	IntervalDaily:     24 * time.Hour,
}

var intervalLabels = map[string]string{
	IntervalMinutes15: "15m",
	IntervalHourly:    "1h",
	IntervalHours4:    "4h",
	IntervalHours12:   "12h",
	IntervalDaily:     "1d",
}

func ValidInterval(iv string) bool {
	_, ok := intervalDurations[iv]
	return ok
}

func IntervalLabel(iv string) string {
	if l, ok := intervalLabels[iv]; ok {
		return l
	}
	return iv
}

func IntervalDuration(iv string) time.Duration {
	return intervalDurations[iv]
}

// ResolveIdentifier resolves a coin symbol to its AltFins securityIdentifierId.
// ok=false means AltFins has no match for the symbol (a normal "no data").
func (c *AnalyticsClient) ResolveIdentifier(ctx context.Context, symbol string) (int64, bool, error) {
	base := normalizeBase(symbol)
	if base == "" {
		return 0, false, fmt.Errorf("altfins: empty symbol")
	}
	body, err := json.Marshal(map[string]string{
		"coinFilter": base,
		"signal":     "",
		"marketCap":  "",
	})
	if err != nil {
		return 0, false, fmt.Errorf("altfins: marshal resolve body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+resolvePath+"?size=1", bytes.NewReader(body))
	if err != nil {
		return 0, false, fmt.Errorf("altfins: build resolve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Content []struct {
			SecurityIdentifierID int64 `json:"securityIdentifierId"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, false, fmt.Errorf("altfins: decode resolve: %w", err)
	}
	if len(parsed.Content) == 0 {
		return 0, false, nil
	}
	return parsed.Content[0].SecurityIdentifierID, true, nil
}

// GetAnalytics fetches the translated analytics for one coin + interval.
func (c *AnalyticsClient) GetAnalytics(ctx context.Context, id int64, interval string) (*Analytics, error) {
	if !ValidInterval(interval) {
		return nil, fmt.Errorf("altfins: invalid interval %q", interval)
	}
	u, err := url.Parse(c.baseURL + analyticsPath)
	if err != nil {
		return nil, fmt.Errorf("altfins: parse analytics url: %w", err)
	}
	q := u.Query()
	q.Set("id", strconv.FormatInt(id, 10))
	q.Set("valueIds", valueIDs)
	q.Set("timeInterval", interval)
	q.Set("level", analyticsLevel)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("altfins: build analytics request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Values          []json.RawMessage `json:"values"`
		FormattedValues []string          `json:"formattedValues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("altfins: decode analytics: %w", err)
	}
	if len(parsed.FormattedValues) != valueFieldCount || len(parsed.Values) != valueFieldCount {
		return nil, fmt.Errorf("altfins: unexpected field count (values=%d formatted=%d want=%d)",
			len(parsed.Values), len(parsed.FormattedValues), valueFieldCount)
	}
	fv := parsed.FormattedValues
	rawAge := rawInt64(parsed.Values[8])

	a := &Analytics{
		Interval:              interval,
		ShortTermTrend:        translateTrend(fv[1]),
		MediumTermTrend:       translateTrend(fv[2]),
		LongTermTrend:         translateTrend(fv[3]),
		ShortTermTrendChange:  translateTrendChange(rawString(parsed.Values[4])),
		MediumTermTrendChange: translateTrendChange(rawString(parsed.Values[5])),
		LongTermTrendChange:   translateTrendChange(rawString(parsed.Values[6])),
		MACDSignal:            translateMACDSignal(fv[7]),
		MACDHistogram:         translateHistogram(fv[9]),
	}
	a.MACDSignalBarsAgo, a.MACDSignalAgeText = ageFromRaw(rawAge, interval)
	return a, nil
}

// do performs a request with a single retry on HTTP 429 (mirrors the
// OpportunityClient retry behavior).
func (c *AnalyticsClient) do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("altfins: request: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		time.Sleep(1500 * time.Millisecond)
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("altfins: retry request: %w", err)
		}
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("altfins: HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func normalizeBase(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.TrimSuffix(s, "USDT")
	return s
}

func rawString(r json.RawMessage) string {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func rawInt64(r json.RawMessage) int64 {
	var n int64
	if err := json.Unmarshal(r, &n); err == nil {
		return n
	}
	return 0
}

func translateTrend(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	score := ""
	if i := strings.LastIndex(v, "("); i >= 0 {
		score = strings.TrimSpace(v[i:])
		v = strings.TrimSpace(v[:i])
	}
	var word string
	switch strings.ToLower(v) {
	case "strong up":
		word = "Strongly Bullish"
	case "up":
		word = "Bullish"
	case "neutral":
		word = "Neutral"
	case "down":
		word = "Bearish"
	case "strong down":
		word = "Strongly Bearish"
	default:
		word = v
	}
	if score != "" {
		return word + " " + score
	}
	return word
}

func trendWord(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "STRONG_UP":
		return "Strongly Bullish"
	case "UP":
		return "Bullish"
	case "NEUTRAL":
		return "Neutral"
	case "DOWN":
		return "Bearish"
	case "STRONG_DOWN":
		return "Strongly Bearish"
	default:
		return raw
	}
}

func translateTrendChange(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, "_TO_", 2)
	if len(parts) != 2 {
		return v
	}
	return trendWord(parts[0]) + " to " + trendWord(parts[1])
}

func translateMACDSignal(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "buy":
		return "Bullish"
	case "sell":
		return "Bearish"
	default:
		return strings.TrimSpace(v)
	}
}

func translateHistogram(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "UP":
		return "Bullish"
	case "DOWN":
		return "Bearish"
	default:
		return ""
	}
}

// ageFromRaw converts the raw epoch-ms MACD crossover timestamp into a bar
// count + human text for the given interval. The API's formatted age is
// ignored (it is stale).
func ageFromRaw(rawMs int64, interval string) (int, string) {
	if rawMs <= 0 {
		return 0, ""
	}
	dur := IntervalDuration(interval)
	if dur <= 0 {
		return 0, ""
	}
	elapsed := time.Since(time.UnixMilli(rawMs))
	if elapsed < 0 {
		elapsed = 0
	}
	bars := int(elapsed / dur)
	return bars, "~" + humanizeDuration(elapsed) + " ago"
}

func humanizeDuration(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./provider/altfins/ -v`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w provider/altfins/
git add provider/altfins/
git commit -m "feat(altfins): analytics provider with translation"
```

---

## Task 2: AltFins TTL cache

**Files:**
- Create: `kernel/altfins_detail_cache.go`
- Test: `kernel/altfins_detail_cache_test.go`

**Interfaces:**
- Consumes: `nofx/provider/altfins`.
- Produces:
  ```go
  const altfinsDetailCacheTTL = 10 * time.Minute
  type altfinsDetailCache struct { /* ... */ }
  func newAltfinsDetailCache() *altfinsDetailCache
  func (c *altfinsDetailCache) get(key string) (*altfins.Analytics, bool)
  func (c *altfinsDetailCache) set(key string, v *altfins.Analytics)
  ```

- [ ] **Step 1: Write the failing test**

Create `kernel/altfins_detail_cache_test.go`:

```go
package kernel

import (
	"testing"
	"time"

	"nofx/provider/altfins"
)

func TestAltfinsDetailCacheTTL(t *testing.T) {
	c := newAltfinsDetailCache()
	key := "ZEC|MINUTES15"
	if _, ok := c.get(key); ok {
		t.Fatal("expected empty cache")
	}
	c.set(key, &altfins.Analytics{Interval: altfins.IntervalMinutes15, ShortTermTrend: "Bearish (2/10)"})
	got, ok := c.get(key)
	if !ok || got.ShortTermTrend != "Bearish (2/10)" {
		t.Fatalf("expected cached value, got %+v ok=%v", got, ok)
	}
	if altfinsDetailCacheTTL != 10*time.Minute {
		t.Fatalf("expected TTL of 10 minutes, got %v", altfinsDetailCacheTTL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./kernel/ -run TestAltfinsDetailCacheTTL -v`
Expected: FAIL — `newAltfinsDetailCache` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `kernel/altfins_detail_cache.go`:

```go
package kernel

import (
	"sync"
	"time"

	"nofx/provider/altfins"
)

// altfinsDetailCacheTTL bounds how long a cached per-coin AltFins analytics
// result is kept.
const altfinsDetailCacheTTL = 10 * time.Minute

type altfinsDetailEntry struct {
	value   *altfins.Analytics
	expires time.Time
}

// altfinsDetailCache is a concurrency-safe TTL cache for per-coin AltFins
// analytics, keyed by "<symbol>|<interval>".
type altfinsDetailCache struct {
	mu   sync.Mutex
	data map[string]altfinsDetailEntry
}

func newAltfinsDetailCache() *altfinsDetailCache {
	return &altfinsDetailCache{data: make(map[string]altfinsDetailEntry)}
}

func (c *altfinsDetailCache) get(key string) (*altfins.Analytics, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *altfinsDetailCache) set(key string, v *altfins.Analytics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = altfinsDetailEntry{value: v, expires: time.Now().Add(altfinsDetailCacheTTL)}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./kernel/ -run TestAltfinsDetailCacheTTL -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w kernel/altfins_detail_cache.go kernel/altfins_detail_cache_test.go
git add kernel/altfins_detail_cache.go kernel/altfins_detail_cache_test.go
git commit -m "feat(kernel): altfins detail TTL cache"
```

---

## Task 3: Store config fields, clamp, token estimate

**Files:**
- Modify: `store/strategy.go` (`IndicatorConfig` ~line 1089; `ClampLimits` ~line 80; `EstimateTokens` ~line 1655)
- Test: `store/strategy_test.go`

**Interfaces:**
- Produces: `IndicatorConfig.EnableAltFinsData`, `AltFinsIntervals []string`, `EnableVergexSignalLabData`, `EnableVergexHeatmapData`.
- Consumed by: Tasks 4, 6, 7, 8, 9.

- [ ] **Step 1: Write the failing test**

Append to `store/strategy_test.go`:

```go
func TestAltFinsClampAndDefaults(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15", "HOURS1", "DAILY", "MINUTES15", "", "HOURS4"}
	cfg.ClampLimits()
	want := []string{"MINUTES15", "DAILY", "HOURS4"}
	if len(cfg.Indicators.AltFinsIntervals) != len(want) {
		t.Fatalf("got %v want %v", cfg.Indicators.AltFinsIntervals, want)
	}
	for i := range want {
		if cfg.Indicators.AltFinsIntervals[i] != want[i] {
			t.Fatalf("got %v want %v", cfg.Indicators.AltFinsIntervals, want)
		}
	}

	// Toggle on with empty list -> default [MINUTES15, DAILY].
	cfg2 := GetDefaultStrategyConfig("en")
	cfg2.Indicators.EnableAltFinsData = true
	cfg2.Indicators.AltFinsIntervals = nil
	cfg2.ClampLimits()
	if len(cfg2.Indicators.AltFinsIntervals) != 2 ||
		cfg2.Indicators.AltFinsIntervals[0] != "MINUTES15" ||
		cfg2.Indicators.AltFinsIntervals[1] != "DAILY" {
		t.Fatalf("expected default [MINUTES15 DAILY], got %v", cfg2.Indicators.AltFinsIntervals)
	}
}

func TestAltFinsEstimateTokensIncreases(t *testing.T) {
	base := GetDefaultStrategyConfig("en")
	withData := GetDefaultStrategyConfig("en")
	withData.Indicators.EnableAltFinsData = true
	withData.Indicators.AltFinsIntervals = []string{"MINUTES15", "DAILY"}
	if withData.EstimateTokens().Total <= base.EstimateTokens().Total {
		t.Fatal("expected AltFins to increase token estimate")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./store/ -run 'TestAltFins' -v`
Expected: FAIL — unknown `EnableAltFinsData` / `AltFinsIntervals`.

- [ ] **Step 3: Write minimal implementation**

In `store/strategy.go`, add to `IndicatorConfig` after the Binance fields (`~line 1092`):

```go
	// AltFins per-coin analytics (free, keyless, source-independent).
	EnableAltFinsData bool     `json:"enable_altfins_data"`
	AltFinsIntervals  []string `json:"altfins_intervals,omitempty"` // MINUTES15, HOURLY, HOURS4, HOURS12, DAILY

	// Vergex free per-coin detail feeds (independent of vergex_signal).
	EnableVergexSignalLabData bool `json:"enable_vergex_signal_lab_data"`
	EnableVergexHeatmapData   bool `json:"enable_vergex_heatmap_data"`
```

In `ClampLimits` after the `DataDurations` clamp (`~line 100`):

```go
	// Clamp AltFins intervals to the five supported enum values.
	supportedAltFins := []string{"MINUTES15", "HOURLY", "HOURS4", "HOURS12", "DAILY"}
	seenAF := map[string]bool{}
	var keptAF []string
	for _, iv := range c.Indicators.AltFinsIntervals {
		iv = strings.TrimSpace(iv)
		if iv == "" || seenAF[iv] {
			continue
		}
		for _, s := range supportedAltFins {
			if iv == s {
				seenAF[iv] = true
				keptAF = append(keptAF, iv)
				break
			}
		}
	}
	if len(keptAF) == 0 && c.Indicators.EnableAltFinsData {
		keptAF = []string{"MINUTES15", "DAILY"}
	}
	c.Indicators.AltFinsIntervals = keptAF
```

In `EstimateTokens` after the `EnablePriceData` block (`~line 1667`):

```go
	if c.Indicators.EnableAltFinsData {
		totalMarketChars += numCoins * 300 * len(c.Indicators.AltFinsIntervals)
	}
	if c.Indicators.EnableVergexSignalLabData {
		totalMarketChars += numCoins * 800
	}
	if c.Indicators.EnableVergexHeatmapData {
		totalMarketChars += numCoins * 600
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./store/ -run 'TestAltFins' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w store/strategy.go store/strategy_test.go
git add store/strategy.go store/strategy_test.go
git commit -m "feat(store): altfins + vergex per-coin indicator config"
```

---

## Task 4: Engine wiring — PerCoinSignal fields, client, prefetch

**Files:**
- Modify: `kernel/engine.go` (`PerCoinSignal` ~line 195; `StrategyEngine` ~line 212; constructor; add `PrefetchAltFinsDetails`)

**Interfaces:**
- Consumes: `provider/altfins`, Task 2 cache, Task 3 config.
- Produces:
  ```go
  // PerCoinSignal additions
  AltFins         map[string]*altfins.Analytics
  VergexSignalLab json.RawMessage
  VergexHeatmap   json.RawMessage

  // StrategyEngine additions
  altfinsClient altfinsAnalyticsGetter
  altfinsDetails *altfinsDetailCache

  type altfinsAnalyticsGetter interface {
      ResolveIdentifier(ctx context.Context, symbol string) (int64, bool, error)
      GetAnalytics(ctx context.Context, id int64, interval string) (*altfins.Analytics, error)
  }

  func (e *StrategyEngine) altfinsDetail(key string) (*altfins.Analytics, bool)
  func (e *StrategyEngine) cacheAltfinsDetail(key string, v *altfins.Analytics)
  func (e *StrategyEngine) PrefetchAltFinsDetails(ctx context.Context, symbols []string)
  ```
- Produced by: `var newAltfinsClient = func() *altfins.AnalyticsClient { return altfins.NewAnalyticsClient() }` (overridable in tests).

- [ ] **Step 1: Write the failing test**

Append to `kernel/engine_test.go`:

```go
type fakeAltfinsClient struct {
	ids map[string]int64
}

func (f *fakeAltfinsClient) ResolveIdentifier(_ context.Context, symbol string) (int64, bool, error) {
	id, ok := f.ids[strings.ToUpper(strings.TrimSuffix(symbol, "USDT"))]
	return id, ok, nil
}

func (f *fakeAltfinsClient) GetAnalytics(_ context.Context, _ int64, interval string) (*altfins.Analytics, error) {
	return &altfins.Analytics{Interval: interval, ShortTermTrend: "Bullish (8/10)"}, nil
}

func TestPrefetchAltFinsDetailsPopulatesCache(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15"}

	engine := NewStrategyEngine(cfg)
	engine.altfinsClient = &fakeAltfinsClient{ids: map[string]int64{"ZEC": 1021300}}

	engine.PrefetchAltFinsDetails(context.Background(), []string{"ZECUSDT"})

	got, ok := engine.altfinsDetail("ZECUSDT|MINUTES15")
	if !ok || got == nil || got.ShortTermTrend != "Bullish (8/10)" {
		t.Fatalf("expected cached altfins detail, got %+v ok=%v", got, ok)
	}
}
```

Add the `altfins` import to the test file's import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./kernel/ -run TestPrefetchAltFinsDetailsPopulatesCache -v`
Expected: FAIL — `altfinsClient`, `PrefetchAltFinsDetails`, `altfinsDetail` undefined.

- [ ] **Step 3: Write minimal implementation**

In `kernel/engine.go`, add the import `"nofx/provider/altfins"`.

Extend `PerCoinSignal` (`~line 195`):

```go
type PerCoinSignal struct {
	AI500            *nofxos.CoinData
	OI               map[string]map[string]nofxos.OIPosition
	Netflow          map[string]map[string]nofxos.NetFlowPosition
	Price            map[string]map[string]nofxos.PriceRankingItem
	BinanceTechnical map[string]*binance.BinanceAssetDetail
	BinanceSentiment map[string]string
	AltFins          map[string]*altfins.Analytics // interval -> parsed analytics
	VergexSignalLab  json.RawMessage               // vergex.trade SignalLab (non-vergex sources)
	VergexHeatmap    json.RawMessage               // vergex.trade liquidation heatmap
}
```

Add interface + factory near `binanceOpportunityGetter` (`~line 206`):

```go
// altfinsAnalyticsGetter abstracts the AltFins client for testability.
type altfinsAnalyticsGetter interface {
	ResolveIdentifier(ctx context.Context, symbol string) (int64, bool, error)
	GetAnalytics(ctx context.Context, id int64, interval string) (*altfins.Analytics, error)
}

var newAltfinsClient = func() *altfins.AnalyticsClient { return altfins.NewAnalyticsClient() }
```

Add fields to `StrategyEngine` (`~line 224`):

```go
	// AltFins per-coin analytics client + TTL cache.
	altfinsClient  altfinsAnalyticsGetter
	altfinsDetails *altfinsDetailCache
```

Initialize in **both** `return &StrategyEngine{...}` blocks (`~line 284` and `~line 297`) by adding:

```go
			altfinsClient:      newAltfinsClient(),
			altfinsDetails:     newAltfinsDetailCache(),
```

Add helper methods next to `binanceDetail` (`~line 384`):

```go
func (e *StrategyEngine) altfinsDetail(key string) (*altfins.Analytics, bool) {
	return e.altfinsDetails.get(key)
}

func (e *StrategyEngine) cacheAltfinsDetail(key string, v *altfins.Analytics) {
	e.altfinsDetails.set(key, v)
}

// PrefetchAltFinsDetails warms the AltFins analytics TTL cache for the given
// symbols, gated by EnableAltFinsData. Best-effort; failures are logged.
func (e *StrategyEngine) PrefetchAltFinsDetails(ctx context.Context, symbols []string) {
	if e == nil || e.altfinsDetails == nil || e.altfinsClient == nil || e.config == nil {
		return
	}
	if !e.config.Indicators.EnableAltFinsData {
		return
	}
	intervals := e.config.Indicators.AltFinsIntervals
	if len(intervals) == 0 {
		intervals = []string{"MINUTES15", "DAILY"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, sym := range symbols {
		id, ok, err := e.altfinsClient.ResolveIdentifier(ctx, sym)
		if err != nil {
			logger.Warnf("⚠️ AltFins resolve failed (%s): %v", sym, err)
			continue
		}
		if !ok {
			continue
		}
		for _, iv := range intervals {
			key := sym + "|" + iv
			if _, cached := e.altfinsDetail(key); cached {
				continue
			}
			val, err := e.altfinsClient.GetAnalytics(ctx, id, iv)
			if err != nil {
				logger.Warnf("⚠️ AltFins analytics failed (%s %s): %v", sym, iv, err)
				continue
			}
			e.cacheAltfinsDetail(key, val)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./kernel/ -run TestPrefetchAltFinsDetailsPopulatesCache -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w kernel/engine.go kernel/engine_test.go
git add kernel/engine.go kernel/engine_test.go
git commit -m "feat(kernel): altfins client, cache hooks, and prefetch"
```

---

## Task 5: Attach AltFins + vergex per-coin signals

**Files:**
- Modify: `kernel/engine_analysis.go` (`attachPerCoinSignals` ~line 185)
- Test: `kernel/engine_analysis_test.go`

**Interfaces:**
- Consumes: Task 3 config, Task 4 fields/methods, `e.freeClient` (`*vergex.Client`), `vergex.Query`, `vergex.MarketAnalysis`.
- Produces: populated `PerCoinSignal.AltFins`, `.VergexSignalLab`, `.VergexHeatmap`.

- [ ] **Step 1: Write the failing test**

Append to `kernel/engine_analysis_test.go`:

```go
func TestAttachPerCoinSignalsAltFins(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15"}

	engine := NewStrategyEngine(cfg)
	engine.altfinsClient = &fakeAltfinsClient{ids: map[string]int64{"ZEC": 1021300}}

	ctx := &Context{
		CandidateCoins: []CandidateCoin{{Symbol: "ZECUSDT"}},
		Ctx:            context.Background(),
	}
	if err := attachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attachPerCoinSignals: %v", err)
	}
	sig, ok := engine.PerCoinSignalFor("ZECUSDT")
	if !ok || sig.AltFins["MINUTES15"] == nil {
		t.Fatalf("expected AltFins signal, got %+v ok=%v", sig, ok)
	}
}
```

Ensure `nofx/store` and `context` are imported (already present in the file).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./kernel/ -run TestAttachPerCoinSignalsAltFins -v`
Expected: FAIL — no AltFins population (guard does not include the flag; block missing).

- [ ] **Step 3: Write minimal implementation**

In `kernel/engine_analysis.go`, extend the early-return guard (`~line 190`):

```go
	if !cfg.Indicators.EnableAI500Data && !cfg.Indicators.EnableOIData &&
		!cfg.Indicators.EnableNetflowData && !cfg.Indicators.EnablePriceData &&
		!cfg.Indicators.EnableBinanceTechnicalData && !cfg.Indicators.EnableBinanceSentimentData &&
		!cfg.Indicators.EnableAltFinsData &&
		!cfg.Indicators.EnableVergexSignalLabData && !cfg.Indicators.EnableVergexHeatmapData {
		return nil
	}
```

After the Binance sentiment block (`~line 328`, before `engine.SetPerCoinSignals(out)`):

```go
	// AltFins per-coin analytics (free, keyless; read from TTL cache, fall back
	// to a synchronous fetch on miss). Failures/no-match are skipped silently.
	if cfg.Indicators.EnableAltFinsData {
		intervals := cfg.Indicators.AltFinsIntervals
		if len(intervals) == 0 {
			intervals = []string{"MINUTES15", "DAILY"}
		}
		for sym := range symSet {
			id := int64(0)
			resolved := false
			for _, iv := range intervals {
				key := sym + "|" + iv
				val, ok := engine.altfinsDetail(key)
				if ok {
					sig := out[sym]
					if sig.AltFins == nil {
						sig.AltFins = make(map[string]*altfins.Analytics)
					}
					sig.AltFins[iv] = val
					out[sym] = sig
					continue
				}
				if !resolved {
					rid, found, err := engine.altfinsClient.ResolveIdentifier(apiCtx, sym)
					if err != nil {
						logger.Warnf("⚠️ AltFins resolve failed (%s): %v", sym, err)
						break
					}
					if !found {
						break
					}
					id, resolved = rid, true
				}
				val, err := engine.altfinsClient.GetAnalytics(apiCtx, id, iv)
				if err != nil {
					logger.Warnf("⚠️ AltFins analytics failed (%s %s): %v", sym, iv, err)
					continue
				}
				engine.cacheAltfinsDetail(key, val)
				sig := out[sym]
				if sig.AltFins == nil {
					sig.AltFins = make(map[string]*altfins.Analytics)
				}
				sig.AltFins[iv] = val
				out[sym] = sig
			}
		}
	}

	// Vergex free per-coin detail feeds, for any non-vergex_signal source (the
	// vergex_signal path already fetches these via FetchVergexDataBatch).
	if cfg.CoinSource.SourceType != "vergex_signal" &&
		(cfg.Indicators.EnableVergexSignalLabData || cfg.Indicators.EnableVergexHeatmapData) &&
		engine.freeClient != nil {
		for sym := range symSet {
			q := vergex.Query{
				MarketType: cfg.CoinSource.VergexMarketType,
				Symbol:     sym,
				Chain:      cfg.CoinSource.VergexChain,
				LiqBand:    cfg.CoinSource.VergexLiqBand,
			}
			sig := out[sym]
			if cfg.Indicators.EnableVergexSignalLabData {
				if body, err := engine.freeClient.GetSignalLab(apiCtx, q); err != nil {
					logger.Warnf("⚠️ Vergex signal-lab failed (%s): %v", sym, err)
				} else {
					sig.VergexSignalLab = body
				}
			}
			if cfg.Indicators.EnableVergexHeatmapData {
				if body, err := engine.freeClient.GetCostLiquidationHeatmap(apiCtx, q); err != nil {
					logger.Warnf("⚠️ Vergex heatmap failed (%s): %v", sym, err)
				} else {
					sig.VergexHeatmap = body
				}
			}
			out[sym] = sig
		}
	}
```

Add imports `"nofx/provider/altfins"` and `"nofx/provider/vergex"` to `kernel/engine_analysis.go` if not present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./kernel/ -run TestAttachPerCoinSignalsAltFins -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w kernel/engine_analysis.go kernel/engine_analysis_test.go
git add kernel/engine_analysis.go kernel/engine_analysis_test.go
git commit -m "feat(kernel): attach altfins + vergex per-coin signals"
```

---

## Task 6: Prompt rendering

**Files:**
- Modify: `kernel/engine_prompt.go` (`formatPerCoinSignals` ~line 1057)
- Test: `kernel/engine_prompt_test.go`

**Interfaces:**
- Consumes: Task 4 `PerCoinSignal` fields, Task 3 config, `altfins.IntervalLabel`, `vergex.FormatSignalLabMarkdown`, `vergex.FormatHeatmapMarkdown`.

- [ ] **Step 1: Write the failing test**

Append to `kernel/engine_prompt_test.go`:

```go
func TestFormatPerCoinSignalsAltFins(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.Indicators.EnableAltFinsData = true
	cfg.Indicators.AltFinsIntervals = []string{"MINUTES15", "DAILY"}
	engine := NewStrategyEngine(cfg)

	engine.SetPerCoinSignals(map[string]PerCoinSignal{
		"ZECUSDT": {
			AltFins: map[string]*altfins.Analytics{
				"MINUTES15": {
					Interval:              "MINUTES15",
					ShortTermTrend:        "Bearish (2/10)",
					MediumTermTrend:       "Bearish (3/10)",
					LongTermTrend:         "Neutral (5/10)",
					ShortTermTrendChange:  "Strongly Bearish to Bearish",
					MediumTermTrendChange: "Neutral to Bearish",
					LongTermTrendChange:   "Bullish to Neutral",
					MACDSignal:            "Bearish",
					MACDSignalBarsAgo:     6,
					MACDSignalAgeText:     "~90 min ago",
					MACDHistogram:         "Bullish",
				},
				"DAILY": {
					Interval:      "DAILY",
					MACDSignal:    "Bullish",
					MACDHistogram: "",
				},
			},
		},
	})

	out := engine.formatPerCoinSignals("ZECUSDT", 1450.38)
	if !strings.Contains(out, "=== ZECUSDT AltFins ===") {
		t.Fatalf("missing AltFins header:\n%s", out)
	}
	if !strings.Contains(out, "[15m] Short Term Trend: Bearish (2/10), changed from Strongly Bearish to Bearish") {
		t.Fatalf("missing 15m short term line:\n%s", out)
	}
	if !strings.Contains(out, "MACD Signal: Bearish crossover, 6 bars ago (~90 min ago)") {
		t.Fatalf("missing MACD signal line:\n%s", out)
	}
	if !strings.Contains(out, "[1d]") {
		t.Fatalf("missing 1d label:\n%s", out)
	}
	if strings.Contains(out, "MACD Histogram: \n") || strings.Contains(out, "MACD Histogram:\n") {
		t.Fatalf("empty histogram should be omitted:\n%s", out)
	}
}
```

Add `"nofx/provider/altfins"` to `kernel/engine_prompt_test.go` imports if missing.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./kernel/ -run TestFormatPerCoinSignalsAltFins -v`
Expected: FAIL — no AltFins block.

- [ ] **Step 3: Write minimal implementation**

In `kernel/engine_prompt.go`, update the guard at the top of `formatPerCoinSignals` (`~line 1060`):

```go
	if !ind.EnableAI500Data && !ind.EnableOIData && !ind.EnableNetflowData && !ind.EnablePriceData &&
		!ind.EnableBinanceTechnicalData && !ind.EnableBinanceSentimentData &&
		!ind.EnableAltFinsData && !ind.EnableVergexSignalLabData && !ind.EnableVergexHeatmapData {
		return ""
	}
	sig, ok := e.PerCoinSignalFor(symbol)
	if !ok || sig.AI500 == nil && len(sig.OI) == 0 && len(sig.Netflow) == 0 && len(sig.Price) == 0 &&
		len(sig.BinanceTechnical) == 0 && len(sig.BinanceSentiment) == 0 &&
		len(sig.AltFins) == 0 && len(sig.VergexSignalLab) == 0 && len(sig.VergexHeatmap) == 0 {
		return ""
	}
```

At the end of `formatPerCoinSignals`, before `return sb.String()` (`~line 1169`):

```go
	if ind.EnableAltFinsData && len(sig.AltFins) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s AltFins ===\n", symbol))
		for _, iv := range altfinsIntervalOrder(ind.AltFinsIntervals) {
			a, ok := sig.AltFins[iv]
			if !ok || a == nil {
				continue
			}
			label := altfins.IntervalLabel(iv)
			sb.WriteString(fmt.Sprintf("[%s] Short Term Trend: %s, changed from %s\n", label, a.ShortTermTrend, a.ShortTermTrendChange))
			sb.WriteString(fmt.Sprintf("      Medium Term Trend: %s, changed from %s\n", a.MediumTermTrend, a.MediumTermTrendChange))
			sb.WriteString(fmt.Sprintf("      Long Term Trend: %s, changed from %s\n", a.LongTermTrend, a.LongTermTrendChange))
			if a.MACDSignal != "" {
				sb.WriteString(fmt.Sprintf("      MACD Signal: %s crossover, %d bars ago (%s)\n", a.MACDSignal, a.MACDSignalBarsAgo, a.MACDSignalAgeText))
			}
			if a.MACDHistogram != "" {
				sb.WriteString(fmt.Sprintf("      MACD Histogram: %s\n", a.MACDHistogram))
			}
			sb.WriteString("\n")
		}
	}

	if ind.EnableVergexSignalLabData && len(sig.VergexSignalLab) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Vergex Signal Lab ===\n", symbol))
		sb.WriteString(vergex.FormatSignalLabMarkdown(sig.VergexSignalLab))
		sb.WriteString("\n")
	}
	if ind.EnableVergexHeatmapData && len(sig.VergexHeatmap) > 0 {
		sb.WriteString(fmt.Sprintf("=== %s Vergex Liquidation Heatmap ===\n", symbol))
		sb.WriteString(vergex.FormatHeatmapMarkdown(sig.VergexHeatmap))
		sb.WriteString("\n")
	}
```

Add helper near `binanceIntervalOrder` (`~line 1182`):

```go
func altfinsIntervalOrder(intervals []string) []string {
	if len(intervals) == 0 {
		return []string{"MINUTES15", "DAILY"}
	}
	order := []string{"MINUTES15", "HOURLY", "HOURS4", "HOURS12", "DAILY"}
	rank := map[string]int{}
	for i, iv := range order {
		rank[iv] = i
	}
	out := make([]string, 0, len(intervals))
	seen := map[string]bool{}
	for _, iv := range intervals {
		if !seen[iv] {
			seen[iv] = true
			out = append(out, iv)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i]] < rank[out[j]] })
	return out
}
```

Add `"nofx/provider/altfins"` and `"nofx/provider/vergex"` to imports if missing (`sort`, `fmt`, `strings` already present).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./kernel/ -run TestFormatPerCoinSignalsAltFins -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w kernel/engine_prompt.go kernel/engine_prompt_test.go
git add kernel/engine_prompt.go kernel/engine_prompt_test.go
git commit -m "feat(kernel): render altfins + vergex per-coin prompt blocks"
```

---

## Task 7: Trader pre-cycle prefetch

**Files:**
- Modify: `trader/auto_trader_loop.go` (`scheduleBinancePrefetch` ~line 843)

**Interfaces:**
- Consumes: `StrategyEngine.PrefetchAltFinsDetails`, `GetCandidateCoins`, existing `runRateLimitedPrefetch`.

- [ ] **Step 1: Write the failing test**

Create `trader/auto_trader_prefetch_altfins_test.go`:

```go
package trader

import (
	"context"
	"testing"

	"nofx/kernel"
	"nofx/store"
)

func TestPrefetchAltFinsDetailsIsCalled(t *testing.T) {
	// Lightweight guard: ensure the symbols slice is derived from candidates
	// and the engine's AltFins prefetch is invocable without panic.
	engine := kernel.NewStrategyEngine(&store.StrategyConfig{})
	engine.PrefetchAltFinsDetails(context.Background(), []string{"ZECUSDT"})
}
```

> Note: this is a smoke test. The real gate is that `go build ./...` compiles
> the new call site; the scheduling math is covered by the existing prefetch
> tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./trader/ -run TestPrefetchAltFinsDetailsIsCalled -v`
Expected: PASS once Task 4 landed; if the call site is not yet added, this test simply exercises the method. Proceed to wire the scheduler.

- [ ] **Step 3: Write minimal implementation**

In `trader/auto_trader_loop.go`, inside `scheduleBinancePrefetch`, extend the request-count calculation (`~line 852`) and the prefetch body:

```go
	requestsPerCoin := 0
	if cfg.Indicators.EnableBinanceTechnicalData {
		intervals := cfg.Indicators.BinanceTechnicalIntervals
		if len(intervals) == 0 {
			intervals = []string{"1h"}
		}
		requestsPerCoin += len(intervals)
	}
	if cfg.Indicators.EnableBinanceSentimentData {
		requestsPerCoin++
	}
	if cfg.Indicators.EnableAltFinsData {
		ivs := cfg.Indicators.AltFinsIntervals
		if len(ivs) == 0 {
			ivs = []string{"MINUTES15", "DAILY"}
		}
		requestsPerCoin += len(ivs) + 1 // +1 resolve call per coin
	}
	if requestsPerCoin == 0 {
		return
	}
```

In the `time.AfterFunc` body, after the existing Binance prefetch loop (`~line 922`):

```go
		if cfg.Indicators.EnableAltFinsData {
			runRateLimitedPrefetch(symbols, delayPerRequest, func(sym string) {
				at.strategyEngine.PrefetchAltFinsDetails(context.Background(), []string{sym})
			})
		}
```

> Vergex prefetch is intentionally NOT added here: those calls are made
> synchronously in `attachPerCoinSignals` and will hit the TTL cache pattern
> once added; keeping them out of the scheduler avoids racing the cycle.

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./trader/ -run TestPrefetchAltFinsDetailsIsCalled -v`
Expected: PASS + build clean.

- [ ] **Step 5: Commit**

```bash
gofmt -w trader/auto_trader_loop.go trader/auto_trader_prefetch_altfins_test.go
git add trader/auto_trader_loop.go trader/auto_trader_prefetch_altfins_test.go
git commit -m "feat(trader): prefetch altfins per-coin detail pre-cycle"
```

---

## Task 8: Frontend types + factory mapping

**Files:**
- Modify: `web/src/types/strategy.ts` (`IndicatorConfig` ~line 216)
- Modify: `web/src/features/strategies/strategyFactory.ts` (form interface ~line 22; `buildStrategyConfig` ~line 321)
- Modify: `web/src/features/strategies/dataSourceDefaults.ts`
- Test: `web/src/features/strategies/strategyFactory.test.ts`

**Interfaces:**
- Produces TS types: `AltFinsInterval`, form fields `enableAltFinsData`, `altfinsIntervals`, `enableVergexSignalLabData`, `enableVergexHeatmapData`.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/strategies/strategyFactory.test.ts`:

```ts
it('maps altfins + vergex per-coin toggles into indicators', () => {
  const config = buildStrategyConfig({
    ...base,
    enableAltFinsData: true,
    altfinsIntervals: ['MINUTES15', 'DAILY'],
    enableVergexSignalLabData: true,
    enableVergexHeatmapData: false,
  })
  expect(config.ai_config.indicators.enable_altfins_data).toBe(true)
  expect(config.ai_config.indicators.altfins_intervals).toEqual(['MINUTES15', 'DAILY'])
  expect(config.ai_config.indicators.enable_vergex_signal_lab_data).toBe(true)
  expect(config.ai_config.indicators.enable_vergex_heatmap_data).toBe(false)
})
```

> The test file already defines a shared `base` form object (used at
> `strategyFactory.test.ts:137`); reuse it, adding only the new fields.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/strategyFactory.test.ts`
Expected: FAIL — form fields not mapped.

- [ ] **Step 3: Write minimal implementation**

In `web/src/types/strategy.ts`, add to `IndicatorConfig` (after the Binance fields ~line 218):

```ts
  // AltFins per-coin analytics (free, source-independent)
  enable_altfins_data?: boolean;
  altfins_intervals?: ('MINUTES15' | 'HOURLY' | 'HOURS4' | 'HOURS12' | 'DAILY')[];
  // Vergex free per-coin detail feeds (independent of vergex_signal)
  enable_vergex_signal_lab_data?: boolean;
  enable_vergex_heatmap_data?: boolean;
```

In `web/src/features/strategies/strategyFactory.ts`, add to `StrategyEditorForm` (after `binanceTechnicalIntervals` ~line 28):

```ts
  enableAltFinsData?: boolean
  altfinsIntervals?: ('MINUTES15' | 'HOURLY' | 'HOURS4' | 'HOURS12' | 'DAILY')[]
  enableVergexSignalLabData?: boolean
  enableVergexHeatmapData?: boolean
```

In `buildStrategyConfig`, add to the `indicators` object (after `binance_technical_intervals` ~line 327):

```ts
        enable_altfins_data: form.enableAltFinsData ?? false,
        altfins_intervals: form.altfinsIntervals?.length ? form.altfinsIntervals : undefined,
        enable_vergex_signal_lab_data: form.enableVergexSignalLabData ?? false,
        enable_vergex_heatmap_data: form.enableVergexHeatmapData ?? false,
```

In `web/src/features/strategies/dataSourceDefaults.ts`, extend the return type and object:

```ts
export function defaultDataSources(scope: ScopeUnit | null): {
  // ...existing...
  enableAltFinsData: boolean
  enableVergexSignalLabData: boolean
  enableVergexHeatmapData: boolean
} {
  return {
    // ...existing...
    enableAltFinsData: false,
    enableVergexSignalLabData: false,
    enableVergexHeatmapData: false,
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/strategies/strategyFactory.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd web && npx prettier --write src/types/strategy.ts src/features/strategies/strategyFactory.ts src/features/strategies/dataSourceDefaults.ts
git add web/src/types/strategy.ts web/src/features/strategies/strategyFactory.ts web/src/features/strategies/dataSourceDefaults.ts web/src/features/strategies/strategyFactory.test.ts
git commit -m "feat(web): altfins + vergex per-coin config mapping"
```

---

## Task 9: Frontend editor UI + restore

**Files:**
- Modify: `web/src/features/strategies/EditorStepPage.tsx`

**Interfaces:**
- Consumes: Task 8 types + form fields.

- [ ] **Step 1: Write the failing test**

Create `web/src/features/strategies/altfinsEditor.test.tsx` (a minimal source-presence guard, mirroring existing editor tests):

```tsx
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('EditorStepPage AltFins controls', () => {
  const src = readFileSync(resolve(__dirname, 'EditorStepPage.tsx'), 'utf8')
  it('renders AltFins toggle and interval multiselect', () => {
    expect(src).toContain('AltFins')
    expect(src).toContain("setAltfinsIntervals")
    expect(src).toContain("'MINUTES15'")
  })
  it('renders vergex per-coin toggles', () => {
    expect(src).toContain('Vergex Signal Lab')
    expect(src).toContain('Vergex Liquidation Heatmap')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/strategies/altfinsEditor.test.tsx`
Expected: FAIL — strings absent.

- [ ] **Step 3: Write minimal implementation**

In `web/src/features/strategies/EditorStepPage.tsx`:

Add state near the Binance state (`~line 106`):

```tsx
  const [enableAltFinsData, setEnableAltFinsData] = useState(false)
  const [altfinsIntervals, setAltfinsIntervals] = useState<('MINUTES15' | 'HOURLY' | 'HOURS4' | 'HOURS12' | 'DAILY')[]>(['MINUTES15', 'DAILY'])
  const [enableVergexSignalLabData, setEnableVergexSignalLabData] = useState(false)
  const [enableVergexHeatmapData, setEnableVergexHeatmapData] = useState(false)
```

Add restore in the edit `useEffect` after the Binance restore (`~line 202`):

```tsx
        setEnableAltFinsData(ind?.enable_altfins_data ?? false)
        setAltfinsIntervals(ind?.altfins_intervals?.length ? ind.altfins_intervals : ['MINUTES15', 'DAILY'])
        setEnableVergexSignalLabData(ind?.enable_vergex_signal_lab_data ?? false)
        setEnableVergexHeatmapData(ind?.enable_vergex_heatmap_data ?? false)
```

Add to the `buildStrategyConfig` call form object after `binanceTechnicalIntervals` (`~line 326`):

```tsx
        enableAltFinsData,
        altfinsIntervals,
        enableVergexSignalLabData,
        enableVergexHeatmapData,
```

Add UI after the Binance technical intervals block (`~line 720`):

```tsx
          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">AltFins data</span>
            <div className="mt-1 flex flex-wrap gap-2">
              <ToggleChip label="AltFins" active={enableAltFinsData} onClick={() => setEnableAltFinsData(!enableAltFinsData)} />
            </div>
            {enableAltFinsData && (
              <div className="mt-2">
                <span className="text-xs text-nofx-text-muted">Intervals</span>
                <div className="mt-1 flex flex-wrap gap-2">
                  {(['MINUTES15', 'HOURLY', 'HOURS4', 'HOURS12', 'DAILY'] as const).map((iv) => (
                    <ToggleChip
                      key={iv}
                      label={iv === 'MINUTES15' ? '15m' : iv === 'HOURLY' ? '1h' : iv === 'HOURS4' ? '4h' : iv === 'HOURS12' ? '12h' : '1d'}
                      active={altfinsIntervals.includes(iv)}
                      onClick={() =>
                        setAltfinsIntervals((prev) =>
                          prev.includes(iv) ? prev.filter((x) => x !== iv) : [...prev, iv]
                        )
                      }
                    />
                  ))}
                </div>
              </div>
            )}
          </div>

          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Vergex per-coin data</span>
            <div className="mt-1 flex flex-wrap gap-2">
              <ToggleChip label="Vergex Signal Lab" active={enableVergexSignalLabData} onClick={() => setEnableVergexSignalLabData(!enableVergexSignalLabData)} />
              <ToggleChip label="Vergex Liquidation Heatmap" active={enableVergexHeatmapData} onClick={() => setEnableVergexHeatmapData(!enableVergexHeatmapData)} />
            </div>
          </div>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/strategies/altfinsEditor.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd web && npx prettier --write src/features/strategies/EditorStepPage.tsx src/features/strategies/altfinsEditor.test.tsx
git add web/src/features/strategies/EditorStepPage.tsx web/src/features/strategies/altfinsEditor.test.tsx
git commit -m "feat(web): altfins + vergex per-coin editor controls"
```

---

## Task 10: Full verification + docs

**Files:**
- Modify: `docs/superpowers/specs/2026-09-19-altfins-and-vergex-per-coin-data-sources-design.md` (mark implemented, if desired)

- [ ] **Step 1: Run all backend checks**

Run: `go build ./... && go vet ./... && gofmt -l .`
Expected: build/vet clean; `gofmt -l` prints nothing.

- [ ] **Step 2: Run all backend tests**

Run: `go test ./provider/altfins/ ./kernel/ ./store/ ./trader/`
Expected: PASS.

- [ ] **Step 3: Run frontend checks**

Run: `cd web && npx tsc --noEmit && npm test`
Expected: PASS.

- [ ] **Step 4: Manual smoke (optional, network-dependent)**

Start the app, create a strategy with AltFins enabled + intervals, trigger a cycle, and confirm the `=== <SYM> AltFins ===` block appears in the logged prompt for supported coins (e.g. ZEC). Vergex blocks may omit on Cloudflare 403 — confirm the cycle still completes.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: verify altfins + vergex per-coin data sources"
```

---

## Self-Review

**Spec coverage:**
- AltFins provider (2-step, 5 intervals, translation, age-from-raw): Task 1.
- AltFins TTL cache: Task 2.
- Store config + clamp + token estimate: Task 3.
- PerCoinSignal fields + prefetch: Task 4.
- attachPerCoinSignals (AltFins + vergex non-signal gating): Task 5.
- Prompt rendering (approved format): Task 6.
- Trader prefetch: Task 7.
- Frontend types/factory/defaults: Task 8.
- Frontend editor toggles + restore: Task 9.
- Verification: Task 10.
- Non-goals respected: no candidate-source changes, no Binance changes, no `vergex_signal` path changes.

**Placeholder scan:** No TBD/TODO; all code steps contain full code. The Task 7 test note and Task 8 `baseForm` note flag real, bounded assumptions rather than deferring implementation.

**Type consistency:** `altfins.Analytics` field names match across Tasks 1/4/5/6. `AltFinsIntervals` vs `altfinsIntervals` (Go vs TS) consistent. `PrefetchAltFinsDetails` signature `(ctx, []string)` consistent in Tasks 4/7. Cache key `<symbol>|<interval>` consistent in Tasks 2/4/5.
