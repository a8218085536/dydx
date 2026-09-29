package tyche

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrNECOMalformedTransferLog = errors.New("neco malformed transfer log")

type NECOLogBackend interface {
	ChainID(context.Context) (*big.Int, error)
	BlockNumber(context.Context) (uint64, error)
	FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error)
}

type NECODepositReceiptBackend interface {
	ChainID(context.Context) (*big.Int, error)
	BlockNumber(context.Context) (uint64, error)
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
}

const defaultNECODepositMaxBlockRange uint64 = 512

type NECODepositWatcherConfig struct {
	FromBlock        uint64
	ToBlock          uint64
	DepositAddress   string
	TokenAddresses   []string
	MinConfirmations uint64
	MaxBlockRange    uint64
}

type NECODepositWatcher struct {
	backend NECOLogBackend
	service *NECODepositService
	conf    NECODepositWatcherConfig
}

func NewNECODepositWatcher(backend NECOLogBackend, service *NECODepositService, conf NECODepositWatcherConfig) *NECODepositWatcher {
	return &NECODepositWatcher{backend: backend, service: service, conf: conf}
}

func (w *NECODepositWatcher) PollOnce(ctx context.Context) ([]NECODeposit, error) {
	chainID, err := w.backend.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := w.backend.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	toBlock := w.conf.ToBlock
	if toBlock == 0 || toBlock > latest {
		toBlock = latest
	}
	logs, err := w.backend.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(w.conf.FromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: tokenAddresses(w.conf.TokenAddresses),
		Topics:    transferToTopics(w.conf.DepositAddress),
	})
	if err != nil {
		return nil, err
	}

	deposits := make([]NECODeposit, 0, len(logs))
	for _, log := range logs {
		transfer, err := NECOTokenTransferFromLog(chainID.String(), confirmations(latest, log.BlockNumber), log)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(transfer.To, w.conf.DepositAddress) {
			continue
		}
		deposit, err := w.service.ProcessTransfer(ctx, transfer)
		if errors.Is(err, ErrNECODepositNotConfirmed) {
			continue
		}
		if err != nil {
			return nil, err
		}
		deposits = append(deposits, deposit)
	}
	return deposits, nil
}

type NECODepositReceiptSyncer struct {
	backend NECODepositReceiptBackend
	service *NECODepositService
	conf    NECODepositWatcherConfig
}

func NewNECODepositReceiptSyncer(backend NECODepositReceiptBackend, service *NECODepositService, conf NECODepositWatcherConfig) *NECODepositReceiptSyncer {
	return &NECODepositReceiptSyncer{backend: backend, service: service, conf: conf}
}

func (s *NECODepositReceiptSyncer) SyncReceipt(ctx context.Context, txHash string) ([]NECODeposit, error) {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return nil, ErrNECODepositTxHashRequired
	}
	receipt, err := s.backend.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, ErrNECODepositReceiptNotFound
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, ErrNECODepositReceiptFailed
	}
	chainID, err := s.backend.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := s.backend.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	deposits := make([]NECODeposit, 0, len(receipt.Logs))
	for _, receiptLog := range receipt.Logs {
		if receiptLog == nil || !s.isCandidateTransferLog(*receiptLog) {
			continue
		}
		log := s.normalizeReceiptLog(*receiptLog, receipt, txHash)
		transfer, err := NECOTokenTransferFromLog(chainID.String(), confirmations(latest, log.BlockNumber), log)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(transfer.To, s.conf.DepositAddress) {
			continue
		}
		deposit, err := s.service.ProcessTransfer(ctx, transfer)
		if err != nil {
			return nil, err
		}
		deposits = append(deposits, deposit)
	}
	if len(deposits) == 0 {
		return nil, ErrNECODepositNoMatchingLog
	}
	return deposits, nil
}

func (s *NECODepositReceiptSyncer) isCandidateTransferLog(log types.Log) bool {
	if len(log.Topics) == 0 || log.Topics[0] != ERC20TransferTopic() {
		return false
	}
	if len(s.conf.TokenAddresses) == 0 {
		return true
	}
	for _, address := range s.conf.TokenAddresses {
		if strings.EqualFold(log.Address.Hex(), address) {
			return true
		}
	}
	return false
}

