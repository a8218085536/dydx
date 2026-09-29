package types

import (
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type arrayType struct {
	value reflect.Value
}

func newArrayType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Array:
		return &arrayType{value: value}
	default:
		panic("is not reflect.Array")
	}
}

func (t *arrayType) StringTy(ty abi.Type) (interface{}, error) {
	value, err := t.BytesTy(ty)
	if err != nil {
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
	return string(value.([]byte)), nil
}

func (t *arrayType) SliceTy(ty abi.Type) (interface{}, error) {
	destVal := reflect.New(ty.GetType()).Elem()
	for i := 0; i < destVal.Len(); i++ {
		value, err := convertType(t.value.Index(i).Interface(), *ty.Elem)
		if err != nil {
			return nil, err
		}
		destVal.Index(i).Set(reflect.ValueOf(value))
	}
	return destVal.Interface(), nil
}

func (t *arrayType) ArrayTy(ty abi.Type) (interface{}, error) {
	if t.value.Len() != ty.Size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", ty.Size, t.value.Len())
	}

	destVal := reflect.New(ty.GetType()).Elem()
	for i := 0; i < destVal.Len(); i++ {
		value, err := convertType(t.value.Index(i).Interface(), *ty.Elem)
		if err != nil {
			return nil, err
		}
		destVal.Index(i).Set(reflect.ValueOf(value))
	}
	return destVal.Interface(), nil

}

func (t *arrayType) TupleTy(ty abi.Type) (interface{}, error) {
	size := len(ty.TupleElems)
	if t.value.Len() != size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", size, t.value.Len())
	}

	destVal := reflect.New(ty.TupleType).Elem()
	for i := range ty.TupleRawNames {
		value, err := convertType(t.value.Index(i).Interface(), *ty.TupleElems[i])
		if err != nil {
			return nil, err
		}
		destVal.FieldByIndex([]int{i}).Set(reflect.ValueOf(value))
	}

	return destVal.Interface(), nil
}

func (t *arrayType) AddressTy(ty abi.Type) (interface{}, error) {
	if t.value.Type().Elem().Kind() != reflect.Uint8 {
		return common.Address{}, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}

	valLen := t.value.Len()
	if valLen != ty.Size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", ty.Size, valLen)
	}

	destVal := reflect.MakeSlice(reflect.TypeOf([]byte(nil)), valLen, valLen)
	reflect.Copy(destVal, t.value)
	return common.BytesToAddress(destVal.Bytes()), nil
}

func (t *arrayType) FixedBytesTy(ty abi.Type) (interface{}, error) {
	if t.value.Type().Elem().Kind() != reflect.Uint8 {
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}

	if t.value.Len() != ty.Size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", ty.Size, t.value.Len())
	}

	destVal := reflect.New(ty.GetType()).Elem()
	reflect.Copy(destVal, t.value)
	return destVal.Interface(), nil
}

func (t *arrayType) BytesTy(ty abi.Type) (interface{}, error) {
	if t.value.Type().Elem().Kind() != reflect.Uint8 {
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}

	destVal := make([]byte, t.value.Len())
	reflect.Copy(reflect.ValueOf(destVal), t.value)
	return destVal, nil
}

func (t *arrayType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.StringTy:
		return t.StringTy(ty)
	case abi.SliceTy:
		return t.SliceTy(ty)
	case abi.ArrayTy:
		return t.ArrayTy(ty)
	case abi.TupleTy:
		return t.TupleTy(ty)
	case abi.AddressTy:
		return t.AddressTy(ty)
	case abi.FixedBytesTy:
		return t.FixedBytesTy(ty)
	case abi.BytesTy:
		return t.BytesTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
}
