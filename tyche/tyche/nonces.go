package tyche

import (
	"context"
	"errors"
	"ethgo/eth"
	"ethgo/model/orders"
	"ethgo/tyche/gaslimit"
	"ethgo/tyche/gasprice"
	"ethgo/util/ethx"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/garyburd/redigo/redis"
)

func (t *Tyche) recoveryNonce(ctx context.Context, nonce uint64, data string) error {

	// 向自己发起一笔金额为 0 的转账
	baseTx := new(types.LegacyTx)
	baseTx.To = (*common.Address)(&t.account)
	baseTx.Nonce = nonce
	baseTx.GasPrice = gasprice.Get()
	baseTx.Gas = gaslimit.Get()
	baseTx.Value = big.NewInt(0)
	if len(data) > 0 {
		baseTx.Data = []byte(data)
	}

	signedTx, err := t.signTx(types.NewTx(baseTx))
	if err != nil {
		panic(err)
	}

	log.Infof("Recovery nonce value: %v, %v", nonce, signedTx.Hash())

	for {
		err = t.sendTransaction(ctx, data, signedTx)
		if err == nil {
			log.Infof("The nonce was successfully recovered: %v", nonce)
			return nil
		}

		if eth.IsRpcError(err) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				continue
			}
		}

		if errors.Is(err, context.Canceled) {
			return err
		}

		return fmt.Errorf("recovery nonce failed: %v %v", nonce, err)
	}
}

func (t *Tyche) bumpingGasTx(id string, orginalTx *types.Transaction) (*types.Transaction, bool, error) {
	nonce := orginalTx.Nonce()
	hash := orginalTx.Hash()

	numberOfRetries, err := orders.NumberOfRetries(id)
	if err != nil {
		return nil, false, err
	}

	// 检查次数限制
	if numberOfRetries >= t.conf.MaxBumpingGasTimes {
		log.Errorf("Too many gas price increases: %v, %v, %v, (%v/%v)", id, nonce, hash, numberOfRetries, t.conf.MaxBumpingGasTimes)
		return nil, false, nil
	}

	bumpedGasPrice, err := gasprice.BumpingLegacyGas(orginalTx.GasPrice())
	if err != nil {
		panic(err)
	}

	// 检查价格限制（天花板价格）
	if orginalTx.GasPrice().Uint64() > 0 && bumpedGasPrice.Uint64() == 0 {
		log.Errorf("Hit gas price bump ceiling: %v, %v, %v", id, nonce, hash)
		return nil, false, nil
	}

	// 生成新的交易
	baseTx := new(types.LegacyTx)
	baseTx.To = orginalTx.To()
	baseTx.Gas = orginalTx.Gas()
	baseTx.Nonce = orginalTx.Nonce()
	baseTx.Value = orginalTx.Value()
	baseTx.Data = orginalTx.Data()

	// 必须高于地板价（现价）
	gasFloorPrice := gasprice.Get()
	if gasFloorPrice.Cmp(bumpedGasPrice) > 0 {
		baseTx.GasPrice = gasFloorPrice
	} else {
		baseTx.GasPrice = bumpedGasPrice
	}

	// 生成签名交易
	signedTx, err := t.signTx(types.NewTx(baseTx))
	if err != nil {
		panic(err)
	}
	return signedTx, true, nil
}

