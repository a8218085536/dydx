package tyche

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestNECODepositWatcherProcessesConfirmedERC20TransfersToDepositAddress(t *testing.T) {
	authStore := NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := NewMemoryLedgerStore()
	depositService := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	backend := &fakeNECOLogBackend{
		chainID:     big.NewInt(9001),
		blockNumber: 120,
		logs: []types.Log{
			erc20TransferLog(
				"0x0000000000000000000000000000000000000ded",
				"0x0000000000000000000000000000000000000001",
				"0x0000000000000000000000000000000000000abc",
				big.NewInt(100),
				100,
				3,
				"0xtx1",
			),
		},
	}
	watcher := NewNECODepositWatcher(backend, depositService, NECODepositWatcherConfig{
		FromBlock:        90,
		ToBlock:          120,
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		TokenAddresses:   []string{"0x0000000000000000000000000000000000000ded"},
		MinConfirmations: 12,
	})

	deposits, err := watcher.PollOnce(context.Background())

	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("len(deposits) = %d, want 1", len(deposits))
	}
	if backend.query.FromBlock.Uint64() != 90 || backend.query.ToBlock.Uint64() != 120 {
		t.Fatalf("query block range = %v-%v", backend.query.FromBlock, backend.query.ToBlock)
	}
	if len(backend.query.Topics) < 3 || len(backend.query.Topics[2]) != 1 || backend.query.Topics[2][0] != common.HexToHash("0x0000000000000000000000000000000000000abc") {
		t.Fatalf("query topics = %+v, want transfer-to deposit address filter", backend.query.Topics)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}
}

func TestNECODepositWatcherSkipsTransfersToOtherAddresses(t *testing.T) {
	depositService := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(NewMemoryLedgerStore()), NewMemoryAuthStore(), NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 1,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	backend := &fakeNECOLogBackend{
		chainID:     big.NewInt(9001),
		blockNumber: 120,
		logs: []types.Log{
			erc20TransferLog(
				"0x0000000000000000000000000000000000000ded",
				"0x0000000000000000000000000000000000000001",
				"0x0000000000000000000000000000000000000999",
				big.NewInt(100),
				100,
				3,
				"0xtx1",
			),
		},
	}
	watcher := NewNECODepositWatcher(backend, depositService, NECODepositWatcherConfig{
		FromBlock:        90,
		ToBlock:          120,
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		TokenAddresses:   []string{"0x0000000000000000000000000000000000000ded"},
		MinConfirmations: 1,
	})

	deposits, err := watcher.PollOnce(context.Background())

	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(deposits) != 0 {
		t.Fatalf("deposits = %+v, want empty", deposits)
	}
}

type fakeNECOLogBackend struct {
	chainID     *big.Int
	blockNumber uint64
	logs        []types.Log
	receipt     *types.Receipt
	query       ethereum.FilterQuery
}

func (b *fakeNECOLogBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return b.chainID, nil
}

func (b *fakeNECOLogBackend) BlockNumber(ctx context.Context) (uint64, error) {
	return b.blockNumber, nil
}

func (b *fakeNECOLogBackend) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	b.query = q
	return b.logs, nil
}

func (b *fakeNECOLogBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return b.receipt, nil
}

func erc20TransferLog(token, from, to string, amount *big.Int, blockNumber uint64, logIndex uint, txHash string) types.Log {
	return types.Log{
		Address:     common.HexToAddress(token),
		Topics:      []common.Hash{ERC20TransferTopic(), common.HexToHash(from), common.HexToHash(to)},
		Data:        common.LeftPadBytes(amount.Bytes(), 32),
		BlockNumber: blockNumber,
		Index:       logIndex,
		TxHash:      common.HexToHash(txHash),
	}
}

func TestNECODepositWatcherRejectsMalformedTransferLog(t *testing.T) {
	_, err := NECOTokenTransferFromLog("9001", 12, types.Log{
		Address: common.HexToAddress("0x0000000000000000000000000000000000000ded"),
		Topics:  []common.Hash{ERC20TransferTopic()},
	})

	if !errors.Is(err, ErrNECOMalformedTransferLog) {
		t.Fatalf("NECOTokenTransferFromLog() error = %v, want %v", err, ErrNECOMalformedTransferLog)
	}
}

