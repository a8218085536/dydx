package app

import (
	"bytes"
	"context"
	"ethgo/tyche"
	"net/http"
	"testing"
)

func TestLedgerMockDepositAPI(t *testing.T) {
	app := &App{ledger: tyche.NewLedgerService(tyche.NewMemoryLedgerStore())}

	rec := postJSON(t, app, "/tyche/api/ledger/deposit/mock", map[string]interface{}{
		"userId": "user-1",
		"asset":  "USDC",
		"amount": "100",
		"refId":  "mock-deposit-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.LedgerDepositStatusCredited)) {
		t.Fatalf("body=%s, want credited", rec.Body.String())
	}
}

func TestLedgerFreezeWithdrawalAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	service := tyche.NewLedgerService(store)
	_, err := service.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	})
	if err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	app := &App{ledger: service}

	rec := postJSON(t, app, "/tyche/api/ledger/withdrawal/freeze", map[string]interface{}{
		"userId":    "user-1",
		"asset":     "USDC",
		"amount":    "40",
		"toAddress": "neco-recipient",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.LedgerWithdrawalStatusFrozen)) {
		t.Fatalf("body=%s, want frozen", rec.Body.String())
	}
}

func TestLedgerMarkWithdrawalPaidAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	service := tyche.NewLedgerService(store)
	withdrawal := createFrozenWithdrawalForAppTest(t, service)
	app := &App{ledger: service}

	rec := postJSON(t, app, "/tyche/api/ledger/withdrawal/paid", map[string]interface{}{
		"withdrawalId": withdrawal.ID,
		"txHash":       "neco-tx-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.LedgerWithdrawalStatusPaid)) {
		t.Fatalf("body=%s, want paid", rec.Body.String())
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "60" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=60 frozen=0", balance)
	}
}

func TestLedgerMarkWithdrawalFailedAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	service := tyche.NewLedgerService(store)
	withdrawal := createFrozenWithdrawalForAppTest(t, service)
	app := &App{ledger: service}

	rec := postJSON(t, app, "/tyche/api/ledger/withdrawal/failed", map[string]interface{}{
		"withdrawalId": withdrawal.ID,
		"reason":       "neco transfer failed",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.LedgerWithdrawalStatusFailed)) {
		t.Fatalf("body=%s, want failed", rec.Body.String())
	}
	balance, _ := store.GetBalance(context.Background(), "user-1", "USDC")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
}

func TestLedgerBalanceAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	service := tyche.NewLedgerService(store)
	if _, err := service.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	app := &App{ledger: service}

	rec := postJSON(t, app, "/tyche/api/ledger/balance", map[string]interface{}{
		"userId": "user-1",
		"asset":  "USDC",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"available":"100"`)) {
		t.Fatalf("body=%s, want available=100", rec.Body.String())
	}
}

type appRecordingDYDXSubmitter struct {
	intent tyche.DYDXIntent
}

func (s *appRecordingDYDXSubmitter) Submit(ctx context.Context, intent tyche.DYDXIntent) error {
	s.intent = intent
	return nil
}

func TestLedgerRepairFrozenBalanceAPI(t *testing.T) {
	store := tyche.NewMemoryLedgerStore()
	service := tyche.NewLedgerService(store)
	if _, err := service.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	if _, err := service.FreezeWithdrawal(context.Background(), tyche.LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	}); err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	app := &App{ledger: service}
	rec := postJSON(t, app, "/tyche/api/ledger/frozen/repair", map[string]interface{}{
		"userId": "user-1",
		"asset":  "USDC",
		"apply":  true,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"expectedFrozen":"40"`)) {
		t.Fatalf("body = %s, want expected frozen amount", rec.Body.String())
	}
}

func createFrozenWithdrawalForAppTest(t *testing.T, service *tyche.LedgerService) tyche.LedgerWithdrawal {
	t.Helper()
	if _, err := service.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "USDC",
		Amount: "100",
		RefID:  "mock-deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := service.FreezeWithdrawal(context.Background(), tyche.LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "USDC",
		Amount:    "40",
		ToAddress: "neco-recipient",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	return withdrawal
}
