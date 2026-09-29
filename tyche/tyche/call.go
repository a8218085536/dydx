package tyche

import (
	"context"
	"ethgo/tyche/types"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type Caller struct {
	Address    common.Address
	MethodName string
	Args       interface{}
}

func (t *Tyche) Call(ctx context.Context, caller Caller) ([]interface{}, error) {
	contract, ok := t.contracts[caller.Address]
	if !ok {
		contract, ok = t.contracts[common.HexToAddress("0x7814471A547ad51471d7fE29e5579CC21cA8B2cE")]
		if !ok {
			return nil, fmt.Errorf("no contract with address: %v", caller.Address.String())
		}
	}
	contract.Address = caller.Address
	method, ok := contract.Methods[caller.MethodName]
	if !ok {
		return nil, fmt.Errorf("no method with name: %v", caller.MethodName)
	}

	inputData, err := types.Pack(method, caller.Args)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		From: t.account,
		To:   (*common.Address)(&contract.Address),
		Data: inputData,
	}

	output, err := t.backend.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	if len(output) == 0 {
		// Make sure we have a contract to operate on, and bail out otherwise.
		if code, err := t.backend.CodeAt(ctx, t.account, nil); err != nil {
			return nil, err
		} else if len(code) == 0 {
			return nil, bind.ErrNoCode
		}
	}

	return contract.Unpack(caller.MethodName, output)
}
