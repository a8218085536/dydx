package eth

import (
	"context"
	"ethgo/util/tps"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type Backend interface {
	// BalanceAt returns the wei balance of the given account.
	// The block number can be nil, in which case the balance is taken from the latest known block.
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)

	// BlockNumber returns the most recent block number
	BlockNumber(ctx context.Context) (uint64, error)

	// BlockByNumber returns a block from the current canonical chain. If number is nil, the
	// latest known block is returned.
	//
	// Note that loading full blocks requires two requests. Use HeaderByNumber
	// if you don't need all transactions or uncle headers.
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)

	// ChainID retrieves the current chain ID for transaction replay protection.
	ChainID(ctx context.Context) (*big.Int, error)

	// Close closes the client, aborting any in-flight requests.
	Close()

	// CodeAt returns the contract code of the given account.
	// The block number can be nil, in which case the code is taken from the latest known block.
	CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)

	TransactionCount(ctx context.Context, account common.Hash) (uint, error)

	// CallContract executes a message call transaction, which is directly executed in the VM
	// of the node, but never mined into the blockchain.
	//
	// blockNumber selects the block height at which the call runs. It can be nil, in which
	// case the code is taken from the latest known block. Note that state from very old
	// blocks might not be available.
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)

	// FilterLogs executes a filter query.
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)

	// NonceAt returns the account nonce of the given account.
	// The block number can be nil, in which case the nonce is taken from the latest known block.
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)

	// PendingNonceAt returns the account nonce of the given account in the pending state.
	// This is the nonce that should be used for the next transaction.
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)

	// SendRawTransaction creates new message call transaction or a
	// contract creation for signed transactions.
	SendRawTransaction(ctx context.Context, signedTxData string) error

	// SendTransaction injects a signed transaction into the pending pool for execution.
	//
	// If the transaction was a contract creation use the TransactionReceipt method to get the
	// contract address after the transaction has been mined.
	SendTransaction(ctx context.Context, tx *types.Transaction) error

	// SuggestGasPrice retrieves the currently suggested gas price to allow a timely
	// execution of a transaction.
	SuggestGasPrice(ctx context.Context) (*big.Int, error)

	// SuggestGasLimit retrieves the currently suggested gas limit to allow a timely
	// execution of a transaction.
	SuggestGasLimit(ctx context.Context) uint64

	// TransactionsPerSecond retrieves the transactions per second limit setting.
	TransactionsPerSecond(ctx context.Context) int64

	// TransactionReceipt returns the receipt of a transaction by transaction hash.
	// Note that the receipt is not available for pending transactions.
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)

	// TransactionByHash returns the transaction with the given hash.
	TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)

	// WaitMined waits for tx to be mined on the blockchain.
	// It stops waiting when the context is canceled.
	WaitMined(ctx context.Context, tx *types.Transaction) (*types.Receipt, error)
}

func New(c *Config) (Backend, error) {
	rc, err := rpc.Dial(c.Addr)
	if err != nil {
		return nil, err
	}

	for _, header := range c.Headers {
		rc.SetHeader(header.Key, header.Value)
	}

	var tpsc = tps.New(c.TransactionsPerSecond)
	return newClient(rc, tpsc, c.DefaultGasLimit), nil
}

type tpsClient struct {
	backend  *ethclient.Client
	c        *rpc.Client
	gasLimit uint64
	tpsc     tps.Ctrl
}

func newClient(c *rpc.Client, tpsc tps.Ctrl, gasLimit uint64) *tpsClient {
	return &tpsClient{
		backend:  ethclient.NewClient(c),
		c:        c,
		gasLimit: gasLimit,
		tpsc:     tpsc,
	}
}

func (c *tpsClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.BalanceAt(ctx, account, blockNumber)
}

func (c *tpsClient) TransactionCount(ctx context.Context, account common.Hash) (uint, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.TransactionCount(ctx, account)
}

func (c *tpsClient) BlockNumber(ctx context.Context) (uint64, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.BlockNumber(ctx)
}

func (c *tpsClient) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.BlockByNumber(ctx, number)
}

func (c *tpsClient) ChainID(ctx context.Context) (*big.Int, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.ChainID(ctx)
}

func (c *tpsClient) Close() {
	c.backend.Close()
}

func (c *tpsClient) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.CodeAt(ctx, account, blockNumber)
}

func (c *tpsClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.CallContract(ctx, msg, blockNumber)
}

func (c *tpsClient) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.FilterLogs(ctx, q)
}

func (c *tpsClient) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.NonceAt(ctx, account, blockNumber)
}

func (c *tpsClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.PendingNonceAt(ctx, account)
}

func (c *tpsClient) SendRawTransaction(ctx context.Context, signedTxData string) error {
	return c.c.CallContext(ctx, nil, "eth_sendRawTransaction", signedTxData)
}

func (c *tpsClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.SendTransaction(ctx, tx)
}

func (c *tpsClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.SuggestGasPrice(ctx)
}

func (c *tpsClient) SuggestGasLimit(ctx context.Context) uint64 {
	return c.gasLimit
}

func (c *tpsClient) TransactionsPerSecond(ctx context.Context) int64 {
	return int64(c.tpsc.Size())
}

func (c *tpsClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.TransactionReceipt(ctx, txHash)
}

func (c *tpsClient) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	var release = c.tpsc.Acquire()
	defer release()
	return c.backend.TransactionByHash(ctx, hash)
}

func (c *tpsClient) WaitMined(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	var release = c.tpsc.Acquire()
	defer release()
	return bind.WaitMined(ctx, c.backend, tx)
}
