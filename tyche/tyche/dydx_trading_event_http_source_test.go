package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXTradingEventHTTPSourceFetchesEventsWithCursor(t *testing.T) {
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fills":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":       http.StatusOK,
				"nextCursor": "fill-next",
				"data": []map[string]string{{
					"eventId":        "fill-1",
					"userId":         "user-1",
					"orderId":        "order-1",
					"market":         "ETH-USD",
					"asset":          "USDC",
					"releasedMargin": "25",
					"fee":            "2",
					"realizedPnl":    "5",
					"orderStatus":    TradingOrderStatusFilled,
				}},
			})
		case "/order-statuses":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":       http.StatusOK,
				"nextCursor": "status-next",
				"data": []map[string]string{{
					"eventId": "status-1",
					"userId":  "user-1",
					"orderId": "order-1",
					"status":  TradingOrderStatusSubmitted,
				}},
			})
		case "/positions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":       http.StatusOK,
				"nextCursor": "position-next",
				"data": []map[string]string{{
					"eventId":       "position-1",
					"userId":        "user-1",
					"market":        "ETH-USD",
					"size":          "1",
					"marginAsset":   "USDC",
					"margin":        "25",
					"unrealizedPnl": "3",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	source := NewDYDXTradingEventHTTPSource(DYDXTradingEventHTTPSourceConfig{
		URL:    server.URL,
		APIKey: "secret",
	})

	fills, nextFills, err := source.FetchFills(context.Background(), "fill-cursor")
	if err != nil {
		t.Fatalf("FetchFills() error = %v", err)
	}
	statuses, nextStatuses, err := source.FetchOrderStatuses(context.Background(), "status-cursor")
	if err != nil {
		t.Fatalf("FetchOrderStatuses() error = %v", err)
	}
	positions, nextPositions, err := source.FetchPositions(context.Background(), "position-cursor")
	if err != nil {
		t.Fatalf("FetchPositions() error = %v", err)
	}

	if len(fills) != 1 || fills[0].OrderStatus != TradingOrderStatusFilled || nextFills != "fill-next" {
		t.Fatalf("fills=%+v next=%q", fills, nextFills)
	}
	if len(statuses) != 1 || statuses[0].Status != TradingOrderStatusSubmitted || nextStatuses != "status-next" {
		t.Fatalf("statuses=%+v next=%q", statuses, nextStatuses)
	}
	if len(positions) != 1 || positions[0].UnrealizedPNL != "3" || nextPositions != "position-next" {
		t.Fatalf("positions=%+v next=%q", positions, nextPositions)
	}
	if seen["/fills"] != "fill-cursor" || seen["/order-statuses"] != "status-cursor" || seen["/positions"] != "position-cursor" {
		t.Fatalf("seen cursors = %+v", seen)
	}
}
