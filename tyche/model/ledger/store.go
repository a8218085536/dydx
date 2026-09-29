package ledger

import (
	"context"
	"errors"
	"ethgo/model"
	"ethgo/tyche"
	"math/big"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/google/uuid"
)

type RedisStore struct{}

const ledgerAtomicMaxAttempts = 3

func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) CreditDeposit(ctx context.Context, request tyche.LedgerDepositRequest) (tyche.LedgerDeposit, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	refKey := keys.Ref(request.RefID)
	balanceKey := keys.Balance(request.UserID, request.Asset)
	amount, ok := parseAmount(request.Amount)
	if !ok {
		return tyche.LedgerDeposit{}, errors.New("invalid amount")
	}

	deposit := tyche.LedgerDeposit{
		ID:     uuid.NewString(),
		UserID: request.UserID,
		Asset:  request.Asset,
		Amount: request.Amount,
		RefID:  request.RefID,
		Status: tyche.LedgerDepositStatusCredited,
	}

	for attempt := 0; attempt < ledgerAtomicMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", refKey, balanceKey); err != nil {
			return tyche.LedgerDeposit{}, err
		}
		exists, err := redis.Bool(red.Do("EXISTS", refKey))
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerDeposit{}, err
		}
		if exists {
			_ = unwatch(red)
			return tyche.LedgerDeposit{}, tyche.ErrLedgerDuplicateReference
		}

		amounts, err := getBalanceAmounts(red, balanceKey)
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerDeposit{}, err
		}
		amounts.Available.Add(amounts.Available, amount)
		committed, err := s.saveDepositCredit(red, balanceKey, amounts, deposit)
		if err != nil {
			return tyche.LedgerDeposit{}, err
		}
		if committed {
			return deposit, nil
		}
	}
	return tyche.LedgerDeposit{}, errors.New("ledger deposit conflict")
}

func (s *RedisStore) FreezeWithdrawal(ctx context.Context, request tyche.LedgerWithdrawalRequest) (tyche.LedgerWithdrawal, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	balanceKey := keys.Balance(request.UserID, request.Asset)
	amounts, err := getBalanceAmounts(red, balanceKey)
	if err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	amount, ok := parseAmount(request.Amount)
	if !ok {
		return tyche.LedgerWithdrawal{}, errors.New("invalid amount")
	}
	if amounts.Available.Cmp(amount) < 0 {
		return tyche.LedgerWithdrawal{}, tyche.ErrLedgerInsufficientBalance
	}
	amounts.Available.Sub(amounts.Available, amount)
	amounts.Frozen.Add(amounts.Frozen, amount)

	withdrawal := tyche.LedgerWithdrawal{
		ID:        uuid.NewString(),
		UserID:    request.UserID,
		Asset:     request.Asset,
		Amount:    request.Amount,
		ToAddress: request.ToAddress,
		Status:    tyche.LedgerWithdrawalStatusFrozen,
	}

	now := time.Now().Unix()
	red.Send("MULTI")
	red.Send("HSET", balanceKey,
		"userId", request.UserID,
		"asset", request.Asset,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"updatedAt", now,
	)
	red.Send("HSET", keys.Withdrawal(withdrawal.ID),
		"id", withdrawal.ID,
		"userId", withdrawal.UserID,
		"asset", withdrawal.Asset,
		"amount", withdrawal.Amount,
		"toAddress", withdrawal.ToAddress,
		"status", withdrawal.Status,
		"createdAt", now,
		"updatedAt", now,
	)
	red.Send("XADD", keys.Journal(), "*",
		"id", uuid.NewString(),
		"userId", request.UserID,
		"asset", request.Asset,
		"type", tyche.LedgerJournalTypeWithdrawalFreeze,
		"amount", request.Amount,
		"refId", withdrawal.ID,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"createdAt", now,
	)
	red.Send("EXEC")
	if err := red.Flush(); err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	return withdrawal, nil
}

