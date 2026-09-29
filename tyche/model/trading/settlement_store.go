package trading

import (
	"context"
	"errors"
	"ethgo/model"
	"ethgo/tyche"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/google/uuid"
)

type RedisSettlementStore struct{}

func NewRedisSettlementStore() *RedisSettlementStore {
	return &RedisSettlementStore{}
}

func (s *RedisSettlementStore) CreateIfAbsent(ctx context.Context, settlement tyche.TradingSettlement) (tyche.TradingSettlement, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	indexKey := keys.CloseOrderSettlement(settlement.UserID, settlement.CloseOrderID)
	for attempt := 0; attempt < tradingCreateMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", indexKey); err != nil {
			return tyche.TradingSettlement{}, false, err
		}
		existingID, err := redis.String(red.Do("GET", indexKey))
		if err == nil {
			_, _ = red.Do("UNWATCH")
			existing, ok, err := s.GetByID(ctx, existingID)
			if err != nil {
				return tyche.TradingSettlement{}, false, err
			}
			if !ok {
				return tyche.TradingSettlement{}, false, errors.New("trading settlement index points to missing settlement")
			}
			return existing, false, nil
		}
		if err != redis.ErrNil {
			_, _ = red.Do("UNWATCH")
			return tyche.TradingSettlement{}, false, err
		}

		if settlement.ID == "" {
			settlement.ID = uuid.NewString()
		}
		now := time.Now().Unix()
		if settlement.CreatedAt == 0 {
			settlement.CreatedAt = now
		}
		settlement.UpdatedAt = now
		if err := red.Send("MULTI"); err != nil {
			_, _ = red.Do("UNWATCH")
			return tyche.TradingSettlement{}, false, err
		}
		if err := queueSaveSettlement(red, settlement); err != nil {
			_ = cancelTradingSyncWatchOrTransaction(red)
			return tyche.TradingSettlement{}, false, err
		}
		reply, err := red.Do("EXEC")
		if err != nil {
			return tyche.TradingSettlement{}, false, err
		}
		if reply != nil {
			return settlement, true, nil
		}
	}
	return tyche.TradingSettlement{}, false, errors.New("trading settlement create conflict")
}

func (s *RedisSettlementStore) Save(ctx context.Context, settlement tyche.TradingSettlement) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	if settlement.ID == "" {
		settlement.ID = uuid.NewString()
	}
	if settlement.CreatedAt == 0 {
		settlement.CreatedAt = time.Now().Unix()
	}
	settlement.UpdatedAt = time.Now().Unix()
	if err := red.Send("MULTI"); err != nil {
		return err
	}
	if err := queueSaveSettlement(red, settlement); err != nil {
		return err
	}
	_, err := red.Do("EXEC")
	return err
}

func (s *RedisSettlementStore) GetByID(ctx context.Context, id string) (tyche.TradingSettlement, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Settlement(id)))
	if err != nil {
		return tyche.TradingSettlement{}, false, err
	}
	if len(values) == 0 {
		return tyche.TradingSettlement{}, false, nil
	}
	var settlement tyche.TradingSettlement
	if err := redis.ScanStruct(values, &settlement); err != nil {
		return tyche.TradingSettlement{}, false, err
	}
	return settlement, true, nil
}

func (s *RedisSettlementStore) GetByCloseOrderID(ctx context.Context, userID, closeOrderID string) (tyche.TradingSettlement, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	id, err := redis.String(red.Do("GET", keys.CloseOrderSettlement(userID, closeOrderID)))
	if err == redis.ErrNil {
		return tyche.TradingSettlement{}, false, nil
	}
	if err != nil {
		return tyche.TradingSettlement{}, false, err
	}
	return s.GetByID(ctx, id)
}

func (s *RedisSettlementStore) GetByLedgerWithdrawalID(ctx context.Context, withdrawalID string) (tyche.TradingSettlement, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	id, err := redis.String(red.Do("GET", keys.WithdrawalSettlement(withdrawalID)))
	if err == redis.ErrNil {
		return tyche.TradingSettlement{}, false, nil
	}
	if err != nil {
		return tyche.TradingSettlement{}, false, err
	}
	return s.GetByID(ctx, id)
}

func (s *RedisSettlementStore) ListByUser(ctx context.Context, userID string) ([]tyche.TradingSettlement, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.UserSettlements(userID)))
	if err != nil {
		return nil, err
	}
	settlements := make([]tyche.TradingSettlement, 0, len(ids))
	for _, id := range ids {
		settlement, ok, err := s.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			settlements = append(settlements, settlement)
		}
	}
	return settlements, nil
}

func (s *RedisSettlementStore) ListByStatus(ctx context.Context, status string) ([]tyche.TradingSettlement, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.StatusSettlements(status)))
	if err != nil {
		return nil, err
	}
	settlements := make([]tyche.TradingSettlement, 0, len(ids))
	for _, id := range ids {
		settlement, ok, err := s.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok && settlement.Status == status {
			settlements = append(settlements, settlement)
		}
	}
	return settlements, nil
}

func queueSaveSettlement(red redis.Conn, settlement tyche.TradingSettlement) error {
	if err := red.Send("HSET", keys.Settlement(settlement.ID),
		"id", settlement.ID,
		"kind", settlement.Kind,
		"userId", settlement.UserID,
		"necoAddress", settlement.NECOAddress,
		"dydxAccount", settlement.DYDXAccount,
		"subaccountNumber", settlement.SubaccountNumber,
		"market", settlement.Market,
		"closeOrderId", settlement.CloseOrderID,
		"closeClientOrderId", settlement.CloseClientOrderID,
		"sourcePositionId", settlement.SourcePositionID,
		"settlementUsd", settlement.SettlementUSD,
		"settlementAsset", settlement.SettlementAsset,
		"settlementAmount", settlement.SettlementAmount,
		"assetUsdPrice", settlement.AssetUSDPrice,
		"priceSnapshotId", settlement.PriceSnapshotID,
		"systemReserved", settlement.SystemReserved,
		"dydxReservedQuantums", settlement.DYDXReservedQuantums,
		"ledgerCreditRefId", settlement.LedgerCreditRefID,
		"ledgerWithdrawalId", settlement.LedgerWithdrawalID,
		"necoTxHash", settlement.NECOTxHash,
		"tradeCloseCallbackUrl", settlement.TradeCloseCallbackURL,
		"tradeCloseCallbackStatus", settlement.TradeCloseCallbackStatus,
		"tradeCloseCallbackReason", settlement.TradeCloseCallbackReason,
		"tradeCloseCallbackAt", settlement.TradeCloseCallbackAt,
		"status", settlement.Status,
		"reason", settlement.Reason,
		"createdAt", settlement.CreatedAt,
		"updatedAt", settlement.UpdatedAt,
	); err != nil {
		return err
	}
	if err := red.Send("SET", keys.CloseOrderSettlement(settlement.UserID, settlement.CloseOrderID), settlement.ID); err != nil {
		return err
	}
	if settlement.LedgerWithdrawalID != "" {
		if err := red.Send("SET", keys.WithdrawalSettlement(settlement.LedgerWithdrawalID), settlement.ID); err != nil {
			return err
		}
	}
	if err := red.Send("SADD", keys.UserSettlements(settlement.UserID), settlement.ID); err != nil {
		return err
	}
	if settlement.Status != "" {
		return red.Send("SADD", keys.StatusSettlements(settlement.Status), settlement.ID)
	}
	return nil
}
