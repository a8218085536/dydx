package types

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestMethod(t *testing.T) {

	{
		balanceOf, err := NewMethod("balanceOf",
			[]string{"address", "uint256"},
			[]string{"uint256"},
		)
		if err != nil {
			panic(err)
		}

		data, err := balanceOf.Inputs.Pack(
			common.HexToAddress("0x3f42a9387A75E92283E0EbA1CC707E4c637c7dEe"),
			big.NewInt(8001),
		)
		if err != nil {
			panic(err)
		}

		t.Logf("%v", hexutil.Encode(data))
	}

	{
		testTuple, err := NewMethod("testTuple",
			[]string{"address", "uint256", "(uint256,string,(address,uint256))"},
			[]string{"uint256"},
		)
		if err != nil {
			panic(err)
		}

		data, err := testTuple.Inputs.Pack(
			common.HexToAddress("0x3f42a9387A75E92283E0EbA1CC707E4c637c7dEe"),
			big.NewInt(8001),
			struct {
				Name0 *big.Int
				Name1 string
				Name2 struct {
					Name0 common.Address
					Name1 *big.Int
				}
			}{
				Name0: big.NewInt(123456),
				Name1: "tuple string",
				Name2: struct {
					Name0 common.Address
					Name1 *big.Int
				}{
					Name0: common.HexToAddress("0x3f42a9387A75E92283E0EbA1CC707E4c637c7dEe"),
					Name1: big.NewInt(654321),
				},
			},
		)
		if err != nil {
			panic(err)
		}

		t.Logf("%v", hexutil.Encode(data))

	}

	{
		testTuple, err := NewMethod("testTuple",
			[]string{"address", "uint256", "(uint256,string,(address,uint256))"},
			[]string{"uint256"},
		)
		if err != nil {
			panic(err)
		}

		var values []interface{}
		values = append(values, "0x3f42a9387A75E92283E0EbA1CC707E4c637c7dEe")
		values = append(values, 8001)
		values = append(values, []interface{}{
			123456,
			"tuple string",
			[]interface{}{
				"0x3f42a9387A75E92283E0EbA1CC707E4c637c7dEe",
				654321,
			},
		})

		var argv []interface{}
		for i, arg := range testTuple.Inputs {
			v, err := convertType(values[i], arg.Type)
			if err != nil {
				panic(err)
			}
			argv = append(argv, v)
		}

		data, err := testTuple.Inputs.Pack(argv...)
		if err != nil {
			panic(err)
		}

		t.Logf("%v", hexutil.Encode(data))
	}
}
