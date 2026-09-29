package tyche

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTradingServicePlacesOrderByChargingAvailableBalanceAndSubmittingDYDXIntent(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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
	if order.Status != TradingOrderStatusPendingSubmit {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusPendingSubmit)
	}
	if submitter.intent.Action != DYDXActionPlaceOrder {
		t.Fatalf("intent action = %q, want %q", submitter.intent.Action, DYDXActionPlaceOrder)
	}
	if submitter.intent.ID != order.ID {
		t.Fatalf("intent ID = %q, want order ID %q", submitter.intent.ID, order.ID)
	}
	if submitter.intent.Params["clientOrderId"] != "client-order-1" {
		t.Fatalf("clientOrderId param = %v", submitter.intent.Params["clientOrderId"])
	}
	if order.DYDXAccount != "dydx1userwallet" || submitter.intent.Params["dydxAccount"] != "dydx1userwallet" {
		t.Fatalf("dydx account order=%q intent=%v", order.DYDXAccount, submitter.intent.Params["dydxAccount"])
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=75 frozen=0", balance)
	}
}

func TestTradingServiceUsesServerSideMarginQuoteAndIgnoresClientChargeAmount(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:     "DEDX",
			ChargeAmount:    "20",
			MarginUSD:       "10",
			PriceSnapshotID: "snapshot-1",
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "FDEX-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "5",
		Size:          "2",
		MarginAsset:   "DEDX",
		MarginAmount:  "1",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.MarginAsset != "DEDX" || order.MarginAmount != "20" {
		t.Fatalf("order margin = %s %s, want 20 DEDX", order.MarginAmount, order.MarginAsset)
	}
	if order.MarginUSD != "10" || order.PriceSnapshotID != "snapshot-1" {
		t.Fatalf("order quote fields = %+v", order)
	}
	if submitter.intent.Params["marginAmount"] != "20" {
		t.Fatalf("intent marginAmount = %v, want server-side quote 20", submitter.intent.Params["marginAmount"])
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "80" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=80 frozen=0", balance)
	}
}

func TestTradingServiceRoutesDEDXUSDToBuiltInIsolatedSubaccount(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100000000000000000000")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "25000000000000000000",
			MarginUSD:    "100",
		},
	})
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-dedx-isolated",
		Market:        "FDEX-USD",
		Side:          "sell",
		OrderType:     "limit",
		Price:         "5",
		Size:          "20",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	subaccountField := reflect.ValueOf(order).FieldByName("SubaccountNumber")
	if !subaccountField.IsValid() || uint32(subaccountField.Uint()) != 127 {
		t.Fatalf("order subaccountNumber = %v, want 127", subaccountField)
	}
	if depositor.request.SubaccountNumber != 127 {
		t.Fatalf("funding subaccountNumber = %d, want 127", depositor.request.SubaccountNumber)
	}
	if submitter.intent.Params["subaccountNumber"] != uint32(127) {
		t.Fatalf("submitted subaccountNumber = %v, want 127", submitter.intent.Params["subaccountNumber"])
	}
}

func TestTradingServiceReturnsEquityTierLimitStatusForDEDXUSDOpeningOrder(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100000000000000000000")
	submitter := &resultTradingSubmitter{
		err: errors.New("broadcast rejected with code 10001: Opening order would exceed equity tier limit of 1. Order count: 1"),
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "25000000000000000000",
			MarginUSD:    "100",
		},
	})
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-equity-tier-limit",
		Market:        "FDEX-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "10",
		Size:          "1",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusEquityTierLimit {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusEquityTierLimit)
	}
	if order.Reason != TradingOrderEquityTierLimitReason {
		t.Fatalf("order reason = %q, want %q", order.Reason, TradingOrderEquityTierLimitReason)
	}
	if order.SubaccountNumber != 127 {
		t.Fatalf("order subaccountNumber = %d, want 127", order.SubaccountNumber)
	}
	stored, ok, err := service.store.GetOrderByClientID(context.Background(), "user-1", "client-order-equity-tier-limit")
	if err != nil {
		t.Fatalf("GetOrderByClientID() error = %v", err)
	}
	if !ok || stored.Status != TradingOrderStatusEquityTierLimit || stored.Reason != TradingOrderEquityTierLimitReason {
		t.Fatalf("stored order = %+v ok=%v, want equity_tier_limit with reason", stored, ok)
	}
}

func TestTradingServicePassesDefaultMinimumDYDXMarginToQuoter(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100000000000000000000")
	submitter := &recordingTradingSubmitter{}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	quoter := &recordingTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "5000000000000000000",
			MarginUSD:    "20",
		},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(quoter)
	service.SetDYDXSubaccountDepositor(depositor)

	if _, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-min-margin",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "1892",
		Size:          "0.001",
		MarginAsset:   "DEDX",
	}); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if quoter.request.MinMarginUSD != "20" {
		t.Fatalf("MinMarginUSD = %q, want 20", quoter.request.MinMarginUSD)
	}
}

func TestTradingServiceFundsRetainedCollateralWithoutReducingUserCharge(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	retained := NewMemoryDYDXRetainedCollateralStore()
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetDYDXRetainedCollateralStore(retained)
	service.SetDYDXSubaccountFundingConfig(DYDXSubaccountFundingConfig{
		CollateralQuantumsPerUSD:  "1000000",
		MinStatefulOrderMarginUSD: "0",
		RetainedCollateralUSD:     "2",
	})
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "25",
			MarginUSD:    "25",
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-retained-collateral",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "0.01",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.MarginUSD != "25" || order.MarginAmount != "25" {
		t.Fatalf("order margin = %+v, want user charge unchanged at 25 DEDX / 25 USD", order)
	}
	if order.DYDXOrderMarginQuantums != "25000000" {
		t.Fatalf("DYDXOrderMarginQuantums = %q, want 25000000", order.DYDXOrderMarginQuantums)
	}
	if order.DYDXRetainedCollateralQuantums != "2000000" {
		t.Fatalf("DYDXRetainedCollateralQuantums = %q, want 2000000", order.DYDXRetainedCollateralQuantums)
	}
	if order.DYDXFundingQuantums != "27000000" || depositor.request.Quantums != "27000000" {
		t.Fatalf("funding quantums order=%q request=%q, want margin plus retained collateral", order.DYDXFundingQuantums, depositor.request.Quantums)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want only user order margin charged", balance)
	}
	retainedQuantums, err := retained.RetainedCollateralQuantums(context.Background(), "dydx1userwallet", 0)
	if err != nil {
		t.Fatalf("RetainedCollateralQuantums() error = %v", err)
	}
	if retainedQuantums != "2000000" {
		t.Fatalf("retained quantums = %q, want 2000000", retainedQuantums)
	}

	secondOrder, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-retained-collateral-second",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "0.01",
		MarginAsset:   "DEDX",
	})
	if err != nil {
		t.Fatalf("second PlaceOrder() error = %v", err)
	}
	if secondOrder.DYDXFundingQuantums != "25000000" || depositor.request.Quantums != "25000000" {
		t.Fatalf("second funding quantums order=%q request=%q, want only order margin", secondOrder.DYDXFundingQuantums, depositor.request.Quantums)
	}
	if secondOrder.DYDXRetainedCollateralQuantums != "" {
		t.Fatalf("second retained top-up = %q, want empty after reserve exists", secondOrder.DYDXRetainedCollateralQuantums)
	}
}