func (s *RedisStore) MarkWithdrawalPaid(ctx context.Context, result tyche.LedgerWithdrawalResult) (tyche.LedgerWithdrawal, error) {
	withdrawal, err := s.getWithdrawal(result.WithdrawalID)
	if err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	balanceKey := keys.Balance(withdrawal.UserID, withdrawal.Asset)
	amounts, err := loadBalanceAmounts(balanceKey)
	if err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	amount, ok := parseAmount(withdrawal.Amount)
	if !ok {
		return tyche.LedgerWithdrawal{}, errors.New("invalid amount")
	}
	if amounts.Frozen.Cmp(amount) < 0 {
		return tyche.LedgerWithdrawal{}, tyche.ErrLedgerInsufficientBalance
	}
	amounts.Frozen.Sub(amounts.Frozen, amount)
	withdrawal.Status = tyche.LedgerWithdrawalStatusPaid
	withdrawal.TxHash = result.TxHash
	return s.saveWithdrawalResult(balanceKey, amounts, withdrawal, tyche.LedgerJournalTypeWithdrawalPaid, result.TxHash)
}

func (s *RedisStore) GetWithdrawal(ctx context.Context, id string) (tyche.LedgerWithdrawal, error) {
	return s.getWithdrawal(id)
}

func (s *RedisStore) ListWithdrawalsByStatus(ctx context.Context, status string, limit int) ([]tyche.LedgerWithdrawal, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	if limit <= 0 {
		limit = 1000
	}
	out := make([]tyche.LedgerWithdrawal, 0)
	cursor := 0
	for {
		values, err := redis.Values(red.Do("SCAN", cursor, "MATCH", keys.Withdrawal("*"), "COUNT", 100))
		if err != nil {
			return nil, err
		}
		var redisKeys []string
		if _, err := redis.Scan(values, &cursor, &redisKeys); err != nil {
			return nil, err
		}
		for _, key := range redisKeys {
			hash, err := redis.Values(red.Do("HGETALL", key))
			if err != nil {
				return nil, err
			}
			if len(hash) == 0 {
				continue
			}
			var withdrawal tyche.LedgerWithdrawal
			if err := redis.ScanStruct(hash, &withdrawal); err != nil {
				return nil, err
			}
			if withdrawal.Status != status {
				continue
			}
			out = append(out, withdrawal)
			if len(out) >= limit {
				return out, nil
			}
		}
		if cursor == 0 {
			return out, nil
		}
	}
}

func (s *RedisStore) RepairFrozenBalance(ctx context.Context, request tyche.LedgerFrozenBalanceRepairRequest) (tyche.LedgerFrozenBalanceRepairReport, error) {
	if request.UserID == "" || request.Asset == "" {
		return tyche.LedgerFrozenBalanceRepairReport{}, errors.New("userId and asset are required")
	}
	var red = model.RedisPool.Get()
	defer red.Close()

	expected := big.NewInt(0)
	cursor := 0
	for {
		values, err := redis.Values(red.Do("SCAN", cursor, "MATCH", keys.Withdrawal("*"), "COUNT", 100))
		if err != nil {
			return tyche.LedgerFrozenBalanceRepairReport{}, err
		}
		var redisKeys []string
		if _, err := redis.Scan(values, &cursor, &redisKeys); err != nil {
			return tyche.LedgerFrozenBalanceRepairReport{}, err
		}
		for _, key := range redisKeys {
			hash, err := redis.Values(red.Do("HGETALL", key))
			if err != nil {
				return tyche.LedgerFrozenBalanceRepairReport{}, err
			}
			if len(hash) == 0 {
				continue
			}
			var withdrawal tyche.LedgerWithdrawal
			if err := redis.ScanStruct(hash, &withdrawal); err != nil {
				return tyche.LedgerFrozenBalanceRepairReport{}, err
			}
			if withdrawal.UserID != request.UserID || withdrawal.Asset != request.Asset || withdrawal.Status != tyche.LedgerWithdrawalStatusFrozen {
				continue
			}
			amount, ok := parseAmount(withdrawal.Amount)
			if !ok {
				return tyche.LedgerFrozenBalanceRepairReport{}, errors.New("invalid withdrawal amount")
			}
			expected.Add(expected, amount)
		}
		if cursor == 0 {
			break
		}
	}

	balanceKey := keys.Balance(request.UserID, request.Asset)
	amounts, err := getBalanceAmounts(red, balanceKey)
	if err != nil {
		return tyche.LedgerFrozenBalanceRepairReport{}, err
	}
	repair := tyche.LedgerFrozenBalanceRepair{
		UserID:         request.UserID,
		Asset:          request.Asset,
		BeforeFrozen:   amounts.Frozen.String(),
		ExpectedFrozen: expected.String(),
		Changed:        amounts.Frozen.Cmp(expected) != 0,
	}
	report := tyche.LedgerFrozenBalanceRepairReport{
		Checked:  1,
		Balances: []tyche.LedgerFrozenBalanceRepair{repair},
	}
	if !repair.Changed {
		return report, nil
	}
	report.Changed = 1
	if request.Apply {
		if err := saveFrozenBalanceRepair(red, balanceKey, request.UserID, request.Asset, amounts, expected); err != nil {
			return tyche.LedgerFrozenBalanceRepairReport{}, err
		}
		report.Applied = 1
		report.Balances[0].Applied = true
	}
	return report, nil
}

