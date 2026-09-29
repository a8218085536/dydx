package trading

import (
	"context"
	"errors"
	"ethgo/model"
	"ethgo/tyche"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

type RedisStore struct{}

const (
	tradingCreateMaxAttempts   = 3
	tradingSyncSaveMaxAttempts = 3
)

func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) GetOrderByClientID(ctx context.Context, userID, clientOrderID string) (tyche.TradingOrder, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	id, err := redis.String(red.Do("GET", keys.ClientOrder(userID, clientOrderID)))
	if err == redis.ErrNil {
		return tyche.TradingOrder{}, false, nil
	}
	if err != nil {
		return tyche.TradingOrder{}, false, err
	}
	return s.GetOrderByID(ctx, userID, id)
}

func (s *RedisStore) GetOrderByID(ctx context.Context, userID, orderID string) (tyche.TradingOrder, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Order(userID, orderID)))
	if err != nil {
		return tyche.TradingOrder{}, false, err
	}
	if len(values) == 0 {
		return tyche.TradingOrder{}, false, nil
	}
	var order tyche.TradingOrder
	if err := redis.ScanStruct(values, &order); err != nil {
		return tyche.TradingOrder{}, false, err
	}
	return order, true, nil
}

func (s *RedisStore) SaveOrder(ctx context.Context, order tyche.TradingOrder) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	now := time.Now().Unix()
	if order.CreatedAt == 0 {
		createdAt, err := redis.Int64(red.Do("HGET", keys.Order(order.UserID, order.ID), "createdAt"))
		if err == nil {
			order.CreatedAt = createdAt
		}
	}
	if order.CreatedAt == 0 {
		order.CreatedAt = now
	}
	order.UpdatedAt = now
	red.Send("MULTI")
	if err := queueSaveOrder(red, order, now); err != nil {
		return err
	}
	red.Send("EXEC")
	return red.Flush()
}

func (s *RedisStore) CreateOrderIfAbsent(ctx context.Context, order tyche.TradingOrder) (tyche.TradingOrder, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	clientKey := keys.ClientOrder(order.UserID, order.ClientOrderID)
	for attempt := 0; attempt < tradingCreateMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", clientKey); err != nil {
			return tyche.TradingOrder{}, false, err
		}

		existingID, err := redis.String(red.Do("GET", clientKey))
		if err == nil {
			_, _ = red.Do("UNWATCH")
			existing, ok, err := s.GetOrderByID(ctx, order.UserID, existingID)
			if err != nil {
				return tyche.TradingOrder{}, false, err
			}
			if !ok {
				return tyche.TradingOrder{}, false, errors.New("trading client order index points to missing order")
			}
			return existing, false, nil
		}
		if err != redis.ErrNil {
			_, _ = red.Do("UNWATCH")
			return tyche.TradingOrder{}, false, err
		}

		now := time.Now().Unix()
		if order.CreatedAt == 0 {
			order.CreatedAt = now
		}
		order.UpdatedAt = now
		if err := red.Send("MULTI"); err != nil {
			_, _ = red.Do("UNWATCH")
			return tyche.TradingOrder{}, false, err
		}
		if err := queueSaveOrder(red, order, now); err != nil {
			_, _ = red.Do("UNWATCH")
			return tyche.TradingOrder{}, false, err
		}
		reply, err := red.Do("EXEC")
		if err != nil {
			return tyche.TradingOrder{}, false, err
		}
		if reply != nil {
			return order, true, nil
		}
	}
	return tyche.TradingOrder{}, false, errors.New("trading order create conflict")
}

func (s *RedisStore) ListOrders(ctx context.Context, userID string) ([]tyche.TradingOrder, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.UserOrders(userID)))
	if err != nil {
		return nil, err
	}
	orders := make([]tyche.TradingOrder, 0, len(ids))
	for _, id := range ids {
		order, ok, err := s.GetOrderByID(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if ok {
			orders = append(orders, order)
		}
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].ClientOrderID == orders[j].ClientOrderID {
			return orders[i].ID < orders[j].ID
		}
		return orders[i].ClientOrderID < orders[j].ClientOrderID
	})
	return orders, nil
}

