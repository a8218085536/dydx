package types

import (
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type floatType struct {
	value reflect.Value
}

func newFloatType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		return &floatType{value: value}
	default:
		panic("is not reflect.Float")
	}
}

func (t *floatType) IntTy(ty abi.Type) (interface{}, error) {
	value := t.value.Float()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("cannot convert %v to %v: invalid float", value, ty.GetType())
	}

	i, _ := big.NewFloat(value).Int(nil)
	return convertNumeric(ty, i)
}

func (t *floatType) StringTy(ty abi.Type) (interface{}, error) {
	return big.NewFloat(t.value.Float()).String(), nil
}

func (t *floatType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.IntTy, abi.UintTy:
		return t.IntTy(ty)
	case abi.StringTy:
		return t.StringTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
}
