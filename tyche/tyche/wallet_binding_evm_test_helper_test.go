package tyche

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func signEVMMessageForTest(t *testing.T, key *ecdsa.PrivateKey, message string) string {
	t.Helper()

	signature, err := crypto.Sign(EVMMessageHash(message).Bytes(), key)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	signature[64] += 27
	return hexutil.Encode(signature)
}
