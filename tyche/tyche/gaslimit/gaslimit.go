package gaslimit

import (
	"context"
	"ethgo/eth"
)

var gasLimit uint64

func Get() uint64 {
	return gasLimit
}

func Init(ctx context.Context, backend eth.Backend) error {
	gasLimit = backend.SuggestGasLimit(ctx)
	return nil
}
