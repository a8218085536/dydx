package tyche

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXCosmosOrderBuilderBuildsStatefulPlaceOrder(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1owner",
		SubaccountNumber: 0,
		GoodTilSeconds:   3600,
		Markets:          []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000", SubticksPerTick: "100000"}},
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-1",
		UserID:        "user-1",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500.5",
		Size:          "1.25",
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	if tx.ID != "order-1" || tx.Action != DYDXActionPlaceOrder || tx.Account != "dydx1owner" || tx.ChainID != "dydx-local-1" {
		t.Fatalf("tx identity = %+v", tx)
	}
	if tx.SignMode != DYDXCosmosSignModeDirect || tx.BroadcastMode != DYDXCosmosBroadcastModeSync {
		t.Fatalf("tx modes = sign %q broadcast %q", tx.SignMode, tx.BroadcastMode)
	}
	if tx.Message.TypeURL != DYDXCosmosMsgPlaceOrderTypeURL {
		t.Fatalf("typeURL = %q, want %q", tx.Message.TypeURL, DYDXCosmosMsgPlaceOrderTypeURL)
	}
	message, ok := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if !ok {
		t.Fatalf("message value = %T, want DYDXCosmosMsgPlaceOrder", tx.Message.Value)
	}
	order := message.Order
	if order.OrderID.SubaccountID.Owner != "dydx1owner" || order.OrderID.SubaccountID.Number != 0 {
		t.Fatalf("subaccount = %+v", order.OrderID.SubaccountID)
	}
	if order.OrderID.ClientID != 123 || order.OrderID.ClobPairID != 7 || order.OrderID.OrderFlags != DYDXOrderFlagsLongTerm {
		t.Fatalf("order id = %+v", order.OrderID)
	}
	if order.Side != DYDXOrderSideBuy || order.Quantums != 1250000 || order.Subticks != 2500500000 {
		t.Fatalf("order = %+v", order)
	}
	if order.GoodTilBlock != 0 {
		t.Fatalf("goodTilBlock = %d, want 0", order.GoodTilBlock)
	}
	if order.GoodTilBlockTime != 1786903600 {
		t.Fatalf("goodTilBlockTime = %d, want 1786903600", order.GoodTilBlockTime)
	}
}

func TestDYDXCosmosOrderBuilderUsesOrderSubaccountNumber(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1owner",
		SubaccountNumber: 0,
		GoodTilSeconds:   3600,
		Markets:          []DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}},
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:               "order-isolated",
		ClientOrderID:    "398831676",
		Market:           "FDEX-USD",
		Side:             "sell",
		OrderType:        "limit",
		Price:            "20",
		Size:             "20",
		SubaccountNumber: 302,
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if message.Order.OrderID.SubaccountID.Number != 302 {
		t.Fatalf("place subaccount number = %d, want 302", message.Order.OrderID.SubaccountID.Number)
	}
}

func TestDYDXCosmosOrderBuilderUsesOrderSubaccountNumberForCancel(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1owner",
		SubaccountNumber: 0,
		GoodTilSeconds:   3600,
		Markets:          []DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}},
	})

	tx, err := builder.BuildCancelOrder(DYDXCancelOrder{
		ID:               "order-isolated-cancel",
		ClientOrderID:    "398831676",
		Market:           "FDEX-USD",
		SubaccountNumber: 302,
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildCancelOrder() error = %v", err)
	}
	message := tx.Message.Value.(DYDXCosmosMsgCancelOrder)
	if message.OrderID.SubaccountID.Number != 302 {
		t.Fatalf("cancel subaccount number = %d, want 302", message.OrderID.SubaccountID.Number)
	}
}

func TestDYDXCosmosOrderBuilderBuildsLongTermPlaceOrder(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:        "dydx-local-1",
		Account:        "dydx1owner",
		GoodTilSeconds: 3600,
		Markets:        validDYDXMarketConfigs(),
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-long-term",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	order := message.Order
	if order.OrderID.OrderFlags != DYDXOrderFlagsLongTerm {
		t.Fatalf("order flags = %d, want %d", order.OrderID.OrderFlags, DYDXOrderFlagsLongTerm)
	}
	if order.GoodTilBlock != 0 {
		t.Fatalf("goodTilBlock = %d, want 0 for long-term order", order.GoodTilBlock)
	}
	if order.GoodTilBlockTime != 1786903600 {
		t.Fatalf("goodTilBlockTime = %d, want 1786903600", order.GoodTilBlockTime)
	}
}

