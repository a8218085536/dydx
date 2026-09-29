package types

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type stringType struct {
	value reflect.Value
}

func newStringType(value reflect.Value) typeConverter {
	switch value.Kind() {
	case reflect.String:
		return &stringType{value: value}
	default:
		panic("is not reflect.String")
	}
}

func (t *stringType) ArrayTy(ty abi.Type) (interface{}, error) {
	s := t.value.String()
	s = strings.Trim(s, "[")
	s = strings.Trim(s, "]")
	l := strings.Split(s, ",")
	data := make([]*big.Int, 0)
	for i := 0; i < len(l); i++ {
		v := l[i]
		fmt.Print(v)
		member, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())

		}
		data = append(data, big.NewInt(int64(member)))

	}

	return data, nil
}
func (t *stringType) IntTy(ty abi.Type) (interface{}, error) {
	i, ok := new(big.Int).SetString(t.value.String(), 10)
	if !ok {
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
	return convertNumeric(ty, i)
}

func (t *stringType) BoolTy(ty abi.Type) (interface{}, error) {
	return strconv.ParseBool(t.value.String())
}

func (t *stringType) StringTy(ty abi.Type) (interface{}, error) {
	return t.value.String(), nil
}

func (t *stringType) AddressTy(ty abi.Type) (interface{}, error) {
	str := t.value.String()
	if common.IsHexAddress(str) {
		return common.HexToAddress(str), nil
	}
	return common.Address{}, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
}

func (t *stringType) FixedBytesTy(ty abi.Type) (interface{}, error) {
	value, err := t.BytesTy(ty)
	if err != nil {
		return nil, err
	}

	valLen := len(value.([]byte))
	if valLen != ty.Size {
		return nil, fmt.Errorf("incorrect length: expected %v, got %v", ty.Size, valLen)
	}
	var destVal = reflect.New(ty.GetType()).Elem()
	reflect.Copy(destVal, reflect.ValueOf(value))
	return destVal.Interface(), nil
}

func (t *stringType) BytesTy(ty abi.Type) (interface{}, error) {
	str := t.value.String()
	if strings.HasPrefix(str, "0x") {
		return hexutil.Decode(str)
	}
	return []byte(str), nil
}

func (t *stringType) Convert(ty abi.Type) (interface{}, error) {
	switch ty.T {
	case abi.IntTy, abi.UintTy:
		return t.IntTy(ty)
	case abi.BoolTy:
		return t.BoolTy(ty)
	case abi.StringTy:
		return t.StringTy(ty)
	case abi.AddressTy:
		return t.AddressTy(ty)
	case abi.FixedBytesTy:
		return t.FixedBytesTy(ty)
	case abi.BytesTy:
		return t.BytesTy(ty)
	case abi.ArrayTy:
		return t.ArrayTy(ty)
	case abi.SliceTy:
		return t.ArrayTy(ty)
	default:
		return nil, fmt.Errorf("cannot convert %v to %v", t.value.Type(), ty.GetType())
	}
}
