package gasprice

import (
	"context"
	"ethgo/eth"
	"math/big"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	price *big.Int
	ptr   *unsafe.Pointer
)

var BumpingLegacyGas func(*big.Int) (*big.Int, error)

func init() {
	ptr = (*unsafe.Pointer)(unsafe.Pointer(&price))
}

func Get() *big.Int {
	return (*big.Int)(atomic.LoadPointer(ptr))
}

func Init(ctx context.Context, backend eth.Backend, estimator Estimator, updateInterval int64) error {
	if updateInterval < 1 {
		updateInterval = 1
	}
	if estimator == nil {
		estimator = &defaultEstimator{}
	}
	BumpingLegacyGas = estimator.BumpingGas

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(updateInterval) * time.Second):
			update(ctx, backend, estimator)
		}
	}()

	return update(ctx, backend, estimator)
}

func update(ctx context.Context, backend eth.Backend, estimator Estimator) error {
	gasPrice, err := backend.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}

	gasPrice, err = estimator.SuggestGasPrice(gasPrice)
	if err != nil {
		return nil
	}

	atomic.StorePointer(ptr, unsafe.Pointer(gasPrice))
	return nil
}
