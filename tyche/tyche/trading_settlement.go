package tyche

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	CloseSettlementModeMarginOnly     = "margin_only"
	CloseSettlementModeFullAccounting = "full_accounting"

	TradingSettlementKindClosePosition = "close_position"
	TradingSettlementKindCancelOrder   = "cancel_order"
	TradingSettlementKindFailedOrder   = "failed_order"

	TradingSettlementStatusPending               = "pending"
	TradingSettlementStatusDEDXCredited          = "dedx_credited"
	TradingSettlementStatusNECOPayoutBroadcasted = "neco_payout_broadcasted"
	TradingSettlementStatusNECOPayoutPaid        = "neco_payout_paid"
	TradingSettlementStatusManualReviewRequired  = "manual_review_required"
	TradingSettlementStatusFailed                = "failed"
	TradingSettlementStatusRejected              = "rejected"

	defaultCloseSettlementAsset         = "DEDX"
	defaultCloseSettlementAssetDecimals = 18
)

var (
	ErrTradingSettlementNotFound      = errors.New("trading settlement not found")
	ErrTradingSettlementManualReview  = errors.New("trading settlement manual review required")
	ErrTradingSettlementInvalidAmount = errors.New("trading settlement invalid amount")
	ErrTradingSettlementRejected      = errors.New("trading settlement rejected")
	ErrTradingSettlementImmutable     = errors.New("trading settlement cannot be changed")
)

type TradingSettlement struct {
	ID                       string `json:"id" redis:"id"`
	Kind                     string `json:"kind" redis:"kind"`
	UserID                   string `json:"userId" redis:"userId"`
	NECOAddress              string `json:"necoAddress" redis:"necoAddress"`
	DYDXAccount              string `json:"dydxAccount" redis:"dydxAccount"`
	SubaccountNumber         uint32 `json:"subaccountNumber" redis:"subaccountNumber"`
	Market                   string `json:"market" redis:"market"`
	CloseOrderID             string `json:"closeOrderId" redis:"closeOrderId"`
	CloseClientOrderID       string `json:"closeClientOrderId" redis:"closeClientOrderId"`
	SourcePositionID         string `json:"sourcePositionId" redis:"sourcePositionId"`
	SettlementUSD            string `json:"settlementUsd" redis:"settlementUsd"`
	SettlementAsset          string `json:"settlementAsset" redis:"settlementAsset"`
	SettlementAmount         string `json:"settlementAmount" redis:"settlementAmount"`
	AssetUSDPrice            string `json:"assetUsdPrice" redis:"assetUsdPrice"`
	PriceSnapshotID          string `json:"priceSnapshotId" redis:"priceSnapshotId"`
	SystemReserved           bool   `json:"systemReserved" redis:"systemReserved"`
	DYDXReservedQuantums     string `json:"dydxReservedQuantums" redis:"dydxReservedQuantums"`
	LedgerCreditRefID        string `json:"ledgerCreditRefId" redis:"ledgerCreditRefId"`
	LedgerWithdrawalID       string `json:"ledgerWithdrawalId" redis:"ledgerWithdrawalId"`
	NECOTxHash               string `json:"necoTxHash" redis:"necoTxHash"`
	TradeCloseCallbackURL    string `json:"tradeCloseCallbackUrl,omitempty" redis:"tradeCloseCallbackUrl"`
	TradeCloseCallbackStatus string `json:"tradeCloseCallbackStatus,omitempty" redis:"tradeCloseCallbackStatus"`
	TradeCloseCallbackReason string `json:"tradeCloseCallbackReason,omitempty" redis:"tradeCloseCallbackReason"`
	TradeCloseCallbackAt     int64  `json:"tradeCloseCallbackAt,omitempty" redis:"tradeCloseCallbackAt"`
	Status                   string `json:"status" redis:"status"`
	Reason                   string `json:"reason" redis:"reason"`
	CreatedAt                int64  `json:"createdAt" redis:"createdAt"`
	UpdatedAt                int64  `json:"updatedAt" redis:"updatedAt"`
}

type TradingSettlementStore interface {
	CreateIfAbsent(context.Context, TradingSettlement) (TradingSettlement, bool, error)
	Save(context.Context, TradingSettlement) error
	GetByID(context.Context, string) (TradingSettlement, bool, error)
	GetByCloseOrderID(context.Context, string, string) (TradingSettlement, bool, error)
	GetByLedgerWithdrawalID(context.Context, string) (TradingSettlement, bool, error)
	ListByUser(context.Context, string) ([]TradingSettlement, error)
	ListByStatus(context.Context, string) ([]TradingSettlement, error)
}

type MemoryTradingSettlementStore struct {
	mu           sync.Mutex
	byID         map[string]TradingSettlement
	byCloseOrder map[string]string
	byWithdrawal map[string]string
	byUser       map[string]map[string]bool
}

func NewMemoryTradingSettlementStore() *MemoryTradingSettlementStore {
	return &MemoryTradingSettlementStore{
		byID:         map[string]TradingSettlement{},
		byCloseOrder: map[string]string{},
		byWithdrawal: map[string]string{},
		byUser:       map[string]map[string]bool{},
	}
}

func NormalizeCloseSettlementMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case CloseSettlementModeFullAccounting:
		return CloseSettlementModeFullAccounting
	default:
		return CloseSettlementModeMarginOnly
	}
}

func (s *MemoryTradingSettlementStore) CreateIfAbsent(ctx context.Context, settlement TradingSettlement) (TradingSettlement, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := closeSettlementKey(settlement.UserID, settlement.CloseOrderID)
	if id, ok := s.byCloseOrder[key]; ok {
		return s.byID[id], false, nil
	}
	if settlement.ID == "" {
		settlement.ID = uuid.NewString()
	}
	now := time.Now().Unix()
	if settlement.CreatedAt == 0 {
		settlement.CreatedAt = now
	}
	settlement.UpdatedAt = now
	s.byID[settlement.ID] = settlement
	s.byCloseOrder[key] = settlement.ID
	if settlement.LedgerWithdrawalID != "" {
		s.byWithdrawal[settlement.LedgerWithdrawalID] = settlement.ID
	}
	if s.byUser[settlement.UserID] == nil {
		s.byUser[settlement.UserID] = map[string]bool{}
	}
	s.byUser[settlement.UserID][settlement.ID] = true
	return settlement, true, nil
}

func (s *MemoryTradingSettlementStore) Save(ctx context.Context, settlement TradingSettlement) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settlement.ID == "" {
		settlement.ID = uuid.NewString()
	}
	if settlement.CreatedAt == 0 {
		if existing, ok := s.byID[settlement.ID]; ok {
			settlement.CreatedAt = existing.CreatedAt
		}
	}
	now := time.Now().Unix()
	if settlement.CreatedAt == 0 {
		settlement.CreatedAt = now
	}
	settlement.UpdatedAt = now
	s.byID[settlement.ID] = settlement
	s.byCloseOrder[closeSettlementKey(settlement.UserID, settlement.CloseOrderID)] = settlement.ID
	if settlement.LedgerWithdrawalID != "" {
		s.byWithdrawal[settlement.LedgerWithdrawalID] = settlement.ID
	}
	if s.byUser[settlement.UserID] == nil {
		s.byUser[settlement.UserID] = map[string]bool{}
	}
	s.byUser[settlement.UserID][settlement.ID] = true
	return nil
}

func (s *MemoryTradingSettlementStore) GetByID(ctx context.Context, id string) (TradingSettlement, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settlement, ok := s.byID[id]
	return settlement, ok, nil
}

func (s *MemoryTradingSettlementStore) GetByCloseOrderID(ctx context.Context, userID, closeOrderID string) (TradingSettlement, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.byCloseOrder[closeSettlementKey(userID, closeOrderID)]
	if !ok {
		return TradingSettlement{}, false, nil
	}
	settlement, ok := s.byID[id]
	return settlement, ok, nil
}

func (s *MemoryTradingSettlementStore) GetByLedgerWithdrawalID(ctx context.Context, withdrawalID string) (TradingSettlement, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.byWithdrawal[withdrawalID]
	if !ok {
		return TradingSettlement{}, false, nil
	}
	settlement, ok := s.byID[id]
	return settlement, ok, nil
}

func (s *MemoryTradingSettlementStore) ListByUser(ctx context.Context, userID string) ([]TradingSettlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := s.byUser[userID]
	settlements := make([]TradingSettlement, 0, len(ids))
	for id := range ids {
		if settlement, ok := s.byID[id]; ok {
			settlements = append(settlements, settlement)
		}
	}
	return settlements, nil
}

func (s *MemoryTradingSettlementStore) ListByStatus(ctx context.Context, status string) ([]TradingSettlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settlements := make([]TradingSettlement, 0)
	for _, settlement := range s.byID {
		if settlement.Status == status {
			settlements = append(settlements, settlement)
		}
	}
	return settlements, nil
}

func closeSettlementKey(userID, closeOrderID string) string {
	return userID + ":" + closeOrderID
}

func settlementAssetAmountRaw(settlementUSD, assetUSDPrice string, decimals uint8) (string, error) {
	if looksLikeAtomicUSD(settlementUSD) {
		return "", ErrTradingSettlementManualReview
	}
	usd, ok := parseDecimalRat(settlementUSD)
	if !ok || usd.Sign() <= 0 {
		return "", ErrTradingSettlementInvalidAmount
	}
	price, ok := parseDecimalRat(assetUSDPrice)
	if !ok || price.Sign() <= 0 {
		return "", ErrTradingSettlementInvalidAmount
	}
	scaled := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	raw := new(big.Rat).Mul(usd, new(big.Rat).SetInt(scaled))
	raw.Quo(raw, price)
	amount := new(big.Int).Quo(raw.Num(), raw.Denom())
	if amount.Sign() <= 0 {
		return "", ErrTradingSettlementInvalidAmount
	}
	return amount.String(), nil
}

func looksLikeAtomicUSD(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, ".eE") {
		return false
	}
	value = strings.TrimPrefix(value, "+")
	if strings.HasPrefix(value, "-") {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return len(strings.TrimLeft(value, "0")) > 12
}
