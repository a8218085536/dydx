package minter

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Account interface {
	Address() common.Address
	NewTransactorWithChainID(chainID *big.Int) (*bind.TransactOpts, error)
}

type account struct {
	address    common.Address
	privateKey *ecdsa.PrivateKey
}

func New(c *Config) (Account, error) {
	if !common.IsHexAddress(c.Account) {
		return nil, errors.New("invalid address")
	}
	address := common.HexToAddress(c.Account)

	pk, err := crypto.HexToECDSA(c.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &account{address: address, privateKey: pk}, nil
}

func (a *account) Address() common.Address {
	return a.address
}

func (a *account) NewTransactorWithChainID(chainID *big.Int) (*bind.TransactOpts, error) {
	return bind.NewKeyedTransactorWithChainID(a.privateKey, chainID)
}
