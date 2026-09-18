// Package altfins provides a client for the unofficial, scraped AltFins
// analytics feed used to enrich per-coin LLM context.
package altfins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
//
// For POST requests the original request body is consumed by the first
// attempt, so the retry must rebuild the request from a buffered copy (or
// GetBody) to avoid sending an empty body.
func (c *AnalyticsClient) do(req *http.Request) (*http.Response, error) {
	clone, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("altfins: request: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		time.Sleep(1500 * time.Millisecond)
		resp, err = c.http.Do(clone)
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

// cloneRequest returns a copy of req that can be sent a second time. The body,
// when present, is buffered so the retry does not reuse a consumed reader.
func cloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil {
		return clone, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("altfins: buffer retry body: %w", err)
		}
		clone.Body = body
		return clone, nil
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("altfins: buffer retry body: %w", err)
	}
	req.Body.Close()
	clone.Body = io.NopCloser(bytes.NewReader(data))
	return clone, nil
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
//
// The human text is derived from bars*intervalDuration (not raw elapsed) so it
// matches the displayed bar count; e.g. 6 bars at 15m renders "~90 min ago".
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
	return bars, "~" + humanizeDuration(time.Duration(bars)*dur) + " ago"
}

// humanizeDuration renders a compact, grammatical age string using the largest
// unit that divides the duration exactly (so 90 min stays "90 min" rather than
// rounding to "1 hour", while 8h and 2d use hours/days).
func humanizeDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour && d%(24*time.Hour) == 0:
		return pluralize(int(d.Hours()/24), "day", "days")
	case d >= time.Hour && d%time.Hour == 0:
		return pluralize(int(d.Hours()), "hour", "hours")
	default:
		return pluralize(int(d.Minutes()), "min", "min")
	}
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
