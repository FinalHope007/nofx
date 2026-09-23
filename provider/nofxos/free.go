package nofxos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"nofx/security"
	"strings"
	"time"
)

const (
	DefaultFreeTrendingBase = "https://vergex.trade"
	trendingPath            = "/trending-crypto"
	categoryPath            = "/trending-category"
)

// FreeTrendingClient fetches the free vergex.trade trending-crypto endpoints
// (OI / netflow / price) and the ai500 trending-category endpoint. No payment.
type FreeTrendingClient struct {
	baseURL string
	http    security.HTTPDoer

	ai500Cache poolCache[[]CoinData]
	oiTopCache poolCache[[]OIPosition]
	oiLowCache poolCache[[]OIPosition]
	nfTopCache poolCache[[]NetFlowPosition]
	nfLowCache poolCache[[]NetFlowPosition]
	pxTopCache poolCache[[]PriceRankingItem]
	pxLowCache poolCache[[]PriceRankingItem]
	oidCache   poolCache[*OIDataEnvelope]
	nfdCache   poolCache[*NetflowEnvelope]
	pxdCache   poolCache[*PriceEnvelope]
}

func NewFreeTrendingClient() *FreeTrendingClient {
	client, err := security.NewFreeHTTPClient(30 * time.Second)
	if err != nil {
		client = security.SafeHTTPClient(30 * time.Second)
	}
	return &FreeTrendingClient{
		baseURL: strings.TrimRight(DefaultFreeTrendingBase, "/"),
		http:    client,
	}
}

// SetBaseURL overrides the base URL (used by tests across packages that can't
// reach the unexported baseURL field, e.g. kernel/engine_free_test.go).
func (c *FreeTrendingClient) SetBaseURL(baseURL string) {
	c.baseURL = strings.TrimRight(baseURL, "/")
}

func (c *FreeTrendingClient) GetOITop(limit int) ([]OIPosition, error) {
	return c.getOIArray("top", limit)
}

func (c *FreeTrendingClient) GetOILow(limit int) ([]OIPosition, error) {
	return c.getOIArray("low", limit)
}

func (c *FreeTrendingClient) getOIArray(which string, limit int) ([]OIPosition, error) {
	raw, err := c.getTrending("oi", "24h", limit)
	if err != nil {
		return nil, err
	}
	var env struct {
		Top []OIPosition `json:"top"`
		Low []OIPosition `json:"low"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending oi: %w", err)
	}
	if which == "low" {
		return env.Low, nil
	}
	return env.Top, nil
}

func (c *FreeTrendingClient) GetNetFlowTop(limit int) ([]NetFlowPosition, error) {
	return c.getNetFlowArray("top", limit)
}

func (c *FreeTrendingClient) GetNetFlowLow(limit int) ([]NetFlowPosition, error) {
	return c.getNetFlowArray("low", limit)
}

func (c *FreeTrendingClient) getNetFlowArray(which string, limit int) ([]NetFlowPosition, error) {
	raw, err := c.getTrending("net_flow", "24h", limit)
	if err != nil {
		return nil, err
	}
	var env struct {
		Top []NetFlowPosition `json:"top"`
		Low []NetFlowPosition `json:"low"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending net_flow: %w", err)
	}
	if which == "low" {
		return env.Low, nil
	}
	return env.Top, nil
}

func (c *FreeTrendingClient) GetPriceTop(limit int) ([]PriceRankingItem, error) {
	return c.getPriceArray("top", limit)
}

func (c *FreeTrendingClient) GetPriceLow(limit int) ([]PriceRankingItem, error) {
	return c.getPriceArray("low", limit)
}