func (s *RedisStore) MarkWithdrawalFailed(ctx context.Context, result tyche.LedgerWithdrawalResult) (tyche.LedgerWithdrawal, error) {
	withdrawal, err := s.getWithdrawal(result.WithdrawalID)
	if err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	balanceKey := keys.Balance(withdrawal.UserID, withdrawal.Asset)
	amounts, err := loadBalanceAmounts(balanceKey)
	if err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	amount, ok := parseAmount(withdrawal.Amount)
	if !ok {
		return tyche.LedgerWithdrawal{}, errors.New("invalid amount")
	}
	if amounts.Frozen.Cmp(amount) < 0 {
		return tyche.LedgerWithdrawal{}, tyche.ErrLedgerInsufficientBalance
	}
	amounts.Available.Add(amounts.Available, amount)
	amounts.Frozen.Sub(amounts.Frozen, amount)
	withdrawal.Status = tyche.LedgerWithdrawalStatusFailed
	withdrawal.Reason = result.Reason
	return s.saveWithdrawalResult(balanceKey, amounts, withdrawal, tyche.LedgerJournalTypeWithdrawalFailed, result.Reason)
}

func (s *RedisStore) ChargeAvailable(ctx context.Context, request tyche.LedgerChargeRequest) (tyche.LedgerBalance, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	refKey := keys.Ref(request.RefID)
	balanceKey := keys.Balance(request.UserID, request.Asset)
	for attempt := 0; attempt < ledgerAtomicMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", refKey, balanceKey); err != nil {
			return tyche.LedgerBalance{}, err
		}
		exists, err := redis.Bool(red.Do("EXISTS", refKey))
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, err
		}
		if exists {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, tyche.ErrLedgerDuplicateReference
		}

		amounts, err := getBalanceAmounts(red, balanceKey)
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, err
		}
		amount, ok := parseAmount(request.Amount)
		if !ok {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, errors.New("invalid amount")
		}
		if amounts.Available.Cmp(amount) < 0 {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, tyche.ErrLedgerInsufficientBalance
		}
		amounts.Available.Sub(amounts.Available, amount)
		balance, committed, err := s.saveAvailableCharge(red, balanceKey, request.UserID, request.Asset, amounts, request.Amount, request.RefID)
		if err != nil {
			return tyche.LedgerBalance{}, err
		}
		if committed {
			return balance, nil
		}
	}
	return tyche.LedgerBalance{}, errors.New("ledger charge conflict")
}

