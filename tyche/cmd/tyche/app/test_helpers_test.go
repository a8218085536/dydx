package app

import (
	"crypto/ecdsa"
	"testing"

	"ethgo/tyche"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func signEVMMessageForAppTest(t *testing.T, key *ecdsa.PrivateKey, message string) string {
	t.Helper()
	signature, err := crypto.Sign(tyche.EVMMessageHash(message).Bytes(), key)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	signature[64] += 27
	return hexutil.Encode(signature)
}