func (s *NECODepositReceiptSyncer) normalizeReceiptLog(log types.Log, receipt *types.Receipt, txHash string) types.Log {
	if log.BlockNumber == 0 && receipt.BlockNumber != nil {
		log.BlockNumber = receipt.BlockNumber.Uint64()
	}
	if log.TxHash == (common.Hash{}) {
		log.TxHash = common.HexToHash(txHash)
	}
	return log
}

func NECOTokenTransferFromLog(chainID string, confirmations uint64, log types.Log) (NECOTokenTransfer, error) {
	if len(log.Topics) < 3 || len(log.Data) != 32 || log.Topics[0] != ERC20TransferTopic() {
		return NECOTokenTransfer{}, ErrNECOMalformedTransferLog
	}
	amount := new(big.Int).SetBytes(log.Data)
	return NECOTokenTransfer{
		ChainID:       chainID,
		TokenAddress:  log.Address.Hex(),
		From:          common.BytesToAddress(log.Topics[1].Bytes()).Hex(),
		To:            common.BytesToAddress(log.Topics[2].Bytes()).Hex(),
		Amount:        amount.String(),
		TxHash:        log.TxHash.Hex(),
		LogIndex:      log.Index,
		Confirmations: confirmations,
	}, nil
}

func ERC20TransferTopic() common.Hash {
	return crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
}

func tokenAddresses(addresses []string) []common.Address {
	out := make([]common.Address, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, common.HexToAddress(address))
	}
	return out
}

func transferToTopics(depositAddress string) [][]common.Hash {
	topics := [][]common.Hash{{ERC20TransferTopic()}}
	if strings.TrimSpace(depositAddress) != "" {
		topics = append(topics, nil, []common.Hash{common.HexToHash(depositAddress)})
	}
	return topics
}

func confirmations(latest, blockNumber uint64) uint64 {
	if latest < blockNumber {
		return 0
	}
	return latest - blockNumber + 1
}

type NECOBlockCursor interface {
	Get(context.Context) (uint64, error)
	Set(context.Context, uint64) error
}

type NECODepositSyncWorker struct {
	backend NECOLogBackend
	service *NECODepositService
	cursor  NECOBlockCursor
	conf    NECODepositWatcherConfig
}

func NewNECODepositSyncWorker(backend NECOLogBackend, service *NECODepositService, cursor NECOBlockCursor, conf NECODepositWatcherConfig) *NECODepositSyncWorker {
	return &NECODepositSyncWorker{backend: backend, service: service, cursor: cursor, conf: conf}
}

func (w *NECODepositSyncWorker) PollOnce(ctx context.Context) ([]NECODeposit, error) {
	fromBlock, err := w.cursor.Get(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := w.backend.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	if latest < w.conf.MinConfirmations {
		return nil, nil
	}
	confirmedTo := latest - w.conf.MinConfirmations + 1
	if confirmedTo < fromBlock {
		return nil, nil
	}

	conf := w.conf
	conf.FromBlock = fromBlock
	conf.ToBlock = minUint64(confirmedTo, fromBlock+w.maxBlockRange()-1)
	watcher := NewNECODepositWatcher(w.backend, w.service, conf)
	deposits, err := watcher.PollOnce(ctx)
	if err != nil {
		return nil, err
	}
	if err := w.cursor.Set(ctx, conf.ToBlock+1); err != nil {
		return nil, err
	}
	return deposits, nil
}

func (w *NECODepositSyncWorker) maxBlockRange() uint64 {
	if w.conf.MaxBlockRange == 0 {
		return defaultNECODepositMaxBlockRange
	}
	return w.conf.MaxBlockRange
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

type MemoryNECOBlockCursor struct {
	next uint64
}

func NewMemoryNECOBlockCursor(next uint64) *MemoryNECOBlockCursor {
	return &MemoryNECOBlockCursor{next: next}
}

func (c *MemoryNECOBlockCursor) Get(context.Context) (uint64, error) {
	return c.next, nil
}

func (c *MemoryNECOBlockCursor) Set(_ context.Context, next uint64) error {
	c.next = next
	return nil
}
