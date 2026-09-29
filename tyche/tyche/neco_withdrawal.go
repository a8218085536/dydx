package tyche

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"strings"
	"time"

	"ethgo/eth"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	NECOWithdrawalStatusBroadcasted = "broadcasted"
	NECOWithdrawalStatusPaid        = "paid"
	NECOWithdrawalStatusFailed      = "failed"

	NECOWithdrawalReasonReceiptNotFound = "neco withdrawal tx not found after receipt timeout"
)

var (
	ErrNECOWithdrawalUnsupportedAsset = errors.New("neco withdrawal unsupported asset")
	ErrNECOWithdrawalNotFound         = errors.New("neco withdrawal not found")
	ErrNECOWithdrawalInvalidHotWallet = errors.New("neco withdrawal invalid hot wallet")
)

var defaultNECOWithdrawalMissingReceiptFailAfter = 10 * time.Minute

type NECOWalletConfig struct {
	ChainID          int64             `json:"chainID" toml:"chainID"`
	HotWalletAddress string            `json:"hotWalletAddress" toml:"hotWalletAddress"`
	HotWalletKey     []byte            `json:"-" toml:"-"`
	TokenContracts   map[string]string `json:"tokenContracts" toml:"tokenContracts"`
	GasLimit         uint64            `json:"gasLimit" toml:"gasLimit"`
}

type NECOWithdrawalRecord struct {
	WithdrawalID string `json:"withdrawalId" redis:"withdrawalId"`
	Asset        string `json:"asset" redis:"asset"`
	Amount       string `json:"amount" redis:"amount"`
	ToAddress    string `json:"toAddress" redis:"toAddress"`
	TxHash       string `json:"txHash" redis:"txHash"`
	Nonce        uint64 `json:"nonce" redis:"nonce"`
	Status       string `json:"status" redis:"status"`
	Reason       string `json:"reason,omitempty" redis:"reason"`
	CreatedAt    int64  `json:"createdAt,omitempty" redis:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt,omitempty" redis:"updatedAt"`
}

type NECOWithdrawalRepairReport struct {
	FrozenWithdrawals       int `json:"frozenWithdrawals"`
	ExistingRecordsSynced   int `json:"existingRecordsSynced"`
	MissingRecordsProcessed int `json:"missingRecordsProcessed"`
	MissingRecordsDetected  int `json:"missingRecordsDetected"`
	Errors                  int `json:"errors"`
}

type NECOWithdrawalStore interface {
	Get(context.Context, string) (NECOWithdrawalRecord, bool, error)
	Save(context.Context, NECOWithdrawalRecord) error
	MarkStatus(context.Context, string, string, string) (NECOWithdrawalRecord, error)
}

type NECOWithdrawalBackend interface {
	ChainID(context.Context) (*big.Int, error)
	PendingNonceAt(context.Context, common.Address) (uint64, error)
	SuggestGasPrice(context.Context) (*big.Int, error)
	SendTransaction(context.Context, *types.Transaction) error
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
}

type NECOWithdrawalSettlementLookup interface {
	GetByLedgerWithdrawalID(context.Context, string) (TradingSettlement, bool, error)
}

type NECOWithdrawalWorker struct {
	ledger      *LedgerService
	store       NECOWithdrawalStore
	backend     NECOWithdrawalBackend
	conf        NECOWalletConfig
	key         *ecdsa.PrivateKey
	account     common.Address
	nonces      *NECONonceManager
	settlements NECOWithdrawalSettlementLookup
}

