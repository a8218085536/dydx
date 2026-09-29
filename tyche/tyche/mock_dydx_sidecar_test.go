package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockDYDXSidecarSubmitsNativeCosmosTx(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{
		TxHash: "mock-tx-1",
	})
	server := httptest.NewServer(sidecar.Handler())
	defer server.Close()
	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:     server.URL + "/dydx/trading",
		Account: "dydx1owner",
	})

	result, err := client.SubmitCosmosTx(context.Background(), nativePlaceOrderCosmosTx())

	if err != nil {
		t.Fatalf("SubmitCosmosTx() error = %v", err)
	}
	if result.TxHash != "mock-tx-1" || result.Status != TradingOrderStatusSubmitted {
		t.Fatalf("result = %+v", result)
	}
	requests := sidecar.Requests()
	if len(requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(requests))
	}
	if requests[0].Action != DYDXActionPlaceOrder {
		t.Fatalf("action = %q, want place_order", requests[0].Action)
	}
}

func TestMockDYDXSidecarCanRejectSubmit(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{
		SubmitStatus: TradingOrderStatusRejected,
		RejectReason: "mock rejected",
	})
	server := httptest.NewServer(sidecar.Handler())
	defer server.Close()
	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:     server.URL + "/dydx/trading",
		Account: "dydx1owner",
	})

	result, err := client.SubmitCosmosTx(context.Background(), nativePlaceOrderCosmosTx())

	if err != nil {
		t.Fatalf("SubmitCosmosTx() error = %v", err)
	}
	if result.Status != TradingOrderStatusRejected {
		t.Fatalf("status = %q, want rejected", result.Status)
	}
}

func TestMockDYDXSidecarServesTradingEvents(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{
		Fills: []TradingFillSyncRequest{{
			EventID:        "fill-1",
			UserID:         "user-1",
			OrderID:        "order-1",
			Market:         "ETH-USD",
			Asset:          "USDC",
			ReleasedMargin: "25",
			Fee:            "2",
			RealizedPNL:    "5",
		}},
		OrderStatuses: []TradingOrderStatusSyncRequest{{
			EventID: "status-1",
			UserID:  "user-1",
			OrderID: "order-1",
			Status:  TradingOrderStatusSubmitted,
		}},
		Positions: []TradingPositionSyncRequest{{
			EventID:       "position-1",
			UserID:        "user-1",
			Market:        "ETH-USD",
			Size:          "1",
			MarginAsset:   "USDC",
			Margin:        "25",
			UnrealizedPNL: "0",
		}},
		NextFillsCursor:       "fills-next",
		NextOrderStatusCursor: "status-next",
		NextPositionsCursor:   "positions-next",
	})
	server := httptest.NewServer(sidecar.Handler())
	defer server.Close()
	source := NewDYDXTradingEventHTTPSource(DYDXTradingEventHTTPSourceConfig{URL: server.URL})

	fills, nextFills, err := source.FetchFills(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchFills() error = %v", err)
	}
	statuses, nextStatuses, err := source.FetchOrderStatuses(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchOrderStatuses() error = %v", err)
	}
	positions, nextPositions, err := source.FetchPositions(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchPositions() error = %v", err)
	}

	if len(fills) != 1 || fills[0].EventID != "fill-1" || nextFills != "fills-next" {
		t.Fatalf("fills=%+v next=%q", fills, nextFills)
	}
	if len(statuses) != 1 || statuses[0].EventID != "status-1" || nextStatuses != "status-next" {
		t.Fatalf("statuses=%+v next=%q", statuses, nextStatuses)
	}
	if len(positions) != 1 || positions[0].EventID != "position-1" || nextPositions != "positions-next" {
		t.Fatalf("positions=%+v next=%q", positions, nextPositions)
	}
}

func TestMockDYDXSidecarServesBlockStatus(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{})
	server := httptest.NewServer(sidecar.Handler())
	defer server.Close()
	provider := NewDYDXHTTPBlockHeightProvider(DYDXHTTPBlockHeightProviderConfig{URL: server.URL + "/status"})

	height, err := provider.CurrentBlockHeight(context.Background())
	blockTimeUnix, timeErr := provider.CurrentBlockTimeUnix(context.Background())

	if err != nil {
		t.Fatalf("CurrentBlockHeight() error = %v", err)
	}
	if timeErr != nil {
		t.Fatalf("CurrentBlockTimeUnix() error = %v", timeErr)
	}
	if height != 100 {
		t.Fatalf("height = %d, want 100", height)
	}
	if blockTimeUnix != 1786936320 {
		t.Fatalf("blockTimeUnix = %d, want 1786936320", blockTimeUnix)
	}
}

func TestMockDYDXSidecarRejectsNonCosmosTxPayload(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{})
	server := httptest.NewServer(sidecar.Handler())
	defer server.Close()

	resp, err := http.Post(server.URL+"/dydx/trading", "application/json", strings.NewReader(`{"id":"bad","action":"place_order","account":"dydx1owner","params":{}}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("http status = %d, want 200", resp.StatusCode)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if result.Code != http.StatusBadRequest || !strings.Contains(result.Msg, "cosmosTx") {
		t.Fatalf("result = %+v, want cosmosTx bad request", result)
	}
}
