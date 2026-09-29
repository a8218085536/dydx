package types

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestEncode(t *testing.T) {

	type testCase struct {
		Type  string
		Value interface{}
	}

	var cases = []testCase{
		// Bool
		{Type: "bool", Value: true},
		{Type: "bool", Value: int8(0)},
		{Type: "bool", Value: int16(1)},
		{Type: "bool", Value: int32(0)},
		{Type: "bool", Value: int64(1)},
		{Type: "bool", Value: uint8(0)},
		{Type: "bool", Value: uint16(1)},
		{Type: "bool", Value: uint32(0)},
		{Type: "bool", Value: uint64(1)},
		{Type: "bool", Value: "true"},
		{Type: "bool", Value: big.NewInt(123)},
		{Type: "bool[]", Value: []bool{true, false, true, true}},
		{Type: "bool[]", Value: []int8{0, 1, 2, 3}},
		{Type: "bool[2]", Value: []bool{true, false}},
		{Type: "bool[2][]", Value: [2][]bool{{true, true}, {false, false}}},
		{Type: "bool[2][2]", Value: [2][2]bool{{true, true}, {false, false}}},
		{Type: "bool[2]", Value: []uint16{1, 0}},
		{Type: "bool[2]", Value: []string{"false", "true"}},
		// Int
		{Type: "int", Value: math.MaxInt},
		{Type: "int[]", Value: []interface{}{math.MaxInt, math.MaxInt}},
		{Type: "int[2]", Value: []interface{}{math.MaxInt, math.MaxInt}},
		{Type: "int[2][]", Value: [2][]interface{}{{math.MaxInt16}, {math.MaxInt16, math.MaxInt8}}},
		{Type: "int8", Value: math.MaxInt8},
		{Type: "int16", Value: math.MaxInt16},
		{Type: "int32", Value: math.MaxInt32},
		{Type: "int32[]", Value: []interface{}{math.MaxInt16, math.MaxInt16}},
		{Type: "int64", Value: math.MaxInt64},
		{Type: "int64[]", Value: []interface{}{math.MaxInt64, "12"}},
		{Type: "int128", Value: big.NewInt(1234567)},
		{Type: "int256", Value: "1234567890000000000000000"},
		// Uint
		{Type: "uint", Value: 0x7fffffffffffffff},
		{Type: "uint[]", Value: []interface{}{math.MaxInt, math.MaxInt}},
		{Type: "uint8", Value: math.MaxUint8},
		{Type: "uint16", Value: math.MaxUint16},
		{Type: "uint32", Value: math.MaxUint32},
		{Type: "uint64", Value: math.MaxInt64},
		{Type: "uint128", Value: big.NewInt(1234567)},
		{Type: "uint256", Value: "987633333330000000000000000"},
		// Bytes
		{Type: "bytes", Value: "bytes"},
		{Type: "bytes", Value: common.HexToAddress("0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")},
		{Type: "bytes[]", Value: []interface{}{"bytes1", "bytes2", common.HexToAddress("0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")}},
		{Type: "bytes8", Value: "bytes678"},
		{Type: "bytes8", Value: []byte{45, 46, 47, 48, 49, 50, 51, 52}},
		{Type: "bytes8", Value: [8]byte{45, 46, 47, 48, 49, 50, 51, 52}},
		{Type: "bytes32", Value: "bytes1234567890-=123456789012345"},
		// String
		{Type: "string", Value: "string"},
		{Type: "string", Value: []byte("hello world")},
		{Type: "string", Value: [8]byte{45, 46, 47, 48, 49, 50, 51, 52}},
		{Type: "string", Value: int32(123)},
		{Type: "string", Value: true},
		{Type: "string", Value: 123.001},
		{Type: "string", Value: common.HexToAddress("0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")},
		{Type: "string", Value: big.NewInt(1234567)},
		// Address
		{Type: "address", Value: "0x54987E5F03b503BFD7Df2c84f1981e2a7d3bC505"},
		{Type: "address", Value: common.HexToAddress("0x893c33FB170eEAf184B8e305F847Cb0316cE9Bc5")},
		{Type: "address", Value: []byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197}},
		{Type: "address", Value: [20]byte{137, 60, 51, 251, 23, 14, 234, 241, 132, 184, 227, 5, 248, 71, 203, 3, 22, 206, 155, 197}},
		// Tuple
		{Type: "(address,uint)", Value: []interface{}{"0x54987E5F03b503BFD7Df2c84f1981e2a7d3bC505", 34567}},
		{Type: "(address,string,(address,uint))", Value: []interface{}{
			"0x54987E5F03b503BFD7Df2c84f1981e2a7d3bC505", "this is string", []interface{}{"0x54987E5F03b503BFD7Df2c84f1981e2a7d3bC505", 12345},
		}},
	}

	var prototypes []string
	var values []interface{}
	for _, v := range cases {
		prototypes = append(prototypes, v.Type)
		values = append(values, v.Value)
	}

	encodedBytes, err := Encode(prototypes, values)
	if err != nil {
		panic(err)
	}

	t.Log(hexutil.Encode(encodedBytes))
}