func (s *RedisStore) ListReconcilableOrders(ctx context.Context, limit int) ([]tyche.TradingOrder, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	if limit <= 0 {
		limit = 100
	}
	refs, err := redis.Strings(red.Do("SMEMBERS", keys.ReconcilableOrders()))
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(refs))
	orders := make([]tyche.TradingOrder, 0, len(refs))
	for _, ref := range refs {
		seen[ref] = struct{}{}
		userID, orderID, ok := splitOrderRef(ref)
		if !ok {
			continue
		}
		order, ok, err := s.GetOrderByID(ctx, userID, orderID)
		if err != nil {
			return nil, err
		}
		if !ok || !shouldListOrderForReconcile(order) {
			_, _ = red.Do("SREM", keys.ReconcilableOrders(), ref)
			continue
		}
		orders = append(orders, order)
	}
	if len(orders) < limit {
		if err := backfillReconcilableOrders(red, &orders, seen, limit); err != nil {
			return nil, err
		}
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].ClientOrderID == orders[j].ClientOrderID {
			return orders[i].ID < orders[j].ID
		}
		return orders[i].ClientOrderID < orders[j].ClientOrderID
	})
	if len(orders) > limit {
		return orders[:limit], nil
	}
	return orders, nil
}

func backfillReconcilableOrders(red redis.Conn, orders *[]tyche.TradingOrder, seen map[string]struct{}, limit int) error {
	cursor := 0
	pattern := keys.Namespace() + ":order:*"
	for {
		values, err := redis.Values(red.Do("SCAN", cursor, "MATCH", pattern, "COUNT", 200))
		if err != nil {
			return err
		}
		var orderKeys []string
		if _, err := redis.Scan(values, &cursor, &orderKeys); err != nil {
			return err
		}
		for _, orderKey := range orderKeys {
			values, err := redis.Values(red.Do("HGETALL", orderKey))
			if err != nil {
				return err
			}
			if len(values) == 0 {
				continue
			}
			var order tyche.TradingOrder
			if err := redis.ScanStruct(values, &order); err != nil {
				return err
			}
			ref := order.UserID + ":" + order.ID
			if _, ok := seen[ref]; ok || !shouldListOrderForReconcile(order) {
				continue
			}
			seen[ref] = struct{}{}
			if _, err := red.Do("SADD", keys.ReconcilableOrders(), ref); err != nil {
				return err
			}
			*orders = append(*orders, order)
			if len(*orders) >= limit {
				return nil
			}
		}
		if cursor == 0 {
			return nil
		}
	}
}

func queueSaveOrder(red redis.Conn, order tyche.TradingOrder, now int64) error {
	ref := order.UserID + ":" + order.ID
	if err := red.Send("HSET", keys.Order(order.UserID, order.ID),
		"id", order.ID,
		"userId", order.UserID,
		"necoAddress", order.NECOAddress,
		"dydxAccount", order.DYDXAccount,
		"subaccountNumber", order.SubaccountNumber,
		"clientOrderId", order.ClientOrderID,
		"market", order.Market,
		"side", order.Side,
		"orderType", order.OrderType,
		"executionOrderType", order.ExecutionOrderType,
		"price", order.Price,
		"referencePrice", order.ReferencePrice,
		"slippageBps", order.SlippageBps,
		"marketPriceSource", order.MarketPriceSource,
		"marketPriceSnapshotId", order.MarketPriceSnapshotID,
		"size", order.Size,
		"marginAsset", order.MarginAsset,
		"marginAmount", order.MarginAmount,
		"marginUsd", order.MarginUSD,
		"priceSnapshotId", order.PriceSnapshotID,
		"dedxUsdPrice", order.DEDXUSDPrice,
		"assetUsdPrice", order.AssetUSDPrice,
		"priceRoute", order.PriceRoute,
		"fundingStatus", order.FundingStatus,
		"fundingAction", order.FundingAction,
		"fundingAsset", order.FundingAsset,
		"fundingAmount", order.FundingAmount,
		"fundingTokenAddress", order.FundingTokenAddress,
		"fundingFromAddress", order.FundingFromAddress,
		"fundingToAddress", order.FundingToAddress,
		"fundingSpenderAddress", order.FundingSpenderAddress,
		"fundingReason", order.FundingReason,
		"fundingWalletBalance", order.FundingWalletBalance,
		"fundingLedgerAvailable", order.FundingLedgerAvailable,
		"dydxFundingTxHash", order.DYDXFundingTxHash,
		"dydxFundingStatus", order.DYDXFundingStatus,
		"dydxFundingAssetId", order.DYDXFundingAssetID,
		"dydxFundingQuantums", order.DYDXFundingQuantums,
		"dydxOrderMarginQuantums", order.DYDXOrderMarginQuantums,
		"dydxRetainedCollateralQuantums", order.DYDXRetainedCollateralQuantums,
		"dydxOrderTxHash", order.DYDXOrderTxHash,
		"dydxCancelTxHash", order.DYDXCancelTxHash,
		"reduceOnly", order.ReduceOnly,
		"closePosition", order.ClosePosition,
		"status", order.Status,
		"reason", order.Reason,
		"createdAt", order.CreatedAt,
		"updatedAt", now,
	); err != nil {
		return err
	}
	if err := red.Send("SET", keys.ClientOrder(order.UserID, order.ClientOrderID), order.ID); err != nil {
		return err
	}
	if err := red.Send("SADD", keys.UserOrders(order.UserID), order.ID); err != nil {
		return err
	}
	if shouldListOrderForReconcile(order) {
		return red.Send("SADD", keys.ReconcilableOrders(), ref)
	}
	return red.Send("SREM", keys.ReconcilableOrders(), ref)
}

