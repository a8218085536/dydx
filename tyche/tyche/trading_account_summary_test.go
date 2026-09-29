package tyche

import (
	"context"
	"testing"
)

func TestTradingAccountSummaryServiceIncludesPositionsOpenOrdersAndStats(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "10000")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusSubmitted},
	})
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, tradingStore, ledger)

	filledOrder, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "filled-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "250",
	})
	if err != nil {
		t.Fatalf("PlaceOrder(filled) error = %v", err)
	}
	filledOrder.Status = TradingOrderStatusFilled
	if err := tradingStore.SaveOrder(context.Background(), filledOrder); err != nil {
		t.Fatalf("SaveOrder(filled) error = %v", err)
	}
	if err := syncStore.SaveFill(context.Background(), TradingFill{
		EventID:            "fill-1",
		TradeHash:          "trade-1",
		UserID:             "user-1",
		OrderID:            filledOrder.ID,
		Market:             "ETH-USD",
		Asset:              "USDC",
		ReleasedMargin:     "250",
		Fee:                "3.75",
		RealizedPNL:        "12.5",
		OrderStatus:        TradingOrderStatusFilled,
		LiquidityRole:      "taker",
		FillAmount:         "1000000000",
		MakerFee:           "-0.75",
		TakerFee:           "3.75",
		MakerOrderSubticks: "2500000000",
	}); err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-1",
		UserID:           "user-1",
		Market:           "ETH-USD",
		SubaccountNumber: 0,
		Size:             "0.5",
		EntryPrice:       "2500",
		MarkPrice:        "2520",
		UnrealizedPNL:    "10",
		MarginAsset:      "USDC",
		Margin:           "125",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	openOrder, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "open-order-1",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "limit",
		Price:         "2600",
		Size:          "0.1",
		MarginAsset:   "USDC",
		MarginAmount:  "26",
	})
	if err != nil {
		t.Fatalf("PlaceOrder(open) error = %v", err)
	}

	service := NewTradingAccountSummaryService(tradingStore, syncService, []DYDXMarketConfig{
		{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000", SubticksPerUnit: "1000000"},
	})
	summary, err := service.Summary(context.Background(), TradingAccountSummaryRequest{UserID: "user-1"})

	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Stats.CumulativeTurnoverUSD != "2500" || summary.Stats.RealizedPNLUSD != "12.5" || summary.Stats.UnrealizedPNLUSD != "10" {
		t.Fatalf("stats = %+v, want turnover=2500 realized=12.5 unrealized=10", summary.Stats)
	}
	if summary.Stats.TradeCount != 1 || summary.Stats.MakerFeeRate != "-0.0003" || summary.Stats.TakerFeeRate != "0.0015" {
		t.Fatalf("stats = %+v, want trade count and ETH major fee rates", summary.Stats)
	}
	if summary.Stats.CumulativeFeeRebateUSD != "0.75" || summary.Stats.NetFeeUSD != "3" {
		t.Fatalf("fee stats = %+v, want rebate=0.75 netFee=3", summary.Stats)
	}
	if len(summary.Positions) != 1 || summary.Positions[0].Market != "ETH-USD" || summary.Positions[0].UnrealizedPNLUSD != "10" || summary.Positions[0].MarginUSD != "125" {
		t.Fatalf("positions = %+v, want ETH position with pnl and margin", summary.Positions)
	}
	if len(summary.OpenOrders) != 1 || summary.OpenOrders[0].ClientOrderID != openOrder.ClientOrderID || summary.OpenOrders[0].OpenedAt == 0 {
		t.Fatalf("open orders = %+v, want open order with openedAt", summary.OpenOrders)
	}
}

func TestTradingAccountSummaryServiceExcludesShortTermOrdersFromOpenOrders(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	orders := []TradingOrder{
		{
			ID:            "resting-limit",
			UserID:        "user-1",
			ClientOrderID: "resting-limit-client",
			Market:        "FDEX-USD",
			OrderType:     TradingOrderTypeLimit,
			Status:        TradingOrderStatusSubmitted,
			MarginUSD:     "25",
		},
		{
			ID:                 "market-partial",
			UserID:             "user-1",
			ClientOrderID:      "market-partial-client",
			Market:             "FDEX-USD",
			OrderType:          TradingOrderTypeMarket,
			ExecutionOrderType: TradingOrderTypeIOC,
			Status:             TradingOrderStatusPartialFilled,
			MarginUSD:          "100",
		},
		{
			ID:                 "ioc-submitted",
			UserID:             "user-1",
			ClientOrderID:      "ioc-submitted-client",
			Market:             "FDEX-USD",
			OrderType:          TradingOrderTypeLimit,
			ExecutionOrderType: TradingOrderTypeIOC,
			Status:             TradingOrderStatusSubmitted,
			MarginUSD:          "200",
		},
		{
			ID:                 "market-cancel-pending",
			UserID:             "user-1",
			ClientOrderID:      "market-cancel-pending-client",
			Market:             "FDEX-USD",
			OrderType:          TradingOrderTypeMarket,
			ExecutionOrderType: TradingOrderTypeIOC,
			Status:             TradingOrderStatusCancelPending,
			MarginUSD:          "300",
		},
		{
			ID:            "resting-partial",
			UserID:        "user-1",
			ClientOrderID: "resting-partial-client",
			Market:        "FDEX-USD",
			OrderType:     TradingOrderTypeLimit,
			Status:        TradingOrderStatusPartialFilled,
			MarginUSD:     "400",
		},
	}
	for _, order := range orders {
		if err := tradingStore.SaveOrder(context.Background(), order); err != nil {
			t.Fatalf("SaveOrder(%s) error = %v", order.ID, err)
		}
	}

	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, tradingStore, NewLedgerService(NewMemoryLedgerStore()))
	service := NewTradingAccountSummaryService(tradingStore, syncService, nil)
	summary, err := service.Summary(context.Background(), TradingAccountSummaryRequest{UserID: "user-1"})

	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if len(summary.OpenOrders) != 1 || summary.OpenOrders[0].ClientOrderID != "resting-limit-client" {
		t.Fatalf("open orders = %+v, want only the resting limit order", summary.OpenOrders)
	}
}
