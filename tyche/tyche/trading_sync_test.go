package tyche

import (
	"context"
	"errors"
	"testing"
)

func TestTradingSyncServiceAppliesFillToLedgerAndMarksOrderFilled(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	fill, err := service.SyncFill(context.Background(), TradingFillSyncRequest{
		EventID:            "fill-1",
		TradeHash:          "height-10-tx-1-event-2",
		UserID:             "user-1",
		OrderID:            order.ID,
		Market:             "ETH-USD",
		Asset:              "USDC",
		ReleasedMargin:     "25",
		Fee:                "2",
		RealizedPNL:        "5",
		LiquidityRole:      "taker",
		MakerAddress:       "dydx1maker",
		TakerAddress:       "dydx1taker",
		MakerOrderID:       "dydx1maker:0:22:1",
		TakerOrderID:       "dydx1taker:0:33:1",
		ClobPairID:         1,
		FillAmount:         "1000000000",
		MakerFee:           "-10",
		TakerFee:           "45",
		MakerOrderSide:     "sell",
		TakerOrderSide:     "buy",
		MakerOrderQuantums: "1000000000",
		MakerOrderSubticks: "1818000000",
		TakerOrderQuantums: "2000000000",
		TakerOrderSubticks: "1819000000",
	})

	if err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}
	if fill.EventID != "fill-1" {
		t.Fatalf("fill event ID = %q, want fill-1", fill.EventID)
	}
	if fill.TradeHash != "height-10-tx-1-event-2" || fill.LiquidityRole != "taker" || fill.MakerAddress != "dydx1maker" || fill.TakerAddress != "dydx1taker" || fill.FillAmount != "1000000000" {
		t.Fatalf("fill trade details = %+v", fill)
	}
	storedFill, err := service.GetFill(context.Background(), "fill-1")
	if err != nil {
		t.Fatalf("GetFill() error = %v", err)
	}
	if storedFill.MakerFee != "-10" || storedFill.TakerFee != "45" || storedFill.MakerOrderSubticks != "1818000000" || storedFill.TakerOrderSubticks != "1819000000" {
		t.Fatalf("stored fill trade details = %+v", storedFill)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "103" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=103 frozen=0", balance)
	}
	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusFilled {
		t.Fatalf("order status = %q, want %q", storedOrder.Status, TradingOrderStatusFilled)
	}
}

func TestTradingSyncServiceEnrichesFillWithMakerTakerNECOAddresses(t *testing.T) {
	bindings := NewMemoryWalletBindingStore()
	if err := bindings.Save(context.Background(), WalletBinding{
		ID:          "maker-binding",
		UserID:      "maker-user",
		NECOAddress: "0x00000000000000000000000000000000000000aa",
		DYDXAddress: "dydx1maker",
		Status:      WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() maker binding error = %v", err)
	}
	if err := bindings.Save(context.Background(), WalletBinding{
		ID:          "taker-binding",
		UserID:      "taker-user",
		NECOAddress: "0x00000000000000000000000000000000000000bb",
		DYDXAddress: "dydx1taker",
		Status:      WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() taker binding error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), NewMemoryTradingStore(), NewLedgerService(NewMemoryLedgerStore()))
	service.SetWalletBindingLookup(bindings)

	fill, err := service.SyncFill(context.Background(), TradingFillSyncRequest{
		EventID:      "fill-addresses",
		TradeHash:    "trade-addresses",
		MakerAddress: "dydx1maker",
		TakerAddress: "dydx1taker",
	})
	if err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}
	if fill.MakerNECOAddress != "0x00000000000000000000000000000000000000aa" ||
		fill.TakerNECOAddress != "0x00000000000000000000000000000000000000bb" ||
		fill.CreatedAt == 0 {
		t.Fatalf("fill = %+v", fill)
	}
}

