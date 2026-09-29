package tyche

import (
	"context"
	"errors"
	"testing"
)

func TestLedgerServiceCreditsMockDeposit(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)

	deposit, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})

	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	if deposit.Status != LedgerDepositStatusCredited {
		t.Fatalf("deposit status = %q, want %q", deposit.Status, LedgerDepositStatusCredited)
	}

	balance, err := store.GetBalance(context.Background(), "user-1", "USDC")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
}

func TestLedgerServiceRejectsDuplicateDeposit(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	request := LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	}

	if _, err := service.CreditMockDeposit(context.Background(), request); err != nil {
		t.Fatalf("first CreditMockDeposit() error = %v", err)
	}
	_, err := service.CreditMockDeposit(context.Background(), request)

	if !errors.Is(err, ErrLedgerDuplicateReference) {
		t.Fatalf("second CreditMockDeposit() error = %v, want %v", err, ErrLedgerDuplicateReference)
	}
}

func TestLedgerServiceFreezesWithdrawal(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}

	withdrawal, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})

	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	if withdrawal.Status != LedgerWithdrawalStatusFrozen {
		t.Fatalf("withdrawal status = %q, want %q", withdrawal.Status, LedgerWithdrawalStatusFrozen)
	}

	balance, err := store.GetBalance(context.Background(), "user-1", "USDC")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "60" || balance.Frozen != "40" {
		t.Fatalf("balance = %+v, want available=60 frozen=40", balance)
	}
}

func TestLedgerServiceFreezeWithdrawalDoesNotCallExternalSubmitter(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}

	withdrawal, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})

	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	if withdrawal.ToAddress != "neco-recipient" {
		t.Fatalf("withdrawal ToAddress = %q, want neco-recipient", withdrawal.ToAddress)
	}
}

func TestLedgerServiceWritesJournals(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)

	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	_, err = service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}

	journals := store.Journals()
	if len(journals) != 2 {
		t.Fatalf("len(journals) = %d, want 2", len(journals))
	}
	if journals[0].Type != LedgerJournalTypeDepositCredit {
		t.Fatalf("journal[0].Type = %q", journals[0].Type)
	}
	if journals[1].Type != LedgerJournalTypeWithdrawalFreeze {
		t.Fatalf("journal[1].Type = %q", journals[1].Type)
	}
}

func TestLedgerServiceRejectsWithdrawalWhenBalanceInsufficient(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)

	_, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})

	if !errors.Is(err, ErrLedgerInsufficientBalance) {
		t.Fatalf("FreezeWithdrawal() error = %v, want %v", err, ErrLedgerInsufficientBalance)
	}
}

func TestLedgerServiceMarksWithdrawalPaid(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}

	paid, err := service.MarkWithdrawalPaid(context.Background(), LedgerWithdrawalResult{
		WithdrawalID: withdrawal.ID,
		TxHash:       "tx-1",
	})

	if err != nil {
		t.Fatalf("MarkWithdrawalPaid() error = %v", err)
	}
	if paid.Status != LedgerWithdrawalStatusPaid {
		t.Fatalf("Status = %q, want %q", paid.Status, LedgerWithdrawalStatusPaid)
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "60" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=60 frozen=0", balance)
	}
}

func TestLedgerServiceMarksWithdrawalFailedAndUnfreezes(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}

	failed, err := service.MarkWithdrawalFailed(context.Background(), LedgerWithdrawalResult{
		WithdrawalID: withdrawal.ID,
		Reason:       "broadcast failed",
	})

	if err != nil {
		t.Fatalf("MarkWithdrawalFailed() error = %v", err)
	}
	if failed.Status != LedgerWithdrawalStatusFailed {
		t.Fatalf("Status = %q, want %q", failed.Status, LedgerWithdrawalStatusFailed)
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
}

func TestLedgerServiceRepairsFrozenBalanceFromFrozenWithdrawals(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	if _, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "1000",
		RefID:  "mock-deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	if _, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	}); err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	balance := store.balances[balanceKey("user-1", "USDC")]
	balance.Frozen = "400"
	store.balances[balanceKey("user-1", "USDC")] = balance

	report, err := service.RepairFrozenBalance(context.Background(), LedgerFrozenBalanceRepairRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Apply:  true,
	})

	if err != nil {
		t.Fatalf("RepairFrozenBalances() error = %v", err)
	}
	if report.Checked != 1 || report.Changed != 1 || report.Applied != 1 {
		t.Fatalf("repair report = %+v, want checked=1 changed=1 applied=1", report)
	}
	repaired, err := service.GetBalance(context.Background(), "user-1", "USDC")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if repaired.Available != "960" || repaired.Frozen != "40" {
		t.Fatalf("balance = %+v, want available=960 frozen=40", repaired)
	}
	if store.journals[len(store.journals)-1].Type != LedgerJournalTypeFrozenRepair {
		t.Fatalf("last journal type = %q, want %q", store.journals[len(store.journals)-1].Type, LedgerJournalTypeFrozenRepair)
	}
}

func TestLedgerServiceGetsWithdrawalByID(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := service.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}

	got, err := service.GetWithdrawal(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("GetWithdrawal() error = %v", err)
	}
	if got.ID != withdrawal.ID || got.Amount != "40" {
		t.Fatalf("withdrawal = %+v", got)
	}
}

func TestLedgerServiceChargesAvailableBalanceForDYDXOrder(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}

	balance, err := service.ChargeAvailable(context.Background(), LedgerChargeRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "25",
		RefID:  "order-1",
	})

	if err != nil {
		t.Fatalf("ChargeAvailable() error = %v", err)
	}
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=75 frozen=0", balance)
	}
}

func TestLedgerServiceRejectsDuplicateChargeReference(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	request := LedgerChargeRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "25",
		RefID:  "order-1",
	}
	if _, err := service.ChargeAvailable(context.Background(), request); err != nil {
		t.Fatalf("first ChargeAvailable() error = %v", err)
	}

	_, err = service.ChargeAvailable(context.Background(), request)

	if !errors.Is(err, ErrLedgerDuplicateReference) {
		t.Fatalf("second ChargeAvailable() error = %v, want %v", err, ErrLedgerDuplicateReference)
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "75" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=75 frozen=0", balance)
	}
}

func TestLedgerServiceRefundsAvailableBalanceForRejectedDYDXOrder(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	if _, err := service.ChargeAvailable(context.Background(), LedgerChargeRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "25",
		RefID:  "order-1",
	}); err != nil {
		t.Fatalf("ChargeAvailable() error = %v", err)
	}

	balance, err := service.RefundAvailable(context.Background(), LedgerRefundRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "25",
		RefID:  "order-refund-1",
	})

	if err != nil {
		t.Fatalf("RefundAvailable() error = %v", err)
	}
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
}

func TestLedgerServiceAppliesDYDXSettlementToAvailableBalance(t *testing.T) {
	store := NewMemoryLedgerStore()
	service := NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	if _, err := service.ChargeAvailable(context.Background(), LedgerChargeRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "25",
		RefID:  "order-1",
	}); err != nil {
		t.Fatalf("ChargeAvailable() error = %v", err)
	}

	balance, err := service.ApplyTradingSettlement(context.Background(), LedgerTradingSettlementRequest{
		UserID:         "user-1",
		Asset:          "USDC",
		ReleasedMargin: "25",
		Fee:            "2",
		RealizedPNL:    "5",
		RefID:          "fill-1",
	})

	if err != nil {
		t.Fatalf("ApplyTradingSettlement() error = %v", err)
	}
	if balance.Available != "103" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=103 frozen=0", balance)
	}
}
