package tyche

import (
	"context"
	"testing"
)

func TestTradingReconcileServiceMarksBroadcastedOrderSubmittedAfterTxCodeZero(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:              "order-1",
		UserID:          "user-1",
		DYDXAccount:     "dydx1abc",
		ClientOrderID:   "123",
		Market:          "ETH-USD",
		Side:            "BUY",
		Size:            "1",
		MarginAsset:     "DEDX",
		MarginAmount:    "4",
		DYDXOrderTxHash: "ORDERHASH",
		Status:          TradingOrderStatusBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "ORDERHASH"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: true, FillAmount: "0"},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "order-1")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusSubmitted {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusSubmitted)
	}
	if source.request.ClobPairID != 1 || source.request.OrderFlags != DYDXOrderFlagsLongTerm {
		t.Fatalf("reconcile request = %+v", source.request)
	}
}

func TestTradingReconcileServiceUsesBuiltInDEDXUSDIsolatedSubaccount(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:              "order-dedx",
		UserID:          "user-1",
		DYDXAccount:     "dydx1abc",
		ClientOrderID:   "398831676",
		Market:          "FDEX-USD",
		Side:            "SELL",
		Size:            "20",
		MarginAsset:     "DEDX",
		MarginAmount:    "20",
		DYDXOrderTxHash: "ORDERHASH-DEDX",
		Status:          TradingOrderStatusBroadcasted,
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "ORDERHASH-DEDX"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: true, FillAmount: "0"},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "FDEX-USD",
			ClobPairID:      302,
			QuantumsPerUnit: "1000000",
		}},
	})

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}
	if source.request.SubaccountNumber != 127 {
		t.Fatalf("reconcile subaccountNumber = %d, want 127", source.request.SubaccountNumber)
	}
}

func TestTradingReconcileServiceMarksFilledFromDirectionMatchedPosition(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:              "order-2",
		UserID:          "user-1",
		DYDXAccount:     "dydx1abc",
		ClientOrderID:   "456",
		Market:          "ETH-USD",
		Side:            "SELL",
		Size:            "0.001",
		MarginAsset:     "DEDX",
		MarginAmount:    "473000000000000000",
		MarginUSD:       "1.892",
		DYDXOrderTxHash: "ORDERHASH2",
		Status:          TradingOrderStatusBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "ORDERHASH2"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount: DYDXSubaccountReconcileSnapshot{
				Found: true,
				PerpetualPositions: []DYDXPerpetualPositionReconcileSnapshot{{
					PerpetualID: 1,
					Quantums:    "-1000000",
				}},
			},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "order-2")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusFilled {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusFilled)
	}
	position, ok, err := syncStore.GetPosition(context.Background(), "user-1", "ETH-USD")
	if err != nil || !ok {
		t.Fatalf("GetPosition() position=%+v ok=%v err=%v", position, ok, err)
	}
	if position.Size != "-0.001" || position.MarginAsset != "DEDX" || position.Margin != "1.892" {
		t.Fatalf("position = %+v", position)
	}
}

func TestTradingReconcileServiceDoesNotMarkBuyFilledFromShortPosition(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:              "order-3",
		UserID:          "user-1",
		DYDXAccount:     "dydx1abc",
		ClientOrderID:   "789",
		Market:          "ETH-USD",
		Side:            "BUY",
		Size:            "0.001",
		MarginAsset:     "DEDX",
		MarginAmount:    "4",
		DYDXOrderTxHash: "ORDERHASH3",
		Status:          TradingOrderStatusBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "ORDERHASH3"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount: DYDXSubaccountReconcileSnapshot{
				Found: true,
				PerpetualPositions: []DYDXPerpetualPositionReconcileSnapshot{{
					PerpetualID: 1,
					Quantums:    "-1000000",
				}},
			},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "order-3")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusSubmitted {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusSubmitted)
	}
}