func TestTradingSyncServiceRejectsDuplicateFillEvent(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	request := TradingFillSyncRequest{
		EventID:        "fill-1",
		UserID:         "user-1",
		OrderID:        order.ID,
		Market:         "ETH-USD",
		Asset:          "USDC",
		ReleasedMargin: "25",
		Fee:            "2",
		RealizedPNL:    "5",
	}
	if _, err := service.SyncFill(context.Background(), request); err != nil {
		t.Fatalf("first SyncFill() error = %v", err)
	}

	_, err = service.SyncFill(context.Background(), request)

	if !errors.Is(err, ErrTradingSyncDuplicateEvent) {
		t.Fatalf("second SyncFill() error = %v, want %v", err, ErrTradingSyncDuplicateEvent)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "103" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want unchanged available=103 frozen=0", balance)
	}
}

func TestTradingSyncServiceKeepsOrderPartialFilledWhenFillSaysPartial(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	if _, err := service.SyncFill(context.Background(), TradingFillSyncRequest{
		EventID:        "fill-partial-1",
		UserID:         "user-1",
		OrderID:        order.ID,
		Market:         "ETH-USD",
		Asset:          "USDC",
		ReleasedMargin: "5",
		Fee:            "1",
		RealizedPNL:    "0",
		OrderStatus:    TradingOrderStatusPartialFilled,
	}); err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}

	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusPartialFilled {
		t.Fatalf("order status = %q, want %q", storedOrder.Status, TradingOrderStatusPartialFilled)
	}
}

func TestTradingSyncServiceMapsCloseFillToClosed(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	tradingStore := NewMemoryTradingStore()
	order := TradingOrder{
		ID:            "close-order-1",
		UserID:        "user-1",
		ClientOrderID: "close-client-1",
		Market:        "ETH-USD",
		Side:          "SELL",
		Size:          "0.001",
		MarginAsset:   "DEDX",
		MarginAmount:  "0",
		ClosePosition: true,
		ReduceOnly:    true,
		Status:        TradingOrderStatusCloseSubmitted,
	}
	if err := tradingStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	if _, err := service.SyncFill(context.Background(), TradingFillSyncRequest{
		EventID:        "close-fill-1",
		UserID:         "user-1",
		OrderID:        order.ID,
		Market:         "ETH-USD",
		Asset:          "DEDX",
		ReleasedMargin: "0",
		Fee:            "0",
		RealizedPNL:    "0",
		OrderStatus:    TradingOrderStatusFilled,
	}); err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}

	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusClosed {
		t.Fatalf("order status = %q, want %q", storedOrder.Status, TradingOrderStatusClosed)
	}
}

func TestTradingSyncServiceMapsCloseStatusEvents(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	order := TradingOrder{
		ID:            "close-order-1",
		UserID:        "user-1",
		ClientOrderID: "close-client-1",
		Market:        "ETH-USD",
		Side:          "SELL",
		Size:          "0.001",
		MarginAsset:   "DEDX",
		MarginAmount:  "0",
		ClosePosition: true,
		ReduceOnly:    true,
		Status:        TradingOrderStatusCloseBroadcasted,
	}
	if err := tradingStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, NewLedgerService(NewMemoryLedgerStore()))

	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID: "close-status-partial",
		UserID:  "user-1",
		OrderID: order.ID,
		Status:  TradingOrderStatusPartialFilled,
	}); err != nil {
		t.Fatalf("SyncOrderStatus(partial) error = %v", err)
	}
	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusClosePartialFilled {
		t.Fatalf("partial close status = %q, want %q", storedOrder.Status, TradingOrderStatusClosePartialFilled)
	}

	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID: "close-status-filled",
		UserID:  "user-1",
		OrderID: order.ID,
		Status:  TradingOrderStatusFilled,
	}); err != nil {
		t.Fatalf("SyncOrderStatus(filled) error = %v", err)
	}
	storedOrder, ok, err = tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusClosed {
		t.Fatalf("filled close status = %q, want %q", storedOrder.Status, TradingOrderStatusClosed)
	}
}

