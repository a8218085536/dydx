package app

import (
	"bytes"
	"context"
	"ethgo/tyche"
	"net/http"
	"testing"
)

func TestTradingPlaceOrderAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	submitter := &appRecordingDYDXSubmitter{}
	app := &App{
		ledger:  ledger,
		trading: tyche.NewTradingService(tyche.NewMemoryTradingStore(), ledger, submitter),
	}

	rec := postJSON(t, app, "/tyche/api/trading/place-order", map[string]interface{}{
		"userId":        "user-1",
		"clientOrderId": "client-order-1",
		"market":        "ETH-USD",
		"side":          "buy",
		"orderType":     "limit",
		"price":         "2500",
		"size":          "1",
		"marginAsset":   "USDC",
		"marginAmount":  "25",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if submitter.intent.Action != tyche.DYDXActionPlaceOrder {
		t.Fatalf("intent = %+v, want place order", submitter.intent)
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=75 frozen=0", balance)
	}
}

func TestTradingPlaceMarketOrderAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	submitter := &appRecordingDYDXSubmitter{}
	trading := tyche.NewTradingService(tyche.NewMemoryTradingStore(), ledger, submitter)
	trading.SetMarketPriceSource(appStaticTradingMarketPriceSource{
		snapshots: map[string]tyche.TradingMarketPriceSnapshot{
			"ETH-USD": {
				Market:     "ETH-USD",
				Price:      "2500",
				Source:     "test-price-source",
				SnapshotID: "dydx-price-snapshot",
			},
		},
	})
	trading.SetMarketOrderSlippageBps(100)
	app := &App{
		ledger:  ledger,
		trading: trading,
	}

	rec := postJSON(t, app, "/tyche/api/trading/place-order", map[string]interface{}{
		"userId":        "user-1",
		"clientOrderId": "client-order-market",
		"market":        "ETH-USD",
		"side":          "buy",
		"orderType":     "market",
		"size":          "1",
		"marginAsset":   "USDC",
		"marginAmount":  "25",
	})

	assertResponseCode(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"orderType":"market"`)) {
		t.Fatalf("body=%s, want market order type", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"executionOrderType":"ioc"`)) || !bytes.Contains([]byte(body), []byte(`"price":"2525"`)) {
		t.Fatalf("body=%s, want protected ioc price", body)
	}
	if submitter.intent.Params["orderType"] != tyche.TradingOrderTypeIOC {
		t.Fatalf("submitted orderType = %v, want ioc", submitter.intent.Params["orderType"])
	}
}

func TestTradingCancelOrderAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	submitter := &appRecordingDYDXSubmitter{}
	trading := tyche.NewTradingService(tyche.NewMemoryTradingStore(), ledger, submitter)
	app := &App{ledger: ledger, trading: trading}
	if _, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	}); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}

	rec := postJSON(t, app, "/tyche/api/trading/cancel-order", map[string]interface{}{
		"userId":        "user-1",
		"clientOrderId": "client-order-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if submitter.intent.Action != tyche.DYDXActionCancelOrder {
		t.Fatalf("intent = %+v, want cancel order", submitter.intent)
	}
}