func TestDYDXCosmosOrderBuilderRejectsPriceNotAlignedToSubticksPerTick(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:        "dydx-local-1",
		Account:        "dydx1owner",
		GoodTilSeconds: 3600,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
			SubticksPerUnit: "1000000",
			SubticksPerTick: "100000",
		}},
	})

	_, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-1",
		UserID:        "user-1",
		DYDXAccount:   "dydx1owner",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "ioc",
		Price:         "1932.0694",
		Size:          "1",
	}, 100, 1786900000)

	if !errors.Is(err, ErrInvalidDYDXAmount) {
		t.Fatalf("BuildPlaceOrder() error = %v, want %v", err, ErrInvalidDYDXAmount)
	}
}

func TestDYDXCosmosOrderBuilderRejectsSizeNotAlignedToStepBaseQuantums(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:        "dydx-local-1",
		Account:        "dydx1owner",
		GoodTilSeconds: 3600,
		Markets: []DYDXMarketConfig{{
			Market:           "ETH-USD",
			ClobPairID:       1,
			QuantumsPerUnit:  "1000000000",
			SubticksPerUnit:  "1000000",
			SubticksPerTick:  "100000",
			StepBaseQuantums: "1000000",
		}},
	})

	_, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-1",
		UserID:        "user-1",
		DYDXAccount:   "dydx1owner",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "1932.1",
		Size:          "0.0005",
	}, 100, 1786900000)

	if !errors.Is(err, ErrInvalidDYDXAmount) {
		t.Fatalf("BuildPlaceOrder() error = %v, want %v", err, ErrInvalidDYDXAmount)
	}
}

func TestDYDXCosmosOrderBuilderBuildsReduceOnlyPlaceOrderAsShortTermIOC(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:           "dydx-local-1",
		SubaccountNumber:  0,
		GoodTilBlockDelta: 20,
		GoodTilSeconds:    3600,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000",
			SubticksPerUnit: "1000000",
		}},
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "close-order-1",
		DYDXAccount:   "dydx1abc",
		ClientOrderID: "12345",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "limit",
		Price:         "2600",
		Size:          "0.5",
		ReduceOnly:    true,
	}, 100, 1780000000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	message, ok := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if !ok {
		t.Fatalf("message value = %T, want DYDXCosmosMsgPlaceOrder", tx.Message.Value)
	}
	if !message.Order.ReduceOnly {
		t.Fatalf("reduceOnly = false, want true")
	}
	if message.Order.OrderID.OrderFlags != DYDXOrderFlagsShortTerm {
		t.Fatalf("order flags = %d, want short-term", message.Order.OrderID.OrderFlags)
	}
	if message.Order.TimeInForce != DYDXTimeInForceIOC {
		t.Fatalf("timeInForce = %d, want IOC", message.Order.TimeInForce)
	}
	if message.Order.GoodTilBlock != 120 {
		t.Fatalf("goodTilBlock = %d, want 120", message.Order.GoodTilBlock)
	}
	if message.Order.GoodTilBlockTime != 0 {
		t.Fatalf("goodTilBlockTime = %d, want 0", message.Order.GoodTilBlockTime)
	}
}