func shouldListOrderForReconcile(order tyche.TradingOrder) bool {
	if isFailedFundedOrder(order) {
		return true
	}
	return isReconcilableOrderStatus(order.Status) && reconcileTxHash(order) != ""
}

func isFailedFundedOrder(order tyche.TradingOrder) bool {
	if order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return false
	}
	switch order.Status {
	case tyche.TradingOrderStatusRejected,
		tyche.TradingOrderStatusSubmitFailed,
		tyche.TradingOrderStatusEquityTierLimit:
		return true
	default:
		return false
	}
}

func reconcileTxHash(order tyche.TradingOrder) string {
	if order.Status == tyche.TradingOrderStatusCancelPending && order.DYDXCancelTxHash != "" {
		return order.DYDXCancelTxHash
	}
	return order.DYDXOrderTxHash
}

func isReconcilableOrderStatus(status string) bool {
	switch status {
	case tyche.TradingOrderStatusBroadcasted,
		tyche.TradingOrderStatusSubmitted,
		tyche.TradingOrderStatusPartialFilled,
		tyche.TradingOrderStatusCancelPending,
		tyche.TradingOrderStatusCloseBroadcasted,
		tyche.TradingOrderStatusCloseSubmitted,
		tyche.TradingOrderStatusClosePartialFilled:
		return true
	default:
		return false
	}
}

func splitOrderRef(ref string) (string, string, bool) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == ':' {
			return ref[:i], ref[i+1:], ref[:i] != "" && ref[i+1:] != ""
		}
	}
	return "", "", false
}

type RedisSyncStore struct{}

func NewRedisSyncStore() *RedisSyncStore {
	return &RedisSyncStore{}
}

