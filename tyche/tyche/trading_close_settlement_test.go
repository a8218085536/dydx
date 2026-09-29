package tyche

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type recordingTradingCloseCallbackSender struct {
	calls   int
	payload TradingCloseCallbackPayload
	err     error
}

func (s *recordingTradingCloseCallbackSender) SendTradeClose(ctx context.Context, payload TradingCloseCallbackPayload) error {
	s.calls++
	s.payload = payload
	return s.err
}

type fixedCloseSettlementAssetOracle struct {
	asset      string
	price      string
	snapshotID string
}

func (o fixedCloseSettlementAssetOracle) AssetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if normalizeAsset(asset) != normalizeAsset(o.asset) {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	return AssetPriceSnapshot{
		ID:       o.snapshotID,
		Asset:    normalizeAsset(o.asset),
		USDAsset: "USD",
		USDPrice: o.price,
	}, nil
}

func closeSettlementOrderFixture() TradingOrder {
	return TradingOrder{
		ID:                  "close-1",
		UserID:              "user-1",
		NECOAddress:         "0x0000000000000000000000000000000000000001",
		DYDXAccount:         "dydx1abc",
		ClientOrderID:       "close-client-1",
		Market:              "ETH-USD",
		Side:                "SELL",
		Size:                "0.5",
		MarginAsset:         "DEDX",
		MarginUSD:           "20",
		DYDXFundingQuantums: "20000000",
		ClosePosition:       true,
		Status:              TradingOrderStatusClosed,
	}
}

func closeSettlementReconcileFixture() DYDXOrderReconcileResult {
	return DYDXOrderReconcileResult{
		Tx: DYDXTxReconcileSnapshot{Found: false, TxHash: "short-term-ioc-not-queryable"},
		Subaccount: DYDXSubaccountReconcileSnapshot{
			Found:              true,
			PerpetualPositions: nil,
		},
	}
}

func TestCloseSettlementServicePaysDEDXOnceForMarginOnlyClose(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	settlements := NewMemoryTradingSettlementStore()
	withdrawals := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &countingNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    1,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, withdrawals, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		},
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	service := NewCloseSettlementService(settlements, ledger, worker, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
	})

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile:  closeSettlementReconcileFixture(),
	})
	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.Status != TradingSettlementStatusNECOPayoutBroadcasted {
		t.Fatalf("status = %q", settlement.Status)
	}
	if settlement.SettlementAmount != "5000000000000000000" {
		t.Fatalf("settlement amount = %q", settlement.SettlementAmount)
	}
	if !settlement.SystemReserved || settlement.DYDXReservedQuantums != "20000000" {
		t.Fatalf("system reserved fields = reserved:%v quantums:%q", settlement.SystemReserved, settlement.DYDXReservedQuantums)
	}

	again, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile:  closeSettlementReconcileFixture(),
	})
	if err != nil {
		t.Fatalf("duplicate SettleClosedPosition() error = %v", err)
	}
	if again.ID != settlement.ID {
		t.Fatalf("duplicate settlement ID = %q, want %q", again.ID, settlement.ID)
	}
	if backend.sentCount != 1 {
		t.Fatalf("sent transactions = %d, want 1", backend.sentCount)
	}
	if len(ledgerStore.Journals()) != 2 {
		t.Fatalf("ledger journal count = %d, want credit+freeze only once", len(ledgerStore.Journals()))
	}
}

func TestCloseSettlementServiceKeepsSettlementPayoutFrozenWhenPayoutCannotStart(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	settlements := NewMemoryTradingSettlementStore()
	withdrawals := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	worker, err := NewNECOWithdrawalWorker(ledger, withdrawals, &countingNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    1,
	}, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts:   map[string]string{},
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	service := NewCloseSettlementService(settlements, ledger, worker, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
	})

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile:  closeSettlementReconcileFixture(),
	})

	if !errors.Is(err, ErrNECOWithdrawalUnsupportedAsset) {
		t.Fatalf("SettleClosedPosition() error = %v, want unsupported asset", err)
	}
	if settlement.Status != TradingSettlementStatusFailed {
		t.Fatalf("settlement status = %q, want failed", settlement.Status)
	}
	balance, err := ledger.GetBalance(context.Background(), settlement.UserID, settlement.SettlementAsset)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "0" || balance.Frozen != settlement.SettlementAmount {
		t.Fatalf("balance = %+v, want settlement payout kept frozen amount %s", balance, settlement.SettlementAmount)
	}
	withdrawal, err := ledger.GetWithdrawal(context.Background(), settlement.LedgerWithdrawalID)
	if err != nil {
		t.Fatalf("GetWithdrawal() error = %v", err)
	}
	if withdrawal.Status != LedgerWithdrawalStatusFrozen {
		t.Fatalf("withdrawal status = %q, want frozen", withdrawal.Status)
	}
}

