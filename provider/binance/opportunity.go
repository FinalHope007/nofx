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

// buildCategories correlates uiModules category items with their matching
// technical_ind_*_signal_* / *_summary_* metrics (matched by the item title
// equalling the metric's label) and attaches the numeric score.
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
	if len(modules) == 0 {
		return nil
	}
	// Index metrics by label so uiModules items can be correlated. The
	// *_signal_* metric's value is the numeric score; the *_summary_* metric's
	// valueLabel is the full narrative.
	signalByLabel := make(map[string]struct{ value, label string })
	summaryByLabel := make(map[string]string)
	for key, m := range metrics {
		if m.Label == "" {
			continue
		}
		if strings.Contains(key, "_signal_") {
			signalByLabel[m.Label] = struct{ value, label string }{value: strings.TrimSpace(m.Value), label: strings.TrimSpace(m.ValueLabel)}
		}
		if strings.Contains(key, "_summary_") {
			summaryByLabel[m.Label] = strings.TrimSpace(m.ValueLabel)
		}
	}

	cats := make([]BinanceCategory, 0, len(modules))
	for _, mod := range modules {
		cat := BinanceCategory{Category: mod.Title}
		for _, item := range mod.Items {
			sig := signalByLabel[item.Title]
			sub := BinanceSubIndicator{
				Title:   item.Title,
				Summary: item.Summary,
				Signal:  item.Signal,
			}
			if sig.value != "" {
				sub.Score = sig.value
			}
			// Prefer the correlated summary narrative; fall back to the
			// uiModules-provided summary if the metric is unavailable.
			if s := summaryByLabel[item.Title]; s != "" {
				sub.Summary = s
			}
			cat.SubIndicators = append(cat.SubIndicators, sub)
		}
		cats = append(cats, cat)
	}
	return cats
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