func (s *RedisSyncStore) SaveFill(ctx context.Context, fill tyche.TradingFill) error {
	return saveUniqueSyncEvent(fill.EventID, "fill", func(red redis.Conn, now int64) error {
		createdAt := fill.CreatedAt
		if createdAt == 0 {
			createdAt = now
		}
		if err := red.Send("HSET", keys.Fill(fill.EventID),
			"eventId", fill.EventID,
			"tradeHash", fill.TradeHash,
			"userId", fill.UserID,
			"orderId", fill.OrderID,
			"market", fill.Market,
			"asset", fill.Asset,
			"releasedMargin", fill.ReleasedMargin,
			"fee", fill.Fee,
			"realizedPnl", fill.RealizedPNL,
			"orderStatus", fill.OrderStatus,
			"liquidityRole", fill.LiquidityRole,
			"makerAddress", fill.MakerAddress,
			"makerNecoAddress", fill.MakerNECOAddress,
			"takerAddress", fill.TakerAddress,
			"takerNecoAddress", fill.TakerNECOAddress,
			"makerOrderId", fill.MakerOrderID,
			"takerOrderId", fill.TakerOrderID,
			"clobPairId", fill.ClobPairID,
			"fillAmount", fill.FillAmount,
			"makerFee", fill.MakerFee,
			"takerFee", fill.TakerFee,
			"makerOrderSide", fill.MakerOrderSide,
			"takerOrderSide", fill.TakerOrderSide,
			"makerOrderQuantums", fill.MakerOrderQuantums,
			"makerOrderSubticks", fill.MakerOrderSubticks,
			"takerOrderQuantums", fill.TakerOrderQuantums,
			"takerOrderSubticks", fill.TakerOrderSubticks,
			"createdAt", createdAt,
		); err != nil {
			return err
		}
		if fill.UserID == "" || fill.OrderID == "" {
			return nil
		}
		return red.Send("SADD", keys.OrderFills(fill.UserID, fill.OrderID), fill.EventID)
	})
}

func (s *RedisSyncStore) SavePosition(ctx context.Context, position tyche.TradingPosition) error {
	return saveUniqueSyncEvent(position.EventID, "position", func(red redis.Conn, now int64) error {
		positionKey := keys.PositionForSubaccount(position.UserID, position.Market, position.SubaccountNumber)
		if tradingPositionSizeIsZero(position.Size) {
			if err := red.Send("DEL", positionKey); err != nil {
				return err
			}
			return red.Send("SREM", keys.UserPositions(position.UserID), positionKey)
		}
		if err := red.Send("HSET", positionKey,
			"eventId", position.EventID,
			"userId", position.UserID,
			"market", position.Market,
			"subaccountNumber", position.SubaccountNumber,
			"size", position.Size,
			"entryPrice", position.EntryPrice,
			"markPrice", position.MarkPrice,
			"unrealizedPnl", position.UnrealizedPNL,
			"marginAsset", position.MarginAsset,
			"margin", position.Margin,
			"updatedAt", now,
		); err != nil {
			return err
		}
		return red.Send("SADD", keys.UserPositions(position.UserID), positionKey)
	})
}

func (s *RedisSyncStore) SaveOrderStatus(ctx context.Context, status tyche.TradingOrderStatusEvent) error {
	return saveUniqueSyncEvent(status.EventID, "order_status", func(red redis.Conn, now int64) error {
		if err := red.Send("HSET", keys.OrderStatus(status.EventID),
			"eventId", status.EventID,
			"userId", status.UserID,
			"orderId", status.OrderID,
			"status", status.Status,
			"reason", status.Reason,
			"refundAsset", status.RefundAsset,
			"refundAmount", status.RefundAmount,
			"createdAt", now,
		); err != nil {
			return err
		}
		return red.Send("SADD", keys.UserOrderStatuses(status.UserID), status.EventID)
	})
}

func saveUniqueSyncEvent(eventID, eventType string, enqueue func(redis.Conn, int64) error) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	eventKey := keys.Event(eventID)
	for attempt := 0; attempt < tradingSyncSaveMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", eventKey); err != nil {
			return err
		}
		exists, err := redis.Bool(red.Do("EXISTS", eventKey))
		if err != nil {
			_, _ = red.Do("UNWATCH")
			return err
		}
		if exists {
			_, _ = red.Do("UNWATCH")
			return tyche.ErrTradingSyncDuplicateEvent
		}

		now := time.Now().Unix()
		if err := red.Send("MULTI"); err != nil {
			_ = cancelTradingSyncWatchOrTransaction(red)
			return err
		}
		if err := red.Send("SET", eventKey, eventType); err != nil {
			_ = cancelTradingSyncWatchOrTransaction(red)
			return err
		}
		if err := enqueue(red, now); err != nil {
			_ = cancelTradingSyncWatchOrTransaction(red)
			return err
		}
		reply, err := red.Do("EXEC")
		if err != nil {
			return err
		}
		if reply != nil {
			return nil
		}
	}
	return errors.New("trading sync event save conflict")
}

