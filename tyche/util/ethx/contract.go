package ethx

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type Contract struct {
	*abi.ABI
	*bind.BoundContract

	Address common.Address
	Name    string
}

func NewContract(address common.Address, abiFile string) (*Contract, error) {
	parsed, err := loadABI(abiFile)
	if err != nil {
		return nil, err
	}

	var contract = bind.NewBoundContract(address, parsed, nil, nil, nil)
	var name = getFileName(abiFile)
	return &Contract{ABI: &parsed, BoundContract: contract, Address: address, Name: name}, nil
}

func loadABI(filename string) (abi.ABI, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return abi.ABI{}, err
	}

	parsed, err := abi.JSON(bytes.NewReader(data))
	if err != nil {
		return abi.ABI{}, err
	}

	return parsed, nil
}

func getFileName(path string) string {
	var _, filename = filepath.Split(path)
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}