func TestTradingServiceUsesRequestedMarginAssetFromAssetQuoter(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDT", "100000000000000000000")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(NewAssetMarginQuoter([]AssetMapping{
		{
			NECOAsset:         "USDT",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "USDT-USD",
			EnabledForTrading: true,
		},
	}, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"USDT": {
				ID:        "snapshot-usdt",
				Asset:     "USDT",
				USDPrice:  "1",
				RouteName: "USDT-USD",
			},
		},
	}))

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-usdt",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "5",
		Size:          "2",
		MarginAsset:   "USDT",
		MarginAmount:  "1",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.MarginAsset != "USDT" || order.MarginAmount != "10000000000000000000" {
		t.Fatalf("order margin = %+v, want 10 USDT atomic", order)
	}
	if order.AssetUSDPrice != "1" || order.PriceRoute != "USDT-USD" {
		t.Fatalf("order price route fields = %+v", order)
	}
	if submitter.intent.Params["marginAsset"] != "USDT" || submitter.intent.Params["marginAmount"] != "10000000000000000000" {
		t.Fatalf("intent params = %+v, want USDT charge", submitter.intent.Params)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDT")
	if balance.Available != "90000000000000000000" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=90 USDT atomic frozen=0", balance)
	}
}

func TestTradingServicePlacesMarketOrderAsProtectedIOCUsingServerPrice(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "10000")
	submitter := &recordingTradingSubmitter{}
	quoter := &recordingTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:     "DEDX",
			ChargeAmount:    "2525",
			MarginUSD:       "2525",
			PriceSnapshotID: "asset-price-snapshot",
		},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(quoter)
	service.SetMarketPriceSource(staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"ETH-USD": {
				Market:     "ETH-USD",
				Price:      "2500",
				Source:     "test-price-source",
				SnapshotID: "dydx-price-snapshot",
			},
		},
	})
	service.SetMarketOrderSlippageBps(100)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-market-buy",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "market",
		Size:          "1",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.OrderType != TradingOrderTypeMarket || order.ExecutionOrderType != TradingOrderTypeIOC {
		t.Fatalf("order types = %q/%q, want market/ioc", order.OrderType, order.ExecutionOrderType)
	}
	if order.Price != "2525" || order.ReferencePrice != "2500" || order.MarketPriceSource != "test-price-source" {
		t.Fatalf("market price fields = %+v", order)
	}
	if order.SlippageBps != 100 {
		t.Fatalf("slippage bps = %d, want 100", order.SlippageBps)
	}
	if quoter.request.Price != "2525" {
		t.Fatalf("quote price = %q, want protected price 2525", quoter.request.Price)
	}
	if submitter.intent.Params["orderType"] != TradingOrderTypeIOC {
		t.Fatalf("submitted orderType = %v, want ioc", submitter.intent.Params["orderType"])
	}
	if submitter.intent.Params["price"] != "2525" {
		t.Fatalf("submitted price = %v, want protected price 2525", submitter.intent.Params["price"])
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "7475" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=7475 frozen=0", balance)
	}
}

func TestTradingServicePlacesSellMarketOrderWithLowerProtectedPrice(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "10000")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXMarkets([]DYDXMarketConfig{{
		Market:          "ETH-USD",
		ClobPairID:      1,
		QuantumsPerUnit: "1000000000",
		SubticksPerUnit: "1000000",
		SubticksPerTick: "100000",
	}})
	service.SetMarketPriceSource(staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"ETH-USD": {
				Market: "ETH-USD",
				Price:  "1951.585252525252525253",
				Source: "test-price-source",
			},
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-market-sell",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "market",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "10",
		SlippageBps:   50,
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Price != "1941.9" {
		t.Fatalf("protected sell price = %q, want 1941.9", order.Price)
	}
	if submitter.intent.Params["orderType"] != TradingOrderTypeIOC {
		t.Fatalf("submitted orderType = %v, want ioc", submitter.intent.Params["orderType"])
	}
}

func TestTradingServiceAlignsMarketOrderProtectedPriceToDYDXTick(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "1000000000000000000000000")
	submitter := &recordingTradingSubmitter{}
	quoter := &recordingTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:     "DEDX",
			ChargeAmount:    "1",
			MarginUSD:       "1932.1",
			PriceSnapshotID: "asset-price-snapshot",
		},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(quoter)
	service.SetDYDXMarkets([]DYDXMarketConfig{{
		Market:          "ETH-USD",
		ClobPairID:      1,
		QuantumsPerUnit: "1000000000",
		SubticksPerUnit: "1000000",
		SubticksPerTick: "100000",
	}})
	service.SetMarketPriceSource(staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"ETH-USD": {
				Market: "ETH-USD",
				Price:  "1951.585252525252525253",
				Source: "test-price-source",
			},
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-market-sell-aligned",
		Market:        "ETH-USD",
		Side:          "sell",
		OrderType:     "market",
		Size:          "1",
		MarginAsset:   "DEDX",
		SlippageBps:   100,
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Price != "1932.1" {
		t.Fatalf("protected sell price = %q, want 1932.1", order.Price)
	}
	if quoter.request.Price != "1932.1" {
		t.Fatalf("quote price = %q, want tick-aligned protected price", quoter.request.Price)
	}
	if submitter.intent.Params["price"] != "1932.1" {
		t.Fatalf("submitted price = %v, want tick-aligned protected price", submitter.intent.Params["price"])
	}
}

func TestTradingServiceRejectsMarketOrderWithoutServerPriceSource(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-market-missing-price",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "market",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "10",
	})

	if !errors.Is(err, ErrTradingMarketPriceSourceNotConfigured) {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, ErrTradingMarketPriceSourceNotConfigured)
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0", submitter.calls)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want unchanged", balance)
	}
}

func TestTradingServiceReturnsFundingRequiredWhenLedgerNeedsNECOWalletPreparation(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "5")
	submitter := &recordingTradingSubmitter{}
	funding := &fixedTradingFundingPreparer{
		result: TradingFundingPrepareResult{
			Status:          TradingFundingStatusWalletActionRequired,
			Action:          TradingFundingActionTransfer,
			Asset:           "DEDX",
			Amount:          "20",
			TokenAddress:    "0x0000000000000000000000000000000000000ded",
			FromAddress:     "0x0000000000000000000000000000000000000001",
			ToAddress:       "0x000000000000000000000000000000000000cafe",
			SpenderAddress:  "",
			Reason:          "transfer DEDX before submitting dydx order",
			WalletBalance:   "100",
			LedgerAvailable: "5",
		},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:     "DEDX",
			ChargeAmount:    "20",
			MarginUSD:       "10",
			PriceSnapshotID: "snapshot-1",
		},
	})
	service.SetFundingPreparer(funding)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "FDEX-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "5",
		Size:          "2",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusFundingRequired {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusFundingRequired)
	}
	if order.FundingStatus != TradingFundingStatusWalletActionRequired || order.FundingAction != TradingFundingActionTransfer {
		t.Fatalf("funding fields = %+v", order)
	}
	if order.FundingAmount != "20" || order.FundingTokenAddress == "" || order.FundingToAddress == "" {
		t.Fatalf("funding instruction = %+v", order)
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 before funding is ready", submitter.calls)
	}
	if funding.request.MissingAmount != "20" || funding.request.LedgerAvailable != "5" {
		t.Fatalf("funding request = %+v", funding.request)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "5" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want unchanged available=5 frozen=0", balance)
	}
}

