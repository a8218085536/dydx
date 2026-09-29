package necowithdrawals

import (
	"context"
	"ethgo/model"
	"ethgo/tyche"
	"time"

	"github.com/garyburd/redigo/redis"
)

type RedisStore struct{}

func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) Get(ctx context.Context, withdrawalID string) (tyche.NECOWithdrawalRecord, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Entity(withdrawalID)))
	if err != nil {
		return tyche.NECOWithdrawalRecord{}, false, err
	}
	if len(values) == 0 {
		return tyche.NECOWithdrawalRecord{}, false, nil
	}
	record, err := scanRecord(values)
	if err != nil {
		return tyche.NECOWithdrawalRecord{}, false, err
	}
	return record, true, nil
}

func (s *RedisStore) Save(ctx context.Context, record tyche.NECOWithdrawalRecord) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	exists, err := redis.Bool(red.Do("EXISTS", keys.Entity(record.WithdrawalID)))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return saveRecord(red, record)
}

func (s *RedisStore) MarkStatus(ctx context.Context, withdrawalID, status, reason string) (tyche.NECOWithdrawalRecord, error) {
	record, ok, err := s.Get(ctx, withdrawalID)
	if err != nil {
		return tyche.NECOWithdrawalRecord{}, err
	}
	if !ok {
		return tyche.NECOWithdrawalRecord{}, tyche.ErrNECOWithdrawalNotFound
	}
	record.Status = status
	record.Reason = reason
	record.UpdatedAt = time.Now().Unix()

	var red = model.RedisPool.Get()
	defer red.Close()
	if err := saveRecord(red, record); err != nil {
		return tyche.NECOWithdrawalRecord{}, err
	}
	return record, nil
}

func saveRecord(red redis.Conn, record tyche.NECOWithdrawalRecord) error {
	now := time.Now().Unix()
	if record.CreatedAt == 0 {
		record.CreatedAt = now
	}
	if record.UpdatedAt == 0 {
		record.UpdatedAt = now
	}
	_, err := red.Do("HSET", keys.Entity(record.WithdrawalID),
		"withdrawalId", record.WithdrawalID,
		"asset", record.Asset,
		"amount", record.Amount,
		"toAddress", record.ToAddress,
		"txHash", record.TxHash,
		"nonce", record.Nonce,
		"status", record.Status,
		"reason", record.Reason,
		"createdAt", record.CreatedAt,
		"updatedAt", record.UpdatedAt,
	)
	return err
}

func scanRecord(values []interface{}) (tyche.NECOWithdrawalRecord, error) {
	var record tyche.NECOWithdrawalRecord
	if err := redis.ScanStruct(values, &record); err != nil {
		return tyche.NECOWithdrawalRecord{}, err
	}
	return record, nil
}
