package walletbindings

import (
	"context"
	"errors"
	"ethgo/model"
	"ethgo/tyche"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

type RedisStore struct{}

func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) Save(ctx context.Context, binding tyche.WalletBinding) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	now := time.Now().Unix()
	args := redis.Args{keys.Entity(binding.ID)}
	args = args.Add("id", binding.ID)
	args = args.Add("userId", binding.UserID)
	args = args.Add("necoAddress", binding.NECOAddress)
	args = args.Add("dydxAddress", binding.DYDXAddress)
	args = args.Add("nonce", binding.Nonce)
	args = args.Add("message", binding.Message)
	args = args.Add("status", binding.Status)
	args = args.Add("createdAt", now)
	args = args.Add("updatedAt", now)

	if err := red.Send("MULTI"); err != nil {
		return err
	}
	if err := red.Send("HSET", args...); err != nil {
		return err
	}
	if err := queueIndexBinding(red, binding); err != nil {
		return err
	}
	_, err := redis.Values(red.Do("EXEC"))
	return err
}

func (s *RedisStore) Get(ctx context.Context, id string) (tyche.WalletBinding, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Entity(id)))
	if err != nil {
		return tyche.WalletBinding{}, err
	}
	if len(values) == 0 {
		return tyche.WalletBinding{}, tyche.ErrWalletBindingNotFound
	}

	var binding tyche.WalletBinding
	if err := redis.ScanStruct(values, &binding); err != nil {
		return tyche.WalletBinding{}, err
	}
	return binding, nil
}

func (s *RedisStore) Activate(ctx context.Context, id string) (tyche.WalletBinding, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	if _, err := red.Do("HSET", keys.Entity(id), "status", tyche.WalletBindingStatusActive, "updatedAt", time.Now().Unix()); err != nil {
		return tyche.WalletBinding{}, err
	}
	binding, err := s.Get(ctx, id)
	if err != nil {
		return tyche.WalletBinding{}, err
	}
	if err := s.repairIndexes(ctx, binding); err != nil {
		return tyche.WalletBinding{}, err
	}
	return binding, nil
}

func (s *RedisStore) GetByNECOAddress(ctx context.Context, address string) (tyche.WalletBinding, error) {
	return s.getByIndexOrScan(ctx, keys.NECOIndex(address), "necoAddress", address)
}

func (s *RedisStore) GetByDYDXAddress(ctx context.Context, address string) (tyche.WalletBinding, error) {
	return s.getByIndexOrScan(ctx, keys.DYDXIndex(address), "dydxAddress", address)
}

func (s *RedisStore) GetByUserID(ctx context.Context, userID string) (tyche.WalletBinding, error) {
	return s.getByIndexOrScan(ctx, keys.UserIndex(userID), "userId", userID)
}

func (s *RedisStore) getByIndexOrScan(ctx context.Context, key, field, value string) (tyche.WalletBinding, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	id, err := redis.String(red.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return tyche.WalletBinding{}, err
	}
	if err == nil {
		binding, getErr := s.Get(ctx, id)
		if getErr == nil && walletBindingMatchesLookup(binding, field, value) && binding.Status == tyche.WalletBindingStatusActive {
			return binding, nil
		}
		if getErr != nil && !errors.Is(getErr, tyche.ErrWalletBindingNotFound) {
			return tyche.WalletBinding{}, getErr
		}
	}
	return s.scanByFieldAndRepair(ctx, field, value)
}

func (s *RedisStore) scanByFieldAndRepair(ctx context.Context, field, value string) (tyche.WalletBinding, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	cursor := 0
	pattern := keys.Entity("*")
	var fallback tyche.WalletBinding
	hasFallback := false
	for {
		var entityKeys []string
		values, err := redis.Values(red.Do("SCAN", cursor, "MATCH", pattern, "COUNT", 200))
		if err != nil {
			return tyche.WalletBinding{}, err
		}
		if _, err := redis.Scan(values, &cursor, &entityKeys); err != nil {
			return tyche.WalletBinding{}, err
		}
		for _, entityKey := range entityKeys {
			values, err := redis.Values(red.Do("HGETALL", entityKey))
			if err != nil {
				return tyche.WalletBinding{}, err
			}
			if len(values) == 0 {
				continue
			}
			var binding tyche.WalletBinding
			if err := redis.ScanStruct(values, &binding); err != nil {
				return tyche.WalletBinding{}, err
			}
			if walletBindingMatchesLookup(binding, field, value) {
				if binding.Status == tyche.WalletBindingStatusActive {
					if err := setIndexBinding(red, binding); err != nil {
						return tyche.WalletBinding{}, err
					}
					return binding, nil
				}
				if !hasFallback {
					fallback = binding
					hasFallback = true
				}
			}
		}
		if cursor == 0 {
			break
		}
	}
	if hasFallback {
		return fallback, nil
	}
	return tyche.WalletBinding{}, tyche.ErrWalletBindingNotFound
}

func (s *RedisStore) repairIndexes(ctx context.Context, binding tyche.WalletBinding) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	return setIndexBinding(red, binding)
}

func queueIndexBinding(red redis.Conn, binding tyche.WalletBinding) error {
	if binding.Status != tyche.WalletBindingStatusActive {
		return nil
	}
	if strings.TrimSpace(binding.NECOAddress) != "" {
		if err := red.Send("SET", keys.NECOIndex(binding.NECOAddress), binding.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(binding.DYDXAddress) != "" {
		if err := red.Send("SET", keys.DYDXIndex(binding.DYDXAddress), binding.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(binding.UserID) != "" {
		if err := red.Send("SET", keys.UserIndex(binding.UserID), binding.ID); err != nil {
			return err
		}
	}
	return nil
}

func setIndexBinding(red redis.Conn, binding tyche.WalletBinding) error {
	if binding.Status != tyche.WalletBindingStatusActive {
		return nil
	}
	if strings.TrimSpace(binding.NECOAddress) != "" {
		if _, err := red.Do("SET", keys.NECOIndex(binding.NECOAddress), binding.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(binding.DYDXAddress) != "" {
		if _, err := red.Do("SET", keys.DYDXIndex(binding.DYDXAddress), binding.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(binding.UserID) != "" {
		if _, err := red.Do("SET", keys.UserIndex(binding.UserID), binding.ID); err != nil {
			return err
		}
	}
	return nil
}

func walletBindingMatchesLookup(binding tyche.WalletBinding, field, value string) bool {
	switch field {
	case "necoAddress":
		return strings.EqualFold(strings.TrimSpace(binding.NECOAddress), strings.TrimSpace(value))
	case "dydxAddress":
		return strings.EqualFold(strings.TrimSpace(binding.DYDXAddress), strings.TrimSpace(value))
	case "userId":
		return strings.TrimSpace(binding.UserID) == strings.TrimSpace(value)
	default:
		return false
	}
}