func TestTradingServiceRequiresNECOWalletFundingEvenWhenLedgerBalanceCoversNewOrder(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	submitter := &recordingTradingSubmitter{}
	funding := &fixedTradingFundingPreparer{
		result: TradingFundingPrepareResult{
			Status:        TradingFundingStatusWalletActionRequired,
			Action:        TradingFundingActionTransfer,
			Asset:         "DEDX",
			Amount:        "20",
			TokenAddress:  "0x0000000000000000000000000000000000000ded",
			FromAddress:   "0x0000000000000000000000000000000000000001",
			ToAddress:     "0x000000000000000000000000000000000000cafe",
			WalletBalance: "1000",
		},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "20",
			MarginUSD:    "10",
		},
	})
	service.SetFundingPreparer(funding)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-ledger-covered",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		Market:        "FDEX-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "5",
		Size:          "2",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusFundingRequired {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusFundingRequired)
	}
	if order.FundingAmount != "20" || order.FundingLedgerAvailable != "100" {
		t.Fatalf("funding fields = %+v, want full order funding with ledger baseline", order)
	}
	if funding.request.MissingAmount != "20" || funding.request.LedgerAvailable != "100" {
		t.Fatalf("funding request = %+v, want full amount and current ledger baseline", funding.request)
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 before wallet funding", submitter.calls)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want unchanged available=100 frozen=0", balance)
	}
}

func TestTradingServiceResumesFundingRequiredOrderAfterNECOFundingIsCredited(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "5")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "20",
			MarginUSD:    "10",
		},
	})
	service.SetFundingPreparer(&fixedTradingFundingPreparer{
		result: TradingFundingPrepareResult{
			Status: TradingFundingStatusWalletActionRequired,
			Action: TradingFundingActionTransfer,
			Asset:  "DEDX",
			Amount: "20",
		},
	})
	request := TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "FDEX-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "5",
		Size:          "2",
	}
	first, err := service.PlaceOrder(context.Background(), request)
	if err != nil {
		t.Fatalf("first PlaceOrder() error = %v", err)
	}
	if first.Status != TradingOrderStatusFundingRequired {
		t.Fatalf("first status = %q, want funding_required", first.Status)
	}
	if _, err := ledger.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "20",
		RefID:  "neco-hidden-funding-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}

	second, err := service.PlaceOrder(context.Background(), request)

	if err != nil {
		t.Fatalf("second PlaceOrder() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second order ID = %q, want existing %q", second.ID, first.ID)
	}
	if second.Status != TradingOrderStatusPendingSubmit {
		t.Fatalf("second status = %q, want %q", second.Status, TradingOrderStatusPendingSubmit)
	}
	if submitter.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitter.calls)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "5" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want old available=5 frozen=0", balance)
	}
}

func TestTradingServiceResumesFundingRequiredOrderFromCreditedLedgerWithoutCheckingWalletAgain(t *testing.T) {
	store := NewMemoryTradingStore()
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "FDEX", "25")
	if err := store.SaveOrder(context.Background(), TradingOrder{
		ID:                     "funding-order-1",
		UserID:                 "user-1",
		DYDXAccount:            "dydx1userwallet",
		ClientOrderID:          "client-order-funded-ledger",
		Market:                 "FDEX-USD",
		Side:                   "BUY",
		OrderType:              "limit",
		Price:                  "0.21",
		Size:                   "100",
		MarginAsset:            "FDEX",
		MarginAmount:           "20",
		MarginUSD:              "20",
		FundingAmount:          "10",
		FundingLedgerAvailable: "5",
		Status:                 TradingOrderStatusFundingRequired,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	funding := &fixedTradingFundingPreparer{err: ErrTradingFundingWalletInsufficient}
	service := NewTradingService(store, ledger, submitter)
	service.SetFundingPreparer(funding)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-funded-ledger",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusBroadcasted)
	}
	if submitter.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitter.calls)
	}
	if funding.request.UserID != "" {
		t.Fatalf("funding preparer was called again: %+v", funding.request)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "FDEX")
	if balance.Available != "5" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=5 frozen=0", balance)
	}
}

func TestTradingServiceResumesFundingRequiredOrderWithRemainingShortfallWithoutCheckingWalletAgain(t *testing.T) {
	store := NewMemoryTradingStore()
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDT", "80")
	if err := store.SaveOrder(context.Background(), TradingOrder{
		ID:                     "funding-order-short-1",
		UserID:                 "user-1",
		NECOAddress:            "0x0000000000000000000000000000000000000001",
		DYDXAccount:            "dydx1userwallet",
		SubaccountNumber:       127,
		ClientOrderID:          "client-order-open-short",
		Market:                 "FDEX-USD",
		Side:                   "SELL",
		OrderType:              "market",
		ExecutionOrderType:     "ioc",
		Price:                  "0.205",
		Size:                   "97",
		MarginAsset:            "USDT",
		MarginAmount:           "100",
		MarginUSD:              "100",
		FundingStatus:          TradingFundingStatusWalletActionRequired,
		FundingAction:          TradingFundingActionTransfer,
		FundingAsset:           "USDT",
		FundingAmount:          "80",
		FundingTokenAddress:    "0x0000000000000000000000000000000000000usd",
		FundingFromAddress:     "0x0000000000000000000000000000000000000001",
		FundingToAddress:       "0x000000000000000000000000000000000000cafe",
		FundingLedgerAvailable: "0",
		Status:                 TradingOrderStatusFundingRequired,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	funding := &fixedTradingFundingPreparer{err: ErrTradingFundingWalletInsufficient}
	service := NewTradingService(store, ledger, submitter)
	service.SetFundingPreparer(funding)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-open-short",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusFundingRequired {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusFundingRequired)
	}
	if order.FundingAmount != "20" || order.FundingLedgerAvailable != "80" {
		t.Fatalf("funding shortfall fields = %+v, want remaining amount=20 ledger=80", order)
	}
	if order.FundingTokenAddress != "0x0000000000000000000000000000000000000usd" || order.FundingToAddress != "0x000000000000000000000000000000000000cafe" {
		t.Fatalf("funding instructions were not preserved: %+v", order)
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 until full funding is credited", submitter.calls)
	}
	if funding.request.UserID != "" {
		t.Fatalf("funding preparer was called again: %+v", funding.request)
	}
}

func TestTradingServiceRaisesExistingFundingRequiredOrderToMinimumMarginBeforeRetry(t *testing.T) {
	store := NewMemoryTradingStore()
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "5000000000000000000")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	if err := store.SaveOrder(context.Background(), TradingOrder{
		ID:            "order-existing",
		UserID:        "user-1",
		DYDXAccount:   "dydx1userwallet",
		ClientOrderID: "client-order-existing-min",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "1892",
		Size:          "0.001",
		MarginAsset:   "DEDX",
		MarginAmount:  "473000000000000000",
		MarginUSD:     "1.892",
		Status:        TradingOrderStatusFundingRequired,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	service := NewTradingService(store, ledger, submitter)
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(NewAssetMarginQuoter([]AssetMapping{
		{
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
	}, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"DEDX": {ID: "snapshot-dedx", Asset: "DEDX", USDPrice: "4", RouteName: "DEDX-NECO-USD"},
		},
	}))

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-existing-min",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.MarginUSD != "20" || order.MarginAmount != "5000000000000000000" {
		t.Fatalf("order margin = %+v, want 20 USD funded by 5 DEDX", order)
	}
	if depositor.request.MarginUSD != "20" || depositor.request.Quantums != "20000000" {
		t.Fatalf("deposit request = %+v, want 20 USD quantums", depositor.request)
	}
}

