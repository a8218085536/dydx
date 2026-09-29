package tyche

import (
	"context"
	"errors"
	"ethgo/model/orders"
	"ethgo/tyche/gasprice"
	"ethgo/tyche/types"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Transactor struct {
	Address    common.Address
	MethodName string
	Args       interface{}
}

func (t *Tyche) Transact(ctx context.Context, id string, transactor Transactor) error {

	contract, ok := t.contracts[transactor.Address]
	if !ok {
		return fmt.Errorf("no contract with address: %v", transactor.Address.String())
	}

	method, ok := contract.Methods[transactor.MethodName]
	if !ok {
		return fmt.Errorf("no method with name: %v", transactor.MethodName)
	}

	inputData, err := types.Pack(method, transactor.Args)
	if err != nil {
		return err
	}

	gasPrice := gasprice.Get()
	if gasPrice == nil {
		return errors.New("try again later")
	}

	return orders.Pending(id, contract.Address.String(), hexutil.Encode(inputData))
}
