package dydxorders

import (
	"encoding/json"
	"errors"
	"ethgo/model"
	"time"

	"github.com/garyburd/redigo/redis"
)

const (
	FIELD_ID         = "id"
	FIELD_ACTION     = "action"
	FIELD_PARAMS     = "params"
	FIELD_STATUS     = "status"
	FIELD_TX_HASH    = "txHash"
	FIELD_REASON     = "reason"
	FIELD_CREATED_AT = "createdAt"
	FIELD_UPDATED_AT = "updatedAt"

	PENDING_STATUS = "pending"
	SUCCEED_STATUS = "succ"
	FAILED_STATUS  = "fail"
)

type Order struct {
	ID        string `redis:"id" json:"id"`
	Action    string `redis:"action" json:"action"`
	Params    string `redis:"params" json:"params"`
	Status    string `redis:"status" json:"status"`
	TxHash    string `redis:"txHash,omitempty" json:"txHash,omitempty"`
	Reason    string `redis:"reason,omitempty" json:"reason,omitempty"`
	CreatedAt int64  `redis:"createdAt" json:"createdAt"`
	UpdatedAt int64  `redis:"updatedAt" json:"updatedAt"`
}

func Pending(id, action string, params map[string]interface{}) error {
	if id == "" {
		return errors.New("id cannot be empty")
	}
	if action == "" {
		return errors.New("action cannot be empty")
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}

	if exists, err := Exists(id); err != nil {
		return err
	} else if exists {
		return errors.New("the ID already exists")
	}

	now := time.Now().Unix()
	var red = model.RedisPool.Get()
	defer red.Close()

	red.Send("MULTI")
	{
		args := redis.Args{keys.Pending()}
		args = args.Add("*")
		args = args.Add(FIELD_ID, id)
		args = args.Add(FIELD_ACTION, action)
		args = args.Add(FIELD_PARAMS, string(paramsJSON))
		red.Send("XADD", args...)
	}
	{
		order := &Order{
			ID:        id,
			Action:    action,
			Params:    string(paramsJSON),
			Status:    PENDING_STATUS,
			CreatedAt: now,
			UpdatedAt: now,
		}
		args := redis.Args{keys.Entity(id)}
		args = args.AddFlat(order)
		red.Send("HSET", args...)
	}
	red.Send("EXEC")
	return red.Flush()
}

func Exists(id string) (bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	reply, err := redis.Int(red.Do("EXISTS", keys.Entity(id)))
	return reply == 1, err
}

func SetStatus(id, status, txHash, reason string) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	args := redis.Args{keys.Entity(id)}
	args = args.Add(FIELD_STATUS, status)
	args = args.Add(FIELD_UPDATED_AT, time.Now().Unix())
	if txHash != "" {
		args = args.Add(FIELD_TX_HASH, txHash)
	}
	if reason != "" {
		args = args.Add(FIELD_REASON, reason)
	}

	red.Send("HSET", args...)
	return red.Flush()
}