func TestTradingServiceFundsDYDXSubaccountBeforeSubmittingOrder(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "10000000000000000000")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "473000000000000000",
			MarginUSD:    "1.892",
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		NECOAddress:   "0xuser",
		ClientOrderID: "client-order-funded",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "1892",
		Size:          "0.001",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if depositor.calls != 1 {
		t.Fatalf("deposit calls = %d, want 1", depositor.calls)
	}
	if depositor.request.UserID != "user-1" || depositor.request.DYDXAccount != "dydx1userwallet" {
		t.Fatalf("deposit request identity = %+v", depositor.request)
	}
	if depositor.request.AssetID != 0 || depositor.request.Quantums != "1892000" || depositor.request.MarginUSD != "1.892" {
		t.Fatalf("deposit amount = %+v, want asset 0 quantums 1892000 marginUsd 1.892", depositor.request)
	}
	if submitter.calls != 1 || submitter.intent.Params["dydxAccount"] != "dydx1userwallet" {
		t.Fatalf("submitter calls=%d intent=%+v", submitter.calls, submitter.intent)
	}
	if order.DYDXFundingTxHash != "funding-tx-1" || order.DYDXFundingQuantums != "1892000" {
		t.Fatalf("funding fields = %+v", order)
	}
	if order.DYDXOrderTxHash != "order-tx-1" || order.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("order broadcast fields = %+v", order)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "9527000000000000000" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want charged DEDX and no frozen funds", balance)
	}
}

func TestTradingServiceDoesNotTrustFilledStatusFromMarketOrderBroadcast(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "10000000000000000000")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusFilled},
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{
			ChargeAsset:  "DEDX",
			ChargeAmount: "473000000000000000",
			MarginUSD:    "1.892",
		},
	})
	service.SetMarketPriceSource(staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"ETH-USD": {
				Market: "ETH-USD",
				Price:  "1892",
				Source: "test-price-source",
			},
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		NECOAddress:   "0xuser",
		ClientOrderID: "client-order-market-filled-broadcast",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "market",
		Size:          "0.001",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("order status = %q, want %q until fill sync confirms execution", order.Status, TradingOrderStatusBroadcasted)
	}
	if order.DYDXOrderTxHash != "order-tx-1" {
		t.Fatalf("DYDXOrderTxHash = %q, want order-tx-1", order.DYDXOrderTxHash)
	}
	stored, ok, err := service.store.GetOrderByClientID(context.Background(), "user-1", "client-order-market-filled-broadcast")
	if err != nil || !ok {
		t.Fatalf("stored order ok=%v err=%v", ok, err)
	}
	if stored.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("stored status = %q, want %q", stored.Status, TradingOrderStatusBroadcasted)
	}
}

func TestTradingServiceRoundsMarginUSDUpToDYDXCollateralQuantumBeforeChargingAndFunding(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "1000000000000000000")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetDYDXSubaccountFundingConfig(DYDXSubaccountFundingConfig{
		CollateralQuantumsPerUSD:  "1000000",
		MinStatefulOrderMarginUSD: "0",
	})
	service.SetMarginQuoter(NewAssetMarginQuoter([]AssetMapping{
		{
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
	}, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"DEDX": {ID: "snapshot-dedx", Asset: "DEDX", USDPrice: "4", RouteName: "DEDX-NECO-USD"},
		},
	}))

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		NECOAddress:   "0xuser",
		ClientOrderID: "client-order-rounded-margin",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "1.0000001",
		Size:          "1",
		MarginAsset:   "DEDX",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.MarginUSD != "1.000001" || order.MarginAmount != "250000250000000000" {
		t.Fatalf("order margin = %+v, want 1.000001 USD charged as 250000250000000000 DEDX atomic", order)
	}
	if depositor.request.MarginUSD != "1.000001" || depositor.request.Quantums != "1000001" {
		t.Fatalf("deposit request = %+v, want collateral-aligned margin", depositor.request)
	}
	if order.DYDXFundingQuantums != "1000001" {
		t.Fatalf("DYDXFundingQuantums = %q, want 1000001", order.DYDXFundingQuantums)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "749999750000000000" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want rounded DEDX charge and no frozen funds", balance)
	}
}

func TestTradingServiceHoldsLedgerWhenDYDXSubaccountFundingAttemptFails(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	submitter := &recordingTradingSubmitter{}
	depositor := &recordingDYDXSubaccountDepositor{err: errors.New("deposit broadcast failed")}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{ChargeAsset: "DEDX", ChargeAmount: "25", MarginUSD: "25"},
	})

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-deposit-fails",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "0.01",
		MarginAsset:   "DEDX",
	})

	if err == nil || err.Error() != "deposit broadcast failed" {
		t.Fatalf("PlaceOrder() error = %v, want deposit broadcast failed", err)
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 when dYdX funding fails", submitter.calls)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want ledger held after uncertain dYdX funding attempt", balance)
	}
	order, ok, err := service.store.GetOrderByClientID(context.Background(), "user-1", "client-order-deposit-fails")
	if err != nil {
		t.Fatalf("GetOrderByClientID() error = %v", err)
	}
	if !ok || order.Status != TradingOrderStatusDYDXFundingFailed {
		t.Fatalf("order = %+v ok=%v, want dydx_funding_failed", order, ok)
	}
}

func TestTradingServiceRetriesDYDXFundingFailureWithoutChargingLedgerAgain(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "order-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	depositor := &recordingDYDXSubaccountDepositor{err: errors.New("temporary deposit failure")}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{ChargeAsset: "DEDX", ChargeAmount: "25", MarginUSD: "25"},
	})
	request := TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-retry-deposit",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "0.01",
		MarginAsset:   "DEDX",
	}
	if _, err := service.PlaceOrder(context.Background(), request); err == nil {
		t.Fatal("first PlaceOrder() error = nil, want temporary deposit failure")
	}
	depositor.err = nil
	depositor.result = DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted}

	order, err := service.PlaceOrder(context.Background(), request)

	if err != nil {
		t.Fatalf("retry PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusBroadcasted || order.DYDXFundingTxHash != "funding-tx-1" {
		t.Fatalf("retried order = %+v, want broadcasted with funding tx", order)
	}
	if depositor.calls != 2 || submitter.calls != 1 {
		t.Fatalf("calls depositor=%d submitter=%d, want 2/1", depositor.calls, submitter.calls)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want only one ledger charge", balance)
	}
}

