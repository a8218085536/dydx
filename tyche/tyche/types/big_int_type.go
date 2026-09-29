package types

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type bigIntType struct {
	value *big.Int
}

func newBigIntType(value *big.Int) typeConverter {
	return &bigIntType{value: value}
}

func (t *bigIntType) IntTy(ty abi.Type) (interface{}, error) {
	return convertNumeric(ty, t.value)
}

func (t *bigIntType) BoolTy(ty abi.Type) (interface{}, error) {
	return t.value.Int64() != 0, nil
}

func (t *bigIntType) StringTy(ty abi.Type) (interface{}, error) {
	return t.value.String(), nil
}

func (t *bigIntType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.IntTy, abi.UintTy:
		return t.IntTy(ty)
	case abi.BoolTy:
		return t.BoolTy(ty)
	case abi.StringTy:
		return t.StringTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %T to %v", t.value, ty.GetType())
	}
}