func (s *RedisStore) RefundAvailable(ctx context.Context, request tyche.LedgerRefundRequest) (tyche.LedgerBalance, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	refKey := keys.Ref(request.RefID)
	balanceKey := keys.Balance(request.UserID, request.Asset)
	amount, ok := parseAmount(request.Amount)
	if !ok {
		return tyche.LedgerBalance{}, errors.New("invalid amount")
	}

	for attempt := 0; attempt < ledgerAtomicMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", refKey, balanceKey); err != nil {
			return tyche.LedgerBalance{}, err
		}
		exists, err := redis.Bool(red.Do("EXISTS", refKey))
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, err
		}
		if exists {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, tyche.ErrLedgerDuplicateReference
		}

		amounts, err := getBalanceAmounts(red, balanceKey)
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, err
		}
		amounts.Available.Add(amounts.Available, amount)
		balance, committed, err := s.saveBalanceJournal(red, balanceKey, request.UserID, request.Asset, amounts, request.Amount, request.RefID, tyche.LedgerJournalTypeAvailableRefund)
		if err != nil {
			return tyche.LedgerBalance{}, err
		}
		if committed {
			return balance, nil
		}
	}
	return tyche.LedgerBalance{}, errors.New("ledger refund conflict")
}

func (s *RedisStore) ApplyTradingSettlement(ctx context.Context, request tyche.LedgerTradingSettlementRequest) (tyche.LedgerBalance, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	refKey := keys.Ref(request.RefID)
	balanceKey := keys.Balance(request.UserID, request.Asset)
	releasedMargin, ok := parseAmount(defaultAmount(request.ReleasedMargin))
	if !ok {
		return tyche.LedgerBalance{}, errors.New("invalid released margin")
	}
	fee, ok := parseAmount(defaultAmount(request.Fee))
	if !ok {
		return tyche.LedgerBalance{}, errors.New("invalid fee")
	}
	realizedPNL, ok := parseSignedAmount(defaultAmount(request.RealizedPNL))
	if !ok {
		return tyche.LedgerBalance{}, errors.New("invalid realized pnl")
	}

	for attempt := 0; attempt < ledgerAtomicMaxAttempts; attempt++ {
		if _, err := red.Do("WATCH", refKey, balanceKey); err != nil {
			return tyche.LedgerBalance{}, err
		}
		exists, err := redis.Bool(red.Do("EXISTS", refKey))
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, err
		}
		if exists {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, tyche.ErrLedgerDuplicateReference
		}

		amounts, err := getBalanceAmounts(red, balanceKey)
		if err != nil {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, err
		}
		next := new(big.Int).Add(amounts.Available, releasedMargin)
		next.Add(next, realizedPNL)
		next.Sub(next, fee)
		if next.Sign() < 0 {
			_ = unwatch(red)
			return tyche.LedgerBalance{}, tyche.ErrLedgerInsufficientBalance
		}
		amounts.Available = next
		balance, committed, err := s.saveBalanceJournal(red, balanceKey, request.UserID, request.Asset, amounts, request.ReleasedMargin, request.RefID, tyche.LedgerJournalTypeTradingSettle)
		if err != nil {
			return tyche.LedgerBalance{}, err
		}
		if committed {
			return balance, nil
		}
	}
	return tyche.LedgerBalance{}, errors.New("ledger trading settlement conflict")
}

func (s *RedisStore) GetBalance(ctx context.Context, userID, asset string) (tyche.LedgerBalance, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	amounts, err := getBalanceAmounts(red, keys.Balance(userID, asset))
	if err != nil {
		return tyche.LedgerBalance{}, err
	}
	return tyche.LedgerBalance{
		UserID:    userID,
		Asset:     asset,
		Available: amounts.Available.String(),
		Frozen:    amounts.Frozen.String(),
	}, nil
}

func (s *RedisStore) HasReference(ctx context.Context, refID string) (bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	return redis.Bool(red.Do("EXISTS", keys.Ref(refID)))
}

func (s *RedisStore) getWithdrawal(id string) (tyche.LedgerWithdrawal, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Withdrawal(id)))
	if err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	if len(values) == 0 {
		return tyche.LedgerWithdrawal{}, tyche.ErrLedgerWithdrawalNotFound
	}

	var withdrawal tyche.LedgerWithdrawal
	if err := redis.ScanStruct(values, &withdrawal); err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	return withdrawal, nil
}

