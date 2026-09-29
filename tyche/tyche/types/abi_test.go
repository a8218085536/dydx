package types

import (
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

func TestAll(t *testing.T) {
	TestAddressTy(t)
	TestIntTy(t)
	TestStringTy(t)
	TestBoolTy(t)
	TestBytesTy(t)
	TestFixedBytesTy(t)
	TestSliceTy(t)
	TestArrayTy(t)
	TestTupleTy(t)
}

func TestAddressTy(t *testing.T) {
	addressTy, _ := abi.NewType("address", "", nil)

	var values []interface{}
	// String
	values = append(values, "0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")
	// Address
	values = append(values, common.HexToAddress("0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5"))
	// Slice
	values = append(values, []byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})
	// Array
	values = append(values, [20]byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})

	{
		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], addressTy)
			if err != nil {
				panic(err)
			}
			t.Logf("addressBy: %v, %T", v, v)
		}
	}

}

func TestIntTy(t *testing.T) {
	// values = append(values, float32(2048.12))
	// values = append(values, float64(4096.34))

	{
		intTy, _ := abi.NewType("int", "", nil)
		var values []interface{}
		values = append(values, int(0))
		values = append(values, int8(8))
		values = append(values, int16(16))
		values = append(values, int32(32))
		values = append(values, int64(64))
		values = append(values, "1024")

		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], intTy)
			if err != nil {
				panic(err)
			}
			t.Logf("intBy: %v, %T", v, v)
		}
	}

	{
		uintTy, _ := abi.NewType("uint", "", nil)

		var values []interface{}
		values = append(values, uint(0))
		values = append(values, uint8(8))
		values = append(values, uint16(16))
		values = append(values, uint32(32))
		values = append(values, uint64(64))
		values = append(values, "1024")
		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], uintTy)
			if err != nil {
				panic(err)
			}
			t.Logf("uintBy: %v, %T", v, v)
		}
	}
}

func TestStringTy(t *testing.T) {
	stringTy, _ := abi.NewType("string", "", nil)

	var values []interface{}
	values = append(values, int(0))
	values = append(values, int8(8))
	values = append(values, int16(16))
	values = append(values, int32(32))
	values = append(values, int64(64))
	values = append(values, uint(0))
	values = append(values, uint8(8))
	values = append(values, uint16(16))
	values = append(values, uint32(32))
	values = append(values, uint64(64))
	values = append(values, false)
	values = append(values, "string 1024")
	values = append(values, []byte("bytes 2048"))
	values = append(values, [2]byte{119, 112})
	values = append(values, float32(2048.12))
	values = append(values, float64(4096.34))

	{
		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], stringTy)
			if err != nil {
				panic(err)
			}
			t.Logf("stringTy: %v, %T", v, v)
		}
	}
}

func TestBoolTy(t *testing.T) {
	boolTy, _ := abi.NewType("bool", "", nil)

	var values []interface{}
	values = append(values, int(0))
	values = append(values, int8(8))
	values = append(values, int16(16))
	values = append(values, int32(32))
	values = append(values, int64(64))
	values = append(values, uint(0))
	values = append(values, uint8(8))
	values = append(values, uint16(16))
	values = append(values, uint32(32))
	values = append(values, uint64(64))
	values = append(values, true)
	values = append(values, false)
	values = append(values, "true")
	values = append(values, "false")
	// values = append(values, 12.1)

	{
		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], boolTy)
			if err != nil {
				panic(err)
			}
			t.Logf("stringTy: %v, %T", v, v)
		}
	}
}

func TestBytesTy(t *testing.T) {
	bytesBy, _ := abi.NewType("bytes", "", nil)

	var values []interface{}
	// String
	values = append(values, "0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")
	values = append(values, "hello world")
	// Slice
	values = append(values, []byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})
	// Array
	values = append(values, [20]byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})

	{
		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], bytesBy)
			if err != nil {
				panic(err)
			}
			t.Logf("bytesBy: %v, %T", v, v)
		}
	}

}

