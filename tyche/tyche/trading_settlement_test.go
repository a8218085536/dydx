package tyche

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryTradingSettlementStoreCreateIfAbsentIsIdempotent(t *testing.T) {
	store := NewMemoryTradingSettlementStore()
	request := TradingSettlement{
		ID:                   "settlement-1",
		UserID:               "user-1",
		NECOAddress:          "0xabc",
		DYDXAccount:          "dydx1abc",
		SubaccountNumber:     0,
		Market:               "ETH-USD",
		CloseOrderID:         "close-order-1",
		CloseClientOrderID:   "client-close-1",
		SettlementAsset:      "DEDX",
		SettlementUSD:        "20",
		SettlementAmount:     "5000000000000000000",
		Status:               TradingSettlementStatusPending,
		SystemReserved:       true,
		DYDXReservedQuantums: "20000000",
	}

	first, created, err := store.CreateIfAbsent(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("first CreateIfAbsent() = (%+v, %v, %v), want created", first, created, err)
	}
	second, created, err := store.CreateIfAbsent(context.Background(), request)
	if err != nil || created {
		t.Fatalf("second CreateIfAbsent() = (%+v, %v, %v), want existing", second, created, err)
	}
	if second.ID != first.ID {
		t.Fatalf("second ID = %q, want %q", second.ID, first.ID)
	}
}

func TestCloseSettlementConfigDefaultsToMarginOnly(t *testing.T) {
	conf := Config{
		ChainType:           ChainTypeDYDX,
		InternalAPIKey:      strings.Repeat("a", 32),
		DYDXTradingURL:      "http://127.0.0.1:8090/dydx/trading",
		DYDXChainID:         "flydex-mainnet-1",
		DYDXHeightURL:       "http://127.0.0.1:26657/status",
		DYDXMarkets:         []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
		ExpectedMD5Password: "x",
		AllowedIP:           "*",
	}
	if err := conf.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if conf.CloseSettlementMode != CloseSettlementModeMarginOnly {
		t.Fatalf("CloseSettlementMode = %q", conf.CloseSettlementMode)
	}
	if conf.CloseSettlementAsset != "DEDX" {
		t.Fatalf("CloseSettlementAsset = %q", conf.CloseSettlementAsset)
	}
	if conf.CloseSettlementAssetDecimals != 18 {
		t.Fatalf("CloseSettlementAssetDecimals = %d", conf.CloseSettlementAssetDecimals)
	}
}

func TestSettlementAssetAmountRawFloorsAndRejectsLikelyAtomicUSD(t *testing.T) {
	amount, err := settlementAssetAmountRaw("20", "4", 18)
	if err != nil {
		t.Fatalf("settlementAssetAmountRaw() error = %v", err)
	}
	if amount != "5000000000000000000" {
		t.Fatalf("amount = %q, want 5000000000000000000", amount)
	}

	amount, err = settlementAssetAmountRaw("1", "3", 6)
	if err != nil {
		t.Fatalf("settlementAssetAmountRaw floor() error = %v", err)
	}
	if amount != "333333" {
		t.Fatalf("floor amount = %q, want 333333", amount)
	}

	if _, err := settlementAssetAmountRaw("499750000000000000000", "4", 18); err == nil {
		t.Fatalf("settlementAssetAmountRaw() error = nil, want raw-like USD rejection")
	}
}