func (c *FreeTrendingClient) getPriceArray(which string, limit int) ([]PriceRankingItem, error) {
	raw, err := c.getTrending("price", "24h", limit)
	if err != nil {
		return nil, err
	}
	var env struct {
		Top []PriceRankingItem `json:"top"`
		Low []PriceRankingItem `json:"low"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending price: %w", err)
	}
	if which == "low" {
		return env.Low, nil
	}
	return env.Top, nil
}

func (c *FreeTrendingClient) getTrending(tab, duration string, limit int) (json.RawMessage, error) {
	params := url.Values{}
	params.Set("tab", tab)
	if duration == "" {
		duration = "24h"
	}
	params.Set("duration", duration)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.get(c.trendingPath(params))
}

// OIDataEnvelope holds the raw top/low per-list OI data for a duration.
type OIDataEnvelope struct {
	Top []OIPosition `json:"top"`
	Low []OIPosition `json:"low"`
}

// NetflowEnvelope holds the raw top/low per-list netflow data for a duration.
type NetflowEnvelope struct {
	Top []NetFlowPosition `json:"top"`
	Low []NetFlowPosition `json:"low"`
}

// PriceEnvelope holds the raw top/low per-list price ranking data for a duration.
type PriceEnvelope struct {
	Top []PriceRankingItem `json:"top"`
	Low []PriceRankingItem `json:"low"`
}

// GetOIData returns the full top/low OI envelope for a given duration.
func (c *FreeTrendingClient) GetOIData(duration string, limit int) (*OIDataEnvelope, error) {
	raw, err := c.getTrending("oi", duration, limit)
	if err != nil {
		return nil, err
	}
	var env OIDataEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending oi: %w", err)
	}
	return &env, nil
}

// GetNetflowData returns the full top/low netflow envelope for a given duration.
func (c *FreeTrendingClient) GetNetflowData(duration string, limit int) (*NetflowEnvelope, error) {
	raw, err := c.getTrending("net_flow", duration, limit)
	if err != nil {
		return nil, err
	}
	var env NetflowEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending net_flow: %w", err)
	}
	return &env, nil
}

// GetPriceData returns the full top/low price ranking envelope for a given duration.
func (c *FreeTrendingClient) GetPriceData(duration string, limit int) (*PriceEnvelope, error) {
	raw, err := c.getTrending("price", duration, limit)
	if err != nil {
		return nil, err
	}
	var env PriceEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parse trending price: %w", err)
	}
	return &env, nil
}

func (c *FreeTrendingClient) GetAI500() ([]CoinData, error) {
	params := url.Values{}
	params.Set("lang", "en")
	params.Set("key", "ai500")
	raw, err := c.get(c.baseURL + categoryPath + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Category struct {
			Assets []struct {
				Symbol         string  `json:"symbol"`
				Pair           string  `json:"pair"`
				Score          float64 `json:"score"`
				StartTime      int64   `json:"startTime"`
				StartPrice     float64 `json:"startPrice"`
				ChangePctValue float64 `json:"changePctValue"`
				Signal         string  `json:"signal"`
			} `json:"assets"`
		} `json:"category"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse trending-category ai500: %w", err)
	}
	coins := make([]CoinData, 0, len(resp.Category.Assets))
	for _, a := range resp.Category.Assets {
		coins = append(coins, CoinData{
			Pair:            a.Pair,
			Score:           a.Score,
			StartTime:       a.StartTime,
			StartPrice:      a.StartPrice,
			IncreasePercent: a.ChangePctValue,
			IsAvailable:     true,
		})
		coins[len(coins)-1].PeakScore = parsePeakScore(a.Signal)
	}
	return coins, nil
}

func (c *FreeTrendingClient) GetAI500Cached() (poolResult[[]CoinData], error) {
	return c.ai500Cache.get(func() ([]CoinData, error) { return c.GetAI500() })
}

func (c *FreeTrendingClient) GetOITopCached(limit int) (poolResult[[]OIPosition], error) {
	return c.oiTopCache.get(func() ([]OIPosition, error) { return c.GetOITop(limit) })
}

func (c *FreeTrendingClient) GetOILowCached(limit int) (poolResult[[]OIPosition], error) {
	return c.oiLowCache.get(func() ([]OIPosition, error) { return c.GetOILow(limit) })
}

func (c *FreeTrendingClient) GetNetFlowTopCached(limit int) (poolResult[[]NetFlowPosition], error) {
	return c.nfTopCache.get(func() ([]NetFlowPosition, error) { return c.GetNetFlowTop(limit) })
}

func (c *FreeTrendingClient) GetNetFlowLowCached(limit int) (poolResult[[]NetFlowPosition], error) {
	return c.nfLowCache.get(func() ([]NetFlowPosition, error) { return c.GetNetFlowLow(limit) })
}

func (c *FreeTrendingClient) GetPriceTopCached(limit int) (poolResult[[]PriceRankingItem], error) {
	return c.pxTopCache.get(func() ([]PriceRankingItem, error) { return c.GetPriceTop(limit) })
}

func (c *FreeTrendingClient) GetPriceLowCached(limit int) (poolResult[[]PriceRankingItem], error) {
	return c.pxLowCache.get(func() ([]PriceRankingItem, error) { return c.GetPriceLow(limit) })
}

func (c *FreeTrendingClient) GetOIDataCached(duration string, limit int) (poolResult[*OIDataEnvelope], error) {
	return c.oidCache.get(func() (*OIDataEnvelope, error) { return c.GetOIData(duration, limit) })
}

func (c *FreeTrendingClient) GetNetflowDataCached(duration string, limit int) (poolResult[*NetflowEnvelope], error) {
	return c.nfdCache.get(func() (*NetflowEnvelope, error) { return c.GetNetflowData(duration, limit) })
}

func (c *FreeTrendingClient) GetPriceDataCached(duration string, limit int) (poolResult[*PriceEnvelope], error) {
	return c.pxdCache.get(func() (*PriceEnvelope, error) { return c.GetPriceData(duration, limit) })
}

// parsePeakScore extracts the numeric peak score from a signal display string
// (e.g. "Peak 87"), returning 0 when no "Peak" marker is present.
func parsePeakScore(signal string) float64 {
	i := strings.LastIndex(signal, "Peak")
	if i < 0 {
		return 0
	}
	var v float64
	rest := strings.TrimSpace(signal[i+4:])
	fmt.Sscanf(rest, "%f", &v)
	return v
}

func (c *FreeTrendingClient) trendingPath(params url.Values) string {
	return c.baseURL + trendingPath + "?" + params.Encode()
}

func (c *FreeTrendingClient) get(fullURL string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("trending request: %w", err)
	}
	security.SetBrowserHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trending GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trending GET: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
