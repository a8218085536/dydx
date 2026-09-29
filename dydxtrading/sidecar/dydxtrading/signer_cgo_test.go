//go:build cgo

package dydxtrading

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/dydxprotocol/v4-chain/protocol/app"
	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"
	"github.com/stretchr/testify/require"
)

func TestSignerBuildsDecodableSignedTxBytes(t *testing.T) {
	priv := secp256k1.GenPrivKey()
	account := sdk.AccAddress(priv.PubKey().Address()).String()
	req := testPlaceOrderRequest(account)
	msg, meta, err := BuildMessage(req)
	require.NoError(t, err)

	signer := NewDirectSigner(app.GetEncodingConfig().TxConfig)
	txBytes, err := signer.Sign(context.Background(), SignRequest{
		ChainID:       meta.ChainID,
		AccountNumber: 11,
		Sequence:      3,
		SignerAddress: account,
		PrivateKey:    priv,
		Messages:      []sdk.Msg{msg},
	})

	require.NoError(t, err)
	require.NotEmpty(t, txBytes)
	decodedTx, err := app.GetEncodingConfig().TxConfig.TxDecoder()(txBytes)
	require.NoError(t, err)
	feeTx, ok := decodedTx.(sdk.FeeTx)
	require.True(t, ok)
	require.Equal(t, DefaultTxGasLimit, feeTx.GetGas())
	require.Equal(t, defaultTxFeeAmount(nil), feeTx.GetFee())
	require.Len(t, decodedTx.GetMsgs(), 1)
	_, ok = decodedTx.GetMsgs()[0].(*clobtypes.MsgPlaceOrder)
	require.True(t, ok)
}