func NewNECOWithdrawalWorker(ledger *LedgerService, store NECOWithdrawalStore, backend NECOWithdrawalBackend, conf NECOWalletConfig) (*NECOWithdrawalWorker, error) {
	key, err := crypto.ToECDSA(conf.HotWalletKey)
	if err != nil {
		return nil, err
	}
	account := crypto.PubkeyToAddress(key.PublicKey)
	if conf.HotWalletAddress != "" && !strings.EqualFold(account.Hex(), conf.HotWalletAddress) {
		return nil, ErrNECOWithdrawalInvalidHotWallet
	}
	if conf.GasLimit == 0 {
		conf.GasLimit = 60000
	}
	return &NECOWithdrawalWorker{
		ledger:  ledger,
		store:   store,
		backend: backend,
		conf:    conf,
		key:     key,
		account: account,
		nonces:  NewNECONonceManager(backend, account),
	}, nil
}

func (w *NECOWithdrawalWorker) SetSettlementLookup(lookup NECOWithdrawalSettlementLookup) {
	if w != nil {
		w.settlements = lookup
	}
}

func (w *NECOWithdrawalWorker) ProcessWithdrawal(ctx context.Context, withdrawalID string) (NECOWithdrawalRecord, error) {
	if record, ok, err := w.store.Get(ctx, withdrawalID); err != nil {
		return NECOWithdrawalRecord{}, err
	} else if ok {
		return record, nil
	}
	withdrawal, err := w.ledger.GetWithdrawal(ctx, withdrawalID)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	if withdrawal.Status != LedgerWithdrawalStatusFrozen {
		return NECOWithdrawalRecord{}, errors.New("ledger withdrawal is not frozen")
	}
	tokenAddress, ok := w.conf.TokenContracts[withdrawal.Asset]
	if !ok {
		return NECOWithdrawalRecord{}, ErrNECOWithdrawalUnsupportedAsset
	}
	amount, ok := parseLedgerAmount(withdrawal.Amount)
	if !ok {
		return NECOWithdrawalRecord{}, errors.New("invalid withdrawal amount")
	}
	chainID, err := w.backend.ChainID(ctx)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	nonce, err := w.reserveNonce(ctx)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	gasPrice, err := w.backend.SuggestGasPrice(ctx)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	data, err := PackERC20Transfer(common.HexToAddress(withdrawal.ToAddress), amount)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	tx := types.NewTransaction(nonce, common.HexToAddress(tokenAddress), big.NewInt(0), w.conf.GasLimit, gasPrice, data)
	signer, err := eth.NewSigner(w.key, chainID)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	signedTx, err := signer.Sign(w.account, tx)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	now := time.Now().Unix()
	record := NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		Asset:        withdrawal.Asset,
		Amount:       withdrawal.Amount,
		ToAddress:    withdrawal.ToAddress,
		TxHash:       signedTx.Hash().Hex(),
		Nonce:        nonce,
		Status:       NECOWithdrawalStatusBroadcasted,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := w.store.Save(ctx, record); err != nil {
		return NECOWithdrawalRecord{}, err
	}
	if err := w.backend.SendTransaction(ctx, signedTx); err != nil {
		record, markErr := w.store.MarkStatus(ctx, withdrawal.ID, NECOWithdrawalStatusFailed, err.Error())
		if markErr != nil {
			return NECOWithdrawalRecord{}, markErr
		}
		if markErr := w.reconcileTerminalLedger(ctx, record); markErr != nil {
			return NECOWithdrawalRecord{}, markErr
		}
		return record, err
	}
	return record, nil
}

func (w *NECOWithdrawalWorker) reserveNonce(ctx context.Context) (uint64, error) {
	if w.nonces == nil {
		return w.backend.PendingNonceAt(ctx, w.account)
	}
	return w.nonces.Reserve(ctx)
}

