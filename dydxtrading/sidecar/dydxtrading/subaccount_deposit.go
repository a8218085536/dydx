//go:build cgo

package dydxtrading

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sendingtypes "github.com/dydxprotocol/v4-chain/protocol/x/sending/types"
	satypes "github.com/dydxprotocol/v4-chain/protocol/x/subaccounts/types"
)

var ErrSubaccountDepositorNotConfigured = errors.New("dydx subaccount depositor is not configured")

type SubaccountDepositorConfig struct {
	ChainID          string
	SystemAddress    string
	SystemPrivateKey cryptotypes.PrivKey
	Signer           Signer
	AccountProvider  AccountProvider
	Broadcaster      Broadcaster
	BroadcastMode    string
	GasLimit         uint64
	FeeAmount        sdk.Coins
}

type SubaccountDepositor struct {
	cfg SubaccountDepositorConfig
}

func NewSubaccountDepositor(cfg SubaccountDepositorConfig) *SubaccountDepositor {
	cfg.GasLimit = defaultTxGasLimit(cfg.GasLimit)
	cfg.FeeAmount = defaultTxFeeAmount(cfg.FeeAmount)
	return &SubaccountDepositor{cfg: cfg}
}

func (d *SubaccountDepositor) DepositToSubaccount(ctx context.Context, req SubaccountDepositRequest) (SubaccountDepositResult, error) {
	if d == nil {
		return SubaccountDepositResult{}, ErrSubaccountDepositorNotConfigured
	}
	if d.cfg.ChainID == "" || strings.TrimSpace(d.cfg.SystemAddress) == "" || d.cfg.SystemPrivateKey == nil {
		return SubaccountDepositResult{}, ErrSubaccountDepositorNotConfigured
	}
	if d.cfg.Signer == nil || d.cfg.AccountProvider == nil || d.cfg.Broadcaster == nil {
		return SubaccountDepositResult{}, ErrSubaccountDepositorNotConfigured
	}
	quantums, err := strconv.ParseUint(strings.TrimSpace(req.Quantums), 10, 64)
	if err != nil {
		return SubaccountDepositResult{}, fmt.Errorf("invalid dYdX subaccount deposit quantums: %w", err)
	}
	msg := sendingtypes.NewMsgDepositToSubaccount(
		d.cfg.SystemAddress,
		satypes.SubaccountId{
			Owner:  req.DYDXAccount,
			Number: req.SubaccountNumber,
		},
		req.AssetID,
		quantums,
	)
	if err := msg.ValidateBasic(); err != nil {
		return SubaccountDepositResult{}, err
	}
	account, err := d.cfg.AccountProvider.Account(ctx, d.cfg.SystemAddress)
	if err != nil {
		return SubaccountDepositResult{}, err
	}
	txBytes, err := d.cfg.Signer.Sign(ctx, SignRequest{
		ChainID:       d.cfg.ChainID,
		AccountNumber: account.AccountNumber,
		Sequence:      account.Sequence,
		SignerAddress: d.cfg.SystemAddress,
		PrivateKey:    d.cfg.SystemPrivateKey,
		Messages:      []sdk.Msg{msg},
		GasLimit:      d.cfg.GasLimit,
		FeeAmount:     d.cfg.FeeAmount,
	})
	if err != nil {
		releaseSequence(d.cfg.AccountProvider, d.cfg.SystemAddress, account.Sequence)
		return SubaccountDepositResult{}, err
	}
	result, err := d.cfg.Broadcaster.Broadcast(ctx, BroadcastRequest{
		TxBytes: txBytes,
		Mode:    firstNonEmpty(d.cfg.BroadcastMode, BroadcastModeSync),
	})
	if err != nil {
		if result.Code == BroadcastCodeTxAlreadyInMempool {
			return SubaccountDepositResult{
				TxHash:           result.TxHash,
				Status:           StatusBroadcasted,
				DYDXAccount:      req.DYDXAccount,
				SubaccountNumber: req.SubaccountNumber,
				AssetID:          req.AssetID,
				Quantums:         req.Quantums,
			}, nil
		}
		releaseSequence(d.cfg.AccountProvider, d.cfg.SystemAddress, account.Sequence)
		return SubaccountDepositResult{}, err
	}
	return SubaccountDepositResult{
		TxHash:           result.TxHash,
		Status:           StatusBroadcasted,
		DYDXAccount:      req.DYDXAccount,
		SubaccountNumber: req.SubaccountNumber,
		AssetID:          req.AssetID,
		Quantums:         req.Quantums,
	}, nil
}
