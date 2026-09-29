package tyche

import (
	"context"
	"errors"
	"testing"

	"github.com/garyburd/redigo/redis"
)

func TestNECODepositServiceCreditsExistingUserAfterConfirmedTransfer(t *testing.T) {
	authStore := NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := NewMemoryLedgerStore()
	service := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})

	deposit, err := service.ProcessTransfer(context.Background(), NECOTokenTransfer{
		ChainID:       "9001",
		TokenAddress:  "0x0000000000000000000000000000000000000ded",
		From:          "0x0000000000000000000000000000000000000001",
		To:            "0x0000000000000000000000000000000000000abc",
		Amount:        "100",
		TxHash:        "0xtx1",
		LogIndex:      2,
		Confirmations: 12,
	})

	if err != nil {
		t.Fatalf("ProcessTransfer() error = %v", err)
	}
	if deposit.Status != NECODepositStatusCredited {
		t.Fatalf("deposit status = %q, want %q", deposit.Status, NECODepositStatusCredited)
	}
	if deposit.UserID != user.UserID {
		t.Fatalf("deposit userID = %q, want %q", deposit.UserID, user.UserID)
	}
	balance, err := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}
}

func TestNECODepositScansRedisFields(t *testing.T) {
	values := []interface{}{
		[]byte("id"), []byte("event-1"),
		[]byte("eventId"), []byte("event-1"),
		[]byte("chainId"), []byte("7260"),
		[]byte("tokenAddress"), []byte("0x0000000000000000000000000000000000000ded"),
		[]byte("asset"), []byte("DEDX"),
		[]byte("from"), []byte("0x0000000000000000000000000000000000000001"),
		[]byte("to"), []byte("0x0000000000000000000000000000000000000abc"),
		[]byte("amount"), []byte("100"),
		[]byte("txHash"), []byte("0xtx1"),
		[]byte("logIndex"), []byte("2"),
		[]byte("userId"), []byte("user-1"),
		[]byte("status"), []byte(NECODepositStatusCredited),
	}
	var deposit NECODeposit

	if err := redis.ScanStruct(values, &deposit); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if deposit.EventID != "event-1" || deposit.Asset != "DEDX" || deposit.Amount != "100" || deposit.Status != NECODepositStatusCredited {
		t.Fatalf("deposit = %+v, want redis fields populated", deposit)
	}
}

func TestNECODepositServiceUsesAssetMappingsForTokenAsset(t *testing.T) {
	authStore := NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := NewMemoryLedgerStore()
	service := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		AssetMappings: []AssetMapping{
			{NECOTokenAddress: "0x0000000000000000000000000000000000000ded", NECOAsset: "DEDX", DYDXAsset: "DEDX", EnabledForTrading: true},
		},
	})

	deposit, err := service.ProcessTransfer(context.Background(), NECOTokenTransfer{
		ChainID:       "9001",
		TokenAddress:  "0x0000000000000000000000000000000000000ded",
		From:          "0x0000000000000000000000000000000000000001",
		To:            "0x0000000000000000000000000000000000000abc",
		Amount:        "50",
		TxHash:        "0xtx1",
		LogIndex:      0,
		Confirmations: 12,
	})

	if err != nil {
		t.Fatalf("ProcessTransfer() error = %v", err)
	}
	if deposit.Asset != "DEDX" {
		t.Fatalf("deposit asset = %q, want DEDX", deposit.Asset)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "50" {
		t.Fatalf("available = %q, want 50", balance.Available)
	}
}

func TestNECODepositServiceStoresPendingDepositForUnknownNECOAddressAndClaimsLater(t *testing.T) {
	authStore := NewMemoryAuthStore()
	ledgerStore := NewMemoryLedgerStore()
	depositStore := NewMemoryNECODepositStore()
	service := NewNECODepositService(depositStore, NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})

	deposit, err := service.ProcessTransfer(context.Background(), NECOTokenTransfer{
		ChainID:       "9001",
		TokenAddress:  "0x0000000000000000000000000000000000000ded",
		From:          "0x0000000000000000000000000000000000000002",
		To:            "0x0000000000000000000000000000000000000abc",
		Amount:        "50",
		TxHash:        "0xtx2",
		LogIndex:      0,
		Confirmations: 12,
	})

	if err != nil {
		t.Fatalf("ProcessTransfer() error = %v", err)
	}
	if deposit.Status != NECODepositStatusPending {
		t.Fatalf("deposit status = %q, want %q", deposit.Status, NECODepositStatusPending)
	}
	if deposit.UserID != "" {
		t.Fatalf("pending deposit userID = %q, want empty", deposit.UserID)
	}

	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000002")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	claimed, err := service.ClaimPending(context.Background(), user.UserID, user.NECOAddress)

	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("len(claimed) = %d, want 1", len(claimed))
	}
	if claimed[0].Status != NECODepositStatusCredited {
		t.Fatalf("claimed status = %q, want %q", claimed[0].Status, NECODepositStatusCredited)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "50" {
		t.Fatalf("available = %q, want 50", balance.Available)
	}
	pending, err := depositStore.PendingByNECOAddress(context.Background(), user.NECOAddress)
	if err != nil {
		t.Fatalf("PendingByNECOAddress() error = %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending deposits = %+v, want empty", pending)
	}
}

func TestNECODepositServiceIsIdempotentByChainTokenTxAndLogIndex(t *testing.T) {
	authStore := NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := NewMemoryLedgerStore()
	service := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	transfer := NECOTokenTransfer{
		ChainID:       "9001",
		TokenAddress:  "0x0000000000000000000000000000000000000ded",
		From:          "0x0000000000000000000000000000000000000001",
		To:            "0x0000000000000000000000000000000000000abc",
		Amount:        "100",
		TxHash:        "0xtx1",
		LogIndex:      2,
		Confirmations: 12,
	}
	if _, err := service.ProcessTransfer(context.Background(), transfer); err != nil {
		t.Fatalf("first ProcessTransfer() error = %v", err)
	}

	deposit, err := service.ProcessTransfer(context.Background(), transfer)

	if err != nil {
		t.Fatalf("second ProcessTransfer() error = %v", err)
	}
	if deposit.Status != NECODepositStatusCredited {
		t.Fatalf("deposit status = %q", deposit.Status)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}
}

func TestNECODepositServiceRejectsUnconfirmedTransfer(t *testing.T) {
	service := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(NewMemoryLedgerStore()), NewMemoryAuthStore(), NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})

	_, err := service.ProcessTransfer(context.Background(), NECOTokenTransfer{
		ChainID:       "9001",
		TokenAddress:  "0x0000000000000000000000000000000000000ded",
		From:          "0x0000000000000000000000000000000000000001",
		To:            "0x0000000000000000000000000000000000000abc",
		Amount:        "100",
		TxHash:        "0xtx1",
		LogIndex:      2,
		Confirmations: 11,
	})

	if !errors.Is(err, ErrNECODepositNotConfirmed) {
		t.Fatalf("ProcessTransfer() error = %v, want %v", err, ErrNECODepositNotConfirmed)
	}
}