func TestTradingReconcileServiceMarksCloseOrderClosedWhenStatefulOrderGone(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-before-close",
		UserID:           "user-1",
		Market:           "ETH-USD",
		SubaccountNumber: 0,
		Size:             "0.001",
		MarginAsset:      "DEDX",
		Margin:           "1.892",
	}); err != nil {
		t.Fatalf("SyncPosition() existing position error = %v", err)
	}
	order := TradingOrder{
		ID:              "close-order-1",
		UserID:          "user-1",
		DYDXAccount:     "dydx1abc",
		ClientOrderID:   "987",
		Market:          "ETH-USD",
		Side:            "SELL",
		Size:            "0.001",
		MarginAsset:     "DEDX",
		MarginAmount:    "0",
		DYDXOrderTxHash: "CLOSETX",
		ClosePosition:   true,
		ReduceOnly:      true,
		Status:          TradingOrderStatusCloseBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "CLOSETX"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "close-order-1")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusClosed {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusClosed)
	}
	if position, err := syncService.GetPositionForSubaccount(context.Background(), "user-1", "ETH-USD", 0); err == nil {
		t.Fatalf("position after close = %+v, want cleared", position)
	} else if err != ErrTradingSyncEventNotFound {
		t.Fatalf("position after close error = %v, want %v", err, ErrTradingSyncEventNotFound)
	}
}

func TestTradingReconcileServiceMarksUnfilledIOCCloseOrderCancelledWhenPositionRemains(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:               "close-order-unfilled-ioc",
		UserID:           "user-1",
		DYDXAccount:      "dydx1abc",
		SubaccountNumber: 127,
		ClientOrderID:    "1660418600",
		Market:           "FDEX-USD",
		Side:             "buy",
		Size:             "20",
		MarginAsset:      "USDT",
		MarginAmount:     "0",
		DYDXOrderTxHash:  "CLOSETX-UNFILLED",
		ClosePosition:    true,
		ReduceOnly:       true,
		Status:           TradingOrderStatusCloseBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: false, TxHash: "CLOSETX-UNFILLED"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount: DYDXSubaccountReconcileSnapshot{
				Found: true,
				PerpetualPositions: []DYDXPerpetualPositionReconcileSnapshot{{
					PerpetualID: 302,
					Quantums:    "-20000000",
				}},
			},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "FDEX-USD",
			ClobPairID:      302,
			QuantumsPerUnit: "1000000",
		}},
	})

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "close-order-unfilled-ioc")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusCloseCancelled {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusCloseCancelled)
	}
}

func TestTradingReconcileServiceSettlesClosedPositionOnce(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:                  "close-order-settlement-1",
		UserID:              "user-1",
		NECOAddress:         "0x0000000000000000000000000000000000000001",
		DYDXAccount:         "dydx1abc",
		ClientOrderID:       "close-client-1",
		Market:              "ETH-USD",
		Side:                "SELL",
		Size:                "0.001",
		MarginAsset:         "DEDX",
		MarginAmount:        "0",
		MarginUSD:           "20",
		DYDXFundingQuantums: "20000000",
		DYDXOrderTxHash:     "CLOSETX2",
		ClosePosition:       true,
		ReduceOnly:          true,
		Status:              TradingOrderStatusCloseBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: false, TxHash: "CLOSETX2"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount:    DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	settler := &recordingCloseSettlementService{}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})
	service.SetCloseSettlementService(settler)

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	if settler.calls != 1 {
		t.Fatalf("settlement calls = %d, want 1", settler.calls)
	}
	if settler.request.CloseOrder.Status != TradingOrderStatusClosed {
		t.Fatalf("settled order status = %q, want closed", settler.request.CloseOrder.Status)
	}
	if settler.request.Reconcile.Subaccount.Found != true {
		t.Fatalf("settlement reconcile = %+v", settler.request.Reconcile)
	}
}

func TestTradingReconcileServiceSettlesCancelledFundedOrderOnce(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:                "order-cancel-1",
		UserID:            "user-1",
		DYDXAccount:       "dydx1abc",
		ClientOrderID:     "client-cancel-1",
		Market:            "ETH-USD",
		Side:              "BUY",
		Size:              "1",
		MarginAsset:       "DEDX",
		MarginAmount:      "5000000000000000000",
		MarginUSD:         "20",
		DYDXFundingTxHash: "funding-tx-1",
		DYDXOrderTxHash:   "place-tx-1",
		DYDXCancelTxHash:  "cancel-tx-1",
		Status:            TradingOrderStatusCancelPending,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "cancel-tx-1"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount:    DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	settler := &recordingCloseSettlementService{}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})
	service.SetCloseSettlementService(settler)

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "order-cancel-1")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusCancelled {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusCancelled)
	}
	if source.request.TxHash != "cancel-tx-1" {
		t.Fatalf("reconcile tx hash = %q, want cancel-tx-1", source.request.TxHash)
	}
	if settler.cancelCalls != 1 || settler.cancelRequest.Order.ID != order.ID {
		t.Fatalf("cancel settlement calls=%d request=%+v", settler.cancelCalls, settler.cancelRequest)
	}
}

