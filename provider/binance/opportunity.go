package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nofx/security"
)

type OpportunityClient struct {
	http    *http.Client
	baseURL string
}

const opportunityBaseURL = "https://www.binance.com/bapi/apex/v1/friendly/apex/web/opportunity"

func NewOpportunityClient() *OpportunityClient {
	return &OpportunityClient{http: security.SafeHTTPClient(30 * time.Second), baseURL: opportunityBaseURL}
}

type opportunityListResponse struct {
	Data struct {
		Items []struct {
			Asset   string `json:"asset"`
			Metrics map[string]struct {
				Value string `json:"value"`
			} `json:"metrics"`
		} `json:"items"`
	} `json:"data"`
}

type OpportunityAsset struct {
	Symbol string
	Score  float64
}

func scoreKeyForScene(scene, interval string) string {
	switch scene {
	case "sentiment":
		return "sentiment_score"
	case "technical":
		if interval == "24h" {
			return "technical_score_1d"
		}
		return "technical_score_1h"
	default:
		return "technical_score_1h"
	}
}

// BinanceMetric holds one flattened metric from the Binance Opportunity
// asset-details response: the human label, the numeric value (score), and the
// display title used to correlate with uiModules categories.
type BinanceMetric struct {
	Value      string // numeric value, e.g. "9.52" (score out of 10)
	ValueLabel string // human label, e.g. "Bullish"
	Label      string // display title, e.g. "RSI (Relative Strength Index)"
}

// BinanceSubIndicator holds one subindicator's summary, signal label, and
// numeric score. It is derived by correlating a uiModules item with its
// technical_ind_*_signal_* metric via the matching label/title.
type BinanceSubIndicator struct {
	Title   string // e.g. "RSI (Relative Strength Index)"
	Summary string // full narrative (valueLabel of the *_summary_* metric)
	Signal  string // e.g. "Overbought" (valueLabel of the *_signal_* metric)
	Score   string // numeric, e.g. "10.00" (value of the *_signal_* metric)
}

// BinanceCategory groups subindicator summaries by category from
// uiModules.technicalIndicatorSummariesModule (technical scene only).
type BinanceCategory struct {
	Category     string                // e.g. "Trend Indicators"
	SubIndicators []BinanceSubIndicator
}

// BinanceAssetDetail is the parsed per-coin Binance Opportunity asset-details
// response. Metrics holds every flattened metric keyed by its API key; the
// Categories hold the uiModules category grouping (technical scene only).
type BinanceAssetDetail struct {
	Metrics    map[string]BinanceMetric
	Categories []BinanceCategory
}

// LabelMap returns the flattened metricKey -> valueLabel map, matching the
// previous GetAssetDetails contract. Used for sentiment (no categories) and
// for backward-compatible label lookups.
func (d *BinanceAssetDetail) LabelMap() map[string]string {
	if d == nil || d.Metrics == nil {
		return nil
	}
	out := make(map[string]string, len(d.Metrics))
	for k, m := range d.Metrics {
		label := strings.TrimSpace(m.ValueLabel)
		if label == "" {
			label = strings.TrimSpace(m.Value)
		}
		if label != "" {
			out[k] = label
		}
	}
	return out
}

type opportunityDetailResponse struct {
	Data struct {
		Metrics map[string]struct {
			Value      string `json:"value"`
			ValueLabel string `json:"valueLabel"`
			Label      string `json:"label"`
		} `json:"metrics"`
		UIModules struct {
			TechnicalIndicatorSummariesModule []struct {
				Title string `json:"title"`
				Items []struct {
					Title   string `json:"title"`
					Summary string `json:"summary"`
					Signal  string `json:"signal"`
				} `json:"items"`
			} `json:"technicalIndicatorSummariesModule"`
		} `json:"uiModules"`
	} `json:"data"`
}

