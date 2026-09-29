package tyche

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type EVMWalletBindingVerifier struct{}

func (EVMWalletBindingVerifier) VerifyNECO(ctx context.Context, binding WalletBinding, signature string) error {
	recovered, err := RecoverEVMMessageAddress(binding.Message, signature)
	if err != nil {
		return err
	}
	if !strings.EqualFold(recovered.Hex(), binding.NECOAddress) {
		return ErrWalletBindingSignatureInvalid
	}
	return nil
}

func (EVMWalletBindingVerifier) VerifyDYDX(context.Context, WalletBinding, string) error {
	return ErrWalletBindingVerifierNotConfigured
}

func RecoverEVMMessageAddress(message, signature string) (common.Address, error) {
	sig := strings.TrimPrefix(signature, "0x")
	rawSig, err := hex.DecodeString(sig)
	if err != nil {
		return common.Address{}, err
	}
	if len(rawSig) != 65 {
		return common.Address{}, ErrWalletBindingSignatureInvalid
	}
	if rawSig[64] >= 27 {
		rawSig[64] -= 27
	}

	hash := EVMMessageHash(message)
	pub, err := crypto.SigToPub(hash.Bytes(), rawSig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}

func EVMMessageHash(message string) common.Hash {
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	return crypto.Keccak256Hash([]byte(prefix), []byte(message))
}
