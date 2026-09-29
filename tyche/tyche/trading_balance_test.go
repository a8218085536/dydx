package tyche

import (
	"context"
	"errors"
	"testing"
)

func TestTradingBalanceServiceCombinesLedgerAndNECOWalletDEDX(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "20")
	reader := fixedNECOTokenBalanceReader{
		balances: map[string]string{
			"0x0000000000000000000000000000000000000ded:0x0000000000000000000000000000000000000001": "80",
		},
	}
	service := NewTradingBalanceService(ledger, reader, []AssetMapping{
		{
			NECOTokenAddress: "0x0000000000000000000000000000000000000ded",
			NECOAsset:        "DEDX",
			DYDXAsset:        "DEDX",
		},
	})

	overview, err := service.Overview(context.Background(), TradingBalanceOverviewRequest{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		Asset:       "DEDX",
	})

	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if overview.LedgerAvailable != "20" || overview.NECOWalletAvailable != "80" || overview.DisplayAvailable != "100" {
		t.Fatalf("overview = %+v, want 20 ledger + 80 wallet = 100 display", overview)
	}
	if overview.NECOTokenAddress != "0x0000000000000000000000000000000000000ded" {
		t.Fatalf("token address = %q", overview.NECOTokenAddress)
	}
}

func TestTradingBalanceServiceDefaultsToFirstTradableAssetWhenAssetIsMissing(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, "user-1", "DEDX", "20")
	creditAvailableBalance(t, ledger, "user-1", "NECO", "10")
	service := NewTradingBalanceService(ledger, nil, []AssetMapping{
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
	})

	overview, err := service.Overview(context.Background(), TradingBalanceOverviewRequest{
		UserID: "user-1",
	})

	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if overview.Asset != "DEDX" || overview.LedgerAvailable != "20" || overview.DisplayAvailable != "20" {
		t.Fatalf("overview = %+v, want default DEDX ledger balance", overview)
	}
}

type fixedNECOTokenBalanceReader struct {
	balances map[string]string
	err      error
}

func (r fixedNECOTokenBalanceReader) BalanceOf(ctx context.Context, tokenAddress, ownerAddress string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	key := normalizeAddress(tokenAddress) + ":" + normalizeAddress(ownerAddress)
	balance, ok := r.balances[key]
	if !ok {
		return "", errors.New("missing balance")
	}
	return balance, nil
}