func TestTradingServiceSettlesFailedFundedOrderWhenSubmitFailsAfterDYDXFunding(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "100")
	submitter := &resultTradingSubmitter{err: errors.New("order broadcast failed")}
	depositor := &recordingDYDXSubaccountDepositor{
		result: DYDXSubaccountDepositResult{TxHash: "funding-tx-1", Status: TradingOrderStatusBroadcasted},
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
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXSubaccountDepositor(depositor)
	service.SetFailedOrderSettlementService(settler)
	service.SetMarginQuoter(fixedTradingMarginQuoter{
		quote: TradingMarginQuote{ChargeAsset: "DEDX", ChargeAmount: "25", MarginUSD: "25"},
	})

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-submit-fails-after-funding",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "0.01",
		MarginAsset:   "DEDX",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
	})

	if err == nil || err.Error() != "order broadcast failed" {
		t.Fatalf("PlaceOrder() error = %v, want order broadcast failed", err)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "75" || balance.Frozen != "25" {
		t.Fatalf("balance = %+v, want failed funded order settlement frozen for payout", balance)
	}
	order, ok, err := service.store.GetOrderByClientID(context.Background(), "user-1", "client-order-submit-fails-after-funding")
	if err != nil {
		t.Fatalf("GetOrderByClientID() error = %v", err)
	}
	if !ok || order.Status != TradingOrderStatusSubmitFailed || order.DYDXFundingTxHash != "funding-tx-1" {
		t.Fatalf("order = %+v ok=%v, want submit_failed with funding tx", order, ok)
	}
	settlement, ok, err := settlementStore.GetByCloseOrderID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetByCloseOrderID() settlement=%+v ok=%v err=%v", settlement, ok, err)
	}
	if settlement.Kind != TradingSettlementKindFailedOrder || settlement.SettlementAmount != "25" {
		t.Fatalf("settlement = %+v, want failed_order amount 25", settlement)
	}
}

func TestTradingServicePlaceOrderIsIdempotentByClientOrderID(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	request := TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	}
	first, err := service.PlaceOrder(context.Background(), request)
	if err != nil {
		t.Fatalf("first PlaceOrder() error = %v", err)
	}

	second, err := service.PlaceOrder(context.Background(), request)

	if err != nil {
		t.Fatalf("second PlaceOrder() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second order ID = %q, want %q", second.ID, first.ID)
	}
	if submitter.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitter.calls)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=75 frozen=0", balance)
	}
}

func TestMemoryTradingStoreCreateOrderIfAbsentReturnsExistingClientOrder(t *testing.T) {
	store := NewMemoryTradingStore()
	first := TradingOrder{
		ID:            "order-1",
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Status:        TradingOrderStatusPendingSubmit,
	}
	second := TradingOrder{
		ID:            "order-2",
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Status:        TradingOrderStatusPendingSubmit,
	}

	created, didCreate, err := store.CreateOrderIfAbsent(context.Background(), first)
	if err != nil {
		t.Fatalf("first CreateOrderIfAbsent() error = %v", err)
	}
	if !didCreate || created.ID != first.ID {
		t.Fatalf("first result = %+v created=%v, want new order", created, didCreate)
	}

	existing, didCreate, err := store.CreateOrderIfAbsent(context.Background(), second)
	if err != nil {
		t.Fatalf("second CreateOrderIfAbsent() error = %v", err)
	}
	if didCreate || existing.ID != first.ID {
		t.Fatalf("second result = %+v created=%v, want existing %q", existing, didCreate, first.ID)
	}
}

func TestTradingServiceCancelOrderSubmitsCancelIntent(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	submitter := &recordingTradingSubmitter{}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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

	cancelled, err := service.CancelOrder(context.Background(), TradingCancelOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
	})

	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if cancelled.ID != order.ID {
		t.Fatalf("cancelled order ID = %q, want %q", cancelled.ID, order.ID)
	}
	if cancelled.Status != TradingOrderStatusCancelPending {
		t.Fatalf("cancelled status = %q, want %q", cancelled.Status, TradingOrderStatusCancelPending)
	}
	if submitter.intent.Action != DYDXActionCancelOrder {
		t.Fatalf("intent action = %q, want %q", submitter.intent.Action, DYDXActionCancelOrder)
	}
	if submitter.intent.ID != order.ID+"-cancel" {
		t.Fatalf("cancel intent ID = %q", submitter.intent.ID)
	}
	if submitter.intent.Params["dydxAccount"] != "dydx1userwallet" {
		t.Fatalf("cancel dydxAccount = %v", submitter.intent.Params["dydxAccount"])
	}
}