func TestTradingCloseCallbackPayloadIncludesFeeDetails(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	settlements := NewMemoryTradingSettlementStore()
	withdrawals := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &countingNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    11,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, withdrawals, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		},
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	service := NewCloseSettlementService(settlements, ledger, worker, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                     CloseSettlementModeFullAccounting,
		SettlementAsset:          "DEDX",
		SettlementAssetDecimals:  18,
		CollateralQuantumsPerUSD: "1000000",
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
			SubticksPerUnit: "1000000",
		}},
	})

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Fill: TradingFill{
			EventID:            "fill-1",
			TradeHash:          "trade-1",
			UserID:             "user-1",
			OrderID:            "close-1",
			Market:             "ETH-USD",
			Asset:              "DEDX",
			ReleasedMargin:     "20",
			Fee:                "0.225",
			RealizedPNL:        "5",
			LiquidityRole:      "taker",
			MakerAddress:       "dydx1maker",
			TakerAddress:       "dydx1taker",
			FillAmount:         "500000000",
			MakerFee:           "-0.05",
			TakerFee:           "0.225",
			MakerOrderSubticks: "2500000000",
		},
		Reconcile: closeSettlementReconcileFixture(),
	})
	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	payload := NewTradingCloseCallbackPayload(settlement, closeSettlementOrderFixture(), TradingFill{
		EventID:            "fill-1",
		TradeHash:          "trade-1",
		UserID:             "user-1",
		OrderID:            "close-1",
		Market:             "ETH-USD",
		Asset:              "DEDX",
		ReleasedMargin:     "20",
		Fee:                "0.225",
		RealizedPNL:        "5",
		LiquidityRole:      "taker",
		MakerAddress:       "dydx1maker",
		MakerNECOAddress:   "0x00000000000000000000000000000000000000aa",
		TakerAddress:       "dydx1taker",
		TakerNECOAddress:   "0x00000000000000000000000000000000000000bb",
		FillAmount:         "500000000",
		MakerFee:           "-0.05",
		TakerFee:           "0.225",
		MakerOrderSubticks: "2500000000",
		CreatedAt:          1787898670,
	}, map[string]DYDXMarketConfig{
		"ETH-USD": {
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
			SubticksPerUnit: "1000000",
		},
	})
	if payload.TradeHash != "trade-1" ||
		payload.Symbol != "ETH-USD" ||
		payload.MarketType != TradingMarketTypeMajor ||
		payload.MakerAddress != "dydx1maker" ||
		payload.MakerNECOAddress != "0x00000000000000000000000000000000000000aa" ||
		payload.TakerAddress != "dydx1taker" ||
		payload.TakerNECOAddress != "0x00000000000000000000000000000000000000bb" ||
		payload.Price != "2500" ||
		payload.Quantity != "0.5" ||
		payload.Turnover != "1250" ||
		payload.MakerFeeRate != "-0.0003" ||
		payload.TakerFeeRate != "0.0015" ||
		payload.MakerFee != "-0.05" ||
		payload.TakerFee != "0.225" ||
		payload.NetFee != "0.175" ||
		payload.TradedAt == nil ||
		payload.TradedAt.Unix() != 1787898670 ||
		payload.NECOAddress != "0x0000000000000000000000000000000000000001" {
		t.Fatalf("callback payload = %+v", payload)
	}
}