func TestDYDXCosmosRequestFactoryBuildsIOCOrderAsShortTerm(t *testing.T) {
	factory := NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder: NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
			ChainID:           "dydx-local-1",
			Account:           "dydx1owner",
			SubaccountNumber:  0,
			GoodTilBlockDelta: 20,
			GoodTilSeconds:    3600,
			Markets:           validDYDXMarketConfigs(),
		}),
		HeightProvider: staticDYDXHeightProvider{height: 100},
		TimeProvider:   staticDYDXBlockTimeProvider{blockTimeUnix: 1786900000},
	})

	params, err := factory.ParamsForIntent(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"userId":        "user-1",
			"dydxAccount":   "dydx1userwallet",
			"clientOrderId": "123",
			"market":        "ETH-USD",
			"side":          "sell",
			"orderType":     "ioc",
			"price":         "2400",
			"size":          "2",
		},
	})

	if err != nil {
		t.Fatalf("ParamsForIntent() error = %v", err)
	}
	tx := params[DYDXCosmosTxParamKey].(DYDXCosmosTxRequest)
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if message.Order.OrderID.OrderFlags != DYDXOrderFlagsShortTerm {
		t.Fatalf("order flags = %d, want short-term", message.Order.OrderID.OrderFlags)
	}
	if message.Order.TimeInForce != DYDXTimeInForceIOC {
		t.Fatalf("timeInForce = %d, want IOC", message.Order.TimeInForce)
	}
	if message.Order.GoodTilBlock != 120 {
		t.Fatalf("goodTilBlock = %d, want 120", message.Order.GoodTilBlock)
	}
	if message.Order.GoodTilBlockTime != 0 {
		t.Fatalf("goodTilBlockTime = %d, want 0", message.Order.GoodTilBlockTime)
	}
}

func TestDYDXCosmosOrderBuilderBuildsStatefulCancelOrder(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1owner",
		SubaccountNumber: 2,
		GoodTilSeconds:   3600,
		Markets:          []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}},
	})

	tx, err := builder.BuildCancelOrder(DYDXCancelOrder{
		ID:            "order-1-cancel",
		UserID:        "user-1",
		OrderID:       "order-1",
		ClientOrderID: "123",
		Market:        "ETH-USD",
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildCancelOrder() error = %v", err)
	}
	if tx.Message.TypeURL != DYDXCosmosMsgCancelOrderTypeURL {
		t.Fatalf("typeURL = %q, want %q", tx.Message.TypeURL, DYDXCosmosMsgCancelOrderTypeURL)
	}
	message, ok := tx.Message.Value.(DYDXCosmosMsgCancelOrder)
	if !ok {
		t.Fatalf("message value = %T, want DYDXCosmosMsgCancelOrder", tx.Message.Value)
	}
	if message.OrderID.ClientID != 123 || message.OrderID.ClobPairID != 7 {
		t.Fatalf("order id = %+v", message.OrderID)
	}
	if message.OrderID.SubaccountID.Owner != "dydx1owner" || message.OrderID.SubaccountID.Number != 2 {
		t.Fatalf("subaccount id = %+v", message.OrderID.SubaccountID)
	}
	if message.OrderID.OrderFlags != DYDXOrderFlagsLongTerm {
		t.Fatalf("order flags = %d, want %d", message.OrderID.OrderFlags, DYDXOrderFlagsLongTerm)
	}
	if message.GoodTilBlock != 0 {
		t.Fatalf("goodTilBlock = %d, want 0", message.GoodTilBlock)
	}
	if message.GoodTilBlockTime != 1786903600 {
		t.Fatalf("goodTilBlockTime = %d, want 1786903600", message.GoodTilBlockTime)
	}
}

func TestDYDXCosmosOrderBuilderUsesOrderDYDXAccount(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1legacy",
		SubaccountNumber: 0,
		GoodTilSeconds:   3600,
		Markets:          validDYDXMarketConfigs(),
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-1",
		DYDXAccount:   "dydx1userwallet",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if tx.Account != "dydx1userwallet" || message.Order.OrderID.SubaccountID.Owner != "dydx1userwallet" {
		t.Fatalf("tx account=%q owner=%q, want dydx1userwallet", tx.Account, message.Order.OrderID.SubaccountID.Owner)
	}
}

func TestDYDXCosmosOrderBuilderRequiresOrderOrFallbackAccount(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:        "dydx-local-1",
		GoodTilSeconds: 3600,
		Markets:        validDYDXMarketConfigs(),
	})

	_, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-1",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
	}, 1786900000)

	if !errors.Is(err, ErrDYDXAccountNotConfigured) {
		t.Fatalf("BuildPlaceOrder() error = %v, want %v", err, ErrDYDXAccountNotConfigured)
	}
}

