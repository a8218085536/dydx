package types

import (
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type typeConverter interface {
	Convert(abi.Type) (interface{}, error)
}

func convertType(input interface{}, ty abi.Type) (interface{}, error) {
	value := reflect.ValueOf(input)
	if value.Type() == ty.GetType() {
		return input, nil
	}

	switch value.Kind() {
	case reflect.Bool:
		return newBoolType(value).Convert(ty)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntType(value).Convert(ty)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintType(value).Convert(ty)
	case reflect.Float32, reflect.Float64:
		return newFloatType(value).Convert(ty)
	case reflect.Array:
		return newArrayType(value).Convert(ty)
	case reflect.Map:
		return newMapType(value).Convert(ty)
	case reflect.Pointer:
		switch v := input.(type) {
		case *big.Int:
			return newBigIntType(v).Convert(ty)
		default:
			return newUnsupportedType(value).Convert(ty)
		}
	case reflect.Slice:
		return newSliceType(value).Convert(ty)
	case reflect.String:
		return newStringType(value).Convert(ty)
	case reflect.Struct:
		return newStructType(value).Convert(ty)
	default:
		return newUnsupportedType(value).Convert(ty)
	}
}
