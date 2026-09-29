//go:build cgo

package dydxtrading

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func ParseTxFeeAmount(value string) (sdk.Coins, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = DefaultTxFeeAmount
	}
	coins, err := sdk.ParseCoinsNormalized(value)
	if err != nil {
		return nil, fmt.Errorf("parse tx fee amount %q: %w", value, err)
	}
	if coins.Empty() {
		return nil, fmt.Errorf("tx fee amount must not be empty")
	}
	return coins, nil
}

func parseOptionalTxFeeAmount(value string) (sdk.Coins, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return ParseTxFeeAmount(value)
}

func defaultTxFeeAmount(value sdk.Coins) sdk.Coins {
	if !value.Empty() {
		return value
	}
	coins, err := ParseTxFeeAmount(DefaultTxFeeAmount)
	if err != nil {
		panic(err)
	}
	return coins
}

func firstNonEmptyCoins(values ...sdk.Coins) sdk.Coins {
	for _, value := range values {
		if !value.Empty() {
			return value
		}
	}
	return nil
}
