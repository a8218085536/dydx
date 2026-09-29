package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXChainHTTPClientBroadcastsToUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["id"] != "broadcast-1" {
			t.Fatalf("id = %v", body["id"])
		}
		if body["account"] != "dydx1sender" {
			t.Fatalf("account = %v", body["account"])
		}

		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"upstream-tx-1"},"msg":"succ"}`))
	}))
	defer server.Close()

	client := NewDYDXChainHTTPClient(DYDXChainHTTPClientConfig{
		URL: server.URL,
	})

	txHash, err := client.Broadcast(context.Background(), DYDXChainRequest{
		ID:      "broadcast-1",
		Action:  DYDXActionPlaceOrder,
		Account: "dydx1sender",
		Params: map[string]interface{}{
			"market": "ETH-USD",
		},
	})

	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}
	if txHash != "upstream-tx-1" {
		t.Fatalf("txHash = %q, want upstream-tx-1", txHash)
	}
}

func TestDYDXChainHTTPClientReturnsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":500,"msg":"insufficient balance"}`))
	}))
	defer server.Close()

	client := NewDYDXChainHTTPClient(DYDXChainHTTPClientConfig{
		URL: server.URL,
	})

	_, err := client.Broadcast(context.Background(), DYDXChainRequest{
		ID:      "broadcast-1",
		Action:  DYDXActionPlaceOrder,
		Account: "dydx1sender",
	})

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDYDXTradingHTTPClientSubmitsCosmosTxWithoutPlaceholderSignature(t *testing.T) {
	var bodies []map[string]interface{}
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"dydx-tx-1","status":"submitted"},"msg":"succ"}`))
	}))
	defer server.Close()

	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:         server.URL,
		Account:     "dydx1legacy",
		BearerToken: "sidecar-token",
	})

	placed, err := client.SubmitCosmosTx(context.Background(), DYDXCosmosTxRequest{
		ID:            "order-1",
		Action:        DYDXActionPlaceOrder,
		Account:       "dydx1sender",
		ChainID:       "dydx-local-1",
		SignMode:      DYDXCosmosSignModeDirect,
		BroadcastMode: DYDXCosmosBroadcastModeSync,
		Message: DYDXCosmosAny{
			TypeURL: DYDXCosmosMsgPlaceOrderTypeURL,
			Value:   DYDXCosmosMsgPlaceOrder{},
		},
	})
	if err != nil {
		t.Fatalf("SubmitCosmosTx() error = %v", err)
	}

	if placed.Status != TradingOrderStatusSubmitted {
		t.Fatalf("status = %+v, want submitted", placed)
	}
	if len(bodies) != 1 {
		t.Fatalf("len(bodies) = %d, want 1", len(bodies))
	}
	if bodies[0]["action"] != DYDXActionPlaceOrder {
		t.Fatalf("action = %v", bodies[0]["action"])
	}
	if bodies[0]["account"] != "dydx1sender" {
		t.Fatalf("account = %v, want tx account dydx1sender", bodies[0]["account"])
	}
	if authHeader != "Bearer sidecar-token" {
		t.Fatalf("authorization = %q", authHeader)
	}
	if _, ok := bodies[0]["signature"]; ok {
		t.Fatalf("body should not contain placeholder signature: %+v", bodies[0])
	}
	params := bodies[0]["params"].(map[string]interface{})
	cosmosTx := params[DYDXCosmosTxParamKey].(map[string]interface{})
	if cosmosTx["chainId"] != "dydx-local-1" || cosmosTx["signMode"] != DYDXCosmosSignModeDirect {
		t.Fatalf("cosmosTx = %+v", cosmosTx)
	}
}
