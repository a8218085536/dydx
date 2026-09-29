package tyche

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestNECOWithdrawalWorkerSignsAndBroadcastsERC20Transfer(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	store := NewMemoryNECOWithdrawalStore()
	backend := &fakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, store, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}

	record, err := worker.ProcessWithdrawal(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("ProcessWithdrawal() error = %v", err)
	}
	if record.Status != NECOWithdrawalStatusBroadcasted {
		t.Fatalf("record status = %q, want %q", record.Status, NECOWithdrawalStatusBroadcasted)
	}
	if record.Nonce != 7 {
		t.Fatalf("nonce = %d, want 7", record.Nonce)
	}
	if record.TxHash == "" {
		t.Fatal("TxHash is empty")
	}
	if backend.sent == nil {
		t.Fatal("sent tx is nil")
	}
	if backend.sent.Nonce() != 7 {
		t.Fatalf("tx nonce = %d, want 7", backend.sent.Nonce())
	}
	if backend.sent.To() == nil || *backend.sent.To() != common.HexToAddress("0x0000000000000000000000000000000000000ded") {
		t.Fatalf("tx to = %v, want token contract", backend.sent.To())
	}
}

func TestNECOWithdrawalWorkerIsIdempotentByWithdrawalID(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	store := NewMemoryNECOWithdrawalStore()
	backend := &fakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, store, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	first, err := worker.ProcessWithdrawal(context.Background(), withdrawal.ID)
	if err != nil {
		t.Fatalf("first ProcessWithdrawal() error = %v", err)
	}
	backend.sent = nil

	second, err := worker.ProcessWithdrawal(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("second ProcessWithdrawal() error = %v", err)
	}
	if second.TxHash != first.TxHash {
		t.Fatalf("second txHash = %q, want %q", second.TxHash, first.TxHash)
	}
	if backend.sent != nil {
		t.Fatal("second call broadcasted another transaction")
	}
}

func TestNECOWithdrawalWorkerReleasesLedgerFreezeWhenBroadcastFails(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	store := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	worker, err := NewNECOWithdrawalWorker(ledger, store, &fakeNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    1,
		sendErr:  errors.New("broadcast failed"),
	}, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}

	if _, err := worker.ProcessWithdrawal(context.Background(), withdrawal.ID); err == nil {
		t.Fatalf("ProcessWithdrawal() error = nil, want broadcast failed")
	}
	balance, err := ledger.GetBalance(context.Background(), withdrawal.UserID, withdrawal.Asset)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=100 frozen=0", balance)
	}
	record, ok, err := store.Get(context.Background(), withdrawal.ID)
	if err != nil || !ok {
		t.Fatalf("Get() record=%+v ok=%v err=%v", record, ok, err)
	}
	if record.Status != NECOWithdrawalStatusFailed || record.Reason != "broadcast failed" {
		t.Fatalf("record = %+v, want failed broadcast record", record)
	}
}

func TestNECOWithdrawalWorkerReservesSequentialNonces(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	first := createFrozenNECOWithdrawal(t, ledger)
	second, err := ledger.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "DEDX",
		Amount:    "10",
		ToAddress: "0x0000000000000000000000000000000000000bbb",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &fakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, NewMemoryNECOWithdrawalStore(), backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	firstRecord, err := worker.ProcessWithdrawal(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("first ProcessWithdrawal() error = %v", err)
	}

	secondRecord, err := worker.ProcessWithdrawal(context.Background(), second.ID)

	if err != nil {
		t.Fatalf("second ProcessWithdrawal() error = %v", err)
	}
	if firstRecord.Nonce != 7 || secondRecord.Nonce != 8 {
		t.Fatalf("nonces = %d,%d; want 7,8", firstRecord.Nonce, secondRecord.Nonce)
	}
}

func TestNECOWithdrawalWorkerMarksPaidAfterSuccessfulReceipt(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	store := NewMemoryNECOWithdrawalStore()
	record := NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		TxHash:       "0xtx1",
		Status:       NECOWithdrawalStatusBroadcasted,
	}
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	backend := &fakeNECOWithdrawalBackend{
		receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
	}
	worker := &NECOWithdrawalWorker{ledger: ledger, store: store, backend: backend}

	paid, err := worker.SyncReceipt(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("SyncReceipt() error = %v", err)
	}
	if paid.Status != NECOWithdrawalStatusPaid {
		t.Fatalf("record status = %q, want %q", paid.Status, NECOWithdrawalStatusPaid)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "60" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=60 frozen=0", balance)
	}
}

