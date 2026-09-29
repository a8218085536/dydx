package types

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type intType struct {
	value reflect.Value
}

func newIntType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &intType{value: value}
	default:
		panic("is not reflect.Int(x)")
	}
}

func (t *intType) IntTy(ty abi.Type) (interface{}, error) {
	return convertNumeric(ty, big.NewInt(t.value.Int()))
}

func (t *intType) BoolTy(ty abi.Type) (interface{}, error) {
	return t.value.Int() != 0, nil
}

func (t *intType) StringTy(ty abi.Type) (interface{}, error) {
	return strconv.FormatInt(t.value.Int(), 10), nil
}

func (t *intType) Convert(ty abi.Type) (interface{}, error) {
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
