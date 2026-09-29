package types

import (
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

func Pack(method abi.Method, argv interface{}) ([]byte, error) {
	var vals []interface{}

	var inputs = reflect.ValueOf(argv)
	switch inputs.Kind() {
	case reflect.Invalid:
	case reflect.Map:
		for _, arg := range method.Inputs {
			field := inputs.MapIndex(reflect.ValueOf(arg.Name))
			if field.Kind() == reflect.Invalid {
				return nil, fmt.Errorf("argument '%v' is missing, bad input for contract method", arg.Name)
			}
			val, err := convertType(field.Interface(), arg.Type)
			if err != nil {
				return nil, fmt.Errorf("while converting argument '%v' from %T to %v %w", arg.Name, val, arg.Type, err)
			}

			vals = append(vals, val)
		}
	case reflect.Slice:
		if inputs.Len() != len(method.Inputs) {
			return nil, fmt.Errorf("incorrect length: expected %v, got %v, bad input for contract method", len(method.Inputs), inputs.Len())
		}
		for index, arg := range method.Inputs {
			val, err := convertType(inputs.Index(index).Interface(), arg.Type)
			if err != nil {
				return nil, fmt.Errorf("while converting argument '%v' from %T to %v %w", arg.Name, val, arg.Type, err)
			}

			vals = append(vals, val)
		}
	default:
		return nil, fmt.Errorf("unknow input type: %T, bad input for contract method", argv)
	}

	var res, err = method.Inputs.Pack(vals...)
	if err != nil {
		return nil, err
	}

	return append(method.ID, res...), nil
}
