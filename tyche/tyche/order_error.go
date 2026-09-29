package tyche

import (
	"context"
	"encoding/json"
	"ethgo/model/orders"
	"ethgo/util"
	"net/http"
	"time"

	"github.com/garyburd/redigo/redis"
)

func (t *Tyche) watchError(ctx context.Context) {
	messageReader, err := orders.NewErrorReader(ERROR_CONSUMER_GROUP_NAME, ERROR_CONSUMER_NAME)
	if err != nil {
		panic(err)
	}

	messageDispatcher := NewMessageDispatcher(messageReader, t.error, t.conf.ErrorNumberOfConcurrent)
	messageDispatcher.Run(ctx)
}

func (t *Tyche) error(ctx context.Context, message *orders.Message) AfterFunc {
	var id = message.ID()
	var hash = message.Hash()
	var nonce = message.Nonce()
	var reason = message.String("error")

	log.Debugf("ENTER @ERROR 订单: %v, %v, %v", id, nonce, hash)
	defer log.Debugf("  LEAVE @ERROR 订单: %v, %v, %v", id, nonce, hash)

	if t.conf.ErrorURI == "" {
		return t.ack(message)
	}

	args := redis.Args{}
	args = args.Add(orders.FIELD_REASON, reason)
	args = args.Add(orders.FIELD_UPDATED_AT, time.Now().Unix())
	if err := orders.Set(id, args...); err != nil {
		log.Errorf("Set order reason failed: %v, %v, %v", id, nonce, err)
		return After(t.conf.RedisRetryInterval, message)
	}

	modifier := orders.NewModifier(id, hash, nonce)
	modifier.Set("chainID", t.chainID.String())
	modifier.Set("address", message.String("address"))
	modifier.Set("error", reason)

	resp, err := util.Post(t.conf.ErrorURI, modifier.Fields())
	if err != nil {
		log.Errorf("POST <ERROR> notification failed: %v, %v", id, err)
		return After(t.conf.NetworkRetryInterval, message)
	}

	var res Response
	if err := json.Unmarshal(resp, &res); err != nil {
		log.Errorf("Unmarshal response failed: %v, %v", id, err)
		return After(t.conf.CallbackRetryInterval, message)
	}

	if res.Code != http.StatusOK {
		log.Errorf("Failure returned: %v, %v, %v", t.conf.ErrorURI, res.Code, res.Message)
		return After(t.conf.CallbackRetryInterval, message)
	}

	if message.Exists("recovery") {
		log.Warnf("The transaction will be replaced: %v, %v", id, nonce)

		err = t.recoveryNonce(ctx, nonce, id)
		if err != nil {
			log.Errorf("Sending trancaction failed: %v, %v", id, err)
		}
	}

	return t.ack(message)
}