func cancelTradingSyncWatchOrTransaction(red redis.Conn) error {
	if _, err := red.Do("DISCARD"); err == nil {
		return nil
	}
	_, err := red.Do("UNWATCH")
	return err
}

func (s *RedisSyncStore) GetFill(ctx context.Context, eventID string) (tyche.TradingFill, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Fill(eventID)))
	if err != nil {
		return tyche.TradingFill{}, false, err
	}
	if len(values) == 0 {
		return tyche.TradingFill{}, false, nil
	}
	var fill tyche.TradingFill
	if err := redis.ScanStruct(values, &fill); err != nil {
		return tyche.TradingFill{}, false, err
	}
	return fill, true, nil
}

func (s *RedisSyncStore) ListFillsByOrder(ctx context.Context, userID, orderID string) ([]tyche.TradingFill, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.OrderFills(userID, orderID)))
	if err != nil {
		return nil, err
	}
	fills := make([]tyche.TradingFill, 0, len(ids))
	for _, id := range ids {
		fill, ok, err := s.GetFill(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			fills = append(fills, fill)
		}
	}
	sort.Slice(fills, func(i, j int) bool {
		return fills[i].EventID < fills[j].EventID
	})
	return fills, nil
}

func (s *RedisSyncStore) GetOrderStatus(ctx context.Context, eventID string) (tyche.TradingOrderStatusEvent, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.OrderStatus(eventID)))
	if err != nil {
		return tyche.TradingOrderStatusEvent{}, false, err
	}
	if len(values) == 0 {
		return tyche.TradingOrderStatusEvent{}, false, nil
	}
	var status tyche.TradingOrderStatusEvent
	if err := redis.ScanStruct(values, &status); err != nil {
		return tyche.TradingOrderStatusEvent{}, false, err
	}
	return status, true, nil
}

func (s *RedisSyncStore) GetPosition(ctx context.Context, userID, market string) (tyche.TradingPosition, bool, error) {
	return s.GetPositionForSubaccount(ctx, userID, market, 0)
}

func (s *RedisSyncStore) GetPositionForSubaccount(ctx context.Context, userID, market string, subaccountNumber uint32) (tyche.TradingPosition, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	positionKey := keys.PositionForSubaccount(userID, market, subaccountNumber)
	values, err := redis.Values(red.Do("HGETALL", positionKey))
	if err != nil {
		return tyche.TradingPosition{}, false, err
	}
	if len(values) == 0 {
		if subaccountNumber == 0 {
			return tyche.TradingPosition{}, false, nil
		}
		positionKey = keys.Position(userID, market)
		values, err = redis.Values(red.Do("HGETALL", positionKey))
		if err != nil {
			return tyche.TradingPosition{}, false, err
		}
		if len(values) == 0 {
			return tyche.TradingPosition{}, false, nil
		}
	}
	var position tyche.TradingPosition
	if err := redis.ScanStruct(values, &position); err != nil {
		return tyche.TradingPosition{}, false, err
	}
	if position.SubaccountNumber != 0 && position.SubaccountNumber != subaccountNumber {
		return tyche.TradingPosition{}, false, nil
	}
	if subaccountNumber != 0 {
		position.SubaccountNumber = subaccountNumber
	}
	if tradingPositionSizeIsZero(position.Size) {
		_, _ = red.Do("DEL", positionKey)
		_, _ = red.Do("SREM", keys.UserPositions(userID), positionKey)
		return tyche.TradingPosition{}, false, nil
	}
	return position, true, nil
}

func (s *RedisSyncStore) ListPositionsByUser(ctx context.Context, userID string) ([]tyche.TradingPosition, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	positionKeys, err := redis.Strings(red.Do("SMEMBERS", keys.UserPositions(userID)))
	if err != nil {
		return nil, err
	}
	positions := make([]tyche.TradingPosition, 0, len(positionKeys))
	for _, key := range positionKeys {
		values, err := redis.Values(red.Do("HGETALL", key))
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			_, _ = red.Do("SREM", keys.UserPositions(userID), key)
			continue
		}
		var position tyche.TradingPosition
		if err := redis.ScanStruct(values, &position); err != nil {
			return nil, err
		}
		if tradingPositionSizeIsZero(position.Size) {
			_, _ = red.Do("DEL", key)
			_, _ = red.Do("SREM", keys.UserPositions(userID), key)
			continue
		}
		if position.UserID == userID {
			positions = append(positions, position)
		}
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Market == positions[j].Market {
			return positions[i].SubaccountNumber < positions[j].SubaccountNumber
		}
		return positions[i].Market < positions[j].Market
	})
	return positions, nil
}