func TestCloseSettlementServiceSendsCallbackWithPartyNECOAddressesAfterPayout(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	settlements := NewMemoryTradingSettlementStore()
	withdrawals := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &countingNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    12,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, withdrawals, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		},
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	bindings := NewMemoryWalletBindingStore()
	for _, binding := range []WalletBinding{
		{ID: "maker-binding", UserID: "maker-user", NECOAddress: "0x00000000000000000000000000000000000000aa", DYDXAddress: "dydx1maker", Status: WalletBindingStatusActive},
		{ID: "taker-binding", UserID: "taker-user", NECOAddress: "0x00000000000000000000000000000000000000bb", DYDXAddress: "dydx1taker", Status: WalletBindingStatusActive},
	} {
		if err := bindings.Save(context.Background(), binding); err != nil {
			t.Fatalf("Save() binding error = %v", err)
		}
	}
	sender := &recordingTradingCloseCallbackSender{}
	service := NewCloseSettlementService(settlements, ledger, worker, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, bindings, CloseSettlementConfig{
		Mode:                     CloseSettlementModeFullAccounting,
		SettlementAsset:          "DEDX",
		SettlementAssetDecimals:  18,
		CollateralQuantumsPerUSD: "1000000",
		TradeCloseCallbackURL:    "https://callback.example/trades",
		Markets: []DYDXMarketConfig{{
			Market:          "ETH-USD",
			ClobPairID:      1,
			QuantumsPerUnit: "1000000000",
			SubticksPerUnit: "1000000",
		}},
	})
	service.SetTradeCloseCallbackSender(sender)

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Fill: TradingFill{
			EventID:            "fill-callback",
			TradeHash:          "trade-callback",
			UserID:             "user-1",
			OrderID:            "close-1",
			Market:             "ETH-USD",
			Asset:              "DEDX",
			ReleasedMargin:     "20",
			Fee:                "0.225",
			RealizedPNL:        "5",
			LiquidityRole:      "taker",
			MakerAddress:       "dydx1maker",
			TakerAddress:       "dydx1taker",
			FillAmount:         "500000000",
			MakerFee:           "-0.05",
			TakerFee:           "0.225",
			MakerOrderSubticks: "2500000000",
			CreatedAt:          1787898670,
		},
		Reconcile: closeSettlementReconcileFixture(),
	})
	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.TradeCloseCallbackStatus != TradingCloseCallbackStatusSent || sender.calls != 1 {
		t.Fatalf("settlement=%+v callback calls=%d", settlement, sender.calls)
	}
	if sender.payload.MakerNECOAddress != "0x00000000000000000000000000000000000000aa" ||
		sender.payload.TakerNECOAddress != "0x00000000000000000000000000000000000000bb" ||
		sender.payload.TradedAt == nil ||
		sender.payload.TradedAt.Unix() != 1787898670 {
		t.Fatalf("callback payload = %+v", sender.payload)
	}
}

func TestCloseSettlementServiceSwapsDEDXUSDSettlementAssetByEntryAsset(t *testing.T) {
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "dedx-price",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		AssetMappings: []AssetMapping{
			{NECOAsset: "DEDX", Decimals: 18, PriceRoute: "DEDX-NECO-USD", EnabledForTrading: true},
			{NECOAsset: "USDT", Decimals: 18, PriceRoute: "USDT-USD", EnabledForTrading: true},
		},
	})
	order := closeSettlementOrderFixture()
	order.Market = "FDEX-USD"
	order.MarginAsset = "DEDX"
	order.MarginUSD = "20"

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})

	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.SettlementAsset != "USDT" || settlement.SettlementAmount != "20000000000000000000" || settlement.AssetUSDPrice != "1" {
		t.Fatalf("settlement = %+v, want DEDX entry to settle as 20 USDT", settlement)
	}

	order.ID = "close-usdt-entry"
	order.ClientOrderID = "close-client-usdt-entry"
	order.MarginAsset = "USDT"
	order.MarginUSD = "20"
	settlement, err = service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})

	if err != nil {
		t.Fatalf("SettleClosedPosition() USDT entry error = %v", err)
	}
	if settlement.SettlementAsset != "DEDX" || settlement.SettlementAmount != "5000000000000000000" || settlement.AssetUSDPrice != "4" {
		t.Fatalf("settlement = %+v, want USDT entry to settle as 5 DEDX", settlement)
	}
}

