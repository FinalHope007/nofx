package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOpportunityAssetsTechnical(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "technical" || r.URL.Query().Get("interval") != "1h" {
			t.Errorf("unexpected query: %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"000000","data":{"items":[
			{"asset":"TREE","rank":1,"metrics":{"technical_score_1h":{"value":"9.45"}}},
			{"asset":"BTC","rank":2,"metrics":{"technical_score_1h":{"value":"7.85"}}}
		]},"success":true}`))
	}))
	defer srv.Close()

	c := NewOpportunityClient()
	c.baseURL = srv.URL
	assets, err := c.GetOpportunityAssets(context.Background(), "1h", "technical")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets[0].Symbol != "TREE" || assets[0].Score != 9.45 {
		t.Fatalf("unexpected first asset: %+v", assets[0])
	}
}