func TestTradingReconcileServiceSettlesFailedFundedOrderWhenTxRejected(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:                "order-rejected-1",
		UserID:            "user-1",
		NECOAddress:       "0x0000000000000000000000000000000000000001",
		DYDXAccount:       "dydx1abc",
		ClientOrderID:     "client-rejected-1",
		Market:            "ETH-USD",
		Side:              "BUY",
		Size:              "1",
		MarginAsset:       "DEDX",
		MarginAmount:      "5000000000000000000",
		MarginUSD:         "20",
		DYDXFundingTxHash: "funding-tx-1",
		DYDXOrderTxHash:   "place-tx-1",
		Status:            TradingOrderStatusBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 12, TxHash: "place-tx-1", RawLog: "invalid place order"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount:    DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	settler := &recordingCloseSettlementService{}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
		}},
	})
	service.SetCloseSettlementService(settler)

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "order-rejected-1")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusRejected {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusRejected)
	}
	if settler.failedCalls != 1 || settler.failedRequest.Order.ID != order.ID {
		t.Fatalf("failed settlement calls=%d request=%+v", settler.failedCalls, settler.failedRequest)
	}
	if settler.failedRequest.Order.Status != TradingOrderStatusRejected {
		t.Fatalf("failed settlement order status = %q, want rejected", settler.failedRequest.Order.Status)
	}
}

func TestTradingReconcileServiceSettlesUnfilledIOCOrderAsCancelled(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:                 "market-ioc-unfilled-1",
		UserID:             "user-1",
		NECOAddress:        "0x0000000000000000000000000000000000000001",
		DYDXAccount:        "dydx1abc",
		SubaccountNumber:   127,
		ClientOrderID:      "507936101",
		Market:             "FDEX-USD",
		Side:               "BUY",
		OrderType:          TradingOrderTypeMarket,
		ExecutionOrderType: TradingOrderTypeIOC,
		Price:              "0.216",
		Size:               "105",
		MarginAsset:        "FDEX",
		MarginAmount:       "105000000000000000000",
		MarginUSD:          "22.68",
		DYDXFundingTxHash:  "funding-tx-1",
		DYDXOrderTxHash:    "market-tx-1",
		Status:             TradingOrderStatusBroadcasted,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "market-tx-1"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount:    DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	settler := &recordingCloseSettlementService{}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		SubaccountNumber: 0,
		Markets: []DYDXMarketConfig{{
			Market:          "FDEX-USD",
			ClobPairID:      302,
			QuantumsPerUnit: "1000000",
		}},
	})
	service.SetCloseSettlementService(settler)

	if _, err := service.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}

	stored, ok, err := orderStore.GetOrderByID(context.Background(), "user-1", "market-ioc-unfilled-1")
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.Status != TradingOrderStatusCancelled {
		t.Fatalf("status = %q, want %q", stored.Status, TradingOrderStatusCancelled)
	}
	if settler.cancelCalls != 1 || settler.cancelRequest.Order.ID != order.ID {
		t.Fatalf("cancel settlement calls=%d request=%+v", settler.cancelCalls, settler.cancelRequest)
	}
}