func TestCloseSettlementServiceSwapsDEDXUSDSettlementAssetByFDEXEntryAsset(t *testing.T) {
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "FDEX",
		price:      "4",
		snapshotID: "fdex-price",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "FDEX",
		SettlementAssetDecimals: 18,
		AssetMappings: []AssetMapping{
			{NECOAsset: "FDEX", Decimals: 18, PriceRoute: "FDEX-USDT", EnabledForTrading: true},
			{NECOAsset: "USDT", Decimals: 18, PriceRoute: "USDT-USD", EnabledForTrading: true},
		},
	})
	order := closeSettlementOrderFixture()
	order.Market = "FDEX-USD"
	order.MarginAsset = "FDEX"
	order.MarginUSD = "20"

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})

	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.SettlementAsset != "USDT" || settlement.SettlementAmount != "20000000000000000000" || settlement.AssetUSDPrice != "1" {
		t.Fatalf("settlement = %+v, want FDEX entry to settle as 20 USDT", settlement)
	}

	order.ID = "close-usdt-entry-fdex"
	order.ClientOrderID = "close-client-usdt-entry-fdex"
	order.MarginAsset = "USDT"
	order.MarginUSD = "20"
	settlement, err = service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})

	if err != nil {
		t.Fatalf("SettleClosedPosition() USDT entry error = %v", err)
	}
	if settlement.SettlementAsset != "FDEX" || settlement.SettlementAmount != "5000000000000000000" || settlement.AssetUSDPrice != "4" {
		t.Fatalf("settlement = %+v, want USDT entry to settle as 5 FDEX", settlement)
	}
}

func TestCloseSettlementServiceKeepsEntryAssetForNonDEDXUSDSettlement(t *testing.T) {
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "dedx-price",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		AssetMappings: []AssetMapping{
			{NECOAsset: "DEDX", Decimals: 18, PriceRoute: "DEDX-NECO-USD", EnabledForTrading: true},
			{NECOAsset: "USDT", Decimals: 18, PriceRoute: "USDT-USD", EnabledForTrading: true},
		},
	})
	order := closeSettlementOrderFixture()
	order.Market = "ETH-USD"
	order.MarginAsset = "USDT"
	order.MarginUSD = "20"

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})

	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.SettlementAsset != "USDT" || settlement.SettlementAmount != "20000000000000000000" || settlement.AssetUSDPrice != "1" {
		t.Fatalf("settlement = %+v, want non-DEDX market to settle as entry USDT", settlement)
	}
}

func TestCloseSettlementServicePaysDEDXOnceForCancelledFundedOrder(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	settlements := NewMemoryTradingSettlementStore()
	withdrawals := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &countingNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, withdrawals, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		},
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	service := NewCloseSettlementService(settlements, ledger, worker, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	order := TradingOrder{
		ID:                  "order-1",
		UserID:              "user-1",
		NECOAddress:         "0x0000000000000000000000000000000000000001",
		DYDXAccount:         "dydx1abc",
		ClientOrderID:       "client-order-1",
		Market:              "ETH-USD",
		Side:                "BUY",
		Size:                "1",
		MarginAsset:         "DEDX",
		MarginUSD:           "20",
		DYDXFundingTxHash:   "funding-tx-1",
		DYDXFundingQuantums: "20000000",
		Status:              TradingOrderStatusCancelled,
	}
	reconcile := closeSettlementReconcileFixture()
	reconcile.StatefulOrder = DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false}

	settlement, err := service.SettleCancelledOrder(context.Background(), CancelSettlementRequest{
		Order:     order,
		Reconcile: reconcile,
	})
	if err != nil {
		t.Fatalf("SettleCancelledOrder() error = %v", err)
	}
	if settlement.Kind != TradingSettlementKindCancelOrder {
		t.Fatalf("settlement kind = %q, want %q", settlement.Kind, TradingSettlementKindCancelOrder)
	}
	if settlement.Status != TradingSettlementStatusNECOPayoutBroadcasted {
		t.Fatalf("status = %q", settlement.Status)
	}
	if settlement.SettlementAmount != "5000000000000000000" {
		t.Fatalf("settlement amount = %q", settlement.SettlementAmount)
	}

	again, err := service.SettleCancelledOrder(context.Background(), CancelSettlementRequest{
		Order:     order,
		Reconcile: reconcile,
	})
	if err != nil {
		t.Fatalf("duplicate SettleCancelledOrder() error = %v", err)
	}
	if again.ID != settlement.ID {
		t.Fatalf("duplicate settlement ID = %q, want %q", again.ID, settlement.ID)
	}
	if backend.sentCount != 1 {
		t.Fatalf("sent transactions = %d, want 1", backend.sentCount)
	}
	if len(ledgerStore.Journals()) != 2 {
		t.Fatalf("ledger journal count = %d, want credit+freeze only once", len(ledgerStore.Journals()))
	}
}

