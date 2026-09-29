package tyche

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestEVMWalletBindingVerifierAcceptsMatchingNECOSignature(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	binding := WalletBinding{
		UserID:      "user-1",
		NECOAddress: address,
		DYDXAddress: "dydx1user",
		Nonce:       "nonce-1",
	}
	binding.Message = BuildWalletBindingMessage(binding)
	signature := signEVMMessageForTest(t, key, binding.Message)

	verifier := EVMWalletBindingVerifier{}
	err = verifier.VerifyNECO(context.Background(), binding, signature)

	if err != nil {
		t.Fatalf("VerifyNECO() error = %v", err)
	}
}

func TestEVMWalletBindingVerifierRejectsMismatchedNECOSignature(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	otherKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	binding := WalletBinding{
		UserID:      "user-1",
		NECOAddress: crypto.PubkeyToAddress(otherKey.PublicKey).Hex(),
		DYDXAddress: "dydx1user",
		Nonce:       "nonce-1",
	}
	binding.Message = BuildWalletBindingMessage(binding)
	signature := signEVMMessageForTest(t, key, binding.Message)

	verifier := EVMWalletBindingVerifier{}
	err = verifier.VerifyNECO(context.Background(), binding, signature)

	if err == nil {
		t.Fatal("expected error")
	}
}
