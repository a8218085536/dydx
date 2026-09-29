package tyche

import (
	"context"
	"errors"
	"ethgo/model/orders"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/garyburd/redigo/redis"
)

func (t *Tyche) watchSent(ctx context.Context) {
	messageReader, err := orders.NewSentReader(SENT_CONSUMER_GROUP_NAME, SENT_CONSUMER_NAME)
	if err != nil {
		panic(err)
	}

	messageDispatcher := NewMessageDispatcher(messageReader, t.sent, t.conf.SentNumberOfConcurrent)
	messageDispatcher.Run(ctx)
}

func (t *Tyche) query(ctx context.Context, hash common.Hash) (*types.Receipt, error) {

QUERY:
	receipt, err := t.backend.TransactionReceipt(ctx, hash)
	if err == nil {
		return receipt, nil
	}

	if errors.Is(err, ethereum.NotFound) {
		return nil, err
	}

	// Wait for the next round.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Second):
		goto QUERY
	}
}

func (t *Tyche) diagnose(ctx context.Context, id, hash string, nonce uint64) bool {
	// 对订单的交易进行加速， 会使 SENT 队列中存多个相同订单不同 hash 的消息。
	status, err := orders.Status(id)
	if err == redis.ErrNil {
		panic(id)
	}
	if err != nil {
		log.Errorf("Get order status failed: %v, %v, %v", id, nonce, err)
		return true
	}

	switch status {
	case orders.SENT_STATUS:
		break
	case orders.SUCCEED_STATUS, orders.FAILED_STATUS, orders.ERROR_STATUS:
		log.Infof("The order has been closed: %v, %v, %v, %v", id, status, nonce, hash)
		return false
	default:
		panic(id)
	}

	// TODO: 获得链上与该 Nonce 匹配的交易， 进一步判定是否存在交易异常
	// 1. 通过 scanner 提供的 api接口
	// 2. 为每个 链 部署一个 repository , 聚合 account， nonce 及 hash， tyche 向 repos 发送查询请求
	// 3. 根据系统中的历史订单， 找出 前一个 nonce 跟后一个nonce 的 blocknumber， 查询中间所有 block 的交易数据，找到 与当前nonce 匹配的 交易
	return true
}

func (t *Tyche) sent(ctx context.Context, message *orders.Message) AfterFunc {
	var id = message.ID()
	var hash = message.Hash()
	var nonce = message.Nonce()

	if t.conf.IsPaused {
		return After(t.conf.SentRetryInterval, message)
	}

	distance := t.limiter.Distance(nonce)
	if distance > 0 {
		// log.Debugf("The <SENT> order will be suspended: %v, %v", id, nonce)
		return After(t.conf.SentRetryInterval, message)
	}

	log.Debugf("ENTER @SENT 订单: %v, %v, %v, %v", id, nonce, hash, t.limiter.max)
	defer log.Debugf("  LEAVE @SENT 订单: %v, %v, %v, %v", id, nonce, hash, t.limiter.max)

	receipt, err := t.query(ctx, common.HexToHash(hash))
	if err != nil {
		switch err {
		case context.Canceled:
			return nil
		case ethereum.NotFound:
			if t.diagnose(ctx, id, hash, nonce) {
				return After(t.conf.WaitMinedRetryInterval, message)
			}
			return t.ack(message)
		}
	}

	t.limiter.Update(nonce)

	release := orders.Lock(id)
	defer release()

	status, err := orders.Status(id)
	switch err {
	case nil:
		break
	case redis.ErrNil:
		panic(id)
	default:
		log.Errorf("Get order status failed: %v, %v, %v", id, nonce, err)
		return After(t.conf.RedisRetryInterval, message)
	}

	if status != orders.SENT_STATUS {
		log.Debugf("The order is not in sent status: %v, %v %v", id, nonce, status)
		return t.ack(message)
	}

	var modifier = orders.NewModifier(id, hash, nonce)
	modifier.Set("sent", message.String("sent"))
	modifier.Set("blockNumber", receipt.BlockNumber.String())
	modifier.Set("index", receipt.TransactionIndex)
	modifier.Set("gasUsed", receipt.GasUsed)
	modifier.Set("status", receipt.Status)

	if receipt.Status == types.ReceiptStatusSuccessful {
		modifier.Set("address", receipt.Logs[0].Address)
		modifier.Set("mined", time.Now().Unix())
		if err := orders.Succeed(modifier); err != nil {
			log.Errorf("Publish <SUCCEED> message failed: %v, %v", id, err)
			return After(t.conf.RedisRetryInterval, message)
		}
		return t.ack(message)
	}

	modifier.Set("failed", time.Now().Unix())
	if err := orders.Failed(modifier); err != nil {
		log.Errorf("Publish <FAILED> message failed: %v, %v", id, err)
		return After(t.conf.RedisRetryInterval, message)
	}

	return t.ack(message)
}
