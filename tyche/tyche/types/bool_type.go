package types

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type boolType struct {
	value reflect.Value
}

func newBoolType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Bool:
		return &boolType{value: value}
	default:
		panic("is not reflect.Bool")
	}
}

func (t *boolType) BoolTy(ty abi.Type) (interface{}, error) {
	return t.value.Bool(), nil
}

func (t *boolType) StringTy(ty abi.Type) (interface{}, error) {
	return strconv.FormatBool(t.value.Bool()), nil
}

func (t *boolType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.BoolTy:
		return t.BoolTy(ty)
	case abi.StringTy:
		return t.StringTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
}
