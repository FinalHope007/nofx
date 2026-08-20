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

type opportunityDetailResponse struct {
	Data struct {
		Metrics map[string]struct {
			Value      string `json:"value"`
			ValueLabel string `json:"valueLabel"`
		} `json:"metrics"`
	} `json:"data"`
}

func (c *OpportunityClient) GetAssetDetails(ctx context.Context, symbol, scene, interval string) (map[string]string, error) {
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
	out := make(map[string]string, len(parsed.Data.Metrics))
	for k, m := range parsed.Data.Metrics {
		label := strings.TrimSpace(m.ValueLabel)
		if label == "" {
			label = strings.TrimSpace(m.Value)
		}
		if label != "" {
			out[k] = label
		}
	}
	return out, nil
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
