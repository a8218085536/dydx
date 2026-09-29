package necodeposits

import (
	"context"
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

func (s *RedisStore) GetByEventID(ctx context.Context, eventID string) (tyche.NECODeposit, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Event(eventID)))
	if err != nil {
		return tyche.NECODeposit{}, false, err
	}
	if len(values) == 0 {
		return tyche.NECODeposit{}, false, nil
	}
	deposit, err := scanDeposit(values)
	if err != nil {
		return tyche.NECODeposit{}, false, err
	}
	return deposit, true, nil
}

func (s *RedisStore) Save(ctx context.Context, deposit tyche.NECODeposit) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	exists, err := redis.Bool(red.Do("EXISTS", keys.Event(deposit.EventID)))
	if err != nil {
		return err
	}
	if exists {
		return tyche.ErrNECODepositDuplicateEvent
	}

	now := time.Now().Unix()
	red.Send("MULTI")
	sendDepositHSET(red, deposit, now)
	if deposit.Status == tyche.NECODepositStatusPending {
		red.Send("SADD", keys.Pending(deposit.From), deposit.EventID)
	}
	if deposit.UserID != "" {
		red.Send("SADD", keys.UserDeposits(deposit.UserID), deposit.EventID)
	}
	if deposit.Status != "" {
		red.Send("SADD", keys.StatusDeposits(deposit.Status), deposit.EventID)
	}
	red.Send("EXEC")
	return red.Flush()
}

func (s *RedisStore) PendingByNECOAddress(ctx context.Context, necoAddress string) ([]tyche.NECODeposit, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	eventIDs, err := redis.Strings(red.Do("SMEMBERS", keys.Pending(necoAddress)))
	if err != nil {
		return nil, err
	}
	deposits := make([]tyche.NECODeposit, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		values, err := redis.Values(red.Do("HGETALL", keys.Event(eventID)))
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			continue
		}
		deposit, err := scanDeposit(values)
		if err != nil {
			return nil, err
		}
		if deposit.Status == tyche.NECODepositStatusPending && strings.EqualFold(deposit.From, necoAddress) {
			deposits = append(deposits, deposit)
		}
	}
	return deposits, nil
}

func (s *RedisStore) MarkCredited(ctx context.Context, eventID, userID string) (tyche.NECODeposit, error) {
	deposit, ok, err := s.GetByEventID(ctx, eventID)
	if err != nil {
		return tyche.NECODeposit{}, err
	}
	if !ok {
		return tyche.NECODeposit{}, tyche.ErrNECODepositNotFound
	}
	deposit.UserID = userID
	deposit.Status = tyche.NECODepositStatusCredited

	var red = model.RedisPool.Get()
	defer red.Close()

	now := time.Now().Unix()
	red.Send("MULTI")
	sendDepositHSET(red, deposit, now)
	red.Send("SREM", keys.Pending(deposit.From), deposit.EventID)
	if deposit.UserID != "" {
		red.Send("SADD", keys.UserDeposits(deposit.UserID), deposit.EventID)
	}
	red.Send("SADD", keys.StatusDeposits(deposit.Status), deposit.EventID)
	red.Send("EXEC")
	if err := red.Flush(); err != nil {
		return tyche.NECODeposit{}, err
	}
	return deposit, nil
}

func (s *RedisStore) ListByUser(ctx context.Context, userID string) ([]tyche.NECODeposit, error) {
	return s.listByIndex(ctx, keys.UserDeposits(userID), func(deposit tyche.NECODeposit) bool {
		return deposit.UserID == userID
	})
}

func (s *RedisStore) ListByStatus(ctx context.Context, status string) ([]tyche.NECODeposit, error) {
	return s.listByIndex(ctx, keys.StatusDeposits(status), func(deposit tyche.NECODeposit) bool {
		return deposit.Status == status
	})
}

func (s *RedisStore) listByIndex(ctx context.Context, key string, keep func(tyche.NECODeposit) bool) ([]tyche.NECODeposit, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	eventIDs, err := redis.Strings(red.Do("SMEMBERS", key))
	if err != nil {
		return nil, err
	}
	deposits := make([]tyche.NECODeposit, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		deposit, ok, err := s.GetByEventID(ctx, eventID)
		if err != nil {
			return nil, err
		}
		if ok && keep(deposit) {
			deposits = append(deposits, deposit)
		}
	}
	return deposits, nil
}

func sendDepositHSET(red redis.Conn, deposit tyche.NECODeposit, now int64) {
	red.Send("HSET", keys.Event(deposit.EventID),
		"id", deposit.ID,
		"eventId", deposit.EventID,
		"chainId", deposit.ChainID,
		"tokenAddress", deposit.TokenAddress,
		"asset", deposit.Asset,
		"from", deposit.From,
		"to", deposit.To,
		"amount", deposit.Amount,
		"txHash", deposit.TxHash,
		"logIndex", deposit.LogIndex,
		"userId", deposit.UserID,
		"status", deposit.Status,
		"updatedAt", now,
	)
}

func scanDeposit(values []interface{}) (tyche.NECODeposit, error) {
	var deposit tyche.NECODeposit
	if err := redis.ScanStruct(values, &deposit); err != nil {
		return tyche.NECODeposit{}, err
	}
	return deposit, nil
}