func TestDYDXCosmosOrderBuilderBuildsPostOnlyPlaceOrder(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1owner",
		SubaccountNumber: 3,
		GoodTilSeconds:   3600,
		Markets:          []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}},
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-post-only",
		ClientOrderID: "124",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "post_only",
		Price:         "2500",
		Size:          "1",
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	order := message.Order
	if order.TimeInForce != DYDXTimeInForcePostOnly {
		t.Fatalf("timeInForce = %d, want %d", order.TimeInForce, DYDXTimeInForcePostOnly)
	}
	if order.Side != DYDXOrderSideSell {
		t.Fatalf("side = %d, want %d", order.Side, DYDXOrderSideSell)
	}
	if order.OrderID.SubaccountID.Number != 3 {
		t.Fatalf("subaccount number = %d, want 3", order.OrderID.SubaccountID.Number)
	}
	if order.GoodTilBlockTime != 1786903600 {
		t.Fatalf("goodTilBlockTime = %d, want 1786903600", order.GoodTilBlockTime)
	}
}

func TestDYDXCosmosOrderBuilderSupportsFractionalQuantumsPerUnit(t *testing.T) {
	builder := NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
		ChainID:          "dydx-local-1",
		Account:          "dydx1owner",
		SubaccountNumber: 0,
		GoodTilSeconds:   3600,
		Markets:          []DYDXMarketConfig{{Market: "PEPE-USD", ClobPairID: 28, QuantumsPerUnit: "0.1", SubticksPerUnit: "10000000000000000"}},
	})

	tx, err := builder.BuildPlaceOrder(DYDXPlaceOrder{
		ID:            "order-pepe",
		ClientOrderID: "125",
		Market:        "PEPE-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "0.0000025",
		Size:          "10",
	}, 1786900000)

	if err != nil {
		t.Fatalf("BuildPlaceOrder() error = %v", err)
	}
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if message.Order.Quantums != 1 || message.Order.Subticks != 25000000000 {
		t.Fatalf("order = %+v", message.Order)
	}
}

func TestDYDXCosmosOrderBuilderRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		config  DYDXCosmosOrderBuilderConfig
		order   DYDXPlaceOrder
		height  uint64
		wantErr error
	}{
		{
			name:    "missing market mapping",
			config:  DYDXCosmosOrderBuilderConfig{ChainID: "dydx-local-1", Account: "dydx1owner"},
			order:   DYDXPlaceOrder{ID: "order-1", ClientOrderID: "123", Market: "ETH-USD", Side: "buy", OrderType: "limit", Price: "2500", Size: "1"},
			height:  100,
			wantErr: ErrDYDXMarketNotConfigured,
		},
		{
			name:    "non numeric client id",
			config:  validDYDXCosmosBuilderConfig(),
			order:   DYDXPlaceOrder{ID: "order-1", ClientOrderID: "client-order-1", Market: "ETH-USD", Side: "buy", OrderType: "limit", Price: "2500", Size: "1"},
			height:  100,
			wantErr: ErrInvalidDYDXClientOrderID,
		},
		{
			name:    "stateful order time window too large",
			config:  DYDXCosmosOrderBuilderConfig{ChainID: "dydx-local-1", Account: "dydx1owner", GoodTilSeconds: dydxStatefulOrderTimeWindowSeconds + 1, Markets: validDYDXMarketConfigs()},
			order:   DYDXPlaceOrder{ID: "order-1", ClientOrderID: "123", Market: "ETH-USD", Side: "buy", OrderType: "limit", Price: "2500", Size: "1"},
			height:  100,
			wantErr: ErrDYDXGoodTilSecondsTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewDYDXCosmosOrderBuilder(tt.config)

			_, err := builder.BuildPlaceOrder(tt.order, tt.height)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("BuildPlaceOrder() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDYDXCosmosRequestFactoryBuildsParamsWithCurrentBlockTime(t *testing.T) {
	factory := NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder:      NewDYDXCosmosOrderBuilder(validDYDXCosmosBuilderConfig()),
		TimeProvider: staticDYDXBlockTimeProvider{blockTimeUnix: 1786900000},
	})

	params, err := factory.ParamsForIntent(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"userId":        "user-1",
			"dydxAccount":   "dydx1userwallet",
			"clientOrderId": "123",
			"market":        "ETH-USD",
			"side":          "sell",
			"orderType":     "limit",
			"price":         "2400",
			"size":          "2",
		},
	})

	if err != nil {
		t.Fatalf("ParamsForIntent() error = %v", err)
	}
	tx, ok := params[DYDXCosmosTxParamKey].(DYDXCosmosTxRequest)
	if !ok {
		t.Fatalf("params[%q] = %T, want DYDXCosmosTxRequest", DYDXCosmosTxParamKey, params[DYDXCosmosTxParamKey])
	}
	message := tx.Message.Value.(DYDXCosmosMsgPlaceOrder)
	if message.Order.Side != DYDXOrderSideSell || message.Order.TimeInForce != DYDXTimeInForceUnspecified {
		t.Fatalf("order side/timeInForce = %+v", message.Order)
	}
	if tx.Account != "dydx1userwallet" || message.Order.OrderID.SubaccountID.Owner != "dydx1userwallet" {
		t.Fatalf("tx account=%q owner=%q, want dydx1userwallet", tx.Account, message.Order.OrderID.SubaccountID.Owner)
	}
	if message.Order.GoodTilBlockTime != 1787504800 {
		t.Fatalf("goodTilBlockTime = %d, want 1787504800", message.Order.GoodTilBlockTime)
	}
}