func TestTradingServiceCancelOrderStoresCancelTxHash(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "place-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	store := NewMemoryTradingStore()
	service := NewTradingService(store, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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
	submitter.result = DYDXTradingResult{TxHash: "cancel-tx-1", Status: TradingOrderStatusBroadcasted}

	cancelled, err := service.CancelOrder(context.Background(), TradingCancelOrderRequest{
		UserID:  "user-1",
		OrderID: order.ID,
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if cancelled.DYDXCancelTxHash != "cancel-tx-1" {
		t.Fatalf("cancel tx = %q, want cancel-tx-1", cancelled.DYDXCancelTxHash)
	}
	stored, ok, err := store.GetOrderByID(context.Background(), "user-1", order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", stored, ok, err)
	}
	if stored.DYDXOrderTxHash != "place-tx-1" || stored.DYDXCancelTxHash != "cancel-tx-1" {
		t.Fatalf("stored tx hashes = order:%q cancel:%q", stored.DYDXOrderTxHash, stored.DYDXCancelTxHash)
	}
}

func TestTradingServiceClosePositionSubmitsOppositeReduceOnlyOrder(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, tradingStore, ledger)
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:     "position-1",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "0.5",
		MarginAsset: "DEDX",
		Margin:      "10",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "close-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(syncService)

	order, err := service.ClosePosition(context.Background(), TradingClosePositionRequest{
		UserID:        "user-1",
		ClientOrderID: "close-order-1",
		Market:        "ETH-USD",
		OrderType:     "limit",
		Price:         "2600",
	})

	if err != nil {
		t.Fatalf("ClosePosition() error = %v", err)
	}
	if order.Status != TradingOrderStatusCloseBroadcasted {
		t.Fatalf("close order status = %q, want %q", order.Status, TradingOrderStatusCloseBroadcasted)
	}
	if order.Side != "sell" || order.Size != "0.5" {
		t.Fatalf("close order side/size = %s/%s, want sell/0.5", order.Side, order.Size)
	}
	if !order.ReduceOnly || !order.ClosePosition {
		t.Fatalf("close order reduceOnly=%v closePosition=%v, want true/true", order.ReduceOnly, order.ClosePosition)
	}
	if order.ExecutionOrderType != TradingOrderTypeIOC {
		t.Fatalf("close order execution type = %q, want %q", order.ExecutionOrderType, TradingOrderTypeIOC)
	}
	if order.MarginAmount != "0" || order.MarginAsset != "DEDX" {
		t.Fatalf("close order margin = %s %s, want 0 DEDX", order.MarginAmount, order.MarginAsset)
	}
	if order.DYDXOrderTxHash != "close-tx-1" {
		t.Fatalf("close order tx = %q, want close-tx-1", order.DYDXOrderTxHash)
	}
	if submitter.intent.Params["reduceOnly"] != true {
		t.Fatalf("reduceOnly intent param = %v, want true", submitter.intent.Params["reduceOnly"])
	}
	if submitter.intent.Params["orderType"] != TradingOrderTypeIOC {
		t.Fatalf("close order intent type = %v, want %q", submitter.intent.Params["orderType"], TradingOrderTypeIOC)
	}
}

func TestTradingServiceClosePositionUsesDEDXUSDIsolatedSubaccountPosition(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "close-tx-127", Status: TradingOrderStatusBroadcasted},
	}
	positions := &subaccountAwarePositionProvider{
		position: TradingPosition{
			UserID:           "user-1",
			Market:           "FDEX-USD",
			SubaccountNumber: 127,
			Size:             "2",
			MarginAsset:      "DEDX",
			Margin:           "40",
		},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(positions)

	order, err := service.ClosePosition(context.Background(), TradingClosePositionRequest{
		UserID:        "user-1",
		ClientOrderID: "close-dedx-127",
		Market:        "FDEX-USD",
		OrderType:     "limit",
		Price:         "5",
	})

	if err != nil {
		t.Fatalf("ClosePosition() error = %v", err)
	}
	if positions.requestedSubaccount != 127 {
		t.Fatalf("position provider subaccount = %d, want 127", positions.requestedSubaccount)
	}
	if order.SubaccountNumber != 127 || submitter.intent.Params["subaccountNumber"] != uint32(127) {
		t.Fatalf("close subaccount order=%d intent=%v, want 127", order.SubaccountNumber, submitter.intent.Params["subaccountNumber"])
	}
	if order.Side != "sell" || order.Size != "2" {
		t.Fatalf("close order side/size = %s/%s, want sell/2", order.Side, order.Size)
	}
}

func TestTradingServicePlaceOrderTreatsOppositePositionAsCloseWithoutFunding(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "close-via-place-tx", Status: TradingOrderStatusBroadcasted},
	}
	positions := &subaccountAwarePositionProvider{
		position: TradingPosition{
			UserID:           "user-1",
			Market:           "FDEX-USD",
			SubaccountNumber: 127,
			Size:             "-30",
			MarginAsset:      "FDEX",
			Margin:           "6.3",
		},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(positions)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "place-close-1",
		Market:        "FDEX-USD",
		Side:          "BUY",
		OrderType:     "LIMIT",
		Price:         "0.21",
		Size:          "30",
		MarginAsset:   "FDEX",
		MarginAmount:  "100",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if positions.requestedSubaccount != 127 {
		t.Fatalf("position provider subaccount = %d, want 127", positions.requestedSubaccount)
	}
	if order.Status != TradingOrderStatusCloseBroadcasted {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusCloseBroadcasted)
	}
	if order.Side != "buy" || order.Size != "30" {
		t.Fatalf("close order side/size = %s/%s, want buy/30", order.Side, order.Size)
	}
	if !order.ClosePosition || !order.ReduceOnly {
		t.Fatalf("close flags reduceOnly=%v closePosition=%v, want true/true", order.ReduceOnly, order.ClosePosition)
	}
	if order.MarginAmount != "0" || order.MarginAsset != "FDEX" {
		t.Fatalf("close order margin = %s %s, want 0 FDEX", order.MarginAmount, order.MarginAsset)
	}
	if submitter.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitter.calls)
	}
	if submitter.intent.Params["orderType"] != TradingOrderTypeIOC {
		t.Fatalf("intent orderType = %v, want %q", submitter.intent.Params["orderType"], TradingOrderTypeIOC)
	}
	if submitter.intent.Params["reduceOnly"] != true {
		t.Fatalf("intent reduceOnly = %v, want true", submitter.intent.Params["reduceOnly"])
	}
}

func TestTradingServicePlaceMarketOrderTreatsOppositePositionAsCloseWithoutFunding(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "close-market-via-place-tx", Status: TradingOrderStatusBroadcasted},
	}
	positions := &subaccountAwarePositionProvider{
		position: TradingPosition{
			UserID:           "user-1",
			Market:           "FDEX-USD",
			SubaccountNumber: 127,
			Size:             "-30",
			MarginAsset:      "FDEX",
			Margin:           "6.3",
		},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(positions)
	service.SetMarketPriceSource(staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"FDEX-USD": {
				Market:     "FDEX-USD",
				Price:      "0.21",
				Source:     "test-price-source",
				SnapshotID: "market-price-snapshot",
			},
		},
	})

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		ClientOrderID: "place-market-close-1",
		Market:        "FDEX-USD",
		Side:          "BUY",
		OrderType:     "MARKET",
		Size:          "30",
		MarginAsset:   "FDEX",
		MarginAmount:  "100",
		SlippageBps:   100,
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusCloseBroadcasted {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusCloseBroadcasted)
	}
	if order.OrderType != TradingOrderTypeMarket || order.ExecutionOrderType != TradingOrderTypeIOC {
		t.Fatalf("order types = %q/%q, want market/ioc", order.OrderType, order.ExecutionOrderType)
	}
	if order.MarginAmount != "0" || order.MarginAsset != "FDEX" {
		t.Fatalf("close order margin = %s %s, want 0 FDEX", order.MarginAmount, order.MarginAsset)
	}
	if order.NECOAddress != "0x0000000000000000000000000000000000000001" {
		t.Fatalf("close order neco address = %q, want original request address", order.NECOAddress)
	}
	if !order.ClosePosition || !order.ReduceOnly {
		t.Fatalf("close flags reduceOnly=%v closePosition=%v, want true/true", order.ReduceOnly, order.ClosePosition)
	}
	if submitter.intent.Params["orderType"] != TradingOrderTypeIOC {
		t.Fatalf("intent orderType = %v, want %q", submitter.intent.Params["orderType"], TradingOrderTypeIOC)
	}
	if submitter.intent.Params["reduceOnly"] != true {
		t.Fatalf("intent reduceOnly = %v, want true", submitter.intent.Params["reduceOnly"])
	}
}

