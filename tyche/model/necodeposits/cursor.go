package necodeposits

import (
	"context"
	"ethgo/model"

	"github.com/garyburd/redigo/redis"
)

type RedisCursorStore struct {
	key        string
	startBlock uint64
}

func NewRedisCursorStore(key string, startBlock ...uint64) *RedisCursorStore {
	store := &RedisCursorStore{key: key}
	if len(startBlock) > 0 {
		store.startBlock = startBlock[0]
	}
	return store
}

func (s *RedisCursorStore) Get(ctx context.Context) (uint64, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	value, err := redis.Uint64(red.Do("GET", s.key))
	if err == redis.ErrNil {
		return s.startBlock, nil
	}
	return value, err
}

func (s *RedisCursorStore) Set(ctx context.Context, value uint64) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("SET", s.key, value)
	return err
}