func TestTradingSyncServiceMatchesDYDXOrderIDByClientOrderID(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "33",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	if _, err := service.SyncFill(context.Background(), TradingFillSyncRequest{
		EventID:        "fill-dydx-1",
		UserID:         "user-1",
		OrderID:        "dydx1trader:0:33:7",
		Market:         "ETH-USD",
		Asset:          "USDC",
		ReleasedMargin: "0",
		Fee:            "2",
		RealizedPNL:    "0",
		OrderStatus:    TradingOrderStatusPartialFilled,
	}); err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}
	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID: "status-dydx-1",
		UserID:  "user-1",
		OrderID: "dydx1trader:0:33:7",
		Status:  TradingOrderStatusFilled,
		Reason:  "fully_filled",
	}); err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}

	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusFilled {
		t.Fatalf("order status = %q, want %q", storedOrder.Status, TradingOrderStatusFilled)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "73" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=73 frozen=0", balance)
	}
}

func TestTradingSyncServiceStoresPositionSnapshot(t *testing.T) {
	store := NewMemoryTradingSyncStore()
	service := NewTradingSyncService(store, NewMemoryTradingStore(), NewLedgerService(NewMemoryLedgerStore()))

	position, err := service.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:       "position-1",
		UserID:        "user-1",
		Market:        "ETH-USD",
		Size:          "1.5",
		EntryPrice:    "2500",
		MarkPrice:     "2600",
		UnrealizedPNL: "150",
		MarginAsset:   "USDC",
		Margin:        "500",
	})

	if err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	if position.Market != "ETH-USD" || position.UnrealizedPNL != "150" {
		t.Fatalf("position = %+v", position)
	}
	stored, ok, err := store.GetPosition(context.Background(), "user-1", "ETH-USD")
	if err != nil || !ok {
		t.Fatalf("GetPosition() position=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.EventID != "position-1" {
		t.Fatalf("stored EventID = %q, want position-1", stored.EventID)
	}
}

func TestTradingSyncServiceStoresPositionSnapshotsPerSubaccount(t *testing.T) {
	store := NewMemoryTradingSyncStore()
	service := NewTradingSyncService(store, NewMemoryTradingStore(), NewLedgerService(NewMemoryLedgerStore()))

	if _, err := service.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-default",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 0,
		Size:             "1",
		MarginAsset:      "DEDX",
		Margin:           "20",
	}); err != nil {
		t.Fatalf("SyncPosition() default error = %v", err)
	}
	if _, err := service.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-isolated",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "2",
		MarginAsset:      "DEDX",
		Margin:           "40",
	}); err != nil {
		t.Fatalf("SyncPosition() isolated error = %v", err)
	}

	position, err := service.GetPositionForSubaccount(context.Background(), "user-1", "FDEX-USD", 127)
	if err != nil {
		t.Fatalf("GetPositionForSubaccount() error = %v", err)
	}
	if position.EventID != "position-isolated" || position.Size != "2" || position.SubaccountNumber != 127 {
		t.Fatalf("position = %+v, want isolated subaccount position", position)
	}
	defaultPosition, err := service.GetPosition(context.Background(), "user-1", "FDEX-USD")
	if err != nil {
		t.Fatalf("GetPosition() error = %v", err)
	}
	if defaultPosition.EventID != "position-default" || defaultPosition.SubaccountNumber != 0 {
		t.Fatalf("default position = %+v, want default subaccount position", defaultPosition)
	}
}

func TestTradingSyncServiceClearsPositionSnapshotWhenSizeIsZero(t *testing.T) {
	store := NewMemoryTradingSyncStore()
	service := NewTradingSyncService(store, NewMemoryTradingStore(), NewLedgerService(NewMemoryLedgerStore()))

	if _, err := service.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-open",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "30",
		MarginAsset:      "FDEX",
		Margin:           "6",
	}); err != nil {
		t.Fatalf("SyncPosition() open error = %v", err)
	}
	if _, err := service.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-flat",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "0",
		MarginAsset:      "FDEX",
		Margin:           "0",
	}); err != nil {
		t.Fatalf("SyncPosition() flat error = %v", err)
	}

	if position, err := service.GetPositionForSubaccount(context.Background(), "user-1", "FDEX-USD", 127); err == nil {
		t.Fatalf("GetPositionForSubaccount() position=%+v, want not found", position)
	} else if err != ErrTradingSyncEventNotFound {
		t.Fatalf("GetPositionForSubaccount() error = %v, want %v", err, ErrTradingSyncEventNotFound)
	}
	positions, err := service.ListPositionsByUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListPositionsByUser() error = %v", err)
	}
	if len(positions) != 0 {
		t.Fatalf("positions = %+v, want none", positions)
	}
}