func TestCloseSettlementServiceRefundsCancelledUnfilledOrderEvenWhenAccountHasOtherPosition(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	service.SetFillLookup(NewMemoryTradingSyncStore())
	reconcile := closeSettlementReconcileFixture()
	reconcile.StatefulOrder = DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false}
	reconcile.Subaccount.PerpetualPositions = []DYDXPerpetualPositionReconcileSnapshot{{
		PerpetualID: 1,
		Quantums:    "1000000",
	}}
	order := closeSettlementOrderFixture()
	order.ID = "cancelled-with-position"
	order.ClosePosition = false
	order.DYDXFundingTxHash = "funding-tx-1"
	order.Status = TradingOrderStatusCancelled

	settlement, err := service.SettleCancelledOrder(context.Background(), CancelSettlementRequest{
		Order:     order,
		Reconcile: reconcile,
	})
	if err != nil {
		t.Fatalf("SettleCancelledOrder() error = %v", err)
	}
	if settlement.Status != TradingSettlementStatusDEDXCredited {
		t.Fatalf("status = %q", settlement.Status)
	}
	if settlement.SettlementAmount != "5000000000000000000" {
		t.Fatalf("settlement amount = %q, want 5000000000000000000", settlement.SettlementAmount)
	}
	if len(ledgerStore.Journals()) != 2 {
		t.Fatalf("ledger journal count = %d, want credit+freeze", len(ledgerStore.Journals()))
	}
}

func TestCloseSettlementServiceFullAccountingSettlesCancelledOrderFromFillsWhenPositionRemains(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	fillStore := NewMemoryTradingSyncStore()
	if err := fillStore.SaveFill(context.Background(), TradingFill{
		EventID:        "cancel-fill-1",
		UserID:         "user-1",
		OrderID:        "dydx1abc:0:321:1",
		Market:         "ETH-USD",
		Asset:          "USD",
		ReleasedMargin: "10",
		Fee:            "0.25",
		RealizedPNL:    "1",
		OrderStatus:    TradingOrderStatusCancelled,
	}); err != nil {
		t.Fatalf("SaveFill() error = %v", err)
	}
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		SubaccountNumber:        0,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	service.SetFillLookup(fillStore)
	order := closeSettlementOrderFixture()
	order.ID = "cancelled-with-fill"
	order.ClientOrderID = "321"
	order.ClosePosition = false
	order.DYDXFundingTxHash = "funding-tx-1"
	order.Status = TradingOrderStatusCancelled
	reconcile := closeSettlementReconcileFixture()
	reconcile.StatefulOrder = DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false}
	reconcile.Subaccount.PerpetualPositions = []DYDXPerpetualPositionReconcileSnapshot{{
		PerpetualID: 1,
		Quantums:    "1000000",
	}}

	settlement, err := service.SettleCancelledOrder(context.Background(), CancelSettlementRequest{
		Order:     order,
		Reconcile: reconcile,
	})
	if err != nil {
		t.Fatalf("SettleCancelledOrder() error = %v", err)
	}
	if settlement.SettlementUSD != "10.75" {
		t.Fatalf("settlement USD = %q, want 10.75", settlement.SettlementUSD)
	}
	if settlement.SettlementAmount != "2687500000000000000" {
		t.Fatalf("settlement amount = %q, want 2687500000000000000", settlement.SettlementAmount)
	}
}

type countingNECOWithdrawalBackend struct {
	chainID   *big.Int
	gasPrice  *big.Int
	nonce     uint64
	sentCount int
}

func (b *countingNECOWithdrawalBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return b.chainID, nil
}

func (b *countingNECOWithdrawalBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return b.nonce, nil
}

func (b *countingNECOWithdrawalBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return b.gasPrice, nil
}

func (b *countingNECOWithdrawalBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.sentCount++
	return nil
}

func (b *countingNECOWithdrawalBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, nil
}

