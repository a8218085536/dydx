package tyche

import (
	"context"
	"errors"
	"testing"
)

func TestNECOWalletFundingPreparerBuildsTransferInstruction(t *testing.T) {
	preparer := NewNECOWalletFundingPreparer(fixedNECOTokenBalanceReader{
		balances: map[string]string{
			"0x0000000000000000000000000000000000000ded:0x0000000000000000000000000000000000000001": "100",
		},
	}, NECOWalletFundingConfig{
		DepositAddress: "0x000000000000000000000000000000000000cafe",
		AssetMappings: []AssetMapping{
			{
				NECOTokenAddress: "0x0000000000000000000000000000000000000ded",
				NECOAsset:        "DEDX",
				DYDXAsset:        "DEDX",
			},
		},
	})

	result, err := preparer.PrepareFunding(context.Background(), TradingFundingPrepareRequest{
		UserID:        "user-1",
		ClientOrderID: "client-order-1",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		ChargeAsset:   "DEDX",
		MissingAmount: "15",
	})

	if err != nil {
		t.Fatalf("PrepareFunding() error = %v", err)
	}
	if result.Status != TradingFundingStatusWalletActionRequired || result.Action != TradingFundingActionTransfer {
		t.Fatalf("result = %+v, want transfer action required", result)
	}
	if result.Amount != "15" || result.TokenAddress != "0x0000000000000000000000000000000000000ded" {
		t.Fatalf("result = %+v, want 15 DEDX token instruction", result)
	}
	if result.FromAddress != "0x0000000000000000000000000000000000000001" || result.ToAddress != "0x000000000000000000000000000000000000cafe" {
		t.Fatalf("transfer endpoints = %+v", result)
	}
	if result.WalletBalance != "100" {
		t.Fatalf("wallet balance = %q", result.WalletBalance)
	}
}

func TestNECOWalletFundingPreparerRejectsInsufficientNECOWalletBalance(t *testing.T) {
	preparer := NewNECOWalletFundingPreparer(fixedNECOTokenBalanceReader{
		balances: map[string]string{
			"0x0000000000000000000000000000000000000ded:0x0000000000000000000000000000000000000001": "10",
		},
	}, NECOWalletFundingConfig{
		DepositAddress: "0x000000000000000000000000000000000000cafe",
		AssetMappings: []AssetMapping{
			{
				NECOTokenAddress: "0x0000000000000000000000000000000000000ded",
				NECOAsset:        "DEDX",
				DYDXAsset:        "DEDX",
			},
		},
	})

	_, err := preparer.PrepareFunding(context.Background(), TradingFundingPrepareRequest{
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		ChargeAsset:   "DEDX",
		MissingAmount: "15",
	})

	if !errors.Is(err, ErrTradingFundingWalletInsufficient) {
		t.Fatalf("PrepareFunding() error = %v, want %v", err, ErrTradingFundingWalletInsufficient)
	}
}