func (w *NECOWithdrawalWorker) SyncReceipt(ctx context.Context, withdrawalID string) (NECOWithdrawalRecord, error) {
	record, ok, err := w.store.Get(ctx, withdrawalID)
	if err != nil {
		return NECOWithdrawalRecord{}, err
	}
	if !ok {
		return NECOWithdrawalRecord{}, ErrNECOWithdrawalNotFound
	}
	if record.Status == NECOWithdrawalStatusPaid || record.Status == NECOWithdrawalStatusFailed {
		if err := w.reconcileTerminalLedger(ctx, record); err != nil {
			return NECOWithdrawalRecord{}, err
		}
		return record, nil
	}
	receipt, err := w.backend.TransactionReceipt(ctx, common.HexToHash(record.TxHash))
	if err != nil {
		if isNECOReceiptNotFound(err) && necoWithdrawalReceiptMissingTooLong(record) {
			failed := record
			failed.Status = NECOWithdrawalStatusFailed
			failed.Reason = NECOWithdrawalReasonReceiptNotFound
			if err := w.reconcileTerminalLedger(ctx, failed); err != nil {
				return NECOWithdrawalRecord{}, err
			}
			return w.store.MarkStatus(ctx, withdrawalID, NECOWithdrawalStatusFailed, NECOWithdrawalReasonReceiptNotFound)
		}
		return NECOWithdrawalRecord{}, err
	}
	if receipt.Status == types.ReceiptStatusSuccessful {
		paid := record
		paid.Status = NECOWithdrawalStatusPaid
		paid.Reason = ""
		if err := w.reconcileTerminalLedger(ctx, paid); err != nil {
			return NECOWithdrawalRecord{}, err
		}
		return w.store.MarkStatus(ctx, withdrawalID, NECOWithdrawalStatusPaid, "")
	}
	failed := record
	failed.Status = NECOWithdrawalStatusFailed
	failed.Reason = "neco transaction failed"
	if err := w.reconcileTerminalLedger(ctx, failed); err != nil {
		return NECOWithdrawalRecord{}, err
	}
	return w.store.MarkStatus(ctx, withdrawalID, NECOWithdrawalStatusFailed, "neco transaction failed")
}

