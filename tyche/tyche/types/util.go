package types

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

var (
	// typeRegex parses the abi sub types
	typeRegex = regexp.MustCompile("([a-zA-Z]+)(([0-9]+)(x([0-9]+))?)?")
)

func parseArguments(argsType []string) (abi.Arguments, error) {
	params := typeRegex.ReplaceAllStringFunc(strings.Join(argsType, ","), func(matched string) string {
		switch matched {
		case "int":
			return "int256"
		case "uint":
			return "uint256"
		default:
			return matched
		}
	})

	// canonical parameter expression
	expression := fmt.Sprintf("$(%v)", params)

	converted, err := abi.ParseSelector(expression)
	if err != nil {
		return nil, err
	}

	var arguments abi.Arguments
	for _, arg := range converted.Inputs {
		cType, err := abi.NewType(arg.Type, arg.InternalType, arg.Components)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, abi.Argument{Type: cType, Indexed: false})
	}
	return arguments, nil
}

func convertNumeric(ty abi.Type, value *big.Int) (interface{}, error) {
	switch ty.T {
	case abi.IntTy:
		return convertInt(ty.Size, value)
	case abi.UintTy:
		return convertUint(ty.Size, value)
	default:
		panic(ty.T)
	}
}

func convertInt(bitSize int, value *big.Int) (interface{}, error) {
	switch bitSize {
	case 8:
		return static_cast[int64, int8](value.Int64())
	case 16:
		return static_cast[int64, int16](value.Int64())
	case 32:
		return static_cast[int64, int32](value.Int64())
	case 64:
		return value.Int64(), nil
	default:
		return value, nil
	}
}

func convertUint(bitSize int, value *big.Int) (interface{}, error) {
	switch bitSize {
	case 8:
		return static_cast[uint64, uint8](value.Uint64())
	case 16:
		return static_cast[uint64, uint16](value.Uint64())
	case 32:
		return static_cast[uint64, uint32](value.Uint64())
	case 64:
		return value.Uint64(), nil
	default:
		return value, nil
	}
}

func static_cast[
	T int64 | uint64,
	R int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64](value T) (R, error) {
	if T(R(value)) == value {
		return R(value), nil
	}
	return R(0), fmt.Errorf("cannot convert %v to %T: overflowed", value, R(0))
}
