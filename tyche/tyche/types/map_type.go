package types

import (
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type mapType struct {
	value reflect.Value
}

func newMapType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Map:
		return &mapType{value: value}
	default:
		panic("is not reflect.Map")
	}
}

func (t *mapType) TupleTy(ty abi.Type) (interface{}, error) {
	destVal := reflect.New(ty.TupleType).Elem()
	for i, fieldName := range ty.TupleRawNames {
		value, err := convertType(t.value.MapIndex(reflect.ValueOf(fieldName)).Interface(), *ty.TupleElems[i])
		if err != nil {
			return nil, err
		}

		destVal.FieldByIndex([]int{i}).Set(reflect.ValueOf(value))
	}

	return destVal.Interface(), nil
}

func (t *mapType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.TupleTy:
		return t.TupleTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
}