func TestTradingClosePositionAPI(t *testing.T) {
	tradingStore := tyche.NewMemoryTradingStore()
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	syncStore := tyche.NewMemoryTradingSyncStore()
	syncService := tyche.NewTradingSyncService(syncStore, tradingStore, ledger)
	if _, err := syncService.SyncPosition(context.Background(), tyche.TradingPositionSyncRequest{
		EventID:     "position-1",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "-0.25",
		MarginAsset: "DEDX",
		Margin:      "5",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	submitter := &appRecordingDYDXSubmitter{}
	trading := tyche.NewTradingService(tradingStore, ledger, submitter)
	trading.SetPositionProvider(syncService)
	app := &App{ledger: ledger, trading: trading}

	rec := postJSON(t, app, "/tyche/api/trading/close-position", map[string]interface{}{
		"userId":        "user-1",
		"clientOrderId": "close-order-1",
		"market":        "ETH-USD",
		"orderType":     "limit",
		"price":         "2400",
	})

	assertResponseCode(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"status":"close_pending"`)) {
		t.Fatalf("body=%s, want close_pending status", body)
	}
	if submitter.intent.Action != tyche.DYDXActionPlaceOrder {
		t.Fatalf("intent = %+v, want place order", submitter.intent)
	}
	if submitter.intent.Params["side"] != "buy" || submitter.intent.Params["reduceOnly"] != true {
		t.Fatalf("intent params = %+v, want buy reduceOnly close", submitter.intent.Params)
	}
}

func TestTradingGetOrderAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	trading := tyche.NewTradingService(tyche.NewMemoryTradingStore(), ledger, &appRecordingDYDXSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
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
	app := &App{trading: trading}

	rec := postJSON(t, app, "/tyche/api/trading/order/get", map[string]interface{}{
		"userId":  "user-1",
		"orderId": order.ID,
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"clientOrderId":"client-order-1"`)) {
		t.Fatalf("body=%s, want client order id", rec.Body.String())
	}
}

func TestTradingOrderListAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	if _, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	}); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	app := &App{trading: trading}

	rec := postJSON(t, app, "/tyche/api/trading/order/list", map[string]interface{}{
		"userId": "user-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"clientOrderId":"client-order-1"`)) {
		t.Fatalf("body=%s, want order list", rec.Body.String())
	}
}