func (s *RedisStore) saveWithdrawalResult(balanceKey string, amounts ledgerAmounts, withdrawal tyche.LedgerWithdrawal, journalType, detail string) (tyche.LedgerWithdrawal, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	now := time.Now().Unix()
	red.Send("MULTI")
	red.Send("HSET", balanceKey,
		"userId", withdrawal.UserID,
		"asset", withdrawal.Asset,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"updatedAt", now,
	)
	red.Send("HSET", keys.Withdrawal(withdrawal.ID),
		"status", withdrawal.Status,
		"txHash", withdrawal.TxHash,
		"reason", withdrawal.Reason,
		"updatedAt", now,
	)
	red.Send("XADD", keys.Journal(), "*",
		"id", uuid.NewString(),
		"userId", withdrawal.UserID,
		"asset", withdrawal.Asset,
		"type", journalType,
		"amount", withdrawal.Amount,
		"refId", withdrawal.ID,
		"detail", detail,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"createdAt", now,
	)
	red.Send("EXEC")
	if err := red.Flush(); err != nil {
		return tyche.LedgerWithdrawal{}, err
	}
	return withdrawal, nil
}

type ledgerAmounts struct {
	Available *big.Int
	Frozen    *big.Int
}

func getBalanceAmounts(red redis.Conn, key string) (ledgerAmounts, error) {
	values, err := redis.Strings(red.Do("HMGET", key, "available", "frozen"))
	if err != nil {
		return ledgerAmounts{}, err
	}

	available := big.NewInt(0)
	if len(values) > 0 && values[0] != "" {
		var ok bool
		available, ok = parseAmount(values[0])
		if !ok {
			return ledgerAmounts{}, errors.New("invalid available balance")
		}
	}

	frozen := big.NewInt(0)
	if len(values) > 1 && values[1] != "" {
		var ok bool
		frozen, ok = parseAmount(values[1])
		if !ok {
			return ledgerAmounts{}, errors.New("invalid frozen balance")
		}
	}

	return ledgerAmounts{
		Available: available,
		Frozen:    frozen,
	}, nil
}

func loadBalanceAmounts(key string) (ledgerAmounts, error) {
	var red = model.RedisPool.Get()
	defer red.Close()
	return getBalanceAmounts(red, key)
}