func TestCloseSettlementServiceFullAccountingWithoutFillRequiresManualReview(t *testing.T) {
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
	})
	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile:  closeSettlementReconcileFixture(),
	})
	if !errors.Is(err, ErrTradingSettlementManualReview) {
		t.Fatalf("error = %v, want manual review", err)
	}
	if settlement.Status != TradingSettlementStatusManualReviewRequired {
		t.Fatalf("status = %q", settlement.Status)
	}
}

func TestCloseSettlementServiceFullAccountingAggregatesSyncedFills(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	fillStore := NewMemoryTradingSyncStore()
	if err := fillStore.SaveFill(context.Background(), TradingFill{
		EventID:        "close-fill-1",
		UserID:         "user-1",
		OrderID:        "dydx1abc:0:123:1",
		Market:         "ETH-USD",
		Asset:          "USD",
		ReleasedMargin: "10",
		Fee:            "0.5",
		RealizedPNL:    "2",
		OrderStatus:    TradingOrderStatusPartialFilled,
	}); err != nil {
		t.Fatalf("SaveFill(first) error = %v", err)
	}
	if err := fillStore.SaveFill(context.Background(), TradingFill{
		EventID:        "close-fill-2",
		UserID:         "user-1",
		OrderID:        "dydx1abc:0:123:1",
		Market:         "ETH-USD",
		Asset:          "USD",
		ReleasedMargin: "5",
		Fee:            "0.1",
		RealizedPNL:    "-1",
		OrderStatus:    TradingOrderStatusFilled,
	}); err != nil {
		t.Fatalf("SaveFill(second) error = %v", err)
	}
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		SubaccountNumber:        0,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	service.SetFillLookup(fillStore)
	order := closeSettlementOrderFixture()
	order.ClientOrderID = "123"

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})
	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.SettlementUSD != "15.4" {
		t.Fatalf("settlement USD = %q, want 15.4", settlement.SettlementUSD)
	}
	if settlement.SettlementAmount != "3850000000000000000" {
		t.Fatalf("settlement amount = %q, want 3850000000000000000", settlement.SettlementAmount)
	}
	if len(ledgerStore.Journals()) != 2 {
		t.Fatalf("ledger journal count = %d, want credit+freeze", len(ledgerStore.Journals()))
	}
}

func TestCloseSettlementServiceFullAccountingRequiresReleasedMargin(t *testing.T) {
	fillStore := NewMemoryTradingSyncStore()
	if err := fillStore.SaveFill(context.Background(), TradingFill{
		EventID:        "fee-only-fill",
		UserID:         "user-1",
		OrderID:        "close-1",
		Market:         "ETH-USD",
		Asset:          "USD",
		ReleasedMargin: "0",
		Fee:            "-1",
		RealizedPNL:    "0",
		OrderStatus:    TradingOrderStatusFilled,
	}); err != nil {
		t.Fatalf("SaveFill() error = %v", err)
	}
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
	})
	service.SetFillLookup(fillStore)

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile:  closeSettlementReconcileFixture(),
	})
	if !errors.Is(err, ErrTradingSettlementManualReview) {
		t.Fatalf("error = %v settlement=%+v, want manual review", err, settlement)
	}
	if settlement.Status != TradingSettlementStatusManualReviewRequired {
		t.Fatalf("status = %q", settlement.Status)
	}
}

func TestCloseSettlementServiceFullAccountingRefundsUnfilledCancelledOrderMargin(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	order := closeSettlementOrderFixture()
	order.ID = "cancel-unfilled-1"
	order.ClosePosition = false
	order.DYDXFundingTxHash = "funding-tx-1"
	order.Status = TradingOrderStatusCancelled
	reconcile := closeSettlementReconcileFixture()
	reconcile.StatefulOrder = DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false}

	settlement, err := service.SettleCancelledOrder(context.Background(), CancelSettlementRequest{
		Order:     order,
		Reconcile: reconcile,
	})
	if err != nil {
		t.Fatalf("SettleCancelledOrder() error = %v", err)
	}
	if settlement.Kind != TradingSettlementKindCancelOrder {
		t.Fatalf("settlement kind = %q, want %q", settlement.Kind, TradingSettlementKindCancelOrder)
	}
	if settlement.SettlementAmount != "5000000000000000000" {
		t.Fatalf("settlement amount = %q, want 5000000000000000000", settlement.SettlementAmount)
	}
	if len(ledgerStore.Journals()) != 2 {
		t.Fatalf("ledger journal count = %d, want credit+freeze", len(ledgerStore.Journals()))
	}
}