func TestFixedBytesTy(t *testing.T) {
	bytes20By, _ := abi.NewType("bytes20", "", nil)

	var values []interface{}
	// String
	values = append(values, "0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")
	values = append(values, "12345678901234567890")
	// values = append(values, "hello world")
	// Slice
	values = append(values, []byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})
	// Array
	values = append(values, [20]byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})

	{
		for i := 0; i < len(values); i++ {
			v, err := convertType(values[i], bytes20By)
			if err != nil {
				panic(err)
			}
			t.Logf("bytesBy: %v, %T", v, v)
		}
	}

}

func TestSliceTy(t *testing.T) {
	bytes20SliceBy, _ := abi.NewType("bytes20[]", "", nil)

	var values []interface{}
	// String
	values = append(values, "0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")
	values = append(values, "12345678901234567890")
	// values = append(values, "hello world")
	// Slice
	values = append(values, []byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})
	// Array
	values = append(values, [20]byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})

	v, err := convertType(values, bytes20SliceBy)
	if err != nil {
		panic(err)
	}
	t.Logf("bytesBy: %v, %T", v, v)
}

func TestArrayTy(t *testing.T) {
	bytes20ArrayBy, _ := abi.NewType("bytes20[4]", "", nil)

	var values []interface{}
	// String
	values = append(values, "0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")
	values = append(values, "12345678901234567890")
	// values = append(values, "hello world")
	// Slice
	values = append(values, []byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})
	// Array
	values = append(values, [20]byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197})

	v, err := convertType(values, bytes20ArrayBy)
	if err != nil {
		panic(err)
	}
	t.Logf("arrayTy: %v, %T", v, v)
}

func TestTupleTy(t *testing.T) {
	{
		tupleBy, _ := abi.NewType("tuple", "StructGo", []abi.ArgumentMarshaling{
			{Name: "Name", Type: "string"},
			{Name: "Age", Type: "uint256[]"},
		})

		type Tuple struct {
			Name string `json:"Name"`
			Age  []int  `json:"Age"`
		}

		var value = Tuple{
			Name: "hello",
			Age:  []int{100, 200},
		}

		v, err := convertType(value, tupleBy)
		if err != nil {
			panic(err)
		}
		t.Logf("tupleBy: %v, %T", v, v)

		var inputs abi.Arguments
		inputs = append(inputs, abi.Argument{Type: tupleBy})

		var args []interface{}
		args = append(args, v)
		t.Log(inputs.Pack(args...))
	}

	{
		tupleBy, _ := abi.NewType("tuple", "StructGoMap", []abi.ArgumentMarshaling{
			{Name: "Name", Type: "string"},
			{Name: "Age", Type: "uint256"},
		})

		var value = make(map[string]interface{})
		value["Name"] = "hello"
		value["Age"] = 100

		v, err := convertType(value, tupleBy)
		if err != nil {
			panic(err)
		}
		t.Logf("tupleBy: %v, %T", v, v)

		var inputs abi.Arguments
		inputs = append(inputs, abi.Argument{Type: tupleBy})

		var args []interface{}
		args = append(args, v)
		t.Log(inputs.Pack(args...))
	}

	{
		tupleBy, _ := abi.NewType("tuple", "StructGo", []abi.ArgumentMarshaling{
			{Name: "Name", Type: "string"},
			{Name: "Age", Type: "uint256"},
			{Name: "Sex", Type: "uint256"},
		})

		var value []interface{}
		value = append(value, "hello")
		value = append(value, 100)
		value = append(value, 0)

		v, err := convertType(value, tupleBy)
		if err != nil {
			panic(err)
		}
		t.Logf("tupleBy: %v, %T", v, v)

		var inputs abi.Arguments
		inputs = append(inputs, abi.Argument{Type: tupleBy})

		var args []interface{}
		args = append(args, v)
		t.Log(inputs.Pack(args...))

	}
}
