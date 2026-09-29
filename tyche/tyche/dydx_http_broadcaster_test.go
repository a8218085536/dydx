package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXHTTPBroadcasterPostsIntentAndReturnsTxHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/broadcast" {
			t.Fatalf("path = %s, want /broadcast", r.URL.Path)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["id"] != "order-1" {
			t.Fatalf("id = %v", body["id"])
		}
		if body["account"] != "dydx1sender" {
			t.Fatalf("account = %v", body["account"])
		}

		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"dydx-tx-1"},"msg":"succ"}`))
	}))
	defer server.Close()

	broadcaster := NewDYDXHTTPBroadcaster(DYDXHTTPBroadcasterConfig{
		URL:     server.URL + "/broadcast",
		Account: "dydx1sender",
	})

	txHash, err := broadcaster.Broadcast(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"market": "ETH-USD",
		},
	})

	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}
	if txHash != "dydx-tx-1" {
		t.Fatalf("txHash = %q, want dydx-tx-1", txHash)
	}
}

func TestDYDXHTTPBroadcasterReturnsGatewayError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":500,"msg":"bad signature"}`))
	}))
	defer server.Close()

	broadcaster := NewDYDXHTTPBroadcaster(DYDXHTTPBroadcasterConfig{
		URL:     server.URL,
		Account: "dydx1sender",
	})

	_, err := broadcaster.Broadcast(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
	})

	if err == nil {
		t.Fatal("expected error")
	}
}
