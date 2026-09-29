//go:build cgo

package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

func TestBankGasTopUpperSendsGasWhenBalanceBelowMinimum(t *testing.T) {
	systemAddress := testAccount()
	targetAddress := testOtherAccount(7)
	signer := &recordingSigner{txBytes: []byte{4, 5, 6}}
	broadcaster := &recordingBroadcaster{txHash: "GASTX"}
	topUpper, err := NewBankGasTopUpper(BankGasTopUpperConfig{
		ChainID:          "dydx-local-1",
		Denom:            "adedx",
		MinBalance:       "1000",
		TopUpAmount:      "5000",
		SystemAddress:    systemAddress,
		SystemPrivateKey: fakePrivKey{id: "system-wallet"},
		Signer:           signer,
		AccountProvider: StaticAccountProvider{
			AccountNumber: 9,
			Sequence:      3,
		},
		Broadcaster:     broadcaster,
		BalanceProvider: fixedBalanceProvider{balance: sdkmath.NewInt(10)},
	})
	require.NoError(t, err)

	err = topUpper.EnsureGas(context.Background(), targetAddress)

	require.NoError(t, err)
	require.Equal(t, systemAddress, signer.req.SignerAddress)
	require.Equal(t, uint64(9), signer.req.AccountNumber)
	require.Equal(t, uint64(3), signer.req.Sequence)
	require.Equal(t, DefaultTxGasLimit, signer.req.GasLimit)
	require.Equal(t, defaultTxFeeAmount(nil), signer.req.FeeAmount)
	require.Len(t, signer.req.Messages, 1)
	require.Equal(t, []byte{4, 5, 6}, broadcaster.txBytes)
}

func TestBankGasTopUpperDoesNothingWhenBalanceIsEnough(t *testing.T) {
	broadcaster := &recordingBroadcaster{txHash: "GASTX"}
	topUpper, err := NewBankGasTopUpper(BankGasTopUpperConfig{
		ChainID:          "dydx-local-1",
		Denom:            "adedx",
		MinBalance:       "1000",
		TopUpAmount:      "5000",
		SystemAddress:    testAccount(),
		SystemPrivateKey: fakePrivKey{id: "system-wallet"},
		Signer:           &recordingSigner{txBytes: []byte{4, 5, 6}},
		AccountProvider: StaticAccountProvider{
			AccountNumber: 9,
			Sequence:      3,
		},
		Broadcaster:     broadcaster,
		BalanceProvider: fixedBalanceProvider{balance: sdkmath.NewInt(1000)},
	})
	require.NoError(t, err)

	err = topUpper.EnsureGas(context.Background(), testOtherAccount(7))

	require.NoError(t, err)
	require.Empty(t, broadcaster.txBytes)
}

func TestBankGasTopUpperRejectsSystemWalletTarget(t *testing.T) {
	systemAddress := testAccount()
	topUpper, err := NewBankGasTopUpper(BankGasTopUpperConfig{
		ChainID:          "dydx-local-1",
		Denom:            "adedx",
		MinBalance:       "1000",
		TopUpAmount:      "5000",
		SystemAddress:    systemAddress,
		SystemPrivateKey: fakePrivKey{id: "system-wallet"},
		Signer:           &recordingSigner{txBytes: []byte{4, 5, 6}},
		AccountProvider: StaticAccountProvider{
			AccountNumber: 9,
			Sequence:      3,
		},
		Broadcaster:     &recordingBroadcaster{txHash: "GASTX"},
		BalanceProvider: fixedBalanceProvider{balance: sdkmath.NewInt(10)},
	})
	require.NoError(t, err)

	err = topUpper.EnsureGas(context.Background(), systemAddress)

	require.ErrorIs(t, err, ErrGasTopUpTargetSystemWallet)
}

func TestRESTBalanceProviderTreatsMissingDenomBalanceAsZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"not found"}`))
	}))
	defer server.Close()

	balance, err := NewRESTBalanceProvider(server.URL, server.Client()).Balance(context.Background(), testOtherAccount(7), "adedx")

	require.NoError(t, err)
	require.True(t, balance.IsZero(), "balance = %s, want zero", balance.String())
}

type fixedBalanceProvider struct {
	balance sdkmath.Int
	err     error
}

func (p fixedBalanceProvider) Balance(context.Context, string, string) (sdkmath.Int, error) {
	if p.err != nil {
		return sdkmath.Int{}, p.err
	}
	return p.balance, nil
}