func TestTradingServicePlaceOrderRejectsOrderCrossingExistingPosition(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "should-not-submit", Status: TradingOrderStatusBroadcasted},
	}
	positions := &subaccountAwarePositionProvider{
		position: TradingPosition{
			UserID:           "user-1",
			Market:           "FDEX-USD",
			SubaccountNumber: 127,
			Size:             "-30",
			MarginAsset:      "FDEX",
			Margin:           "6.3",
		},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(positions)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "cross-position-1",
		Market:        "FDEX-USD",
		Side:          "BUY",
		OrderType:     "LIMIT",
		Price:         "0.21",
		Size:          "50",
		MarginAsset:   "FDEX",
		MarginAmount:  "100",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != "order_crosses_existing_position" {
		t.Fatalf("order status = %q, want order_crosses_existing_position", order.Status)
	}
	if order.Reason == "" {
		t.Fatalf("order reason is empty")
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0", submitter.calls)
	}
	stored, ok, err := tradingStore.GetOrderByClientID(context.Background(), "user-1", "cross-position-1")
	if err != nil || !ok {
		t.Fatalf("stored order ok=%v err=%v, want stored", ok, err)
	}
	if stored.Status != order.Status || stored.Reason != order.Reason {
		t.Fatalf("stored order status/reason = %q/%q, want %q/%q", stored.Status, stored.Reason, order.Status, order.Reason)
	}
}

func TestTradingServicePlaceOrderClearsStalePositionWhenChainSubaccountIsFlat(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	creditAvailableBalance(t, ledger, "user-1", "FDEX", "1000")
	syncService := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "stale-position",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "-30",
		MarginAsset:      "FDEX",
		Margin:           "6.3",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "place-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	reconcile := &recordingDYDXOrderReconcileSource{
		result: DYDXOrderReconcileResult{
			Subaccount: DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(syncService)
	service.SetDYDXMarkets([]DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}})
	service.SetDYDXPositionReconcileSource(reconcile)

	order, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "stale-position-open",
		Market:        "FDEX-USD",
		Side:          "BUY",
		OrderType:     "LIMIT",
		Price:         "0.21",
		Size:          "50",
		MarginAsset:   "FDEX",
		MarginAmount:  "100",
	})

	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.Status != TradingOrderStatusBroadcasted {
		t.Fatalf("order status = %q, want %q", order.Status, TradingOrderStatusBroadcasted)
	}
	if submitter.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitter.calls)
	}
	if reconcile.request.DYDXAccount != "dydx1userwallet" || reconcile.request.SubaccountNumber != 127 || reconcile.request.ClobPairID != 302 {
		t.Fatalf("reconcile request = %+v, want account/subaccount/clob", reconcile.request)
	}
	if position, err := syncService.GetPositionForSubaccount(context.Background(), "user-1", "FDEX-USD", 127); err == nil {
		t.Fatalf("stale position after PlaceOrder() = %+v, want cleared", position)
	} else if !errors.Is(err, ErrTradingSyncEventNotFound) {
		t.Fatalf("GetPositionForSubaccount() error = %v, want %v", err, ErrTradingSyncEventNotFound)
	}
}

func TestTradingServiceClosePositionClearsStalePositionWhenChainSubaccountIsFlat(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	syncService := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "stale-close-position",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "30",
		MarginAsset:      "FDEX",
		Margin:           "6.3",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "should-not-submit", Status: TradingOrderStatusBroadcasted},
	}
	reconcile := &recordingDYDXOrderReconcileSource{
		result: DYDXOrderReconcileResult{
			Subaccount: DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(syncService)
	service.SetDYDXMarkets([]DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}})
	service.SetDYDXPositionReconcileSource(reconcile)

	_, err := service.ClosePosition(context.Background(), TradingClosePositionRequest{
		UserID:        "user-1",
		ClientOrderID: "close-stale-position",
		Market:        "FDEX-USD",
		OrderType:     "LIMIT",
		Price:         "0.21",
	})

	if !errors.Is(err, ErrTradingPositionNotFound) {
		t.Fatalf("ClosePosition() error = %v, want %v", err, ErrTradingPositionNotFound)
	}
	if submitter.calls != 0 {
		t.Fatalf("submit calls = %d, want 0", submitter.calls)
	}
	if reconcile.request.DYDXAccount != "dydx1userwallet" || reconcile.request.SubaccountNumber != 127 || reconcile.request.ClobPairID != 302 {
		t.Fatalf("reconcile request = %+v, want account/subaccount/clob", reconcile.request)
	}
	if position, err := syncService.GetPositionForSubaccount(context.Background(), "user-1", "FDEX-USD", 127); err == nil {
		t.Fatalf("stale position after ClosePosition() = %+v, want cleared", position)
	} else if !errors.Is(err, ErrTradingSyncEventNotFound) {
		t.Fatalf("GetPositionForSubaccount() error = %v, want %v", err, ErrTradingSyncEventNotFound)
	}
}

func TestTradingServiceClosePositionDoesNotExposeAtomicMarginAsUSD(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, tradingStore, ledger)
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:     "position-atomic-margin",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "2",
		MarginAsset: "DEDX",
		Margin:      "499750000000000000000",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{TxHash: "close-tx-1", Status: TradingOrderStatusBroadcasted},
	}
	service := NewTradingService(tradingStore, ledger, submitter)
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	service.SetPositionProvider(syncService)

	order, err := service.ClosePosition(context.Background(), TradingClosePositionRequest{
		UserID:        "user-1",
		ClientOrderID: "close-order-atomic-margin",
		Market:        "ETH-USD",
		OrderType:     "limit",
		Price:         "2600",
	})
	if err != nil {
		t.Fatalf("ClosePosition() error = %v", err)
	}
	if order.MarginUSD != "" {
		t.Fatalf("close order marginUsd = %q, want empty for atomic-like position margin", order.MarginUSD)
	}
}

func TestTradingServiceClosePositionRejectsMissingPosition(t *testing.T) {
	tradingStore := NewMemoryTradingStore()
	service := NewTradingService(tradingStore, NewLedgerService(NewMemoryLedgerStore()), &recordingTradingSubmitter{})
	service.SetPositionProvider(NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, NewLedgerService(NewMemoryLedgerStore())))

	_, err := service.ClosePosition(context.Background(), TradingClosePositionRequest{
		UserID:        "user-1",
		ClientOrderID: "close-order-1",
		Market:        "ETH-USD",
		OrderType:     "limit",
		Price:         "2600",
	})

	if !errors.Is(err, ErrTradingPositionNotFound) {
		t.Fatalf("ClosePosition() error = %v, want %v", err, ErrTradingPositionNotFound)
	}
}

func TestTradingServiceRequiresDYDXBindingWhenResolverIsConfigured(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	service := NewTradingService(NewMemoryTradingStore(), ledger, &recordingTradingSubmitter{})
	service.SetDYDXAccountResolver(fixedDYDXAccountResolver{err: ErrWalletBindingNotFound})

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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

	if !errors.Is(err, ErrTradingDYDXBindingRequired) {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, ErrTradingDYDXBindingRequired)
	}
}

func TestTradingServiceRejectsCancelForUnknownOrder(t *testing.T) {
	service := NewTradingService(NewMemoryTradingStore(), NewLedgerService(NewMemoryLedgerStore()), &recordingTradingSubmitter{})

	_, err := service.CancelOrder(context.Background(), TradingCancelOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "missing-order",
	})

	if !errors.Is(err, ErrTradingOrderNotFound) {
		t.Fatalf("CancelOrder() error = %v, want %v", err, ErrTradingOrderNotFound)
	}
}

