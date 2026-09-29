package tyche

import (
	"context"
	"ethgo/eth"
	"ethgo/model/orders"

	"github.com/ethereum/go-ethereum/core/types"
)

func (t *Tyche) ack(message *orders.Message) AfterFunc {
	if err := message.Ack(); err != nil {
		log.Errorf("Ack message failed: %v, %v", message.Source(), err)
		return After(t.conf.RedisRetryInterval, message)
	}
	return nil
}

func (t *Tyche) signTx(tx *types.Transaction) (*types.Transaction, error) {
	return t.signer.Sign(t.account, tx)
}

func (t *Tyche) sendTransaction(ctx context.Context, id interface{}, tx *types.Transaction) error {
	err := t.backend.SendTransaction(ctx, tx)
	if err != nil {
		log.Warnf("Sending transaction failed: %v, %v, %v, %v", id, tx.Nonce(), tx.Hash(), err)
	}
	return eth.FilterError(err)
}
