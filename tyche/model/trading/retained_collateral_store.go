package trading

import (
	"context"
	"ethgo/model"
	"strings"

	"github.com/garyburd/redigo/redis"
)

type RedisRetainedCollateralStore struct{}

func NewRedisRetainedCollateralStore() *RedisRetainedCollateralStore {
	return &RedisRetainedCollateralStore{}
}

func (s *RedisRetainedCollateralStore) RetainedCollateralQuantums(ctx context.Context, dydxAccount string, subaccountNumber uint32) (string, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	value, err := redis.String(red.Do("GET", keys.RetainedCollateral(normalizeRetainedCollateralAccount(dydxAccount), subaccountNumber)))
	if err == redis.ErrNil {
		return "0", nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "0", nil
	}
	return value, nil
}

func (s *RedisRetainedCollateralStore) SaveRetainedCollateralQuantums(ctx context.Context, dydxAccount string, subaccountNumber uint32, quantums string) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("SET", keys.RetainedCollateral(normalizeRetainedCollateralAccount(dydxAccount), subaccountNumber), strings.TrimSpace(quantums))
	return err
}

func normalizeRetainedCollateralAccount(dydxAccount string) string {
	return strings.ToLower(strings.TrimSpace(dydxAccount))
}