func TestNECODepositSyncWorkerAdvancesCursorAfterPollingConfirmedRange(t *testing.T) {
	authStore := NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := NewMemoryLedgerStore()
	depositService := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	backend := &fakeNECOLogBackend{
		chainID:     big.NewInt(9001),
		blockNumber: 120,
		logs: []types.Log{
			erc20TransferLog(
				"0x0000000000000000000000000000000000000ded",
				"0x0000000000000000000000000000000000000001",
				"0x0000000000000000000000000000000000000abc",
				big.NewInt(100),
				100,
				3,
				"0xtx1",
			),
		},
	}
	cursor := NewMemoryNECOBlockCursor(90)
	worker := NewNECODepositSyncWorker(backend, depositService, cursor, NECODepositWatcherConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		TokenAddresses:   []string{"0x0000000000000000000000000000000000000ded"},
		MinConfirmations: 12,
	})

	deposits, err := worker.PollOnce(context.Background())

	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("len(deposits) = %d, want 1", len(deposits))
	}
	if backend.query.FromBlock.Uint64() != 90 || backend.query.ToBlock.Uint64() != 109 {
		t.Fatalf("query block range = %v-%v, want 90-109", backend.query.FromBlock, backend.query.ToBlock)
	}
	next, err := cursor.Get(context.Background())
	if err != nil {
		t.Fatalf("cursor.Get() error = %v", err)
	}
	if next != 110 {
		t.Fatalf("cursor = %d, want 110", next)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}
}

func TestNECODepositSyncWorkerPollsAtMostMaxBlockRange(t *testing.T) {
	depositService := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(NewMemoryLedgerStore()), NewMemoryAuthStore(), NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 1,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	backend := &fakeNECOLogBackend{
		chainID:     big.NewInt(9001),
		blockNumber: 200,
	}
	cursor := NewMemoryNECOBlockCursor(90)
	worker := NewNECODepositSyncWorker(backend, depositService, cursor, NECODepositWatcherConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		TokenAddresses:   []string{"0x0000000000000000000000000000000000000ded"},
		MinConfirmations: 1,
		MaxBlockRange:    10,
	})

	deposits, err := worker.PollOnce(context.Background())

	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(deposits) != 0 {
		t.Fatalf("deposits = %+v, want empty", deposits)
	}
	if backend.query.FromBlock.Uint64() != 90 || backend.query.ToBlock.Uint64() != 99 {
		t.Fatalf("query block range = %v-%v, want 90-99", backend.query.FromBlock, backend.query.ToBlock)
	}
	next, err := cursor.Get(context.Background())
	if err != nil {
		t.Fatalf("cursor.Get() error = %v", err)
	}
	if next != 100 {
		t.Fatalf("cursor = %d, want 100", next)
	}
}

func TestNECODepositReceiptSyncerCreditsTransferFromReceipt(t *testing.T) {
	authStore := NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := NewMemoryLedgerStore()
	depositService := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	transferLog := erc20TransferLog(
		"0x0000000000000000000000000000000000000ded",
		"0x0000000000000000000000000000000000000001",
		"0x0000000000000000000000000000000000000abc",
		big.NewInt(100),
		100,
		3,
		"0xtx1",
	)
	backend := &fakeNECOLogBackend{
		chainID:     big.NewInt(9001),
		blockNumber: 120,
		receipt: &types.Receipt{
			Status:      types.ReceiptStatusSuccessful,
			BlockNumber: big.NewInt(100),
			Logs:        []*types.Log{&transferLog},
		},
	}
	syncer := NewNECODepositReceiptSyncer(backend, depositService, NECODepositWatcherConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		TokenAddresses:   []string{"0x0000000000000000000000000000000000000ded"},
		MinConfirmations: 12,
	})

	deposits, err := syncer.SyncReceipt(context.Background(), "0xtx1")

	if err != nil {
		t.Fatalf("SyncReceipt() error = %v", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("len(deposits) = %d, want 1", len(deposits))
	}
	if deposits[0].Status != NECODepositStatusCredited {
		t.Fatalf("deposit status = %q, want credited", deposits[0].Status)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}

	deposits, err = syncer.SyncReceipt(context.Background(), "0xtx1")
	if err != nil {
		t.Fatalf("second SyncReceipt() error = %v", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("second len(deposits) = %d, want 1", len(deposits))
	}
	balance, _ = ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available after duplicate sync = %q, want 100", balance.Available)
	}
}
