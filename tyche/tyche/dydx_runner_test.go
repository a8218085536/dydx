package tyche

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDYDXRunnerReturnsEnqueueError(t *testing.T) {
	runner := NewDYDXRunner(minimalConfig())
	want := errors.New("enqueue failed")
	runner.enqueue = func(intent DYDXIntent) error {
		return want
	}

	err := runner.Submit(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"market": "ETH-USD",
		},
	})

	if !errors.Is(err, want) {
		t.Fatalf("Submit() error = %v, want %v", err, want)
	}
}

func TestDYDXRunnerRejectsWithdrawIntentAction(t *testing.T) {
	runner := NewDYDXRunner(minimalConfig())
	runner.enqueue = func(intent DYDXIntent) error {
		return nil
	}

	err := runner.Submit(context.Background(), DYDXIntent{
		ID:     "withdrawal-1",
		Action: "withdraw",
	})

	if !errors.Is(err, ErrUnsupportedDYDXAction) {
		t.Fatalf("Submit() error = %v, want %v", err, ErrUnsupportedDYDXAction)
	}
}

func TestDYDXRunnerRejectsUnknownIntentAction(t *testing.T) {
	runner := NewDYDXRunner(minimalConfig())

	err := runner.Submit(context.Background(), DYDXIntent{
		ID:     "unknown-1",
		Action: "stake",
	})

	if !errors.Is(err, ErrUnsupportedDYDXAction) {
		t.Fatalf("Submit() error = %v, want %v", err, ErrUnsupportedDYDXAction)
	}
}

func TestDYDXRunnerEnqueuesSupportedIntent(t *testing.T) {
	runner := NewDYDXRunner(minimalConfig())
	var got DYDXIntent
	runner.enqueue = func(intent DYDXIntent) error {
		got = intent
		return nil
	}

	err := runner.Submit(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"market": "ETH-USD",
		},
	})

	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if got.ID != "order-1" || got.Action != DYDXActionPlaceOrder {
		t.Fatalf("enqueued intent = %+v", got)
	}
}

