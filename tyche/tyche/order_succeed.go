package tyche

import (
	"context"
	"encoding/json"
	"ethgo/model/orders"
	"ethgo/util"
	"net/http"
)

func (t *Tyche) watchSucceed(ctx context.Context) {
	messageReader, err := orders.NewSucceedReader(SUCCEED_CONSUMER_GROUP_NAME, SUCCEED_CONSUMER_NAME)
	if err != nil {
		panic(err)
	}

	messageDispatcher := NewMessageDispatcher(messageReader, t.succeed, t.conf.SucceedNumberOfConcurrent)
	messageDispatcher.Run(ctx)
}

func (t *Tyche) succeed(ctx context.Context, message *orders.Message) AfterFunc {
	var id = message.ID()
	var hash = message.Hash()
	var nonce = message.Nonce()
	var blockNumber = message.String("blockNumber")
	log.Debugf("ENTER @SUCCEED 订单: %v, %v, %v, %v", id, nonce, hash, blockNumber)
	defer log.Debugf("  LEAVE @SUCCEED 订单: %v, %v, %v, %v", id, nonce, hash, blockNumber)

	if t.conf.SucceedURI == "" {
		return t.ack(message)
	}

	var modifier = orders.NewModifier(id, hash, nonce)
	modifier.Set("chainID", t.chainID.String())
	modifier.Set("sent", message.String("sent"))
	modifier.Set("mined", message.String("mined"))
	modifier.Set("address", message.String("address"))
	modifier.Set("blockNumber", message.String("blockNumber"))
	modifier.Set("index", message.String("index"))
	modifier.Set("gasUsed", message.String("gasUsed"))
	modifier.Set("status", message.String("status"))

	resp, err := util.Post(t.conf.SucceedURI, modifier.Fields())
	if err != nil {
		log.Errorf("POST <SUCCEED> notification failed: %v, %v", id, err)
		return After(t.conf.NetworkRetryInterval, message)
	}

	var res Response
	if err := json.Unmarshal(resp, &res); err != nil {
		log.Errorf("Unmarshal response failed: %v, %v", id, err)
		return After(t.conf.CallbackRetryInterval, message)
	}

	if res.Code != http.StatusOK {
		log.Errorf("Failure returned: %v, %v, %v", t.conf.SucceedURI, res.Code, res.Message)
		return After(t.conf.CallbackRetryInterval, message)
	}

	return t.ack(message)
}
