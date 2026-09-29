package types

import (
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type sliceType struct {
	value reflect.Value
}

func newSliceType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.Slice:
		return &sliceType{value: value}
	default:
		panic("is not reflect.Slice")
	}
}

func (t *sliceType) StringTy(ty abi.Type) (interface{}, error) {
	bytes, err := t.BytesTy(ty)
	if err != nil {
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
	return string(bytes.([]byte)), nil
}

func (t *sliceType) SliceTy(ty abi.Type) (interface{}, error) {
	valLen := t.value.Len()
	destVal := reflect.MakeSlice(ty.GetType(), valLen, valLen)
	for i := 0; i < destVal.Len(); i++ {
		value, err := convertType(t.value.Index(i).Interface(), *ty.Elem)
		if err != nil {
			return nil, err
		}
		destVal.Index(i).Set(reflect.ValueOf(value))
	}
	return destVal.Interface(), nil
}

func (t *sliceType) ArrayTy(ty abi.Type) (interface{}, error) {
	valLen := t.value.Len()
	if valLen != ty.Size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", ty.Size, valLen)
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

func (t *sliceType) TupleTy(ty abi.Type) (interface{}, error) {
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

func (t *sliceType) AddressTy(ty abi.Type) (interface{}, error) {
	if t.value.Type().Elem().Kind() != reflect.Uint8 {
		return common.Address{}, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}

	if t.value.Len() != ty.Size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", ty.Size, t.value.Len())
	}
	return common.BytesToAddress(t.value.Bytes()), nil
}

func (t *sliceType) FixedBytesTy(ty abi.Type) (interface{}, error) {
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

func (t *sliceType) BytesTy(ty abi.Type) (interface{}, error) {
	if t.value.Type().Elem().Kind() != reflect.Uint8 {
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
	return t.value.Bytes(), nil
}

func (t *sliceType) Convert(ty abi.Type) (interface{}, error) {
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