func TestDYDXRunnerRunSkipsLegacyQueueWhenTradingSidecarIsConfigured(t *testing.T) {
	conf := minimalConfig()
	configureValidDYDXTrading(conf)
	runner := NewDYDXRunner(conf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

func TestDYDXRunnerSubmitUsesCosmosTradingRequestWhenConfigured(t *testing.T) {
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"dydx-tx-1","status":"submitted"},"msg":"succ"}`))
	}))
	defer server.Close()

	conf := minimalConfig()
	conf.DYDXTradingURL = server.URL
	conf.DYDXChainID = "dydx-local-1"
	conf.DYDXGoodTilSeconds = 3600
	conf.DYDXMarkets = []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
	runner := NewDYDXRunner(conf)
	runner.tradingRequestFactory = NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder: NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
			ChainID:        "dydx-local-1",
			Account:        conf.Account,
			GoodTilSeconds: 3600,
			Markets:        conf.DYDXMarkets,
		}),
		TimeProvider: staticDYDXBlockTimeProvider{blockTimeUnix: 1786900000},
	})
	runner.enqueue = func(intent DYDXIntent) error {
		t.Fatal("enqueue should not be called when DYDXTradingURL is configured")
		return nil
	}

	err := runner.Submit(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"userId":        "user-1",
			"clientOrderId": "123",
			"market":        "ETH-USD",
			"side":          "buy",
			"orderType":     "limit",
			"price":         "2500",
			"size":          "1",
			"marginAsset":   "USDC",
			"marginAmount":  "25",
		},
	})

	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if body["action"] != DYDXActionPlaceOrder {
		t.Fatalf("body action = %v", body["action"])
	}
	if _, ok := body["signature"]; ok {
		t.Fatalf("body should not contain placeholder signature: %+v", body)
	}
	params, ok := body["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params = %T", body["params"])
	}
	cosmosTx, ok := params[DYDXCosmosTxParamKey].(map[string]interface{})
	if !ok {
		t.Fatalf("cosmosTx = %T", params[DYDXCosmosTxParamKey])
	}
	if cosmosTx["chainId"] != "dydx-local-1" || cosmosTx["signMode"] != DYDXCosmosSignModeDirect {
		t.Fatalf("cosmosTx = %+v", cosmosTx)
	}
	message := cosmosTx["message"].(map[string]interface{})
	if message["typeUrl"] != DYDXCosmosMsgPlaceOrderTypeURL {
		t.Fatalf("message = %+v", message)
	}
}

func TestDYDXRunnerSubmitCancelUsesCosmosTradingRequestWhenConfigured(t *testing.T) {
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"dydx-cancel-1","status":"submitted"},"msg":"succ"}`))
	}))
	defer server.Close()

	conf := minimalConfig()
	conf.DYDXTradingURL = server.URL
	conf.DYDXChainID = "dydx-local-1"
	conf.DYDXSubaccountNumber = 4
	conf.DYDXGoodTilSeconds = 3600
	conf.DYDXMarkets = []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
	runner := NewDYDXRunner(conf)
	runner.tradingRequestFactory = NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder: NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
			ChainID:          "dydx-local-1",
			Account:          conf.Account,
			SubaccountNumber: 4,
			GoodTilSeconds:   3600,
			Markets:          conf.DYDXMarkets,
		}),
		TimeProvider: staticDYDXBlockTimeProvider{blockTimeUnix: 1786900000},
	})
	runner.enqueue = func(intent DYDXIntent) error {
		t.Fatal("enqueue should not be called when DYDXTradingURL is configured")
		return nil
	}

	err := runner.Submit(context.Background(), DYDXIntent{
		ID:     "order-1-cancel",
		Action: DYDXActionCancelOrder,
		Params: map[string]interface{}{
			"userId":        "user-1",
			"orderId":       "order-1",
			"clientOrderId": "123",
			"market":        "ETH-USD",
		},
	})

	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if body["action"] != DYDXActionCancelOrder {
		t.Fatalf("body action = %v", body["action"])
	}
	params := body["params"].(map[string]interface{})
	cosmosTx := params[DYDXCosmosTxParamKey].(map[string]interface{})
	if cosmosTx["action"] != DYDXActionCancelOrder || cosmosTx["chainId"] != "dydx-local-1" {
		t.Fatalf("cosmosTx = %+v", cosmosTx)
	}
	message := cosmosTx["message"].(map[string]interface{})
	if message["typeUrl"] != DYDXCosmosMsgCancelOrderTypeURL {
		t.Fatalf("message = %+v", message)
	}
	value := message["value"].(map[string]interface{})
	orderID := value["orderId"].(map[string]interface{})
	subaccountID := orderID["subaccountId"].(map[string]interface{})
	if subaccountID["owner"] != conf.Account || subaccountID["number"] != float64(4) {
		t.Fatalf("subaccount = %+v", subaccountID)
	}
	if orderID["clientId"] != float64(123) || orderID["clobPairId"] != float64(7) || orderID["orderFlags"] != float64(DYDXOrderFlagsLongTerm) {
		t.Fatalf("order id = %+v", orderID)
	}
	if value["goodTilBlockTime"] != float64(1786903600) {
		t.Fatalf("goodTilBlockTime = %v, want 1786903600", value["goodTilBlockTime"])
	}
}

func TestDYDXRunnerSubmitWithResultReturnsTradingStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"txHash":"dydx-tx-1","status":"rejected"},"msg":"succ"}`))
	}))
	defer server.Close()

	conf := minimalConfig()
	conf.DYDXTradingURL = server.URL
	conf.DYDXChainID = "dydx-local-1"
	conf.DYDXGoodTilSeconds = 3600
	conf.DYDXMarkets = []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
	runner := NewDYDXRunner(conf)
	runner.tradingRequestFactory = NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder: NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
			ChainID:        "dydx-local-1",
			Account:        conf.Account,
			GoodTilSeconds: 3600,
			Markets:        conf.DYDXMarkets,
		}),
		TimeProvider: staticDYDXBlockTimeProvider{blockTimeUnix: 1786900000},
	})

	result, err := runner.SubmitWithResult(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"clientOrderId": "123",
			"market":        "ETH-USD",
			"side":          "buy",
			"orderType":     "limit",
			"price":         "2500",
			"size":          "1",
		},
	})

	if err != nil {
		t.Fatalf("SubmitWithResult() error = %v", err)
	}
	if result.TxHash != "dydx-tx-1" || result.Status != TradingOrderStatusRejected {
		t.Fatalf("result = %+v", result)
	}
}