func (s *RedisStore) saveAvailableCharge(red redis.Conn, balanceKey, userID, asset string, amounts ledgerAmounts, amount, refID string) (tyche.LedgerBalance, bool, error) {
	now := time.Now().Unix()
	if err := red.Send("MULTI"); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	if err := red.Send("HSET", balanceKey,
		"userId", userID,
		"asset", asset,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"updatedAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	if err := red.Send("SET", keys.Ref(refID), "1"); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	if err := red.Send("XADD", keys.Journal(), "*",
		"id", uuid.NewString(),
		"userId", userID,
		"asset", asset,
		"type", tyche.LedgerJournalTypeAvailableCharge,
		"amount", amount,
		"refId", refID,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"createdAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	reply, err := red.Do("EXEC")
	if err != nil {
		return tyche.LedgerBalance{}, false, err
	}
	if reply == nil {
		return tyche.LedgerBalance{}, false, nil
	}
	return tyche.LedgerBalance{
		UserID:    userID,
		Asset:     asset,
		Available: amounts.Available.String(),
		Frozen:    amounts.Frozen.String(),
	}, true, nil
}

func (s *RedisStore) saveDepositCredit(red redis.Conn, balanceKey string, amounts ledgerAmounts, deposit tyche.LedgerDeposit) (bool, error) {
	now := time.Now().Unix()
	if err := red.Send("MULTI"); err != nil {
		_ = cancelWatchOrTransaction(red)
		return false, err
	}
	if err := red.Send("HSET", balanceKey,
		"userId", deposit.UserID,
		"asset", deposit.Asset,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"updatedAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return false, err
	}
	if err := red.Send("SET", keys.Ref(deposit.RefID), deposit.ID); err != nil {
		_ = cancelWatchOrTransaction(red)
		return false, err
	}
	if err := red.Send("HSET", keys.Deposit(deposit.ID),
		"id", deposit.ID,
		"userId", deposit.UserID,
		"asset", deposit.Asset,
		"amount", deposit.Amount,
		"refId", deposit.RefID,
		"status", deposit.Status,
		"createdAt", now,
		"updatedAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return false, err
	}
	if err := red.Send("XADD", keys.Journal(), "*",
		"id", uuid.NewString(),
		"userId", deposit.UserID,
		"asset", deposit.Asset,
		"type", tyche.LedgerJournalTypeDepositCredit,
		"amount", deposit.Amount,
		"refId", deposit.RefID,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"createdAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return false, err
	}
	reply, err := red.Do("EXEC")
	if err != nil {
		return false, err
	}
	return reply != nil, nil
}

func (s *RedisStore) saveBalanceJournal(red redis.Conn, balanceKey, userID, asset string, amounts ledgerAmounts, amount, refID, journalType string) (tyche.LedgerBalance, bool, error) {
	now := time.Now().Unix()
	if err := red.Send("MULTI"); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	if err := red.Send("HSET", balanceKey,
		"userId", userID,
		"asset", asset,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"updatedAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	if err := red.Send("SET", keys.Ref(refID), "1"); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	if err := red.Send("XADD", keys.Journal(), "*",
		"id", uuid.NewString(),
		"userId", userID,
		"asset", asset,
		"type", journalType,
		"amount", amount,
		"refId", refID,
		"available", amounts.Available.String(),
		"frozen", amounts.Frozen.String(),
		"createdAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return tyche.LedgerBalance{}, false, err
	}
	reply, err := red.Do("EXEC")
	if err != nil {
		return tyche.LedgerBalance{}, false, err
	}
	if reply == nil {
		return tyche.LedgerBalance{}, false, nil
	}
	return tyche.LedgerBalance{
		UserID:    userID,
		Asset:     asset,
		Available: amounts.Available.String(),
		Frozen:    amounts.Frozen.String(),
	}, true, nil
}

func unwatch(red redis.Conn) error {
	_, err := red.Do("UNWATCH")
	return err
}

func cancelWatchOrTransaction(red redis.Conn) error {
	if _, err := red.Do("DISCARD"); err == nil {
		return nil
	}
	return unwatch(red)
}

func parseAmount(value string) (*big.Int, bool) {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok || amount.Sign() < 0 {
		return nil, false
	}
	return amount, true
}

func parseSignedAmount(value string) (*big.Int, bool) {
	amount, ok := new(big.Int).SetString(value, 10)
	return amount, ok
}

func defaultAmount(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func saveFrozenBalanceRepair(red redis.Conn, balanceKey, userID, asset string, amounts ledgerAmounts, expected *big.Int) error {
	now := time.Now().Unix()
	diff := new(big.Int).Sub(expected, amounts.Frozen)
	if err := red.Send("MULTI"); err != nil {
		return err
	}
	if err := red.Send("HSET", balanceKey,
		"userId", userID,
		"asset", asset,
		"available", amounts.Available.String(),
		"frozen", expected.String(),
		"updatedAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return err
	}
	if err := red.Send("XADD", keys.Journal(), "*",
		"id", uuid.NewString(),
		"userId", userID,
		"asset", asset,
		"type", tyche.LedgerJournalTypeFrozenRepair,
		"amount", diff.String(),
		"refId", "ledger-frozen-repair:"+userID+":"+asset,
		"detail", "ledger frozen balance repaired from frozen withdrawals",
		"available", amounts.Available.String(),
		"frozen", expected.String(),
		"createdAt", now,
	); err != nil {
		_ = cancelWatchOrTransaction(red)
		return err
	}
	_, err := red.Do("EXEC")
	return err
}
