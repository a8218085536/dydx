package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDYDXTradingHTTPClientSubmitCosmosTxUsesNativeSidecarContract(t *testing.T) {
	var gotAuth string
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"dydx-tx-1","status":"submitted"},"msg":"succ"}`))
	}))
	defer server.Close()
	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:         server.URL,
		Account:     "dydx1owner",
		BearerToken: "internal-key",
	})

	result, err := client.SubmitCosmosTx(context.Background(), nativePlaceOrderCosmosTx())

	if err != nil {
		t.Fatalf("SubmitCosmosTx() error = %v", err)
	}
	if result.TxHash != "dydx-tx-1" || result.Status != TradingOrderStatusSubmitted {
		t.Fatalf("result = %+v", result)
	}
	if gotAuth != "Bearer internal-key" {
		t.Fatalf("authorization = %q, want Bearer internal-key", gotAuth)
	}
	if body["id"] != "order-1" || body["action"] != DYDXActionPlaceOrder || body["account"] != "dydx1owner" {
		t.Fatalf("body identity = %+v", body)
	}
	params, ok := body["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params = %T", body["params"])
	}
	cosmosTx, ok := params[DYDXCosmosTxParamKey].(map[string]interface{})
	if !ok {
		t.Fatalf("cosmosTx = %T", params[DYDXCosmosTxParamKey])
	}
	if cosmosTx["id"] != "order-1" || cosmosTx["action"] != DYDXActionPlaceOrder || cosmosTx["account"] != "dydx1owner" {
		t.Fatalf("cosmos tx identity = %+v", cosmosTx)
	}
	if cosmosTx["chainId"] != "dydx-local-1" || cosmosTx["signMode"] != DYDXCosmosSignModeDirect || cosmosTx["broadcastMode"] != DYDXCosmosBroadcastModeSync {
		t.Fatalf("cosmos tx modes = %+v", cosmosTx)
	}
	message, ok := cosmosTx["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("message = %T", cosmosTx["message"])
	}
	if message["typeUrl"] != DYDXCosmosMsgPlaceOrderTypeURL {
		t.Fatalf("typeUrl = %v", message["typeUrl"])
	}
	value, ok := message["value"].(map[string]interface{})
	if !ok {
		t.Fatalf("message value = %T", message["value"])
	}
	order, ok := value["order"].(map[string]interface{})
	if !ok {
		t.Fatalf("order = %T", value["order"])
	}
	orderID := order["orderId"].(map[string]interface{})
	if orderID["clientId"] != float64(123) || orderID["orderFlags"] != float64(DYDXOrderFlagsLongTerm) || orderID["clobPairId"] != float64(7) {
		t.Fatalf("order id = %+v", orderID)
	}
	subaccountID := orderID["subaccountId"].(map[string]interface{})
	if subaccountID["owner"] != "dydx1owner" || subaccountID["number"] != float64(0) {
		t.Fatalf("subaccount id = %+v", subaccountID)
	}
}

func TestDYDXTradingHTTPClientRejectsHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()
	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:     server.URL,
		Account: "dydx1owner",
	})

	_, err := client.SubmitCosmosTx(context.Background(), nativePlaceOrderCosmosTx())

	if err == nil || !strings.Contains(err.Error(), "http 502") {
		t.Fatalf("SubmitCosmosTx() error = %v, want http 502", err)
	}
}

func TestDYDXTradingHTTPClientTreatsAlreadyInMempoolAsBroadcasted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":400,"data":{"txHash":"dydx-tx-1"},"msg":"broadcast rejected with code 19: tx already in mempool"}`))
	}))
	defer server.Close()
	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:     server.URL,
		Account: "dydx1owner",
	})

	result, err := client.SubmitCosmosTx(context.Background(), nativePlaceOrderCosmosTx())

	if err != nil {
		t.Fatalf("SubmitCosmosTx() error = %v", err)
	}
	if result.TxHash != "dydx-tx-1" || result.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("result = %+v, want broadcasted duplicate mempool tx", result)
	}
}

func TestDYDXTradingHTTPClientTreatsCode19WithEmptyRawLogAsBroadcasted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":400,"data":{"txHash":"dydx-tx-2"},"msg":"broadcast rejected with code 19: "}`))
	}))
	defer server.Close()
	client := NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
		URL:     server.URL,
		Account: "dydx1owner",
	})

	result, err := client.SubmitCosmosTx(context.Background(), nativePlaceOrderCosmosTx())

	if err != nil {
		t.Fatalf("SubmitCosmosTx() error = %v", err)
	}
	if result.TxHash != "dydx-tx-2" || result.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("result = %+v, want broadcasted code 19 tx with empty raw log", result)
	}
}

func nativePlaceOrderCosmosTx() DYDXCosmosTxRequest {
	return DYDXCosmosTxRequest{
		ID:            "order-1",
		Action:        DYDXActionPlaceOrder,
		Account:       "dydx1owner",
		ChainID:       "dydx-local-1",
		SignMode:      DYDXCosmosSignModeDirect,
		BroadcastMode: DYDXCosmosBroadcastModeSync,
		Message: DYDXCosmosAny{
			TypeURL: DYDXCosmosMsgPlaceOrderTypeURL,
			Value: DYDXCosmosMsgPlaceOrder{
				Order: DYDXCosmosOrder{
					OrderID: DYDXCosmosOrderID{
						SubaccountID: DYDXCosmosSubaccountID{Owner: "dydx1owner"},
						ClientID:     123,
						OrderFlags:   DYDXOrderFlagsLongTerm,
						ClobPairID:   7,
					},
					Side:             DYDXOrderSideBuy,
					Quantums:         1000000,
					Subticks:         2500000000,
					GoodTilBlockTime: 1786903600,
					TimeInForce:      DYDXTimeInForceUnspecified,
				},
			},
		},
	}
}