func TestTradingSyncServiceRefundsRejectedOrderChargeAndMarksRejected(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	status, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID: "status-1",
		UserID:  "user-1",
		OrderID: order.ID,
		Status:  TradingOrderStatusRejected,
		Reason:  "dydx rejected",
	})

	if err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}
	if status.Status != TradingOrderStatusRejected {
		t.Fatalf("status = %+v, want rejected", status)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusRejected {
		t.Fatalf("order status = %q, want rejected", storedOrder.Status)
	}
}

func TestTradingSyncServiceSettlesRejectedOrderAfterDYDXFunding(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	trading.SetDYDXSubaccountDepositor(&recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	trading.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{ChargeAsset: "DEDX", ChargeAmount: "25", MarginUSD: "25"},
	})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-funded-reject",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "DEDX",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	settlementStore := NewMemoryTradingSettlementStore()
	settler := NewCloseSettlementService(settlementStore, ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	service.SetOrderSettlementService(settler)

	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID:      "status-funded-rejected",
		UserID:       "user-1",
		OrderID:      order.ID,
		Status:       TradingOrderStatusRejected,
		Reason:       "dydx rejected after funding",
		RefundAsset:  "DEDX",
		RefundAmount: "25",
	}); err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}

	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "75" || balance.Frozen != "25" {
		t.Fatalf("balance = %+v, want failed funded order settlement frozen for payout", balance)
	}
	storedOrder, ok, err := tradingStore.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusRejected || storedOrder.DYDXFundingTxHash != "funding-tx-1" {
		t.Fatalf("order = %+v, want rejected with retained funding tx", storedOrder)
	}
	settlement, ok, err := settlementStore.GetByCloseOrderID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetByCloseOrderID() settlement=%+v ok=%v err=%v", settlement, ok, err)
	}
	if settlement.Kind != TradingSettlementKindFailedOrder || settlement.SettlementAmount != "25" {
		t.Fatalf("settlement = %+v, want failed_order amount 25", settlement)
	}
}

func TestTradingSyncServiceRefundsBroadcastedRejectedOrderWithoutDYDXFunding(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-broadcasted-reject",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID: "status-broadcasted-rejected",
		UserID:  "user-1",
		OrderID: order.ID,
		Status:  TradingOrderStatusRejected,
		Reason:  "dydx rejected before funding",
	}); err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}

	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want rejected non-funded order refunded", balance)
	}
}

func TestTradingSyncServiceRefundsCancelledRemainderOnce(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	request := TradingOrderStatusSyncRequest{
		EventID:      "cancel-1",
		UserID:       "user-1",
		OrderID:      order.ID,
		Status:       TradingOrderStatusCancelled,
		RefundAsset:  "USDC",
		RefundAmount: "10",
	}

	if _, err := service.SyncOrderStatus(context.Background(), request); err != nil {
		t.Fatalf("first SyncOrderStatus() error = %v", err)
	}
	_, err = service.SyncOrderStatus(context.Background(), request)

	if !errors.Is(err, ErrTradingSyncDuplicateEvent) {
		t.Fatalf("second SyncOrderStatus() error = %v, want %v", err, ErrTradingSyncDuplicateEvent)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "85" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=85 frozen=0", balance)
	}
}

func TestTradingSyncServiceDoesNotRefundCancelledOrderAfterDYDXFunding(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	trading.SetDYDXSubaccountDepositor(&recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	trading.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{ChargeAsset: "DEDX", ChargeAmount: "25", MarginUSD: "25"},
	})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-funded-cancel",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "DEDX",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)

	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID:      "status-funded-cancelled",
		UserID:       "user-1",
		OrderID:      order.ID,
		Status:       TradingOrderStatusCancelled,
		RefundAsset:  "DEDX",
		RefundAmount: "10",
	}); err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}

	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want no Ledger refund after dYdX funding tx", balance)
	}
}