func TestDYDXCosmosRequestFactoryRequiresBlockTimeProvider(t *testing.T) {
	factory := NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder: NewDYDXCosmosOrderBuilder(validDYDXCosmosBuilderConfig()),
	})

	_, err := factory.ParamsForIntent(context.Background(), DYDXIntent{
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

	if !errors.Is(err, ErrDYDXBlockTimeProviderNotConfigured) {
		t.Fatalf("ParamsForIntent() error = %v, want %v", err, ErrDYDXBlockTimeProviderNotConfigured)
	}
}

func TestDYDXHTTPBlockHeightProviderReadsCometBFTStatusResponse(t *testing.T) {
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-Tyche-Internal-Key")
		_, _ = w.Write([]byte(`{"result":{"sync_info":{"latest_block_height":"456","latest_block_time":"2026-08-17T03:12:00Z"}}}`))
	}))
	defer server.Close()
	provider := NewDYDXHTTPBlockHeightProvider(DYDXHTTPBlockHeightProviderConfig{
		URL:    server.URL,
		APIKey: "internal-key",
	})

	height, err := provider.CurrentBlockHeight(context.Background())

	if err != nil {
		t.Fatalf("CurrentBlockHeight() error = %v", err)
	}
	if height != 456 {
		t.Fatalf("height = %d, want 456", height)
	}
	if gotAPIKey != "internal-key" {
		t.Fatalf("api key = %q, want internal-key", gotAPIKey)
	}
}

func TestDYDXHTTPBlockHeightProviderReadsCometBFTBlockTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"sync_info":{"latest_block_height":"456","latest_block_time":"2026-08-17T03:12:00.123456789Z"}}}`))
	}))
	defer server.Close()
	provider := NewDYDXHTTPBlockHeightProvider(DYDXHTTPBlockHeightProviderConfig{URL: server.URL})

	blockTimeUnix, err := provider.CurrentBlockTimeUnix(context.Background())

	if err != nil {
		t.Fatalf("CurrentBlockTimeUnix() error = %v", err)
	}
	if blockTimeUnix != 1786936320 {
		t.Fatalf("blockTimeUnix = %d, want 1786936320", blockTimeUnix)
	}
}

func validDYDXCosmosBuilderConfig() DYDXCosmosOrderBuilderConfig {
	return DYDXCosmosOrderBuilderConfig{
		ChainID:        "dydx-local-1",
		Account:        "dydx1owner",
		GoodTilSeconds: defaultDYDXGoodTilSeconds,
		Markets:        validDYDXMarketConfigs(),
	}
}

func validDYDXMarketConfigs() []DYDXMarketConfig {
	return []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
}

type staticDYDXHeightProvider struct {
	height uint64
}

func (p staticDYDXHeightProvider) CurrentBlockHeight(context.Context) (uint64, error) {
	return p.height, nil
}

type staticDYDXBlockTimeProvider struct {
	blockTimeUnix uint64
}

func (p staticDYDXBlockTimeProvider) CurrentBlockTimeUnix(context.Context) (uint64, error) {
	return p.blockTimeUnix, nil
}