func TestCloseSettlementServiceRefundsFailedFundedOrderWithOriginalMarginAmount(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), ledger, nil, fixedCloseSettlementAssetOracle{
		asset:      "FDEX",
		price:      "0.2",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode: CloseSettlementModeFullAccounting,
		AssetMappings: []AssetMapping{{
			NECOAsset: "FDEX",
			Decimals:  18,
		}},
		Markets: []DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}},
	})
	order := closeSettlementOrderFixture()
	order.ID = "failed-funded-1"
	order.Market = "FDEX-USD"
	order.ClosePosition = false
	order.Status = TradingOrderStatusSubmitFailed
	order.MarginAsset = "FDEX"
	order.MarginAmount = "105637655244723513245"
	order.MarginUSD = "22.785"
	order.AssetUSDPrice = "0.215690133856299183"
	order.PriceSnapshotID = "neco-v2-route:25808170:pair"
	order.DYDXFundingTxHash = "funding-tx-1"
	order.DYDXOrderMarginQuantums = "22785000"

	settlement, err := service.SettleFailedOrder(context.Background(), FailedOrderSettlementRequest{
		Order: order,
	})
	if err != nil {
		t.Fatalf("SettleFailedOrder() error = %v", err)
	}
	if settlement.Kind != TradingSettlementKindFailedOrder {
		t.Fatalf("settlement kind = %q, want %q", settlement.Kind, TradingSettlementKindFailedOrder)
	}
	if settlement.SettlementAsset != "FDEX" {
		t.Fatalf("settlement asset = %q, want FDEX", settlement.SettlementAsset)
	}
	if settlement.SettlementAmount != "105637655244723513245" {
		t.Fatalf("settlement amount = %q, want original margin amount", settlement.SettlementAmount)
	}
	if settlement.SettlementUSD != "22.785" || settlement.AssetUSDPrice != "0.215690133856299183" || settlement.PriceSnapshotID != "neco-v2-route:25808170:pair" {
		t.Fatalf("settlement pricing fields = %+v", settlement)
	}
	if settlement.DYDXReservedQuantums != "22785000" || !settlement.SystemReserved {
		t.Fatalf("reserved fields = %+v", settlement)
	}
	if settlement.Status != TradingSettlementStatusDEDXCredited {
		t.Fatalf("status = %q", settlement.Status)
	}

	again, err := service.SettleFailedOrder(context.Background(), FailedOrderSettlementRequest{
		Order: order,
	})
	if err != nil {
		t.Fatalf("duplicate SettleFailedOrder() error = %v", err)
	}
	if again.ID != settlement.ID {
		t.Fatalf("duplicate settlement ID = %q, want %q", again.ID, settlement.ID)
	}
	if len(ledgerStore.Journals()) != 2 {
		t.Fatalf("ledger journal count = %d, want credit+freeze only once", len(ledgerStore.Journals()))
	}
}

func TestCloseSettlementServiceRawLikeMarginUSDRequiresManualReview(t *testing.T) {
	order := closeSettlementOrderFixture()
	order.MarginUSD = "499750000000000000000"
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
	})

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  closeSettlementReconcileFixture(),
	})
	if !errors.Is(err, ErrTradingSettlementManualReview) {
		t.Fatalf("error = %v, want manual review", err)
	}
	if settlement.Status != TradingSettlementStatusManualReviewRequired {
		t.Fatalf("status = %q", settlement.Status)
	}
}

func TestCloseSettlementServiceWaitsWhenMarketPositionStillExists(t *testing.T) {
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{
		asset:      "DEDX",
		price:      "4",
		snapshotID: "price-1",
	}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeMarginOnly,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
		Markets:                 []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
	})
	reconcile := closeSettlementReconcileFixture()
	reconcile.Subaccount.PerpetualPositions = []DYDXPerpetualPositionReconcileSnapshot{{
		PerpetualID: 1,
		Quantums:    "1000000",
	}}

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile:  reconcile,
	})
	if !errors.Is(err, ErrTradingSettlementNotConfirmed) {
		t.Fatalf("error = %v settlement=%+v, want not confirmed", err, settlement)
	}
}
