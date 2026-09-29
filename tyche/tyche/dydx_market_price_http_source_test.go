package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXHTTPMarketPriceSourceFetchesPriceByConfiguredMarket(t *testing.T) {
	var gotAPIKey string
	var gotMarketID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		gotMarketID = r.URL.Query().Get("marketId")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": http.StatusOK,
			"data": map[string]interface{}{
				"market":     "ETH-USD",
				"marketId":   1,
				"price":      "2500",
				"source":     "dydx-rest",
				"snapshotId": "dydx-market-price:1:2500",
			},
		})
	}))
	defer server.Close()

	source := NewDYDXHTTPMarketPriceSource(DYDXHTTPMarketPriceSourceConfig{
		URL:    server.URL + "/prices",
		APIKey: "secret",
		Markets: []DYDXMarketConfig{
			{Market: "ETH-USD", ClobPairID: 1},
		},
	})

	snapshot, err := source.MarketPrice(context.Background(), TradingMarketPriceRequest{Market: "ETH-USD"})

	if err != nil {
		t.Fatalf("MarketPrice() error = %v", err)
	}
	if gotAPIKey != "secret" || gotMarketID != "1" {
		t.Fatalf("request apiKey=%q marketId=%q", gotAPIKey, gotMarketID)
	}
	if snapshot.Price != "2500" || snapshot.Source != "dydx-rest" || snapshot.SnapshotID == "" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