func tradingPositionSizeIsZero(size string) bool {
	trimmed := strings.TrimSpace(size)
	if trimmed == "" {
		return false
	}
	value, ok := new(big.Rat).SetString(trimmed)
	return ok && value.Sign() == 0
}

func (s *RedisSyncStore) ListOrderStatuses(ctx context.Context, userID string) ([]tyche.TradingOrderStatusEvent, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.UserOrderStatuses(userID)))
	if err != nil {
		return nil, err
	}
	statuses := make([]tyche.TradingOrderStatusEvent, 0, len(ids))
	for _, id := range ids {
		status, ok, err := s.GetOrderStatus(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			statuses = append(statuses, status)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].EventID < statuses[j].EventID
	})
	return statuses, nil
}

func (s *RedisSyncStore) GetTradingSyncCursor(ctx context.Context) (tyche.TradingSyncCursor, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.SyncCursor()))
	if err != nil {
		return tyche.TradingSyncCursor{}, err
	}
	if len(values) == 0 {
		return tyche.TradingSyncCursor{}, nil
	}
	var cursor tyche.TradingSyncCursor
	if err := redis.ScanStruct(values, &cursor); err != nil {
		return tyche.TradingSyncCursor{}, err
	}
	return cursor, nil
}

func (s *RedisSyncStore) SaveTradingSyncCursor(ctx context.Context, cursor tyche.TradingSyncCursor) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("HSET", keys.SyncCursor(),
		"fillsCursor", cursor.FillsCursor,
		"orderStatusCursor", cursor.OrderStatusCursor,
		"positionsCursor", cursor.PositionsCursor,
		"updatedAt", time.Now().Unix(),
	)
	return err
}

type RedisTPSLStore struct{}

func NewRedisTPSLStore() *RedisTPSLStore {
	return &RedisTPSLStore{}
}

func (s *RedisTPSLStore) CreateTPSLIfAbsent(ctx context.Context, entry tyche.TradingTPSL) (tyche.TradingTPSL, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	key := keys.TPSL(entry.UserID, entry.ID)
	for attempt := 0; attempt < tradingCreateMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", key); err != nil {
			return tyche.TradingTPSL{}, false, err
		}
		exists, err := redis.Bool(red.Do("EXISTS", key))
		if err != nil {
			_, _ = red.Do("UNWATCH")
			return tyche.TradingTPSL{}, false, err
		}
		if exists {
			_, _ = red.Do("UNWATCH")
			existing, ok, err := s.GetTPSLByID(ctx, entry.UserID, entry.ID)
			return existing, false, errIfMissing(ok, err, "trading tpsl index points to missing entry")
		}
		if err := red.Send("MULTI"); err != nil {
			_ = cancelTradingSyncWatchOrTransaction(red)
			return tyche.TradingTPSL{}, false, err
		}
		if err := queueSaveTPSL(red, entry); err != nil {
			_ = cancelTradingSyncWatchOrTransaction(red)
			return tyche.TradingTPSL{}, false, err
		}
		reply, err := red.Do("EXEC")
		if err != nil {
			return tyche.TradingTPSL{}, false, err
		}
		if reply != nil {
			return entry, true, nil
		}
	}
	return tyche.TradingTPSL{}, false, errors.New("trading tpsl create conflict")
}

func (s *RedisTPSLStore) SaveTPSL(ctx context.Context, entry tyche.TradingTPSL) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	if err := red.Send("MULTI"); err != nil {
		return err
	}
	if err := queueSaveTPSL(red, entry); err != nil {
		return err
	}
	if _, err := red.Do("EXEC"); err != nil {
		return err
	}
	return nil
}

