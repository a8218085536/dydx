//go:build cgo

package dydxtrading

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/dydxprotocol/v4-chain/protocol/app"
)

type DirectSigner struct {
	txConfig client.TxConfig
}

func NewDirectSigner(txConfig client.TxConfig) DirectSigner {
	if txConfig == nil {
		txConfig = app.GetEncodingConfig().TxConfig
	}
	return DirectSigner{txConfig: txConfig}
}

func (s DirectSigner) Sign(ctx context.Context, req SignRequest) ([]byte, error) {
	if req.PrivateKey == nil {
		return nil, fmt.Errorf("missing private key")
	}
	if req.SignerAddress == "" {
		return nil, fmt.Errorf("missing signer address")
	}
	if req.ChainID == "" {
		return nil, fmt.Errorf("missing chain id")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("missing messages")
	}

	txBuilder := s.txConfig.NewTxBuilder()
	txBuilder.SetGasLimit(defaultTxGasLimit(req.GasLimit))
	txBuilder.SetFeeAmount(defaultTxFeeAmount(req.FeeAmount))
	if err := txBuilder.SetMsgs(req.Messages...); err != nil {
		return nil, err
	}

	emptySig := signing.SignatureV2{
		PubKey: req.PrivateKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
		Sequence: req.Sequence,
	}
	if err := txBuilder.SetSignatures(emptySig); err != nil {
		return nil, err
	}

	signerData := xauthsigning.SignerData{
		Address:       req.SignerAddress,
		ChainID:       req.ChainID,
		AccountNumber: req.AccountNumber,
		Sequence:      req.Sequence,
		PubKey:        req.PrivateKey.PubKey(),
	}
	signature, err := tx.SignWithPrivKey(
		ctx,
		signing.SignMode_SIGN_MODE_DIRECT,
		signerData,
		txBuilder,
		req.PrivateKey,
		s.txConfig,
		req.Sequence,
	)
	if err != nil {
		return nil, err
	}
	if err := txBuilder.SetSignatures(signature); err != nil {
		return nil, err
	}
	return s.txConfig.TxEncoder()(txBuilder.GetTx())
}

func PrivateKeyFromHex(value string) (cryptotypes.PrivKey, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	keyBytes, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("decode private key hex: %w", err)
	}
	if len(keyBytes) != secp256k1.PrivKeySize {
		return nil, fmt.Errorf("invalid private key length %d", len(keyBytes))
	}
	return &secp256k1.PrivKey{Key: keyBytes}, nil
}
