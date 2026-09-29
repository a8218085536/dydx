package types

import "github.com/ethereum/go-ethereum/accounts/abi"

func NewMethod(methodName string, inputType []string, outputType []string) (abi.Method, error) {
	inputs, err := parseArguments(inputType)
	if err != nil {
		return abi.Method{}, err
	}

	outputs, err := parseArguments(outputType)
	if err != nil {
		return abi.Method{}, err
	}
	return abi.NewMethod(methodName, methodName, abi.Function, "", false, false, inputs, outputs), nil
}