func TestTradingReconcileWorkerOnlyPollsReconcilableOrders(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncService := NewTradingSyncService(NewMemoryTradingSyncStore(), orderStore, NewLedgerService(NewMemoryLedgerStore()))
	orders := []TradingOrder{
		{ID: "order-1", UserID: "user-1", ClientOrderID: "1", Market: "ETH-USD", Side: "BUY", Size: "1", DYDXAccount: "dydx1abc", DYDXOrderTxHash: "HASH1", Status: TradingOrderStatusBroadcasted},
		{ID: "order-2", UserID: "user-1", ClientOrderID: "2", Market: "ETH-USD", Side: "BUY", Size: "1", DYDXAccount: "dydx1abc", DYDXOrderTxHash: "HASH2", Status: TradingOrderStatusFilled},
		{ID: "order-3", UserID: "user-1", ClientOrderID: "3", Market: "ETH-USD", Side: "BUY", Size: "1", DYDXAccount: "dydx1abc", DYDXOrderTxHash: "HASH3", Status: TradingOrderStatusPartialFilled},
	}
	for _, order := range orders {
		if err := orderStore.SaveOrder(context.Background(), order); err != nil {
			t.Fatalf("SaveOrder() error = %v", err)
		}
	}
	source := &recordingDYDXReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: true},
		},
	}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		Markets: []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1"}},
	})
	worker := NewTradingReconcileWorker(orderStore, service, 10)

	if err := worker.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if source.calls != 2 {
		t.Fatalf("reconcile calls = %d, want 2", source.calls)
	}
}

func TestTradingReconcileWorkerSettlesFailedFundedOrderWithoutOrderTx(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncService := NewTradingSyncService(NewMemoryTradingSyncStore(), orderStore, NewLedgerService(NewMemoryLedgerStore()))
	order := TradingOrder{
		ID:                "submit-failed-no-tx",
		UserID:            "user-1",
		NECOAddress:       "0x0000000000000000000000000000000000000001",
		DYDXAccount:       "dydx1abc",
		ClientOrderID:     "client-submit-failed-no-tx",
		Market:            "ETH-USD",
		Side:              "BUY",
		Size:              "1",
		MarginAsset:       "DEDX",
		MarginAmount:      "5000000000000000000",
		MarginUSD:         "20",
		DYDXFundingTxHash: "funding-tx-1",
		Status:            TradingOrderStatusSubmitFailed,
	}
	if err := orderStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	source := &recordingDYDXReconcileSource{}
	settler := &recordingCloseSettlementService{}
	service := NewTradingReconcileService(source, syncService, TradingReconcileConfig{
		Markets: []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000"}},
	})
	service.SetCloseSettlementService(settler)
	worker := NewTradingReconcileWorker(orderStore, service, 10)

	if err := worker.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if source.calls != 0 {
		t.Fatalf("reconcile source calls = %d, want 0", source.calls)
	}
	if settler.failedCalls != 1 || settler.failedRequest.Order.ID != order.ID {
		t.Fatalf("failed settlement calls=%d request=%+v", settler.failedCalls, settler.failedRequest)
	}
}

type recordingDYDXReconcileSource struct {
	request DYDXOrderReconcileRequest
	result  DYDXOrderReconcileResult
	calls   int
}

func (s *recordingDYDXReconcileSource) ReconcileOrder(ctx context.Context, request DYDXOrderReconcileRequest) (DYDXOrderReconcileResult, error) {
	s.calls++
	s.request = request
	return s.result, nil
}

type recordingCloseSettlementService struct {
	calls         int
	request       CloseSettlementRequest
	cancelCalls   int
	cancelRequest CancelSettlementRequest
	failedCalls   int
	failedRequest FailedOrderSettlementRequest
}

func (s *recordingCloseSettlementService) SettleClosedPosition(ctx context.Context, request CloseSettlementRequest) (TradingSettlement, error) {
	s.calls++
	s.request = request
	return TradingSettlement{ID: "settlement-1", Status: TradingSettlementStatusNECOPayoutBroadcasted}, nil
}

func (s *recordingCloseSettlementService) SettleCancelledOrder(ctx context.Context, request CancelSettlementRequest) (TradingSettlement, error) {
	s.cancelCalls++
	s.cancelRequest = request
	return TradingSettlement{ID: "cancel-settlement-1", Status: TradingSettlementStatusNECOPayoutBroadcasted}, nil
}

func (s *recordingCloseSettlementService) SettleFailedOrder(ctx context.Context, request FailedOrderSettlementRequest) (TradingSettlement, error) {
	s.failedCalls++
	s.failedRequest = request
	return TradingSettlement{ID: "failed-settlement-1", Status: TradingSettlementStatusNECOPayoutBroadcasted}, nil
}
