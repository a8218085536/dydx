package tyche

import (
	"context"
	"encoding/json"
	"ethgo/eth"
	"ethgo/model/orders"
	"ethgo/util"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/garyburd/redigo/redis"
)

func (t *Tyche) watchFailed(ctx context.Context) {
	messageReader, err := orders.NewFailedReader(FAILED_CONSUMER_GROUP_NAME, FAILED_CONSUMER_NAME)
	if err != nil {
		panic(err)
	}

	messageDispatcher := NewMessageDispatcher(messageReader, t.failed, t.conf.FailedNumberOfConcurrent)
	messageDispatcher.Run(ctx)
}

func (t *Tyche) failed(ctx context.Context, message *orders.Message) AfterFunc {
	var id = message.ID()
	var hash = message.Hash()
	var nonce = message.Nonce()

	log.Debugf("ENTER @FAILED 订单: %v, %v, %v", id, nonce, hash)
	defer log.Debugf("  LEAVE @FAILED 订单: %v, %v, %v", id, nonce, hash)

	tx, _, err := t.backend.TransactionByHash(ctx, common.HexToHash(hash))
	if err != nil {
		return After(t.conf.NetworkRetryInterval, message)
	}

	var msg = ethereum.CallMsg{
		From:     t.account,
		To:       tx.To(),
		Data:     tx.Data(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasPrice(),
		Value:    tx.Value(),
	}

	blockNumber, _ := new(big.Int).SetString(message.String("blockNumber"), 10)
	reason, err := eth.UnpackRevert(t.backend.CallContract(ctx, msg, blockNumber))
	if err != nil {
		return After(t.conf.NetworkRetryInterval, message)
	}

	args := redis.Args{}
	args = args.Add(orders.FIELD_REASON, reason)
	args = args.Add(orders.FIELD_UPDATED_AT, time.Now().Unix())
	if err := orders.Set(id, args...); err != nil {
		log.Errorf("Set order reason failed: %v, %v, %v", id, nonce, err)
		return After(t.conf.RedisRetryInterval, message)
	}

	log.Errorf("The order has failed: %v, %v", id, reason)

	modifier := orders.NewModifier(id, hash, nonce)
	modifier.Set("chainID", t.chainID.String())
	modifier.Set("sent", message.String("sent"))
	modifier.Set("failed", message.String("failed"))
	modifier.Set("address", tx.To().String())
	modifier.Set("blockNumber", message.String("blockNumber"))
	modifier.Set("index", message.String("index"))
	modifier.Set("gasUsed", message.String("gasUsed"))
	modifier.Set("status", message.String("status"))
	modifier.Set("reason", reason)

	if t.conf.FailedURI == "" {
		return t.ack(message)
	}

	resp, err := util.Post(t.conf.FailedURI, modifier.Fields())
	if err != nil {
		log.Errorf("POST <FAILED> notification failed: %v, %v", id, err)
		return After(t.conf.NetworkRetryInterval, message)
	}

	var res Response
	if err := json.Unmarshal(resp, &res); err != nil {
		log.Errorf("Unmarshal response failed: %v, %v", id, err)
		return After(t.conf.CallbackRetryInterval, message)
	}

	if res.Code != http.StatusOK {
		log.Errorf("Failure returned: %v, %v, %v", t.conf.FailedURI, res.Code, res.Message)
		return After(t.conf.CallbackRetryInterval, message)
	}

	return t.ack(message)
}
