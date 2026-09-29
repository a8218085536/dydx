package tyche

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
)

const (
	LedgerDepositStatusCredited = "credited"

	LedgerWithdrawalStatusFrozen = "frozen"
	LedgerWithdrawalStatusPaid   = "paid"
	LedgerWithdrawalStatusFailed = "failed"

	LedgerJournalTypeDepositCredit    = "deposit_credit"
	LedgerJournalTypeWithdrawalFreeze = "withdrawal_freeze"
	LedgerJournalTypeWithdrawalPaid   = "withdrawal_paid"
	LedgerJournalTypeWithdrawalFailed = "withdrawal_failed"
	LedgerJournalTypeAvailableCharge  = "available_charge"
	LedgerJournalTypeAvailableRefund  = "available_refund"
	LedgerJournalTypeTradingSettle    = "trading_settle"
	LedgerJournalTypeFrozenRepair     = "frozen_repair"
)

var (
	ErrLedgerDuplicateReference  = errors.New("ledger duplicate reference")
	ErrLedgerInsufficientBalance = errors.New("ledger insufficient balance")
	ErrLedgerWithdrawalNotFound  = errors.New("ledger withdrawal not found")
)

type LedgerBalance struct {
	UserID    string `json:"userId"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

type LedgerDeposit struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RefID  string `json:"refId"`
	Status string `json:"status"`
}

type LedgerWithdrawal struct {
	ID        string `json:"id" redis:"id"`
	UserID    string `json:"userId" redis:"userId"`
	Asset     string `json:"asset" redis:"asset"`
	Amount    string `json:"amount" redis:"amount"`
	ToAddress string `json:"toAddress" redis:"toAddress"`
	Status    string `json:"status" redis:"status"`
	TxHash    string `json:"txHash,omitempty" redis:"txHash"`
	Reason    string `json:"reason,omitempty" redis:"reason"`
	CreatedAt int64  `json:"createdAt,omitempty" redis:"createdAt"`
	UpdatedAt int64  `json:"updatedAt,omitempty" redis:"updatedAt"`
}

type LedgerJournal struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Asset     string `json:"asset"`
	Type      string `json:"type"`
	Amount    string `json:"amount"`
	RefID     string `json:"refId"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

type LedgerDepositRequest struct {
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RefID  string `json:"refId"`
}

type LedgerWithdrawalRequest struct {
	UserID    string `json:"userId"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	ToAddress string `json:"toAddress"`
}

type LedgerWithdrawalResult struct {
	WithdrawalID string `json:"withdrawalId"`
	TxHash       string `json:"txHash"`
	Reason       string `json:"reason"`
}

type LedgerChargeRequest struct {
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RefID  string `json:"refId"`
}

type LedgerRefundRequest struct {
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RefID  string `json:"refId"`
}

type LedgerTradingSettlementRequest struct {
	UserID         string `json:"userId"`
	Asset          string `json:"asset"`
	ReleasedMargin string `json:"releasedMargin"`
	Fee            string `json:"fee"`
	RealizedPNL    string `json:"realizedPnl"`
	RefID          string `json:"refId"`
}

type LedgerFrozenBalanceRepair struct {
	UserID         string `json:"userId"`
	Asset          string `json:"asset"`
	BeforeFrozen   string `json:"beforeFrozen"`
	ExpectedFrozen string `json:"expectedFrozen"`
	Changed        bool   `json:"changed"`
	Applied        bool   `json:"applied"`
}

type LedgerFrozenBalanceRepairRequest struct {
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
	Apply  bool   `json:"apply"`
}

type LedgerFrozenBalanceRepairReport struct {
	Checked  int                         `json:"checked"`
	Changed  int                         `json:"changed"`
	Applied  int                         `json:"applied"`
	Balances []LedgerFrozenBalanceRepair `json:"balances"`
}

type LedgerStore interface {
	CreditDeposit(context.Context, LedgerDepositRequest) (LedgerDeposit, error)
	FreezeWithdrawal(context.Context, LedgerWithdrawalRequest) (LedgerWithdrawal, error)
	GetWithdrawal(context.Context, string) (LedgerWithdrawal, error)
	MarkWithdrawalPaid(context.Context, LedgerWithdrawalResult) (LedgerWithdrawal, error)
	MarkWithdrawalFailed(context.Context, LedgerWithdrawalResult) (LedgerWithdrawal, error)
	ChargeAvailable(context.Context, LedgerChargeRequest) (LedgerBalance, error)
	RefundAvailable(context.Context, LedgerRefundRequest) (LedgerBalance, error)
	ApplyTradingSettlement(context.Context, LedgerTradingSettlementRequest) (LedgerBalance, error)
	GetBalance(context.Context, string, string) (LedgerBalance, error)
	HasReference(context.Context, string) (bool, error)
}

type LedgerService struct {
	store LedgerStore
}

func NewLedgerService(store LedgerStore) *LedgerService {
	return &LedgerService{store: store}
}

func (s *LedgerService) CreditMockDeposit(ctx context.Context, request LedgerDepositRequest) (LedgerDeposit, error) {
	return s.store.CreditDeposit(ctx, request)
}

func (s *LedgerService) FreezeWithdrawal(ctx context.Context, request LedgerWithdrawalRequest) (LedgerWithdrawal, error) {
	return s.store.FreezeWithdrawal(ctx, request)
}

func (s *LedgerService) GetWithdrawal(ctx context.Context, id string) (LedgerWithdrawal, error) {
	return s.store.GetWithdrawal(ctx, id)
}

func (s *LedgerService) ListWithdrawalsByStatus(ctx context.Context, status string, limit int) ([]LedgerWithdrawal, error) {
	if s == nil || s.store == nil {
		return []LedgerWithdrawal{}, nil
	}
	lister, ok := s.store.(interface {
		ListWithdrawalsByStatus(context.Context, string, int) ([]LedgerWithdrawal, error)
	})
	if !ok {
		return []LedgerWithdrawal{}, nil
	}
	return lister.ListWithdrawalsByStatus(ctx, status, limit)
}

func (s *LedgerService) RepairFrozenBalance(ctx context.Context, request LedgerFrozenBalanceRepairRequest) (LedgerFrozenBalanceRepairReport, error) {
	if s == nil || s.store == nil {
		return LedgerFrozenBalanceRepairReport{}, nil
	}
	repairer, ok := s.store.(interface {
		RepairFrozenBalance(context.Context, LedgerFrozenBalanceRepairRequest) (LedgerFrozenBalanceRepairReport, error)
	})
	if !ok {
		return LedgerFrozenBalanceRepairReport{}, nil
	}
	return repairer.RepairFrozenBalance(ctx, request)
}

func (s *LedgerService) MarkWithdrawalPaid(ctx context.Context, result LedgerWithdrawalResult) (LedgerWithdrawal, error) {
	return s.store.MarkWithdrawalPaid(ctx, result)
}

func (s *LedgerService) MarkWithdrawalFailed(ctx context.Context, result LedgerWithdrawalResult) (LedgerWithdrawal, error) {
	return s.store.MarkWithdrawalFailed(ctx, result)
}

func (s *LedgerService) ChargeAvailable(ctx context.Context, request LedgerChargeRequest) (LedgerBalance, error) {
	return s.store.ChargeAvailable(ctx, request)
}

func (s *LedgerService) RefundAvailable(ctx context.Context, request LedgerRefundRequest) (LedgerBalance, error) {
	return s.store.RefundAvailable(ctx, request)
}

func (s *LedgerService) ApplyTradingSettlement(ctx context.Context, request LedgerTradingSettlementRequest) (LedgerBalance, error) {
	return s.store.ApplyTradingSettlement(ctx, request)
}

func (s *LedgerService) GetBalance(ctx context.Context, userID, asset string) (LedgerBalance, error) {
	return s.store.GetBalance(ctx, userID, asset)
}

func (s *LedgerService) HasReference(ctx context.Context, refID string) (bool, error) {
	return s.store.HasReference(ctx, refID)
}

type MemoryLedgerStore struct {
	balances    map[string]LedgerBalance
	refs        map[string]bool
	journals    []LedgerJournal
	withdrawals map[string]LedgerWithdrawal
}

func NewMemoryLedgerStore() *MemoryLedgerStore {
	return &MemoryLedgerStore{
		balances:    map[string]LedgerBalance{},
		refs:        map[string]bool{},
		journals:    []LedgerJournal{},
		withdrawals: map[string]LedgerWithdrawal{},
	}
}

func (s *MemoryLedgerStore) CreditDeposit(ctx context.Context, request LedgerDepositRequest) (LedgerDeposit, error) {
	if s.refs[request.RefID] {
		return LedgerDeposit{}, ErrLedgerDuplicateReference
	}
	balance := s.balance(request.UserID, request.Asset)
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return LedgerDeposit{}, errors.New("invalid available balance")
	}
	amount, ok := parseLedgerAmount(request.Amount)
	if !ok {
		return LedgerDeposit{}, errors.New("invalid amount")
	}
	balance.Available = new(big.Int).Add(available, amount).String()
	s.balances[balanceKey(request.UserID, request.Asset)] = balance
	s.refs[request.RefID] = true
	s.journals = append(s.journals, LedgerJournal{
		ID:        uuid.NewString(),
		UserID:    request.UserID,
		Asset:     request.Asset,
		Type:      LedgerJournalTypeDepositCredit,
		Amount:    request.Amount,
		RefID:     request.RefID,
		Available: balance.Available,
		Frozen:    balance.Frozen,
	})

	return LedgerDeposit{
		ID:     uuid.NewString(),
		UserID: request.UserID,
		Asset:  request.Asset,
		Amount: request.Amount,
		RefID:  request.RefID,
		Status: LedgerDepositStatusCredited,
	}, nil
}

func (s *MemoryLedgerStore) FreezeWithdrawal(ctx context.Context, request LedgerWithdrawalRequest) (LedgerWithdrawal, error) {
	balance := s.balance(request.UserID, request.Asset)
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid available balance")
	}
	frozen, ok := parseLedgerAmount(balance.Frozen)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid frozen balance")
	}
	amount, ok := parseLedgerAmount(request.Amount)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid amount")
	}
	if available.Cmp(amount) < 0 {
		return LedgerWithdrawal{}, ErrLedgerInsufficientBalance
	}

	balance.Available = new(big.Int).Sub(available, amount).String()
	balance.Frozen = new(big.Int).Add(frozen, amount).String()
	s.balances[balanceKey(request.UserID, request.Asset)] = balance
	s.journals = append(s.journals, LedgerJournal{
		ID:        uuid.NewString(),
		UserID:    request.UserID,
		Asset:     request.Asset,
		Type:      LedgerJournalTypeWithdrawalFreeze,
		Amount:    request.Amount,
		RefID:     "",
		Available: balance.Available,
		Frozen:    balance.Frozen,
	})

	now := time.Now().Unix()
	withdrawal := LedgerWithdrawal{
		ID:        uuid.NewString(),
		UserID:    request.UserID,
		Asset:     request.Asset,
		Amount:    request.Amount,
		ToAddress: request.ToAddress,
		Status:    LedgerWithdrawalStatusFrozen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.withdrawals[withdrawal.ID] = withdrawal
	return withdrawal, nil
}

func (s *MemoryLedgerStore) MarkWithdrawalPaid(ctx context.Context, result LedgerWithdrawalResult) (LedgerWithdrawal, error) {
	withdrawal, ok := s.withdrawals[result.WithdrawalID]
	if !ok {
		return LedgerWithdrawal{}, ErrLedgerWithdrawalNotFound
	}
	balance := s.balance(withdrawal.UserID, withdrawal.Asset)
	frozen, ok := parseLedgerAmount(balance.Frozen)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid frozen balance")
	}
	amount, ok := parseLedgerAmount(withdrawal.Amount)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid amount")
	}
	if frozen.Cmp(amount) < 0 {
		return LedgerWithdrawal{}, ErrLedgerInsufficientBalance
	}
	balance.Frozen = new(big.Int).Sub(frozen, amount).String()
	s.balances[balanceKey(withdrawal.UserID, withdrawal.Asset)] = balance
	withdrawal.Status = LedgerWithdrawalStatusPaid
	withdrawal.TxHash = result.TxHash
	withdrawal.UpdatedAt = time.Now().Unix()
	s.withdrawals[withdrawal.ID] = withdrawal
	s.journals = append(s.journals, LedgerJournal{
		ID:        uuid.NewString(),
		UserID:    withdrawal.UserID,
		Asset:     withdrawal.Asset,
		Type:      LedgerJournalTypeWithdrawalPaid,
		Amount:    withdrawal.Amount,
		RefID:     withdrawal.ID,
		Available: balance.Available,
		Frozen:    balance.Frozen,
	})
	return withdrawal, nil
}

func (s *MemoryLedgerStore) GetWithdrawal(ctx context.Context, id string) (LedgerWithdrawal, error) {
	withdrawal, ok := s.withdrawals[id]
	if !ok {
		return LedgerWithdrawal{}, ErrLedgerWithdrawalNotFound
	}
	return withdrawal, nil
}

func (s *MemoryLedgerStore) ListWithdrawalsByStatus(ctx context.Context, status string, limit int) ([]LedgerWithdrawal, error) {
	out := make([]LedgerWithdrawal, 0)
	for _, withdrawal := range s.withdrawals {
		if withdrawal.Status != status {
			continue
		}
		out = append(out, withdrawal)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MemoryLedgerStore) MarkWithdrawalFailed(ctx context.Context, result LedgerWithdrawalResult) (LedgerWithdrawal, error) {
	withdrawal, ok := s.withdrawals[result.WithdrawalID]
	if !ok {
		return LedgerWithdrawal{}, ErrLedgerWithdrawalNotFound
	}
	balance := s.balance(withdrawal.UserID, withdrawal.Asset)
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid available balance")
	}
	frozen, ok := parseLedgerAmount(balance.Frozen)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid frozen balance")
	}
	amount, ok := parseLedgerAmount(withdrawal.Amount)
	if !ok {
		return LedgerWithdrawal{}, errors.New("invalid amount")
	}
	if frozen.Cmp(amount) < 0 {
		return LedgerWithdrawal{}, ErrLedgerInsufficientBalance
	}
	balance.Available = new(big.Int).Add(available, amount).String()
	balance.Frozen = new(big.Int).Sub(frozen, amount).String()
	s.balances[balanceKey(withdrawal.UserID, withdrawal.Asset)] = balance
	withdrawal.Status = LedgerWithdrawalStatusFailed
	withdrawal.Reason = result.Reason
	withdrawal.UpdatedAt = time.Now().Unix()
	s.withdrawals[withdrawal.ID] = withdrawal
	s.journals = append(s.journals, LedgerJournal{
		ID:        uuid.NewString(),
		UserID:    withdrawal.UserID,
		Asset:     withdrawal.Asset,
		Type:      LedgerJournalTypeWithdrawalFailed,
		Amount:    withdrawal.Amount,
		RefID:     withdrawal.ID,
		Available: balance.Available,
		Frozen:    balance.Frozen,
	})
	return withdrawal, nil
}

func (s *MemoryLedgerStore) ChargeAvailable(ctx context.Context, request LedgerChargeRequest) (LedgerBalance, error) {
	if s.refs[request.RefID] {
		return LedgerBalance{}, ErrLedgerDuplicateReference
	}
	balance := s.balance(request.UserID, request.Asset)
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return LedgerBalance{}, errors.New("invalid available balance")
	}
	amount, ok := parseLedgerAmount(request.Amount)
	if !ok {
		return LedgerBalance{}, errors.New("invalid amount")
	}
	if available.Cmp(amount) < 0 {
		return LedgerBalance{}, ErrLedgerInsufficientBalance
	}

	balance.Available = new(big.Int).Sub(available, amount).String()
	return s.saveBalanceCharge(request.UserID, request.Asset, balance, request.Amount, request.RefID)
}

func (s *MemoryLedgerStore) RefundAvailable(ctx context.Context, request LedgerRefundRequest) (LedgerBalance, error) {
	if s.refs[request.RefID] {
		return LedgerBalance{}, ErrLedgerDuplicateReference
	}
	balance := s.balance(request.UserID, request.Asset)
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return LedgerBalance{}, errors.New("invalid available balance")
	}
	amount, ok := parseLedgerAmount(request.Amount)
	if !ok {
		return LedgerBalance{}, errors.New("invalid amount")
	}
	balance.Available = new(big.Int).Add(available, amount).String()
	return s.saveBalanceJournal(request.UserID, request.Asset, balance, request.Amount, request.RefID, LedgerJournalTypeAvailableRefund)
}

func (s *MemoryLedgerStore) ApplyTradingSettlement(ctx context.Context, request LedgerTradingSettlementRequest) (LedgerBalance, error) {
	if s.refs[request.RefID] {
		return LedgerBalance{}, ErrLedgerDuplicateReference
	}
	balance := s.balance(request.UserID, request.Asset)
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return LedgerBalance{}, errors.New("invalid available balance")
	}
	releasedMargin, ok := parseLedgerAmount(defaultLedgerAmount(request.ReleasedMargin))
	if !ok {
		return LedgerBalance{}, errors.New("invalid released margin")
	}
	fee, ok := parseLedgerAmount(defaultLedgerAmount(request.Fee))
	if !ok {
		return LedgerBalance{}, errors.New("invalid fee")
	}
	realizedPNL, ok := parseSignedLedgerAmount(defaultLedgerAmount(request.RealizedPNL))
	if !ok {
		return LedgerBalance{}, errors.New("invalid realized pnl")
	}
	next := new(big.Int).Add(available, releasedMargin)
	next.Add(next, realizedPNL)
	next.Sub(next, fee)
	if next.Sign() < 0 {
		return LedgerBalance{}, ErrLedgerInsufficientBalance
	}
	balance.Available = next.String()
	return s.saveBalanceJournal(request.UserID, request.Asset, balance, request.ReleasedMargin, request.RefID, LedgerJournalTypeTradingSettle)
}

func (s *MemoryLedgerStore) GetBalance(ctx context.Context, userID, asset string) (LedgerBalance, error) {
	return s.balance(userID, asset), nil
}

func (s *MemoryLedgerStore) HasReference(ctx context.Context, refID string) (bool, error) {
	return s.refs[refID], nil
}

func (s *MemoryLedgerStore) RepairFrozenBalance(ctx context.Context, request LedgerFrozenBalanceRepairRequest) (LedgerFrozenBalanceRepairReport, error) {
	if request.UserID == "" || request.Asset == "" {
		return LedgerFrozenBalanceRepairReport{}, errors.New("userId and asset are required")
	}
	expected := big.NewInt(0)
	for _, withdrawal := range s.withdrawals {
		if withdrawal.UserID != request.UserID || withdrawal.Asset != request.Asset {
			continue
		}
		if withdrawal.Status != LedgerWithdrawalStatusFrozen {
			continue
		}
		amount, ok := parseLedgerAmount(withdrawal.Amount)
		if !ok {
			return LedgerFrozenBalanceRepairReport{}, errors.New("invalid withdrawal amount")
		}
		expected.Add(expected, amount)
	}

	key := balanceKey(request.UserID, request.Asset)
	balance := s.balance(request.UserID, request.Asset)
	before, ok := parseLedgerAmount(balance.Frozen)
	if !ok {
		return LedgerFrozenBalanceRepairReport{}, errors.New("invalid frozen balance")
	}
	repair := LedgerFrozenBalanceRepair{
		UserID:         request.UserID,
		Asset:          request.Asset,
		BeforeFrozen:   before.String(),
		ExpectedFrozen: expected.String(),
		Changed:        before.Cmp(expected) != 0,
	}
	report := LedgerFrozenBalanceRepairReport{
		Checked:  1,
		Balances: []LedgerFrozenBalanceRepair{repair},
	}
	if !repair.Changed {
		return report, nil
	}
	report.Changed = 1
	if request.Apply {
		balance.Frozen = expected.String()
		s.balances[key] = balance
		report.Applied = 1
		report.Balances[0].Applied = true
		s.journals = append(s.journals, LedgerJournal{
			ID:        uuid.NewString(),
			UserID:    request.UserID,
			Asset:     request.Asset,
			Type:      LedgerJournalTypeFrozenRepair,
			Amount:    new(big.Int).Sub(expected, before).String(),
			RefID:     "ledger-frozen-repair:" + request.UserID + ":" + request.Asset,
			Available: balance.Available,
			Frozen:    balance.Frozen,
		})
	}
	return report, nil
}

func (s *MemoryLedgerStore) Journals() []LedgerJournal {
	return append([]LedgerJournal{}, s.journals...)
}

func (s *MemoryLedgerStore) balance(userID, asset string) LedgerBalance {
	key := balanceKey(userID, asset)
	if balance, ok := s.balances[key]; ok {
		return balance
	}
	return LedgerBalance{
		UserID:    userID,
		Asset:     asset,
		Available: "0",
		Frozen:    "0",
	}
}

func (s *MemoryLedgerStore) saveBalanceCharge(userID, asset string, balance LedgerBalance, amount, refID string) (LedgerBalance, error) {
	return s.saveBalanceJournal(userID, asset, balance, amount, refID, LedgerJournalTypeAvailableCharge)
}

func (s *MemoryLedgerStore) saveBalanceJournal(userID, asset string, balance LedgerBalance, amount, refID, journalType string) (LedgerBalance, error) {
	s.balances[balanceKey(userID, asset)] = balance
	s.refs[refID] = true
	s.journals = append(s.journals, LedgerJournal{
		ID:        uuid.NewString(),
		UserID:    userID,
		Asset:     asset,
		Type:      journalType,
		Amount:    amount,
		RefID:     refID,
		Available: balance.Available,
		Frozen:    balance.Frozen,
	})
	return balance, nil
}

func balanceKey(userID, asset string) string {
	return userID + ":" + asset
}

func parseLedgerAmount(value string) (*big.Int, bool) {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok || amount.Sign() < 0 {
		return nil, false
	}
	return amount, true
}

func parseSignedLedgerAmount(value string) (*big.Int, bool) {
	amount, ok := new(big.Int).SetString(value, 10)
	return amount, ok
}

func defaultLedgerAmount(value string) string {
	if value == "" {
		return "0"
	}
	return value
}
