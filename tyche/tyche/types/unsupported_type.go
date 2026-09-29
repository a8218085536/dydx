package types

import (
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type unsupportedType struct {
	value reflect.Value
}

func newUnsupportedType(value reflect.Value) typeConverter {
	return &unsupportedType{value: value}
}

func (t *unsupportedType) Convert(ty abi.Type) (interface{}, error) {
	return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
}