func TestTradingServiceRejectsPlaceOrderWhenAvailableBalanceInsufficient(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	creditAvailableBalance(t, ledger, "user-1", "USDC", "10")
	service := NewTradingService(NewMemoryTradingStore(), ledger, &recordingTradingSubmitter{})

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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

	if !errors.Is(err, ErrLedgerInsufficientBalance) {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, ErrLedgerInsufficientBalance)
	}
}

func TestTradingServiceRefundsAvailableBalanceWhenDYDXSubmitFails(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	want := errors.New("dydx height provider is not configured")
	service := NewTradingService(NewMemoryTradingStore(), ledger, &failingTradingSubmitter{err: want})

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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

	if !errors.Is(err, want) {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, want)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
	order, ok, err := service.store.GetOrderByClientID(context.Background(), "user-1", "client-order-1")
	if err != nil {
		t.Fatalf("GetOrderByClientID() error = %v", err)
	}
	if !ok || order.Status != TradingOrderStatusRejected {
		t.Fatalf("order = %+v ok=%v, want rejected order", order, ok)
	}
}

func TestTradingServiceRefundsAvailableBalanceWhenDYDXSubmitReturnsRejected(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	submitter := &resultTradingSubmitter{result: DYDXTradingResult{Status: TradingOrderStatusRejected}}
	service := NewTradingService(NewMemoryTradingStore(), ledger, submitter)

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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

	if err == nil || !errors.Is(err, ErrDYDXOrderRejected) {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, ErrDYDXOrderRejected)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
	order, ok, err := service.store.GetOrderByClientID(context.Background(), "user-1", "client-order-1")
	if err != nil {
		t.Fatalf("GetOrderByClientID() error = %v", err)
	}
	if !ok || order.Status != TradingOrderStatusRejected {
		t.Fatalf("order = %+v ok=%v, want rejected order", order, ok)
	}
}

func TestTradingServiceRefundsAvailableBalanceWhenOrderSaveFails(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "USDC", "100")
	want := errors.New("trading store down")
	service := NewTradingService(&failingSaveTradingStore{err: want}, ledger, &recordingTradingSubmitter{})

	_, err := service.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
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

	if !errors.Is(err, want) {
		t.Fatalf("PlaceOrder() error = %v, want %v", err, want)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
}

func creditAvailableBalance(t *testing.T, ledger *LedgerService, userID, asset, amount string) {
	t.Helper()
	if _, err := ledger.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
		RefID:  "deposit-" + userID + "-" + asset,
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
}

type recordingTradingSubmitter struct {
	intent DYDXIntent
	calls  int
}

func (s *recordingTradingSubmitter) Submit(ctx context.Context, intent DYDXIntent) error {
	s.intent = intent
	s.calls++
	return nil
}

type failingTradingSubmitter struct {
	err error
}

func (s *failingTradingSubmitter) Submit(ctx context.Context, intent DYDXIntent) error {
	return s.err
}

type resultTradingSubmitter struct {
	result DYDXTradingResult
	err    error
	intent DYDXIntent
	calls  int
}

func (s *resultTradingSubmitter) Submit(ctx context.Context, intent DYDXIntent) error {
	_, err := s.SubmitWithResult(ctx, intent)
	return err
}

func (s *resultTradingSubmitter) SubmitWithResult(ctx context.Context, intent DYDXIntent) (DYDXTradingResult, error) {
	s.intent = intent
	s.calls++
	return s.result, s.err
}

type recordingDYDXSubaccountDepositor struct {
	request DYDXSubaccountDepositRequest
	result  DYDXSubaccountDepositResult
	err     error
	calls   int
}

func (d *recordingDYDXSubaccountDepositor) DepositToSubaccount(ctx context.Context, request DYDXSubaccountDepositRequest) (DYDXSubaccountDepositResult, error) {
	d.request = request
	d.calls++
	return d.result, d.err
}

type failingSaveTradingStore struct {
	err error
}

type fixedTradingMarginQuoter struct {
	quote TradingMarginQuote
	err   error
}

func (q fixedTradingMarginQuoter) QuoteMargin(ctx context.Context, request TradingMarginQuoteRequest) (TradingMarginQuote, error) {
	return q.quote, q.err
}

type recordingTradingMarginQuoter struct {
	request TradingMarginQuoteRequest
	quote   TradingMarginQuote
	err     error
}

func (q *recordingTradingMarginQuoter) QuoteMargin(ctx context.Context, request TradingMarginQuoteRequest) (TradingMarginQuote, error) {
	q.request = request
	return q.quote, q.err
}

type staticTradingMarketPriceSource struct {
	snapshots map[string]TradingMarketPriceSnapshot
	err       error
}

func (s staticTradingMarketPriceSource) MarketPrice(ctx context.Context, request TradingMarketPriceRequest) (TradingMarketPriceSnapshot, error) {
	if s.err != nil {
		return TradingMarketPriceSnapshot{}, s.err
	}
	snapshot, ok := s.snapshots[request.Market]
	if !ok {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceNotFound
	}
	return snapshot, nil
}

type fixedTradingFundingPreparer struct {
	request TradingFundingPrepareRequest
	result  TradingFundingPrepareResult
	err     error
}

type fixedDYDXAccountResolver struct {
	account string
	err     error
}

func (r fixedDYDXAccountResolver) DYDXAccountForUser(context.Context, string) (string, error) {
	return r.account, r.err
}

type recordingDYDXOrderReconcileSource struct {
	request DYDXOrderReconcileRequest
	result  DYDXOrderReconcileResult
	err     error
	calls   int
}

func (s *recordingDYDXOrderReconcileSource) ReconcileOrder(_ context.Context, request DYDXOrderReconcileRequest) (DYDXOrderReconcileResult, error) {
	s.request = request
	s.calls++
	return s.result, s.err
}

type subaccountAwarePositionProvider struct {
	position            TradingPosition
	requestedSubaccount uint32
}

func (p *subaccountAwarePositionProvider) GetPosition(context.Context, string, string) (TradingPosition, error) {
	return TradingPosition{}, ErrTradingSyncEventNotFound
}

func (p *subaccountAwarePositionProvider) GetPositionForSubaccount(_ context.Context, userID, market string, subaccountNumber uint32) (TradingPosition, error) {
	p.requestedSubaccount = subaccountNumber
	if p.position.UserID == userID && p.position.Market == market && p.position.SubaccountNumber == subaccountNumber {
		return p.position, nil
	}
	return TradingPosition{}, ErrTradingSyncEventNotFound
}

func (p *fixedTradingFundingPreparer) PrepareFunding(ctx context.Context, request TradingFundingPrepareRequest) (TradingFundingPrepareResult, error) {
	p.request = request
	return p.result, p.err
}

func (s *failingSaveTradingStore) GetOrderByClientID(ctx context.Context, userID, clientOrderID string) (TradingOrder, bool, error) {
	return TradingOrder{}, false, nil
}

func (s *failingSaveTradingStore) GetOrderByID(ctx context.Context, userID, orderID string) (TradingOrder, bool, error) {
	return TradingOrder{}, false, nil
}

func (s *failingSaveTradingStore) SaveOrder(ctx context.Context, order TradingOrder) error {
	return s.err
}
