package tyche

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestTradingWithMockSidecarRefundsSynchronousRejectedOrder(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{
		SubmitStatus: TradingOrderStatusRejected,
		RejectReason: "mock rejected",
	})
	server := httptest.NewServer(sidecar.Handler())
	defer server.Close()

	conf := minimalConfig()
	conf.DYDXTradingURL = server.URL + "/dydx/trading"
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

	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	service := NewTradingService(tradingStore, ledger, runner)

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})

	if err != ErrDYDXOrderRejected {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, ErrDYDXOrderRejected)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
	order, ok, err := tradingStore.GetOrderByClientID(context.Background(), "user-1", "123")
	if err != nil || !ok {
		t.Fatalf("GetOrderByClientID() order=%+v ok=%v err=%v", order, ok, err)
	}
	if order.Status != TradingOrderStatusRejected {
		t.Fatalf("order status = %q, want rejected", order.Status)
	}
	if len(sidecar.Requests()) != 1 {
		t.Fatalf("mock sidecar requests = %d, want 1", len(sidecar.Requests()))
	}
}

func TestTradingWithMockSidecarSyncsPartialFillCancelledRemainderAndFilledSettlement(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "200")
	tradingStore := NewMemoryTradingStore()

	runner := newMockSidecarRunner(t, nil)
	trading := NewTradingService(tradingStore, ledger, runner)
	firstOrder, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "101",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("first PlaceOrder() error = %v", err)
	}
	secondOrder, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "102",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "limit",
		Price:         "2600",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "30",
	})
	if err != nil {
		t.Fatalf("second PlaceOrder() error = %v", err)
	}

	eventSidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{
		Fills: []TradingFillSyncRequest{
			{
				EventID:        "fill-partial-1",
				UserID:         "user-1",
				OrderID:        firstOrder.ID,
				Market:         "ETH-USD",
				Asset:          "USDC",
				ReleasedMargin: "5",
				Fee:            "1",
				RealizedPNL:    "0",
				OrderStatus:    TradingOrderStatusPartialFilled,
			},
			{
				EventID:        "fill-filled-1",
				UserID:         "user-1",
				OrderID:        secondOrder.ID,
				Market:         "ETH-USD",
				Asset:          "USDC",
				ReleasedMargin: "30",
				Fee:            "2",
				RealizedPNL:    "7",
				OrderStatus:    TradingOrderStatusFilled,
			},
			{
				EventID:        "fill-filled-1",
				UserID:         "user-1",
				OrderID:        secondOrder.ID,
				Market:         "ETH-USD",
				Asset:          "USDC",
				ReleasedMargin: "30",
				Fee:            "2",
				RealizedPNL:    "7",
				OrderStatus:    TradingOrderStatusFilled,
			},
		},
		OrderStatuses: []TradingOrderStatusSyncRequest{
			{
				EventID: "status-submitted-1",
				UserID:  "user-1",
				OrderID: firstOrder.ID,
				Status:  TradingOrderStatusSubmitted,
			},
			{
				EventID:      "status-cancelled-1",
				UserID:       "user-1",
				OrderID:      firstOrder.ID,
				Status:       TradingOrderStatusCancelled,
				RefundAsset:  "USDC",
				RefundAmount: "10",
			},
			{
				EventID:      "status-cancelled-1",
				UserID:       "user-1",
				OrderID:      firstOrder.ID,
				Status:       TradingOrderStatusCancelled,
				RefundAsset:  "USDC",
				RefundAmount: "10",
			},
		},
		NextFillsCursor:       "fills-1",
		NextOrderStatusCursor: "statuses-1",
		NextPositionsCursor:   "positions-1",
	})
	eventServer := httptest.NewServer(eventSidecar.Handler())
	defer eventServer.Close()

	syncService := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	worker := NewTradingSyncWorker(
		NewDYDXTradingEventHTTPSource(DYDXTradingEventHTTPSourceConfig{URL: eventServer.URL}),
		syncService,
		NewMemoryTradingSyncCursorStore(TradingSyncCursor{}),
	)
	if err := worker.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	firstStored, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", firstOrder.ID)
	if err != nil || !ok {
		t.Fatalf("first GetOrderByID() order=%+v ok=%v err=%v", firstStored, ok, err)
	}
	if firstStored.Status != TradingOrderStatusCancelled {
		t.Fatalf("first status = %q, want %q", firstStored.Status, TradingOrderStatusCancelled)
	}
	secondStored, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", secondOrder.ID)
	if err != nil || !ok {
		t.Fatalf("second GetOrderByID() order=%+v ok=%v err=%v", secondStored, ok, err)
	}
	if secondStored.Status != TradingOrderStatusFilled {
		t.Fatalf("second status = %q, want %q", secondStored.Status, TradingOrderStatusFilled)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "194" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=194 frozen=0", balance)
	}
}

func newMockSidecarRunner(t *testing.T, conf *Config) *DYDXRunner {
	t.Helper()
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{SubmitStatus: TradingOrderStatusSubmitted})
	server := httptest.NewServer(sidecar.Handler())
	t.Cleanup(server.Close)

	if conf == nil {
		conf = minimalConfig()
	}
	conf.DYDXTradingURL = server.URL + "/dydx/trading"
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
	return runner
}
