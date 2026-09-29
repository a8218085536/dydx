package tyche

import (
	"context"
	"testing"
)

func TestTradingAccountValuationServiceTotalsWalletLedgerAndTradingExposure(t *testing.T) {
	userID := "user-1"
	necoAddress := "0x0000000000000000000000000000000000000001"
	dedxToken := "0x0000000000000000000000000000000000000ded"
	usdtToken := "0x0000000000000000000000000000000000000usd"
	mappings := []AssetMapping{
		{
			NECOTokenAddress:  dedxToken,
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
		{
			NECOTokenAddress:  usdtToken,
			NECOAsset:         "USDT",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "USDT-USD",
			EnabledForTrading: true,
		},
	}
	ledger := NewLedgerService(NewMemoryLedgerStore())
	creditAvailableBalance(t, ledger, userID, "DEDX", "1500000000000000000")
	if _, err := ledger.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    userID,
		Asset:     "DEDX",
		Amount:    "500000000000000000",
		ToAddress: necoAddress,
	}); err != nil {
		t.Fatalf("FreezeWithdrawal(DEDX) error = %v", err)
	}
	creditAvailableBalance(t, ledger, userID, "USDT", "8000000000000000000")
	if _, err := ledger.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    userID,
		Asset:     "USDT",
		Amount:    "1000000000000000000",
		ToAddress: necoAddress,
	}); err != nil {
		t.Fatalf("FreezeWithdrawal(USDT) error = %v", err)
	}
	balances := NewTradingBalanceService(ledger, fixedNECOTokenBalanceReader{
		balances: map[string]string{
			normalizeAddress(dedxToken) + ":" + normalizeAddress(necoAddress): "2000000000000000000",
			normalizeAddress(usdtToken) + ":" + normalizeAddress(necoAddress): "5000000000000000000",
		},
	}, mappings)

	tradingStore := NewMemoryTradingStore()
	if err := tradingStore.SaveOrder(context.Background(), TradingOrder{
		ID:               "open-order-1",
		UserID:           userID,
		ClientOrderID:    "client-open-1",
		Market:           "ETH-USD",
		SubaccountNumber: 0,
		Status:           TradingOrderStatusSubmitted,
		MarginUSD:        "25",
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := tradingStore.SaveOrder(context.Background(), TradingOrder{
		ID:                 "stale-market-order-1",
		UserID:             userID,
		ClientOrderID:      "client-market-stale-1",
		Market:             "ETH-USD",
		SubaccountNumber:   0,
		OrderType:          TradingOrderTypeMarket,
		ExecutionOrderType: TradingOrderTypeIOC,
		Status:             TradingOrderStatusPartialFilled,
		MarginUSD:          "999",
	}); err != nil {
		t.Fatalf("SaveOrder(stale market) error = %v", err)
	}
	syncStore := NewMemoryTradingSyncStore()
	syncService := NewTradingSyncService(syncStore, tradingStore, ledger)
	if _, err := syncService.SyncPosition(context.Background(), TradingPositionSyncRequest{
		EventID:          "position-1",
		UserID:           userID,
		Market:           "ETH-USD",
		SubaccountNumber: 0,
		Size:             "0.5",
		UnrealizedPNL:    "-5",
		MarginAsset:      "USDT",
		Margin:           "100",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	summary := NewTradingAccountSummaryService(tradingStore, syncService, []DYDXMarketConfig{
		{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000", SubticksPerUnit: "1000000"},
	})
	service := NewTradingAccountValuationService(balances, summary, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"DEDX": {ID: "dedx-price-1", Asset: "DEDX", USDPrice: "4", RouteName: "DEDX-NECO-USD"},
			"USDT": {ID: "usdt-price-1", Asset: "USDT", USDPrice: "1", RouteName: "USDT-USD"},
		},
	}, mappings)

	valuation, err := service.Valuation(context.Background(), TradingAccountValuationRequest{
		UserID:      userID,
		NECOAddress: necoAddress,
	})

	if err != nil {
		t.Fatalf("Valuation() error = %v", err)
	}
	if valuation.TotalValueUSDT != "147" {
		t.Fatalf("TotalValueUSDT = %q, want 147", valuation.TotalValueUSDT)
	}
	if len(valuation.Assets) != 2 {
		t.Fatalf("assets = %+v, want DEDX and USDT", valuation.Assets)
	}
	if valuation.Assets[0].Asset != "DEDX" || valuation.Assets[0].TotalAmount != "3.5" || valuation.Assets[0].ValueUSDT != "14" {
		t.Fatalf("DEDX asset = %+v, want 3.5 DEDX worth 14 USDT", valuation.Assets[0])
	}
	if valuation.Assets[1].Asset != "USDT" || valuation.Assets[1].TotalAmount != "13" || valuation.Assets[1].ValueUSDT != "13" {
		t.Fatalf("USDT asset = %+v, want 13 USDT worth 13 USDT", valuation.Assets[1])
	}
	if valuation.Trading.PositionMarginUSD != "100" || valuation.Trading.OpenOrderMarginUSD != "25" || valuation.Trading.UnrealizedPNLUSD != "-5" || valuation.Trading.EstimatedEquityUSD != "120" {
		t.Fatalf("trading valuation = %+v, want position=100 open=25 pnl=-5 equity=120", valuation.Trading)
	}
}
