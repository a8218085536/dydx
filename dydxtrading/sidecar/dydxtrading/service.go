//go:build cgo

package dydxtrading

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"
)

var (
	ErrProxyWalletNotFound                = errors.New("dydx proxy wallet not found")
	ErrSystemWalletTradingAccount         = errors.New("dydx system wallet cannot be used as trading account")
	ErrTradingAccountMessageOwnerMismatch = errors.New("dydx trading account does not match message owner")
)

type ServiceConfig struct {
	Signer             Signer
	AccountProvider    AccountProvider
	Broadcaster        Broadcaster
	PrivateKeyProvider ProxyWalletPrivateKeyProvider
	SystemAddress      string
	GasTopUpper        GasTopUpper
	GasLimit           uint64
	FeeAmount          sdk.Coins
}

type Service struct {
	signer             Signer
	accountProvider    AccountProvider
	broadcaster        Broadcaster
	privateKeyProvider ProxyWalletPrivateKeyProvider
	systemAddress      string
	gasTopUpper        GasTopUpper
	gasLimit           uint64
	feeAmount          sdk.Coins
	accountLocksMu     sync.Mutex
	accountLocks       map[string]*sync.Mutex
}

type ProxyWalletPrivateKeyProvider interface {
	PrivateKeyForAddress(context.Context, string) (cryptotypes.PrivKey, error)
}

type GasTopUpper interface {
	EnsureGas(context.Context, string) error
}

type sequenceReleaser interface {
	Release(address string, sequence uint64)
}

func NewService(cfg ServiceConfig) *Service {
	return &Service{
		signer:             cfg.Signer,
		accountProvider:    cfg.AccountProvider,
		broadcaster:        cfg.Broadcaster,
		privateKeyProvider: cfg.PrivateKeyProvider,
		systemAddress:      cfg.SystemAddress,
		gasTopUpper:        cfg.GasTopUpper,
		gasLimit:           defaultTxGasLimit(cfg.GasLimit),
		feeAmount:          defaultTxFeeAmount(cfg.FeeAmount),
		accountLocks:       make(map[string]*sync.Mutex),
	}
}

func (s *Service) Submit(ctx context.Context, req TycheTradingRequest) (TycheTradingResponse, error) {
	if s.signer == nil {
		return TycheTradingResponse{}, fmt.Errorf("missing signer")
	}
	if s.accountProvider == nil {
		return TycheTradingResponse{}, fmt.Errorf("missing account provider")
	}
	if s.broadcaster == nil {
		return TycheTradingResponse{}, fmt.Errorf("missing broadcaster")
	}
	if s.privateKeyProvider == nil {
		return TycheTradingResponse{}, fmt.Errorf("missing proxy wallet private key provider")
	}

	msg, meta, err := BuildMessage(req)
	if err != nil {
		return TycheTradingResponse{}, err
	}
	if sameAddress(meta.Account, s.systemAddress) {
		return TycheTradingResponse{}, ErrSystemWalletTradingAccount
	}
	if owner := MessageOwner(msg); owner != "" && !sameAddress(meta.Account, owner) {
		return TycheTradingResponse{}, ErrTradingAccountMessageOwnerMismatch
	}
	privateKey, err := s.privateKeyProvider.PrivateKeyForAddress(ctx, meta.Account)
	if err != nil {
		return TycheTradingResponse{}, err
	}
	if privateKey == nil {
		return TycheTradingResponse{}, ErrProxyWalletNotFound
	}
	if s.gasTopUpper != nil {
		if err := s.gasTopUpper.EnsureGas(ctx, meta.Account); err != nil {
			return TycheTradingResponse{}, err
		}
	}
	unlockAccount := s.lockAccount(meta.Account)
	defer unlockAccount()
	account, err := s.accountProvider.Account(ctx, meta.Account)
	if err != nil {
		return TycheTradingResponse{}, err
	}
	sequenceConsumed := messageConsumesSequence(msg)
	txBytes, err := s.signer.Sign(ctx, SignRequest{
		ChainID:       meta.ChainID,
		AccountNumber: account.AccountNumber,
		Sequence:      account.Sequence,
		SignerAddress: meta.Account,
		PrivateKey:    privateKey,
		Messages:      []sdk.Msg{msg},
		GasLimit:      firstNonZeroUint64(meta.GasLimit, s.gasLimit),
		FeeAmount:     firstNonEmptyCoins(meta.FeeAmount, s.feeAmount),
	})
	if err != nil {
		s.releaseSequence(meta.Account, account.Sequence)
		return TycheTradingResponse{}, err
	}
	result, err := s.broadcaster.Broadcast(ctx, BroadcastRequest{
		TxBytes: txBytes,
		Mode:    meta.BroadcastMode,
	})
	if err != nil {
		if result.Code == BroadcastCodeTxAlreadyInMempool {
			if !sequenceConsumed {
				s.releaseSequence(meta.Account, account.Sequence)
			}
			return TycheTradingResponse{
				Code: 200,
				Msg:  "succ",
				Data: TycheTradingData{
					TxHash: result.TxHash,
					Status: StatusBroadcasted,
				},
			}, nil
		}
		s.releaseSequence(meta.Account, account.Sequence)
		return TycheTradingResponse{}, err
	}
	if !sequenceConsumed {
		s.releaseSequence(meta.Account, account.Sequence)
	}
	return TycheTradingResponse{
		Code: 200,
		Msg:  "succ",
		Data: TycheTradingData{
			TxHash: result.TxHash,
			Status: StatusBroadcasted,
		},
	}, nil
}

func (s *Service) lockAccount(address string) func() {
	normalized := strings.ToLower(strings.TrimSpace(address))
	s.accountLocksMu.Lock()
	lock := s.accountLocks[normalized]
	if lock == nil {
		lock = &sync.Mutex{}
		s.accountLocks[normalized] = lock
	}
	s.accountLocksMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (s *Service) releaseSequence(address string, sequence uint64) {
	releaser, ok := s.accountProvider.(sequenceReleaser)
	if !ok {
		return
	}
	releaser.Release(address, sequence)
}

func messageConsumesSequence(msg sdk.Msg) bool {
	switch typed := msg.(type) {
	case *clobtypes.MsgPlaceOrder:
		return typed.Order.OrderId.IsStatefulOrder()
	case *clobtypes.MsgCancelOrder:
		return typed.OrderId.IsStatefulOrder()
	default:
		return true
	}
}

func sameAddress(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && strings.EqualFold(left, right)
}
