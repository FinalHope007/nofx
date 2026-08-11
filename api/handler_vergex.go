package api

import (
	"context"
	"fmt"
	"net/http"
	"nofx/logger"
	"nofx/provider/vergex"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// Free-mode note: the /vergex/* handlers below serve the free vergex.trade
// endpoints (no Claw402 wallet required). The optional Bearer token is read from
// VERGEX_API_TOKEN (global config); requests are plain HTTP GETs. No automated
// test is added here because the api package has no lightweight vergex handler
// test server — verification is via `go build`/`go vet` plus manual curl.

func (s *Server) handleVergexSignalRanking(c *gin.Context) {
	client, cerr := s.freeVergexClientForRequest(c)
	if cerr != nil {
		logger.Warnf("Vergex signal-ranking client init failed: %v", cerr)
		c.JSON(http.StatusBadGateway, gin.H{"error": cerr.Error()})
		return
	}
	data, err := client.GetSignalRanking(context.Background(), vergex.Query{})
	if err != nil {
		logger.Warnf("Vergex signal-ranking failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	limit := parsePositiveInt(c.Query("limit"), vergex.MaxSignalRankingItems)
	marketType := strings.TrimSpace(c.Query("marketType"))
	if d := strings.TrimSpace(c.Query("direction")); d != "" {
		switch d {
		case "gainers", "losers":
			dd, derr := client.GetStockMovers(d, limit)
			if derr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": derr.Error()})
				return
			}
			data = dd
		case "trending":
			dd, derr := client.GetStockTrending(limit)
			if derr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": derr.Error()})
				return
			}
			data = dd
		}
	}
	items := vergex.FilterSignalRankingItems(data.Items, marketType, limit)
	c.JSON(http.StatusOK, gin.H{"items": items, "raw": data.Raw})
}

func (s *Server) handleVergexSignalLab(c *gin.Context) {
	client, cerr := s.freeVergexClientForRequest(c)
	if cerr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": cerr.Error()})
		return
	}
	body, err := client.GetSignalLab(context.Background(), vergex.Query{
		MarketType: withDefault(strings.TrimSpace(c.Query("marketType")), vergex.DefaultMarketType),
		Symbol:     strings.TrimSpace(c.Query("symbol")),
		Chain:      strings.TrimSpace(c.Query("chain")),
		LiqBand:    strings.TrimSpace(c.Query("liqBand")),
	})
	if err != nil {
		logger.Warnf("Vergex signal-lab failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func (s *Server) handleVergexCostLiquidationHeatmap(c *gin.Context) {
	client, cerr := s.freeVergexClientForRequest(c)
	if cerr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": cerr.Error()})
		return
	}
	body, err := client.GetCostLiquidationHeatmap(context.Background(), vergex.Query{
		MarketType: withDefault(strings.TrimSpace(c.Query("marketType")), vergex.DefaultMarketType),
		Symbol:     strings.TrimSpace(c.Query("symbol")),
		Chain:      strings.TrimSpace(c.Query("chain")),
		LiqBand:    strings.TrimSpace(c.Query("liqBand")),
	})
	if err != nil {
		logger.Warnf("Vergex cost-liquidation-heatmap failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// handleVergexFlowMarkets proxies the Vergex net-flow market ranking via the
// free vergex.trade endpoint. The upstream JSON is passed through verbatim:
// { data: { window, by, inflow: [{ symbol, netFlow, buyNotional, sellNotional,
// trades, latestPrice }, ...] } }.
func (s *Server) handleVergexFlowMarkets(c *gin.Context) {
	client, cerr := s.freeVergexClientForRequest(c)
	if cerr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": cerr.Error()})
		return
	}
	chain := withDefault(strings.TrimSpace(c.Query("chain")), "mainnet")
	window := withDefault(strings.TrimSpace(c.Query("window")), "1h")
	limit := parsePositiveInt(c.Query("limit"), 25)
	body, err := client.GetFlowMarkets(context.Background(), chain, window, limit)
	if err != nil {
		logger.Warnf("Vergex flow-markets failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func (s *Server) freeVergexClientForRequest(c *gin.Context) (*vergex.Client, error) {
	_ = c.GetString("user_id") // free endpoints are public; token is global config, not per-user
	client, err := vergex.NewFreeClient(vergex.DefaultFreeBaseURL, os.Getenv("VERGEX_API_TOKEN"), &logger.MCPLogger{})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func withDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