func (s *RedisTPSLStore) GetTPSLByID(ctx context.Context, userID, id string) (tyche.TradingTPSL, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.TPSL(userID, id)))
	if err != nil {
		return tyche.TradingTPSL{}, false, err
	}
	if len(values) == 0 {
		return tyche.TradingTPSL{}, false, nil
	}
	var entry tyche.TradingTPSL
	if err := redis.ScanStruct(values, &entry); err != nil {
		return tyche.TradingTPSL{}, false, err
	}
	return entry, true, nil
}

func (s *RedisTPSLStore) ListTPSLByUser(ctx context.Context, userID string) ([]tyche.TradingTPSL, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.UserTPSL(userID)))
	if err != nil {
		return nil, err
	}
	entries := make([]tyche.TradingTPSL, 0, len(ids))
	for _, id := range ids {
		entry, ok, err := s.GetTPSLByID(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if ok {
			entries = append(entries, entry)
		}
	}
	sortTPSLStoreEntries(entries)
	return entries, nil
}

func (s *RedisTPSLStore) ListTPSLByGroup(ctx context.Context, userID, groupID string) ([]tyche.TradingTPSL, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	ids, err := redis.Strings(red.Do("SMEMBERS", keys.GroupTPSL(userID, groupID)))
	if err != nil {
		return nil, err
	}
	entries := make([]tyche.TradingTPSL, 0, len(ids))
	for _, id := range ids {
		entry, ok, err := s.GetTPSLByID(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if ok {
			entries = append(entries, entry)
		}
	}
	sortTPSLStoreEntries(entries)
	return entries, nil
}

func (s *RedisTPSLStore) ListWaitingTPSL(ctx context.Context, limit int) ([]tyche.TradingTPSL, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	if limit <= 0 {
		limit = 100
	}
	refs, err := redis.Strings(red.Do("SMEMBERS", keys.WaitingTPSL()))
	if err != nil {
		return nil, err
	}
	entries := make([]tyche.TradingTPSL, 0, len(refs))
	for _, ref := range refs {
		userID, id, ok := splitOrderRef(ref)
		if !ok {
			_, _ = red.Do("SREM", keys.WaitingTPSL(), ref)
			continue
		}
		entry, ok, err := s.GetTPSLByID(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if !ok || entry.Status != tyche.TradingTPSLStatusWaitingTrigger {
			_, _ = red.Do("SREM", keys.WaitingTPSL(), ref)
			continue
		}
		entries = append(entries, entry)
	}
	sortTPSLStoreEntries(entries)
	if len(entries) > limit {
		return entries[:limit], nil
	}
	return entries, nil
}

func queueSaveTPSL(red redis.Conn, entry tyche.TradingTPSL) error {
	ref := entry.UserID + ":" + entry.ID
	if err := red.Send("HSET", keys.TPSL(entry.UserID, entry.ID),
		"id", entry.ID,
		"groupId", entry.GroupID,
		"userId", entry.UserID,
		"dydxAccount", entry.DYDXAccount,
		"market", entry.Market,
		"kind", entry.Kind,
		"positionSide", entry.PositionSide,
		"triggerPrice", entry.TriggerPrice,
		"size", entry.Size,
		"closeClientOrderId", entry.CloseClientOrderID,
		"closeOrderId", entry.CloseOrderID,
		"slippageBps", entry.SlippageBps,
		"status", entry.Status,
		"reason", entry.Reason,
		"createdAt", entry.CreatedAt,
		"updatedAt", entry.UpdatedAt,
		"triggeredAt", entry.TriggeredAt,
	); err != nil {
		return err
	}
	if err := red.Send("SADD", keys.UserTPSL(entry.UserID), entry.ID); err != nil {
		return err
	}
	if err := red.Send("SADD", keys.GroupTPSL(entry.UserID, entry.GroupID), entry.ID); err != nil {
		return err
	}
	if entry.Status == tyche.TradingTPSLStatusWaitingTrigger {
		return red.Send("SADD", keys.WaitingTPSL(), ref)
	}
	return red.Send("SREM", keys.WaitingTPSL(), ref)
}

func sortTPSLStoreEntries(entries []tyche.TradingTPSL) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt == entries[j].CreatedAt {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].CreatedAt < entries[j].CreatedAt
	})
}

func errIfMissing(ok bool, err error, message string) error {
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(message)
	}
	return nil
}
