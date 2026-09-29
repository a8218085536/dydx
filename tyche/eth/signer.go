package eth

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Signer struct {
	key     *ecdsa.PrivateKey
	keyAddr common.Address
	signer  types.Signer
}

func NewSigner(key *ecdsa.PrivateKey, chainID *big.Int) (*Signer, error) {
	keyAddr := crypto.PubkeyToAddress(key.PublicKey)
	if chainID == nil {
		return nil, bind.ErrNoChainID
	}
	return &Signer{
		key:     key,
		keyAddr: keyAddr,
		signer:  types.LatestSignerForChainID(chainID),
	}, nil
}

func (s *Signer) Sign(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
	if address != s.keyAddr {
		return nil, bind.ErrNotAuthorized
	}
	signature, err := crypto.Sign(s.signer.Hash(tx).Bytes(), s.key)
	if err != nil {
		return nil, err
	}
	return tx.WithSignature(s.signer, signature)
}