func (c *OpportunityClient) GetAssetDetails(ctx context.Context, symbol, scene, interval string) (*BinanceAssetDetail, error) {
	if interval == "" {
		interval = "1h"
	}
	url := fmt.Sprintf("%s/asset-details?asset=%s&type=%s&interval=%s&quote=USDT",
		c.baseURL, symbol, scene, interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("binance opportunity: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance opportunity: request: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		time.Sleep(1500 * time.Millisecond)
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("binance opportunity: retry request: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance opportunity: HTTP %d", resp.StatusCode)
	}

	var parsed opportunityDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("binance opportunity: decode: %w", err)
	}

	detail := &BinanceAssetDetail{
		Metrics:    make(map[string]BinanceMetric, len(parsed.Data.Metrics)),
		Categories: buildCategories(parsed.Data.UIModules.TechnicalIndicatorSummariesModule, parsed.Data.Metrics),
	}
	for k, m := range parsed.Data.Metrics {
		detail.Metrics[k] = BinanceMetric{
			Value:      strings.TrimSpace(m.Value),
			ValueLabel: strings.TrimSpace(m.ValueLabel),
			Label:      strings.TrimSpace(m.Label),
		}
	}
	return detail, nil
}

// staticTechnicalCategories maps category name -> subindicator labels, matching
// the 1h endpoint's uiModules grouping. The Binance 24h technical detail returns
// an empty technicalIndicatorSummariesModule, so we fall back to this static
// grouping to keep 24h subindicators categorized exactly like the 1h detail.
var staticTechnicalCategories = []struct {
	category string
	labels   []string
}{
	{"Trend Indicators", []string{"MA (Moving Average)", "MACD (Moving Average Convergence Divergence)", "ADX (Average Directional Index)"}},
	{"Volatility Indicators", []string{"Bollinger Bands", "ATR (Average True Range)"}},
	{"Momentum Indicators", []string{"RSI (Relative Strength Index)", "Stochastic Oscillator"}},
	{"Volume & Price Indicators", []string{"Volume", "MFI (Money Flow Index)"}},
}

// buildCategories correlates uiModules category items with their matching
// technical_ind_*_signal_* / *_summary_* metrics and attaches the numeric score.
// When the endpoint provides no uiModules (e.g. the 24h technical detail), it
// falls back to staticTechnicalCategories so subindicators are still grouped.
// Correlation is done by the metric key's base indicator name (e.g. "adx" from
// technical_ind_adx_signal_1d), which is robust to the summary metric carrying a
// placeholder label (e.g. "technical_ind_adx_summary_1d_name") on 24h.
func buildCategories(modules []struct {
	Title string `json:"title"`
	Items []struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Signal  string `json:"signal"`
	} `json:"items"`
}, metrics map[string]struct {
	Value      string `json:"value"`
	ValueLabel string `json:"valueLabel"`
	Label      string `json:"label"`
}) []BinanceCategory {
	// Index metrics by base indicator name. The *_signal_* metric holds the
	// numeric score + human label; the *_summary_* metric holds the narrative.
	signalByBase := make(map[string]struct{ value, label string })
	summaryByBase := make(map[string]string)
	labelByBase := make(map[string]string)
	for key, m := range metrics {
		base, ok := technicalIndBase(key)
		if !ok {
			continue
		}
		if strings.Contains(key, "_signal_") {
			signalByBase[base] = struct{ value, label string }{value: strings.TrimSpace(m.Value), label: strings.TrimSpace(m.ValueLabel)}
			labelByBase[base] = strings.TrimSpace(m.Label)
		}
		if strings.Contains(key, "_summary_") {
			summaryByBase[base] = strings.TrimSpace(m.ValueLabel)
		}
	}

	// Prefer the endpoint-provided category grouping (1h detail).
	if len(modules) > 0 {
		cats := make([]BinanceCategory, 0, len(modules))
		for _, mod := range modules {
			cat := BinanceCategory{Category: mod.Title}
			for _, item := range mod.Items {
				cat.SubIndicators = append(cat.SubIndicators, buildSubIndicator(item.Title, item.Summary, item.Signal, labelByBase, signalByBase, summaryByBase))
			}
			cats = append(cats, cat)
		}
		return cats
	}

	// Fallback: derive the grouping from staticTechnicalCategories for intervals
	// (e.g. 24h) whose uiModules is empty. Only include categories that have at
	// least one present subindicator metric.
	cats := make([]BinanceCategory, 0, len(staticTechnicalCategories))
	for _, sc := range staticTechnicalCategories {
		cat := BinanceCategory{Category: sc.category}
		for _, label := range sc.labels {
			base := baseForLabel(label, labelByBase)
			if base == "" {
				continue
			}
			if _, ok := signalByBase[base]; ok {
				cat.SubIndicators = append(cat.SubIndicators, buildSubIndicator(label, summaryByBase[base], signalByBase[base].label, labelByBase, signalByBase, summaryByBase))
			}
		}
		if len(cat.SubIndicators) > 0 {
			cats = append(cats, cat)
		}
	}
	return cats
}

