//go:build cgo

package dydxtrading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

var (
	ErrGasTopUpNotConfigured      = errors.New("dydx gas top-up is not configured")
	ErrGasTopUpTargetSystemWallet = errors.New("dydx gas top-up target cannot be the system wallet")
)

type BalanceProvider interface {
	Balance(context.Context, string, string) (sdkmath.Int, error)
}

type BankGasTopUpperConfig struct {
	ChainID          string
	Denom            string
	MinBalance       string
	TopUpAmount      string
	SystemAddress    string
	SystemPrivateKey cryptotypes.PrivKey
	Signer           Signer
	AccountProvider  AccountProvider
	Broadcaster      Broadcaster
	BalanceProvider  BalanceProvider
	BroadcastMode    string
	GasLimit         uint64
	FeeAmount        sdk.Coins
	ConfirmAttempts  int
	ConfirmInterval  time.Duration
}

type BankGasTopUpper struct {
	conf        BankGasTopUpperConfig
	minBalance  sdkmath.Int
	topUpAmount sdkmath.Int
}

func NewBankGasTopUpper(conf BankGasTopUpperConfig) (*BankGasTopUpper, error) {
	if conf.ChainID == "" || conf.Denom == "" || conf.SystemAddress == "" || conf.SystemPrivateKey == nil || conf.Signer == nil || conf.AccountProvider == nil || conf.Broadcaster == nil || conf.BalanceProvider == nil {
		return nil, ErrGasTopUpNotConfigured
	}
	minBalance, err := parseGasInt(conf.MinBalance, "min balance")
	if err != nil {
		return nil, err
	}
	topUpAmount, err := parseGasInt(conf.TopUpAmount, "top-up amount")
	if err != nil {
		return nil, err
	}
	if !topUpAmount.IsPositive() {
		return nil, fmt.Errorf("top-up amount must be positive")
	}
	if conf.BroadcastMode == "" {
		conf.BroadcastMode = BroadcastModeSync
	}
	conf.GasLimit = defaultTxGasLimit(conf.GasLimit)
	conf.FeeAmount = defaultTxFeeAmount(conf.FeeAmount)
	if conf.ConfirmInterval == 0 {
		conf.ConfirmInterval = 500 * time.Millisecond
	}
	return &BankGasTopUpper{
		conf:        conf,
		minBalance:  minBalance,
		topUpAmount: topUpAmount,
	}, nil
}

func (g *BankGasTopUpper) EnsureGas(ctx context.Context, targetAddress string) error {
	if g == nil {
		return nil
	}
	targetAddress = strings.TrimSpace(targetAddress)
	if targetAddress == "" {
		return ErrProxyWalletAddressRequired
	}
	if sameAddress(targetAddress, g.conf.SystemAddress) {
		return ErrGasTopUpTargetSystemWallet
	}
	balance, err := g.conf.BalanceProvider.Balance(ctx, targetAddress, g.conf.Denom)
	if err != nil {
		return err
	}
	if balance.GTE(g.minBalance) {
		return nil
	}
	from, err := sdk.AccAddressFromBech32(g.conf.SystemAddress)
	if err != nil {
		return err
	}
	to, err := sdk.AccAddressFromBech32(targetAddress)
	if err != nil {
		return err
	}
	msg := banktypes.NewMsgSend(from, to, sdk.NewCoins(sdk.NewCoin(g.conf.Denom, g.topUpAmount)))
	account, err := g.conf.AccountProvider.Account(ctx, g.conf.SystemAddress)
	if err != nil {
		return err
	}
	txBytes, err := g.conf.Signer.Sign(ctx, SignRequest{
		ChainID:       g.conf.ChainID,
		AccountNumber: account.AccountNumber,
		Sequence:      account.Sequence,
		SignerAddress: g.conf.SystemAddress,
		PrivateKey:    g.conf.SystemPrivateKey,
		Messages:      []sdk.Msg{msg},
		GasLimit:      g.conf.GasLimit,
		FeeAmount:     g.conf.FeeAmount,
	})
	if err != nil {
		releaseSequence(g.conf.AccountProvider, g.conf.SystemAddress, account.Sequence)
		return err
	}
	if _, err := g.conf.Broadcaster.Broadcast(ctx, BroadcastRequest{
		TxBytes: txBytes,
		Mode:    g.conf.BroadcastMode,
	}); err != nil {
		releaseSequence(g.conf.AccountProvider, g.conf.SystemAddress, account.Sequence)
		return err
	}
	if g.conf.ConfirmAttempts > 0 {
		return g.waitForGas(ctx, targetAddress)
	}
	return nil
}

func (g *BankGasTopUpper) waitForGas(ctx context.Context, targetAddress string) error {
	for attempt := 0; attempt < g.conf.ConfirmAttempts; attempt++ {
		balance, err := g.conf.BalanceProvider.Balance(ctx, targetAddress, g.conf.Denom)
		if err == nil && balance.GTE(g.minBalance) {
			return nil
		}
		if attempt == g.conf.ConfirmAttempts-1 {
			if err != nil {
				return err
			}
			return fmt.Errorf("gas top-up not confirmed for %s", targetAddress)
		}
		timer := time.NewTimer(g.conf.ConfirmInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

type RESTBalanceProvider struct {
	baseURL string
	client  *http.Client
}

func NewRESTBalanceProvider(baseURL string, client *http.Client) RESTBalanceProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return RESTBalanceProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (p RESTBalanceProvider) Balance(ctx context.Context, address, denom string) (sdkmath.Int, error) {
	if p.baseURL == "" {
		return sdkmath.Int{}, fmt.Errorf("missing bank REST base URL")
	}
	endpoint := p.baseURL + "/cosmos/bank/v1beta1/balances/" + url.PathEscape(address) + "/by_denom?denom=" + url.QueryEscape(denom)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return sdkmath.Int{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return sdkmath.Int{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return sdkmath.ZeroInt(), nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return sdkmath.Int{}, fmt.Errorf("bank balance query returned http %d", resp.StatusCode)
	}
	var body struct {
		Balance struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return sdkmath.Int{}, err
	}
	amount := body.Balance.Amount
	if amount == "" {
		amount = "0"
	}
	parsed, ok := sdkmath.NewIntFromString(amount)
	if !ok || parsed.IsNegative() {
		return sdkmath.Int{}, fmt.Errorf("invalid bank balance amount %q", amount)
	}
	return parsed, nil
}

func parseGasInt(value, name string) (sdkmath.Int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "0"
	}
	parsed, ok := sdkmath.NewIntFromString(value)
	if !ok || parsed.IsNegative() {
		return sdkmath.Int{}, fmt.Errorf("invalid gas %s %q", name, value)
	}
	return parsed, nil
}

func releaseSequence(provider AccountProvider, address string, sequence uint64) {
	releaser, ok := provider.(sequenceReleaser)
	if !ok {
		return
	}
	releaser.Release(address, sequence)
}
