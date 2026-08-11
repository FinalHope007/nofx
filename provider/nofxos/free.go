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
	http    *http.Client
}

func NewFreeTrendingClient() *FreeTrendingClient {
	return &FreeTrendingClient{
		baseURL: strings.TrimRight(DefaultFreeTrendingBase, "/"),
		http:    security.SafeHTTPClient(30 * time.Second),
	}
}

func (c *FreeTrendingClient) GetOITop(limit int) ([]OIPosition, error) {
	return c.getOIArray("top", limit)
}

func (c *FreeTrendingClient) GetOILow(limit int) ([]OIPosition, error) {
	return c.getOIArray("low", limit)
}

func (c *FreeTrendingClient) getOIArray(which string, limit int) ([]OIPosition, error) {
	raw, err := c.getTrending("oi", limit)
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
	raw, err := c.getTrending("net_flow", limit)
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
	raw, err := c.getTrending("price", limit)
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

func (c *FreeTrendingClient) getTrending(tab string, limit int) (json.RawMessage, error) {
	params := url.Values{}
	params.Set("tab", tab)
	params.Set("duration", "24h")
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.get(c.trendingPath(params))
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
	}
	return coins, nil
}

func (c *FreeTrendingClient) trendingPath(params url.Values) string {
	return c.baseURL + trendingPath + "?" + params.Encode()
}

func (c *FreeTrendingClient) get(fullURL string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("trending request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; nofx)")
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