// technicalIndBase extracts the base indicator name from a metric key, e.g.
// "adx" from "technical_ind_adx_signal_1d" or "adx" from
// "technical_ind_adx_summary_1d". Returns ok=false for non-technical metrics.
func technicalIndBase(key string) (string, bool) {
	const prefix = "technical_ind_"
	if !strings.HasPrefix(key, prefix) {
		return "", false
	}
	rest := key[len(prefix):]
	// Strip the _signal_<suffix> or _summary_<suffix> tail.
	for _, marker := range []string{"_signal_", "_summary_"} {
		if idx := strings.Index(rest, marker); idx >= 0 {
			return rest[:idx], true
		}
	}
	return "", false
}

// baseForLabel reverse-maps a human subindicator title to its base indicator
// name using the labelByBase index (built from the signal metrics' labels).
func baseForLabel(label string, labelByBase map[string]string) string {
	for base, l := range labelByBase {
		if l == label {
			return base
		}
	}
	return ""
}

// buildSubIndicator assembles a BinanceSubIndicator from a category item, its
// correlated signal metric (score + label) and summary metric.
func buildSubIndicator(title, itemSummary, itemSignal string, labelByBase map[string]string, signalByBase map[string]struct{ value, label string }, summaryByBase map[string]string) BinanceSubIndicator {
	base := baseForLabel(title, labelByBase)
	var sig struct{ value, label string }
	if base != "" {
		sig = signalByBase[base]
	}
	sub := BinanceSubIndicator{
		Title:  title,
		Signal: itemSignal,
	}
	if sig.value != "" {
		sub.Score = sig.value
	}
	// Prefer the correlated summary narrative (by base); fall back to the
	// uiModules-/static-provided summary, then to the signal label.
	sub.Summary = itemSummary
	if base != "" {
		if s := summaryByBase[base]; s != "" {
			sub.Summary = s
		}
	}
	if sub.Signal == "" {
		sub.Signal = sig.label
	}
	if sub.Summary == "" {
		sub.Summary = sub.Signal
	}
	return sub
}

func (c *OpportunityClient) GetOpportunityAssets(ctx context.Context, interval, scene string) ([]OpportunityAsset, error) {
	if scene == "" {
		scene = "technical"
	}
	if interval == "" {
		interval = "1h"
	}
	url := fmt.Sprintf("%s/assets?interval=%s&type=%s", c.baseURL, interval, scene)
	if scene == "sentiment" {
		url = fmt.Sprintf("%s/assets?interval=24h&type=sentiment", c.baseURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("binance opportunity: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance opportunity: request: %w", err)
	}
	defer resp.Body.Close()
	var parsed opportunityListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("binance opportunity: decode: %w", err)
	}
	key := scoreKeyForScene(scene, interval)
	out := make([]OpportunityAsset, 0, len(parsed.Data.Items))
	for _, it := range parsed.Data.Items {
		raw, ok := it.Metrics[key]
		if !ok || raw.Value == "" {
			continue
		}
		score, err := strconv.ParseFloat(strings.TrimSpace(raw.Value), 64)
		if err != nil {
			continue
		}
		out = append(out, OpportunityAsset{Symbol: it.Asset, Score: score})
	}
	return out, nil
}