func TestTradingAccountSummaryAPIIsPublic(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	syncStore := tyche.NewMemoryTradingSyncStore()
	syncService := tyche.NewTradingSyncService(syncStore, tradingStore, ledger)
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "summary-open-order",
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
	order.Status = tyche.TradingOrderStatusSubmitted
	if err := tradingStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	app := &App{
		tradingAccountSummary: tyche.NewTradingAccountSummaryService(trading, syncService, []tyche.DYDXMarketConfig{
			{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000", SubticksPerUnit: "1000000"},
		}),
	}

	rec := postJSON(t, app, "/tyche/api/trading/account/summary", map[string]interface{}{
		"userId": "user-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"clientOrderId":"summary-open-order"`)) ||
		!bytes.Contains([]byte(body), []byte(`"makerFeeRate":"-0.0003"`)) {
		t.Fatalf("body=%s, want open order and public summary fee rate", body)
	}
}

func TestTradingAccountSummaryAPIResolvesUserIDFromNECOAddress(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
		UserID:        "user-1",
		ClientOrderID: "summary-neco-order",
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
	order.Status = tyche.TradingOrderStatusSubmitted
	if err := tradingStore.SaveOrder(context.Background(), order); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	bindings := tyche.NewMemoryWalletBindingStore()
	if err := bindings.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	syncStore := tyche.NewMemoryTradingSyncStore()
	syncService := tyche.NewTradingSyncService(syncStore, tradingStore, ledger)
	app := &App{
		walletBindings: tyche.NewWalletBindingService(bindings, appAcceptBindingVerifier{}),
		tradingAccountSummary: tyche.NewTradingAccountSummaryService(trading, syncService, []tyche.DYDXMarketConfig{
			{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000", SubticksPerUnit: "1000000"},
		}),
	}

	rec := postJSON(t, app, "/tyche/api/trading/account/summary", map[string]interface{}{
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})

	assertResponseCode(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"userId":"user-1"`)) ||
		!bytes.Contains([]byte(body), []byte(`"clientOrderId":"summary-neco-order"`)) {
		t.Fatalf("body=%s, want summary resolved from neco address", body)
	}
}

func TestTradingAccountValuationAPIRequiresInternalKeyAndReturnsTotal(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "1000000000000000000",
		RefID:  "valuation-deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	mappings := []tyche.AssetMapping{
		{
			NECOTokenAddress:  "0x0000000000000000000000000000000000000ded",
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
	}
	balances := tyche.NewTradingBalanceService(ledger, appFixedNECOTokenBalanceReader{
		balance: "2000000000000000000",
	}, mappings)
	tradingStore := tyche.NewMemoryTradingStore()
	syncStore := tyche.NewMemoryTradingSyncStore()
	sync := tyche.NewTradingSyncService(syncStore, tradingStore, ledger)
	summary := tyche.NewTradingAccountSummaryService(tradingStore, sync, nil)
	app := &App{
		conf: &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		tradingAccountValuation: tyche.NewTradingAccountValuationService(balances, summary, appStaticAssetUSDOracle{
			snapshots: map[string]tyche.AssetPriceSnapshot{
				"DEDX": {ID: "dedx-price-1", Asset: "DEDX", USDPrice: "4", RouteName: "DEDX-NECO-USD"},
			},
		}, mappings),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	unauthorized := postJSONWithAPIKey(t, app, "/tyche/api/trading/account/valuation", "", map[string]interface{}{
		"userId":      "user-1",
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})
	assertResponseCode(t, unauthorized, http.StatusUnauthorized)

	rec := postJSONWithAPIKey(t, app, "/tyche/api/trading/account/valuation", "secret", map[string]interface{}{
		"userId":      "user-1",
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})
	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"totalValueUsdt":"12"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"totalAmount":"3"`)) {
		t.Fatalf("body=%s, want DEDX valuation total", rec.Body.String())
	}
}

func TestTradingAccountValuationAPIResolvesUserIDFromNECOAddress(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "1000000000000000000",
		RefID:  "valuation-deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	mappings := []tyche.AssetMapping{
		{
			NECOTokenAddress:  "0x0000000000000000000000000000000000000ded",
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
	}
	bindings := tyche.NewMemoryWalletBindingStore()
	if err := bindings.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	balances := tyche.NewTradingBalanceService(ledger, appFixedNECOTokenBalanceReader{
		balance: "2000000000000000000",
	}, mappings)
	tradingStore := tyche.NewMemoryTradingStore()
	syncStore := tyche.NewMemoryTradingSyncStore()
	sync := tyche.NewTradingSyncService(syncStore, tradingStore, ledger)
	summary := tyche.NewTradingAccountSummaryService(tradingStore, sync, nil)
	app := &App{
		conf: &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		walletBindings: tyche.NewWalletBindingService(
			bindings,
			appAcceptBindingVerifier{},
		),
		tradingAccountValuation: tyche.NewTradingAccountValuationService(balances, summary, appStaticAssetUSDOracle{
			snapshots: map[string]tyche.AssetPriceSnapshot{
				"DEDX": {ID: "dedx-price-1", Asset: "DEDX", USDPrice: "4", RouteName: "DEDX-NECO-USD"},
			},
		}, mappings),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	rec := postJSONWithAPIKey(t, app, "/tyche/api/trading/account/valuation", "secret", map[string]interface{}{
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"userId":"user-1"`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte(`"totalValueUsdt":"12"`)) {
		t.Fatalf("body=%s, want valuation resolved from neco address", rec.Body.String())
	}
}

func TestTradingSettlementGetAndListAPI(t *testing.T) {
	settlements := tyche.NewMemoryTradingSettlementStore()
	settlement, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:                 "settlement-1",
		Kind:               tyche.TradingSettlementKindCancelOrder,
		UserID:             "user-1",
		CloseOrderID:       "close-order-1",
		CloseClientOrderID: "close-client-1",
		SettlementAsset:    "DEDX",
		SettlementAmount:   "5000000000000000000",
		Status:             tyche.TradingSettlementStatusNECOPayoutBroadcasted,
		SystemReserved:     true,
	})
	if err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	app := &App{tradingSettlements: settlements}

	get := postJSON(t, app, "/tyche/api/trading/settlement/get", map[string]interface{}{
		"userId":       "user-1",
		"settlementId": settlement.ID,
	})
	assertResponseCode(t, get, http.StatusOK)
	if !bytes.Contains(get.Body.Bytes(), []byte(`"status":"neco_payout_broadcasted"`)) {
		t.Fatalf("get body=%s", get.Body.String())
	}

	getByOrderID := postJSON(t, app, "/tyche/api/trading/settlement/get", map[string]interface{}{
		"userId":  "user-1",
		"orderId": "close-order-1",
	})
	assertResponseCode(t, getByOrderID, http.StatusOK)
	if !bytes.Contains(getByOrderID.Body.Bytes(), []byte(`"kind":"cancel_order"`)) {
		t.Fatalf("get by orderId body=%s", getByOrderID.Body.String())
	}

	list := postJSON(t, app, "/tyche/api/trading/settlement/list", map[string]interface{}{
		"userId": "user-1",
	})
	assertResponseCode(t, list, http.StatusOK)
	if !bytes.Contains(list.Body.Bytes(), []byte(`"closeOrderId":"close-order-1"`)) {
		t.Fatalf("list body=%s", list.Body.String())
	}
}

func TestTradingSettlementManualReviewListAPI(t *testing.T) {
	settlements := tyche.NewMemoryTradingSettlementStore()
	if _, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:              "settlement-manual-1",
		UserID:          "user-1",
		CloseOrderID:    "close-order-1",
		SettlementAsset: "DEDX",
		SystemReserved:  true,
		Status:          tyche.TradingSettlementStatusManualReviewRequired,
		Reason:          "missing fill accounting",
	}); err != nil {
		t.Fatalf("CreateIfAbsent() manual error = %v", err)
	}
	if _, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:               "settlement-paid-1",
		UserID:           "user-1",
		CloseOrderID:     "close-order-2",
		SettlementAsset:  "DEDX",
		SettlementAmount: "5000000000000000000",
		SystemReserved:   true,
		Status:           tyche.TradingSettlementStatusNECOPayoutPaid,
	}); err != nil {
		t.Fatalf("CreateIfAbsent() paid error = %v", err)
	}
	app := &App{tradingSettlements: settlements}

	rec := postJSON(t, app, "/tyche/api/trading/settlement/manual-review/list", map[string]interface{}{
		"userId": "user-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"id":"settlement-manual-1"`)) {
		t.Fatalf("body=%s, want manual review settlement", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"id":"settlement-paid-1"`)) {
		t.Fatalf("body=%s, should not include paid settlement", rec.Body.String())
	}
}

func TestTradingSettlementManualPayoutAPIIsIdempotent(t *testing.T) {
	settlements := tyche.NewMemoryTradingSettlementStore()
	if _, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:                   "settlement-manual-1",
		UserID:               "user-1",
		NECOAddress:          "0x0000000000000000000000000000000000000001",
		CloseOrderID:         "close-order-1",
		SettlementAsset:      "DEDX",
		SystemReserved:       true,
		Status:               tyche.TradingSettlementStatusManualReviewRequired,
		Reason:               "missing full accounting",
		DYDXReservedQuantums: "20000000",
	}); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	ledgerStore := tyche.NewMemoryLedgerStore()
	app := &App{
		tradingSettlements: settlements,
		ledger:             tyche.NewLedgerService(ledgerStore),
		closeSettlement: tyche.NewCloseSettlementService(settlements, tyche.NewLedgerService(ledgerStore), nil, nil, nil, tyche.CloseSettlementConfig{
			SettlementAsset:         "DEDX",
			SettlementAssetDecimals: 18,
		}),
	}
	body := map[string]interface{}{
		"userId":           "user-1",
		"settlementId":     "settlement-manual-1",
		"settlementUsd":    "20",
		"settlementAmount": "5000000000000000000",
		"reason":           "admin confirmed fill accounting",
	}

	first := postJSON(t, app, "/tyche/api/trading/settlement/manual-payout", body)
	second := postJSON(t, app, "/tyche/api/trading/settlement/manual-payout", body)

	assertResponseCode(t, first, http.StatusOK)
	assertResponseCode(t, second, http.StatusOK)
	if !bytes.Contains(second.Body.Bytes(), []byte(`"status":"dedx_credited"`)) {
		t.Fatalf("body=%s, want credited settlement", second.Body.String())
	}
	if journals := ledgerStore.Journals(); len(journals) != 2 {
		t.Fatalf("journal count=%d, want 2 idempotent credit+freeze journals", len(journals))
	}
}

func TestTradingSettlementRejectAPI(t *testing.T) {
	settlements := tyche.NewMemoryTradingSettlementStore()
	if _, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:              "settlement-manual-1",
		UserID:          "user-1",
		CloseOrderID:    "close-order-1",
		SettlementAsset: "DEDX",
		Status:          tyche.TradingSettlementStatusManualReviewRequired,
		Reason:          "invalid accounting",
	}); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	app := &App{
		tradingSettlements: settlements,
		closeSettlement:    tyche.NewCloseSettlementService(settlements, tyche.NewLedgerService(tyche.NewMemoryLedgerStore()), nil, nil, nil, tyche.CloseSettlementConfig{}),
	}

	rec := postJSON(t, app, "/tyche/api/trading/settlement/reject", map[string]interface{}{
		"userId":       "user-1",
		"settlementId": "settlement-manual-1",
		"reason":       "admin rejected negative pnl",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"rejected"`)) {
		t.Fatalf("body=%s, want rejected settlement", rec.Body.String())
	}
}

func TestTradingReconciliationReportFlagsSettlementIssues(t *testing.T) {
	settlements := tyche.NewMemoryTradingSettlementStore()
	if _, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:                 "settlement-1",
		UserID:             "user-1",
		CloseOrderID:       "close-order-1",
		SettlementAsset:    "DEDX",
		SettlementAmount:   "0",
		LedgerWithdrawalID: "missing-withdrawal",
		SystemReserved:     true,
		Status:             tyche.TradingSettlementStatusNECOPayoutBroadcasted,
	}); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	app := &App{
		tradingSettlements: settlements,
		ledger:             tyche.NewLedgerService(tyche.NewMemoryLedgerStore()),
	}

	rec := postJSON(t, app, "/tyche/api/trading/reconciliation/report", map[string]interface{}{
		"userId":       "user-1",
		"settlementId": "settlement-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"ledger_withdrawal_missing"`)) {
		t.Fatalf("body=%s, want ledger missing issue", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"invalid_settlement_amount"`)) {
		t.Fatalf("body=%s, want invalid amount issue", rec.Body.String())
	}
}

func TestTradingBalanceOverviewAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "20",
		RefID:  "deposit-dedx-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	app := &App{
		tradingBalances: tyche.NewTradingBalanceService(ledger, appFixedNECOTokenBalanceReader{
			balance: "80",
		}, []tyche.AssetMapping{
			{
				NECOTokenAddress: "0x0000000000000000000000000000000000000ded",
				NECOAsset:        "DEDX",
				DYDXAsset:        "DEDX",
			},
		}),
	}

	rec := postJSON(t, app, "/tyche/api/trading/balance/overview", map[string]interface{}{
		"userId":      "user-1",
		"necoAddress": "0x0000000000000000000000000000000000000001",
		"asset":       "DEDX",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"displayAvailable":"100"`)) {
		t.Fatalf("body=%s, want combined display balance", rec.Body.String())
	}
}

func TestTradingBalanceOverviewAPIDefaultsAssetWhenMissing(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "20",
		RefID:  "deposit-dedx-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	app := &App{
		tradingBalances: tyche.NewTradingBalanceService(ledger, nil, []tyche.AssetMapping{
			{
				NECOTokenAddress:  "0x0000000000000000000000000000000000000ded",
				NECOAsset:         "DEDX",
				DYDXAsset:         "USD",
				EnabledForTrading: true,
			},
			{
				NECOTokenAddress:  "0x0000000000000000000000000000000000000ec0",
				NECOAsset:         "NECO",
				DYDXAsset:         "USD",
				EnabledForTrading: true,
			},
		}),
	}

	rec := postJSON(t, app, "/tyche/api/trading/balance/overview", map[string]interface{}{
		"userId": "user-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"asset":"DEDX"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"displayAvailable":"20"`)) {
		t.Fatalf("body=%s, want default DEDX balance", rec.Body.String())
	}
}

func TestTradingMarketsAPI(t *testing.T) {
	app := &App{conf: &Config{Tyche: &tyche.Config{
		DYDXMarkets: []tyche.DYDXMarketConfig{
			{Market: "BTC-USD", ClobPairID: 0, QuantumsPerUnit: "10000000000", SubticksPerUnit: "1000000"},
			{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000", SubticksPerUnit: "1000000"},
		},
	}}}

	rec := postJSON(t, app, "/tyche/api/trading/markets", map[string]interface{}{})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"market":"ETH-USD"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"clobPairId":1`)) {
		t.Fatalf("body=%s, want ETH-USD market", rec.Body.String())
	}
}

func TestTradingAssetsAPIReturnsOnlyTradableAssets(t *testing.T) {
	app := &App{conf: &Config{Tyche: &tyche.Config{
		AssetMappings: []tyche.AssetMapping{
			{
				NECOTokenAddress:  "0x0000000000000000000000000000000000000ded",
				NECOAsset:         "DEDX",
				DYDXAsset:         "USD",
				Decimals:          18,
				PriceRoute:        "DEDX-NECO-USD",
				EnabledForTrading: true,
			},
			{
				NECOTokenAddress:  "0x0000000000000000000000000000000000000ddd",
				NECOAsset:         "USDT",
				DYDXAsset:         "USD",
				Decimals:          18,
				PriceRoute:        "USDT-USD",
				EnabledForTrading: true,
			},
			{
				NECOTokenAddress:  "0x0000000000000000000000000000000000000aaa",
				NECOAsset:         "LOCKED",
				DYDXAsset:         "USD",
				Decimals:          18,
				PriceRoute:        "LOCKED-USD",
				EnabledForTrading: false,
			},
		},
	}}}

	rec := postJSON(t, app, "/tyche/api/trading/assets", map[string]interface{}{})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"necoAsset":"USDT"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"priceRoute":"USDT-USD"`)) {
		t.Fatalf("body=%s, want USDT tradable asset", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"necoAsset":"LOCKED"`)) {
		t.Fatalf("body=%s, should not include disabled trading asset", rec.Body.String())
	}
}

func TestTradingQuoteAPI(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	trading := tyche.NewTradingService(tyche.NewMemoryTradingStore(), ledger, &appRecordingDYDXSubmitter{})
	trading.SetMarginQuoter(tyche.NewAssetMarginQuoter([]tyche.AssetMapping{
		{
			NECOAsset:         "USDT",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "USDT-USD",
			EnabledForTrading: true,
		},
	}, appStaticAssetUSDOracle{
		snapshots: map[string]tyche.AssetPriceSnapshot{
			"USDT": {
				ID:        "snapshot-usdt",
				Asset:     "USDT",
				USDPrice:  "1",
				RouteName: "USDT-USD",
			},
		},
	}))
	app := &App{trading: trading}

	rec := postJSON(t, app, "/tyche/api/trading/quote", map[string]interface{}{
		"market":      "ETH-USD",
		"price":       "5",
		"size":        "2",
		"marginAsset": "USDT",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"chargeAsset":"USDT"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"chargeAmount":"10000000000000000000"`)) {
		t.Fatalf("body=%s, want USDT quote", rec.Body.String())
	}
}

func TestDYDXSyncFillAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
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
	app := &App{
		ledger:      ledger,
		trading:     trading,
		tradingSync: tyche.NewTradingSyncService(tyche.NewMemoryTradingSyncStore(), tradingStore, ledger),
	}

	rec := postJSON(t, app, "/tyche/api/dydx/sync/fill", map[string]interface{}{
		"eventId":        "fill-1",
		"userId":         "user-1",
		"orderId":        order.ID,
		"market":         "ETH-USD",
		"asset":          "USDC",
		"releasedMargin": "25",
		"fee":            "2",
		"realizedPnl":    "5",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"eventId":"fill-1"`)) {
		t.Fatalf("body=%s, want fill event", rec.Body.String())
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "103" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=103 frozen=0", balance)
	}
}

func TestDYDXSyncPositionAPI(t *testing.T) {
	syncStore := tyche.NewMemoryTradingSyncStore()
	app := &App{
		tradingSync: tyche.NewTradingSyncService(syncStore, tyche.NewMemoryTradingStore(), tyche.NewLedgerService(tyche.NewMemoryLedgerStore())),
	}

	rec := postJSON(t, app, "/tyche/api/dydx/sync/position", map[string]interface{}{
		"eventId":       "position-1",
		"userId":        "user-1",
		"market":        "ETH-USD",
		"size":          "1.5",
		"entryPrice":    "2500",
		"markPrice":     "2600",
		"unrealizedPnl": "150",
		"marginAsset":   "USDC",
		"margin":        "500",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"unrealizedPnl":"150"`)) {
		t.Fatalf("body=%s, want unrealized pnl", rec.Body.String())
	}

	query := postJSON(t, app, "/tyche/api/dydx/sync/position/get", map[string]interface{}{
		"userId": "user-1",
		"market": "ETH-USD",
	})
	assertResponseCode(t, query, http.StatusOK)
	if !bytes.Contains(query.Body.Bytes(), []byte(`"eventId":"position-1"`)) {
		t.Fatalf("body=%s, want position event", query.Body.String())
	}
}

func TestDYDXSyncPositionGetAPIUsesMarketSubaccount(t *testing.T) {
	syncStore := tyche.NewMemoryTradingSyncStore()
	tradingStore := tyche.NewMemoryTradingStore()
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	tradingService := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	tradingService.SetDYDXMarkets([]tyche.DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}})
	app := &App{
		trading:     tradingService,
		tradingSync: tyche.NewTradingSyncService(syncStore, tradingStore, ledger),
	}
	if _, err := app.tradingSync.SyncPosition(context.Background(), tyche.TradingPositionSyncRequest{
		EventID:          "position-dedx-127",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "2",
		MarginAsset:      "DEDX",
		Margin:           "40",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}

	rec := postJSON(t, app, "/tyche/api/dydx/sync/position/get", map[string]interface{}{
		"userId": "user-1",
		"market": "FDEX-USD",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"subaccountNumber":127`)) {
		t.Fatalf("body=%s, want FDEX-USD position from subaccount 127", rec.Body.String())
	}
}

func TestDYDXSyncPositionGetAPIReturnsEmptyPositionWhenNoneExists(t *testing.T) {
	syncStore := tyche.NewMemoryTradingSyncStore()
	tradingStore := tyche.NewMemoryTradingStore()
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	tradingService := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	tradingService.SetDYDXMarkets([]tyche.DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}})
	app := &App{
		trading:     tradingService,
		tradingSync: tyche.NewTradingSyncService(syncStore, tradingStore, ledger),
	}

	rec := postJSON(t, app, "/tyche/api/dydx/sync/position/get", map[string]interface{}{
		"userId": "user-1",
		"market": "FDEX-USD",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"size":"0"`)) {
		t.Fatalf("body=%s, want empty position size", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"subaccountNumber":127`)) {
		t.Fatalf("body=%s, want FDEX-USD empty position from subaccount 127", rec.Body.String())
	}
}

func TestDYDXSyncOrderStatusRejectedAPIRefundsBalance(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
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
	app := &App{
		ledger:      ledger,
		trading:     trading,
		tradingSync: tyche.NewTradingSyncService(tyche.NewMemoryTradingSyncStore(), tradingStore, ledger),
	}

	rec := postJSON(t, app, "/tyche/api/dydx/sync/order-status", map[string]interface{}{
		"eventId": "status-1",
		"userId":  "user-1",
		"orderId": order.ID,
		"status":  tyche.TradingOrderStatusRejected,
		"reason":  "dydx rejected",
	})

	assertResponseCode(t, rec, http.StatusOK)
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}

	query := postJSON(t, app, "/tyche/api/dydx/sync/order-status/get", map[string]interface{}{
		"eventId": "status-1",
	})
	assertResponseCode(t, query, http.StatusOK)
	if !bytes.Contains(query.Body.Bytes(), []byte(`"reason":"dydx rejected"`)) {
		t.Fatalf("body=%s, want rejected reason", query.Body.String())
	}
}

func TestDYDXSyncFillGetAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
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
	app := &App{
		ledger:      ledger,
		trading:     trading,
		tradingSync: tyche.NewTradingSyncService(tyche.NewMemoryTradingSyncStore(), tradingStore, ledger),
	}
	if _, err := app.tradingSync.SyncFill(context.Background(), tyche.TradingFillSyncRequest{
		EventID:        "fill-1",
		UserID:         "user-1",
		OrderID:        order.ID,
		Market:         "ETH-USD",
		Asset:          "USDC",
		ReleasedMargin: "25",
		Fee:            "2",
		RealizedPNL:    "5",
	}); err != nil {
		t.Fatalf("SyncFill() error = %v", err)
	}

	rec := postJSON(t, app, "/tyche/api/dydx/sync/fill/get", map[string]interface{}{
		"eventId": "fill-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"fee":"2"`)) {
		t.Fatalf("body=%s, want fill fee", rec.Body.String())
	}
}

func TestDYDXSyncFailuresAndRetryAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	ledger := tyche.NewLedgerService(store)
	creditAppAvailableBalance(t, ledger)
	tradingStore := tyche.NewMemoryTradingStore()
	trading := tyche.NewTradingService(tradingStore, ledger, &appRecordingDYDXSubmitter{})
	order, err := trading.PlaceOrder(context.Background(), tyche.TradingPlaceOrderRequest{
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
	syncStore := tyche.NewMemoryTradingSyncStore()
	app := &App{
		ledger:      ledger,
		trading:     trading,
		tradingSync: tyche.NewTradingSyncService(syncStore, tradingStore, ledger),
	}
	if _, err := app.tradingSync.SyncOrderStatus(context.Background(), tyche.TradingOrderStatusSyncRequest{
		EventID: "status-rejected-1",
		UserID:  "user-1",
		OrderID: order.ID,
		Status:  tyche.TradingOrderStatusRejected,
		Reason:  "mock rejected",
	}); err != nil {
		t.Fatalf("SyncOrderStatus() error = %v", err)
	}

	failures := postJSON(t, app, "/tyche/api/dydx/sync/failures", map[string]interface{}{
		"userId": "user-1",
	})
	assertResponseCode(t, failures, http.StatusOK)
	if !bytes.Contains(failures.Body.Bytes(), []byte(`"reason":"mock rejected"`)) {
		t.Fatalf("body=%s, want rejected failure", failures.Body.String())
	}

	retry := postJSON(t, app, "/tyche/api/dydx/sync/retry", map[string]interface{}{
		"eventId": "status-rejected-1",
	})
	assertResponseCode(t, retry, http.StatusOK)
	if !bytes.Contains(retry.Body.Bytes(), []byte(`"eventId":"status-rejected-1"`)) {
		t.Fatalf("body=%s, want retried event", retry.Body.String())
	}
}

func creditAppAvailableBalance(t *testing.T, ledger *tyche.LedgerService) {
	t.Helper()
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
}

type appFixedNECOTokenBalanceReader struct {
	balance string
	err     error
}

func (r appFixedNECOTokenBalanceReader) BalanceOf(ctx context.Context, tokenAddress, ownerAddress string) (string, error) {
	return r.balance, r.err
}

type appStaticAssetUSDOracle struct {
	snapshots map[string]tyche.AssetPriceSnapshot
}

func (o appStaticAssetUSDOracle) AssetUSDPrice(ctx context.Context, asset string) (tyche.AssetPriceSnapshot, error) {
	snapshot, ok := o.snapshots[asset]
	if !ok {
		return tyche.AssetPriceSnapshot{}, tyche.ErrNECOPriceOracleNotConfigured
	}
	return snapshot, nil
}

type appStaticTradingMarketPriceSource struct {
	snapshots map[string]tyche.TradingMarketPriceSnapshot
}

func (s appStaticTradingMarketPriceSource) MarketPrice(ctx context.Context, request tyche.TradingMarketPriceRequest) (tyche.TradingMarketPriceSnapshot, error) {
	snapshot, ok := s.snapshots[request.Market]
	if !ok {
		return tyche.TradingMarketPriceSnapshot{}, tyche.ErrTradingMarketPriceNotFound
	}
	return snapshot, nil
}