func TestTradingSyncServiceSettlesCancelledOrderAfterDYDXFunding(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100000000000000000000")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	trading.SetDYDXSubaccountDepositor(&recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	})
	trading.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{ChargeAsset: "DEDX", ChargeAmount: "6250000000000000000", MarginUSD: "25"},
	})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-funded-cancel-settle",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "DEDX",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	settlementStore := NewMemoryTradingSettlementStore()
	settler := NewCloseSettlementService(settlementStore, ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	service.SetOrderSettlementService(settler)

	if _, err := service.SyncOrderStatus(context.Background(), TradingOrderStatusSyncRequest{
		EventID: "status-funded-cancelled-settle",
		UserID:  "user-1",
		OrderID: order.ID,
		Status:  TradingOrderStatusCancelled,
	}); err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}

	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "93750000000000000000" || balance.Frozen != "6250000000000000000" {
		t.Fatalf("balance = %+v, want cancelled funded order settlement frozen for payout", balance)
	}
	settlement, ok, err := settlementStore.GetByCloseOrderID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetByCloseOrderID() settlement=%+v ok=%v err=%v", settlement, ok, err)
	}
	if settlement.Kind != TradingSettlementKindCancelOrder || settlement.SettlementAmount != "6250000000000000000" {
		t.Fatalf("settlement = %+v, want cancel_order amount 6250000000000000000", settlement)
	}
}

func TestTradingSyncWorkerPollsEventsAndAdvancesCursor(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	service := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	cursor := NewMemoryTradingSyncCursorStore(TradingSyncCursor{
		FillsCursor:       "f0",
		OrderStatusCursor: "s0",
		PositionsCursor:   "p0",
	})
	source := &recordingTradingEventSource{
		fillCursor:       "f0",
		positionCursor:   "p0",
		nextFillCursor:   "f1",
		nextPosCursor:    "p1",
		statusCursor:     "s0",
		nextStatusCursor: "s1",
		fills: []TradingFillSyncRequest{{
			EventID:        "fill-1",
			UserID:         "user-1",
			OrderID:        order.ID,
			Market:         "ETH-USD",
			Asset:          "USDC",
			ReleasedMargin: "25",
			Fee:            "2",
			RealizedPNL:    "5",
		}},
		positions: []TradingPositionSyncRequest{{
			EventID:       "position-1",
			UserID:        "user-1",
			Market:        "ETH-USD",
			Size:          "0",
			MarginAsset:   "USDC",
			Margin:        "0",
			UnrealizedPNL: "0",
		}},
	}
	worker := NewTradingSyncWorker(source, service, cursor)

	if err := worker.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	saved, err := cursor.GetTradingSyncCursor(context.Background())
	if err != nil {
		t.Fatalf("GetTradingSyncCursor() error = %v", err)
	}
	if saved.FillsCursor != "f1" || saved.OrderStatusCursor != "s1" || saved.PositionsCursor != "p1" {
		t.Fatalf("cursor = %+v, want fills=f1 orderStatus=s1 positions=p1", saved)
	}
	if source.statusCursor != "s0" {
		t.Fatalf("status cursor used = %q, want s0", source.statusCursor)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "103" {
		t.Fatalf("available = %q, want 103", balance.Available)
	}
}

type recordingTradingEventSource struct {
	fillCursor       string
	positionCursor   string
	statusCursor     string
	nextFillCursor   string
	nextPosCursor    string
	nextStatusCursor string
	fills            []TradingFillSyncRequest
	positions        []TradingPositionSyncRequest
	statuses         []TradingOrderStatusSyncRequest
}

func (s *recordingTradingEventSource) FetchFills(ctx context.Context, cursor string) ([]TradingFillSyncRequest, string, error) {
	s.fillCursor = cursor
	return s.fills, s.nextFillCursor, nil
}

func (s *recordingTradingEventSource) FetchPositions(ctx context.Context, cursor string) ([]TradingPositionSyncRequest, string, error) {
	s.positionCursor = cursor
	return s.positions, s.nextPosCursor, nil
}

func (s *recordingTradingEventSource) FetchOrderStatuses(ctx context.Context, cursor string) ([]TradingOrderStatusSyncRequest, string, error) {
	s.statusCursor = cursor
	return s.statuses, s.nextStatusCursor, nil
}