func (w *NECOWithdrawalWorker) RepairFrozenWithdrawals(ctx context.Context, limit int) (NECOWithdrawalRepairReport, error) {
	report := NECOWithdrawalRepairReport{}
	if w == nil || w.ledger == nil || w.store == nil {
		return report, nil
	}
	withdrawals, err := w.ledger.ListWithdrawalsByStatus(ctx, LedgerWithdrawalStatusFrozen, limit)
	if err != nil {
		return report, err
	}
	var firstErr error
	for _, withdrawal := range withdrawals {
		report.FrozenWithdrawals++
		_, ok, err := w.store.Get(ctx, withdrawal.ID)
		if err != nil {
			report.Errors++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			if _, err = w.SyncReceipt(ctx, withdrawal.ID); err != nil {
				report.Errors++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			report.ExistingRecordsSynced++
			continue
		}
		report.MissingRecordsDetected++
	}
	return report, firstErr
}

func (w *NECOWithdrawalWorker) reconcileTerminalLedger(ctx context.Context, record NECOWithdrawalRecord) error {
	if w == nil || w.ledger == nil || record.WithdrawalID == "" {
		return nil
	}
	withdrawal, err := w.ledger.GetWithdrawal(ctx, record.WithdrawalID)
	if err != nil {
		return err
	}
	if withdrawal.Status != LedgerWithdrawalStatusFrozen {
		return nil
	}
	switch record.Status {
	case NECOWithdrawalStatusPaid:
		_, err = w.ledger.MarkWithdrawalPaid(ctx, LedgerWithdrawalResult{
			WithdrawalID: record.WithdrawalID,
			TxHash:       record.TxHash,
		})
	case NECOWithdrawalStatusFailed:
		releaseFailed, releaseErr := w.shouldReleaseFailedWithdrawal(ctx, record.WithdrawalID)
		if releaseErr != nil {
			return releaseErr
		}
		if !releaseFailed {
			return nil
		}
		_, err = w.ledger.MarkWithdrawalFailed(ctx, LedgerWithdrawalResult{
			WithdrawalID: record.WithdrawalID,
			Reason:       firstNonEmptyTradingString(record.Reason, "neco transaction failed"),
		})
	}
	return err
}

func (w *NECOWithdrawalWorker) shouldReleaseFailedWithdrawal(ctx context.Context, withdrawalID string) (bool, error) {
	if w == nil || w.settlements == nil || strings.TrimSpace(withdrawalID) == "" {
		return true, nil
	}
	settlement, ok, err := w.settlements.GetByLedgerWithdrawalID(ctx, withdrawalID)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	switch settlement.Kind {
	case TradingSettlementKindClosePosition,
		TradingSettlementKindCancelOrder,
		TradingSettlementKindFailedOrder:
		return false, nil
	default:
		return true, nil
	}
}

func PackERC20Transfer(to common.Address, amount *big.Int) ([]byte, error) {
	arguments := abi.Arguments{
		{Type: mustABIType("address")},
		{Type: mustABIType("uint256")},
	}
	payload, err := arguments.Pack(to, amount)
	if err != nil {
		return nil, err
	}
	methodID := crypto.Keccak256([]byte("transfer(address,uint256)"))[:4]
	return append(methodID, payload...), nil
}

func mustABIType(name string) abi.Type {
	typ, err := abi.NewType(name, "", nil)
	if err != nil {
		panic(err)
	}
	return typ
}

type MemoryNECOWithdrawalStore struct {
	records map[string]NECOWithdrawalRecord
}

type NECONonceManager struct {
	backend interface {
		PendingNonceAt(context.Context, common.Address) (uint64, error)
	}
	account     common.Address
	initialized bool
	next        uint64
}

func NewNECONonceManager(backend interface {
	PendingNonceAt(context.Context, common.Address) (uint64, error)
}, account common.Address) *NECONonceManager {
	return &NECONonceManager{backend: backend, account: account}
}

func (m *NECONonceManager) Reserve(ctx context.Context) (uint64, error) {
	if !m.initialized {
		nonce, err := m.backend.PendingNonceAt(ctx, m.account)
		if err != nil {
			return 0, err
		}
		m.next = nonce
		m.initialized = true
	}
	nonce := m.next
	m.next++
	return nonce, nil
}

func NewMemoryNECOWithdrawalStore() *MemoryNECOWithdrawalStore {
	return &MemoryNECOWithdrawalStore{records: map[string]NECOWithdrawalRecord{}}
}

func (s *MemoryNECOWithdrawalStore) Get(ctx context.Context, withdrawalID string) (NECOWithdrawalRecord, bool, error) {
	record, ok := s.records[withdrawalID]
	return record, ok, nil
}

func (s *MemoryNECOWithdrawalStore) Save(ctx context.Context, record NECOWithdrawalRecord) error {
	if _, ok := s.records[record.WithdrawalID]; ok {
		return nil
	}
	now := time.Now().Unix()
	if record.CreatedAt == 0 {
		record.CreatedAt = now
	}
	if record.UpdatedAt == 0 {
		record.UpdatedAt = record.CreatedAt
	}
	s.records[record.WithdrawalID] = record
	return nil
}

func (s *MemoryNECOWithdrawalStore) MarkStatus(ctx context.Context, withdrawalID, status, reason string) (NECOWithdrawalRecord, error) {
	record, ok := s.records[withdrawalID]
	if !ok {
		return NECOWithdrawalRecord{}, ErrNECOWithdrawalNotFound
	}
	record.Status = status
	record.Reason = reason
	record.UpdatedAt = time.Now().Unix()
	s.records[withdrawalID] = record
	return record, nil
}

func isNECOReceiptNotFound(err error) bool {
	return errors.Is(err, ethereum.NotFound) || strings.EqualFold(strings.TrimSpace(err.Error()), "not found")
}

func necoWithdrawalReceiptMissingTooLong(record NECOWithdrawalRecord) bool {
	checkpoint := record.UpdatedAt
	if checkpoint == 0 {
		checkpoint = record.CreatedAt
	}
	if checkpoint == 0 {
		return true
	}
	return time.Since(time.Unix(checkpoint, 0)) >= defaultNECOWithdrawalMissingReceiptFailAfter
}
