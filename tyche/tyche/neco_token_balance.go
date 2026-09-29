package tyche

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const necoERC20BalanceABIJSON = `[
	{"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

var ErrNECOTokenBalanceInvalidAddress = errors.New("neco token balance invalid address")

type NECOContractCaller interface {
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
}

type NECOERC20BalanceReader struct {
	caller NECOContractCaller
	abi    abi.ABI
}

func NewNECOERC20BalanceReader(caller NECOContractCaller) *NECOERC20BalanceReader {
	parsed, err := abi.JSON(strings.NewReader(necoERC20BalanceABIJSON))
	if err != nil {
		panic(err)
	}
	return &NECOERC20BalanceReader{caller: caller, abi: parsed}
}

func (r *NECOERC20BalanceReader) BalanceOf(ctx context.Context, tokenAddress, ownerAddress string) (string, error) {
	if r == nil || r.caller == nil {
		return "", ErrNECOPriceOracleNotConfigured
	}
	if !common.IsHexAddress(tokenAddress) || !common.IsHexAddress(ownerAddress) {
		return "", ErrNECOTokenBalanceInvalidAddress
	}
	input, err := r.abi.Pack("balanceOf", common.HexToAddress(ownerAddress))
	if err != nil {
		return "", err
	}
	to := common.HexToAddress(tokenAddress)
	output, err := r.caller.CallContract(ctx, ethereum.CallMsg{
		To:   &to,
		Data: input,
	}, nil)
	if err != nil {
		return "", err
	}
	unpacked, err := r.abi.Unpack("balanceOf", output)
	if err != nil {
		return "", err
	}
	if len(unpacked) != 1 {
		return "", ErrNECOTokenBalanceInvalidAddress
	}
	balance, ok := unpacked[0].(*big.Int)
	if !ok {
		return "", ErrNECOTokenBalanceInvalidAddress
	}
	return balance.String(), nil
}