func TestNECOWithdrawalWorkerReconcilesFailedRecordWithFrozenLedger(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	store := NewMemoryNECOWithdrawalStore()
	if err := store.Save(context.Background(), NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		TxHash:       "0xtx1",
		Status:       NECOWithdrawalStatusFailed,
		Reason:       "neco transaction failed",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	worker := &NECOWithdrawalWorker{ledger: ledger, store: store}

	record, err := worker.SyncReceipt(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("SyncReceipt() error = %v", err)
	}
	if record.Status != NECOWithdrawalStatusFailed {
		t.Fatalf("record status = %q, want failed", record.Status)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want failed withdrawal released back to available", balance)
	}
}

func TestNECOWithdrawalWorkerFailsOldMissingReceiptAndReleasesLedger(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	store := NewMemoryNECOWithdrawalStore()
	if err := store.Save(context.Background(), NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		Asset:        withdrawal.Asset,
		Amount:       withdrawal.Amount,
		ToAddress:    withdrawal.ToAddress,
		TxHash:       "0xtx1",
		Status:       NECOWithdrawalStatusBroadcasted,
		UpdatedAt:    time.Now().Add(-defaultNECOWithdrawalMissingReceiptFailAfter - time.Second).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	worker := &NECOWithdrawalWorker{
		ledger: ledger,
		store:  store,
		backend: &fakeNECOWithdrawalBackend{
			receiptErr: ethereum.NotFound,
		},
	}

	record, err := worker.SyncReceipt(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("SyncReceipt() error = %v", err)
	}
	if record.Status != NECOWithdrawalStatusFailed || record.Reason != NECOWithdrawalReasonReceiptNotFound {
		t.Fatalf("record = %+v, want failed missing receipt", record)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "100" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want missing receipt released back to available", balance)
	}
}

func TestNECOWithdrawalWorkerKeepsTradingSettlementWithdrawalFrozenWhenReceiptMissing(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	settlements := NewMemoryTradingSettlementStore()
	if _, _, err := settlements.CreateIfAbsent(context.Background(), TradingSettlement{
		UserID:             withdrawal.UserID,
		Kind:               TradingSettlementKindClosePosition,
		CloseOrderID:       "close-order-1",
		SettlementAsset:    withdrawal.Asset,
		SettlementAmount:   withdrawal.Amount,
		LedgerWithdrawalID: withdrawal.ID,
		Status:             TradingSettlementStatusNECOPayoutBroadcasted,
	}); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	store := NewMemoryNECOWithdrawalStore()
	if err := store.Save(context.Background(), NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		Asset:        withdrawal.Asset,
		Amount:       withdrawal.Amount,
		ToAddress:    withdrawal.ToAddress,
		TxHash:       "0xtx1",
		Status:       NECOWithdrawalStatusBroadcasted,
		UpdatedAt:    time.Now().Add(-defaultNECOWithdrawalMissingReceiptFailAfter - time.Second).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	worker := &NECOWithdrawalWorker{
		ledger:      ledger,
		store:       store,
		backend:     &fakeNECOWithdrawalBackend{receiptErr: ethereum.NotFound},
		settlements: settlements,
	}

	record, err := worker.SyncReceipt(context.Background(), withdrawal.ID)

	if err != nil {
		t.Fatalf("SyncReceipt() error = %v", err)
	}
	if record.Status != NECOWithdrawalStatusFailed || record.Reason != NECOWithdrawalReasonReceiptNotFound {
		t.Fatalf("record = %+v, want failed missing receipt", record)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "60" || balance.Frozen != "40" {
		t.Fatalf("balance = %+v, want trading settlement withdrawal to remain frozen", balance)
	}
}

func TestNECOWithdrawalWorkerKeepsRecentMissingReceiptPending(t *testing.T) {
	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	store := NewMemoryNECOWithdrawalStore()
	if err := store.Save(context.Background(), NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		Asset:        withdrawal.Asset,
		Amount:       withdrawal.Amount,
		ToAddress:    withdrawal.ToAddress,
		TxHash:       "0xtx1",
		Status:       NECOWithdrawalStatusBroadcasted,
		UpdatedAt:    time.Now().Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	worker := &NECOWithdrawalWorker{
		ledger: ledger,
		store:  store,
		backend: &fakeNECOWithdrawalBackend{
			receiptErr: ethereum.NotFound,
		},
	}

	if _, err := worker.SyncReceipt(context.Background(), withdrawal.ID); !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("SyncReceipt() error = %v, want ethereum.NotFound", err)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), "user-1", "DEDX")
	if balance.Available != "60" || balance.Frozen != "40" {
		t.Fatalf("balance = %+v, want recent missing receipt still frozen", balance)
	}
}

func TestNECOWithdrawalWorkerRepairFrozenWithdrawalsDetectsMissingRecordWithoutBroadcasting(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	withdrawal := createFrozenNECOWithdrawal(t, ledger)
	store := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &fakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}
	worker, err := NewNECOWithdrawalWorker(ledger, store, backend, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}

	report, err := worker.RepairFrozenWithdrawals(context.Background(), 100)

	if err != nil {
		t.Fatalf("RepairFrozenWithdrawals() error = %v", err)
	}
	if report.MissingRecordsDetected != 1 || report.MissingRecordsProcessed != 0 || report.FrozenWithdrawals != 1 {
		t.Fatalf("repair report = %+v, want one missing frozen withdrawal detected without processing", report)
	}
	record, ok, err := store.Get(context.Background(), withdrawal.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok || backend.sent != nil {
		t.Fatalf("record=%+v ok=%v sent=%v, want missing record left unbroadcasted", record, ok, backend.sent)
	}
}

func createFrozenNECOWithdrawal(t *testing.T, ledger *LedgerService) LedgerWithdrawal {
	t.Helper()
	if _, err := ledger.CreditMockDeposit(context.Background(), LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "100",
		RefID:  "deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := ledger.FreezeWithdrawal(context.Background(), LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "DEDX",
		Amount:    "40",
		ToAddress: "0x0000000000000000000000000000000000000aaa",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	return withdrawal
}

type fakeNECOWithdrawalBackend struct {
	chainID    *big.Int
	gasPrice   *big.Int
	nonce      uint64
	sent       *types.Transaction
	receipt    *types.Receipt
	receiptErr error
	sendErr    error
}

func (b *fakeNECOWithdrawalBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return b.chainID, nil
}

func (b *fakeNECOWithdrawalBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return b.nonce, nil
}

func (b *fakeNECOWithdrawalBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return b.gasPrice, nil
}

func (b *fakeNECOWithdrawalBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.sent = tx
	return b.sendErr
}

func (b *fakeNECOWithdrawalBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	if b.receiptErr != nil {
		return nil, b.receiptErr
	}
	return b.receipt, nil
}