func (t *Tyche) repairOrder(ctx context.Context, id, txData string) error {
	orginalTx, err := ethx.Unmarshal(txData)
	if err != nil {
		panic(err)
	}

	signedTx, isBumpingGas, err := t.bumpingGasTx(id, orginalTx)
	if err != nil {
		return err
	}

	// 不能加速的订单， 通过重发之前的交易来避免出现， 交易被矿工从 Pending 中移除的问题。
	if !isBumpingGas {
		for {
			err = t.sendTransaction(ctx, id, orginalTx)
			if err == nil {
				log.Infof("The transaction was resend: %v, %v, %v", id, orginalTx.Nonce(), orginalTx.Hash())
				return nil
			}

			if eth.IsRpcError(err) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
					continue
				}
			}

			return err
		}
	}

	bytes, err := signedTx.MarshalBinary()
	if err != nil {
		panic(err)
	}

	nonce := signedTx.Nonce()
	hash := signedTx.Hash()
	bumpedTxData := hexutil.Encode(bytes)

	sentModifier := orders.NewModifier(id, hash.String(), nonce)
	sentModifier.Set("sent", time.Now().Unix())
	err = orders.Replace(id, hash.String(), bumpedTxData, sentModifier)
	if err != nil {
		return fmt.Errorf("replace transaction data failed: %v, %w", id, err)
	}

	log.Infof("The transaction to speed up: %v, %v, %v, %v", id, nonce, orginalTx.Hash(), hash)

	for {
		err = t.sendTransaction(ctx, id, signedTx)
		if err == nil {
			log.Infof("The transaction was accelerated: %v, %v", id, nonce)
			return nil
		}

		if eth.IsRpcError(err) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				continue
			}
		}

		if errors.Is(err, context.Canceled) {
			return err
		}

		// 太意外了， 希望下一轮加速能有个好运气吧！
		return fmt.Errorf("transaction acceleration failed: %v, %v, %v", id, nonce, err)
	}
}

func (t *Tyche) repairNonce(ctx context.Context, nonce uint64) error {
	id, err := orders.ID(nonce)
	switch err {
	case nil:
		break
	case redis.ErrNil:
		log.Errorf("Non existing order with nonce value: %v", nonce)
		return t.recoveryNonce(ctx, nonce, id)
	default:
		return err
	}

	release := orders.Lock(id)
	defer release()

	order, err := orders.Get(id, orders.FIELD_STATUS, orders.FIELD_TX_DATA)
	switch err {
	case nil:
		switch order.Status {
		case orders.PENDING_STATUS:
			return fmt.Errorf("pending status: %v, %v", id, nonce)
		case orders.SENT_STATUS:
			return t.repairOrder(ctx, id, order.TxData)
		}

		log.Warnf("Not in sent status: %v, %v, %v", id, nonce, order.Status)
		return nil

	case redis.ErrNil:
		log.Errorf("Non existing order entity: %v", id, nonce)
		return t.recoveryNonce(ctx, nonce, id)
	default:
		return fmt.Errorf("get order data failed: %v, %v", id, err)
	}
}

var errNonceTimeout = errors.New("nonce is timeout")

func (t *Tyche) waitNonce(ctx context.Context, targetNonce uint64, keepalive time.Duration) error {
	checkInterval := time.Duration(t.conf.NonceCheckInterval) * time.Second
	if keepalive < t.suggestNonceKeepalive {
		keepalive = t.suggestNonceKeepalive
	}

	var startTime = time.Now()
	for {
		if time.Since(startTime) > keepalive {
			return errNonceTimeout
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(checkInterval):
		}

		nonce, err := t.backend.NonceAt(ctx, t.account, nil)
		switch {
		case err == nil:
			if nonce > targetNonce {
				t.suggestNonceKeepalive = time.Since(startTime) + checkInterval
				return nil
			}
		case errors.Is(err, context.Canceled):
			return context.Canceled
		}
	}
}

func (t *Tyche) watchNonce(ctx context.Context) {
	checkInterval := time.Duration(t.conf.NonceCheckInterval) * time.Second
	keepalive := time.Duration(t.conf.NonceKeepalive) * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(checkInterval):
		}

		// 链上 Nonce
		nonce, err := t.backend.NonceAt(ctx, t.account, nil)
		if err != nil {
			continue
		}
		t.limiter.Update(nonce)

		// 本地 Nonce
		lastNonce, err := orders.NonceAt()
		if err != nil {
			continue
		}

		if lastNonce > nonce {
			err := t.waitNonce(ctx, nonce, keepalive)
			switch err {
			case nil:
				continue
			case errNonceTimeout:
				break
			case context.Canceled:
				return
			default:
				panic(err)
			}

			log.Warnf("Timeout nonce value detected: %v", nonce)

			if err := t.repairNonce(ctx, nonce); err != nil {
				log.Errorf("Repair nonce value failed: %v, %v", nonce, err)
				continue
			}

			log.Infof("Nonce value was repaired successfully: %v", nonce)
		}
	}
}
