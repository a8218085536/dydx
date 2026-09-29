package types

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type uintType struct {
	value reflect.Value
}

func newUintType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &uintType{value: value}
	default:
		panic("is not reflect.Uint(x)")
	}
}

func (t *uintType) IntTy(ty abi.Type) (interface{}, error) {
	return convertNumeric(ty, new(big.Int).SetUint64(t.value.Uint()))
}

func (t *uintType) BoolTy(ty abi.Type) (interface{}, error) {
	return t.value.Uint() != 0, nil
}

func (t *uintType) StringTy(ty abi.Type) (interface{}, error) {
	return strconv.FormatUint(t.value.Uint(), 10), nil
}

func (t *uintType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.IntTy, abi.UintTy:
		return t.IntTy(ty)
	case abi.BoolTy:
		return t.BoolTy(ty)
	case abi.StringTy:
		return t.StringTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
}
