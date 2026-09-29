package tyche

import (
	"context"
	"strings"
)

type TradingSettlementManualPayoutRequest struct {
	UserID           string `json:"userId"`
	SettlementID     string `json:"settlementId"`
	OrderID          string `json:"orderId"`
	CloseOrderID     string `json:"closeOrderId"`
	SettlementUSD    string `json:"settlementUsd"`
	SettlementAmount string `json:"settlementAmount"`
	Reason           string `json:"reason"`
}

type TradingSettlementRejectRequest struct {
	UserID       string `json:"userId"`
	SettlementID string `json:"settlementId"`
	OrderID      string `json:"orderId"`
	CloseOrderID string `json:"closeOrderId"`
	Reason       string `json:"reason"`
}

func (s *CloseSettlementService) ManualPayout(ctx context.Context, request TradingSettlementManualPayoutRequest) (TradingSettlement, error) {
	if s == nil || s.store == nil {
		return TradingSettlement{}, ErrTradingSettlementNotFound
	}
	settlement, err := s.findSettlement(ctx, request.UserID, request.SettlementID, firstNonEmptyTradingString(request.OrderID, request.CloseOrderID))
	if err != nil {
		return TradingSettlement{}, err
	}
	switch settlement.Status {
	case TradingSettlementStatusRejected:
		return settlement, ErrTradingSettlementRejected
	case TradingSettlementStatusNECOPayoutBroadcasted, TradingSettlementStatusNECOPayoutPaid:
		return settlement, nil
	case TradingSettlementStatusDEDXCredited:
		return s.continueSettlement(ctx, settlement)
	}
	if settlement.Status != TradingSettlementStatusManualReviewRequired &&
		settlement.Status != TradingSettlementStatusFailed &&
		settlement.Status != TradingSettlementStatusPending {
		return settlement, ErrTradingSettlementImmutable
	}

	updated, err := s.applyManualPayoutFields(ctx, settlement, request)
	if err != nil {
		return settlement, err
	}
	updated.Status = TradingSettlementStatusPending
	updated.SystemReserved = true
	if request.Reason != "" {
		updated.Reason = strings.TrimSpace(request.Reason)
	}
	if err := s.store.Save(ctx, updated); err != nil {
		return TradingSettlement{}, err
	}
	return s.continueSettlement(ctx, updated)
}

func (s *CloseSettlementService) RejectSettlement(ctx context.Context, request TradingSettlementRejectRequest) (TradingSettlement, error) {
	if s == nil || s.store == nil {
		return TradingSettlement{}, ErrTradingSettlementNotFound
	}
	settlement, err := s.findSettlement(ctx, request.UserID, request.SettlementID, firstNonEmptyTradingString(request.OrderID, request.CloseOrderID))
	if err != nil {
		return TradingSettlement{}, err
	}
	switch settlement.Status {
	case TradingSettlementStatusRejected:
		return settlement, nil
	case TradingSettlementStatusDEDXCredited, TradingSettlementStatusNECOPayoutBroadcasted, TradingSettlementStatusNECOPayoutPaid:
		return settlement, ErrTradingSettlementImmutable
	}
	settlement.Status = TradingSettlementStatusRejected
	settlement.Reason = strings.TrimSpace(request.Reason)
	if settlement.Reason == "" {
		settlement.Reason = "manual settlement rejected"
	}
	if err := s.store.Save(ctx, settlement); err != nil {
		return TradingSettlement{}, err
	}
	return settlement, nil
}

func (s *CloseSettlementService) findSettlement(ctx context.Context, userID, settlementID, closeOrderID string) (TradingSettlement, error) {
	if settlementID != "" {
		settlement, ok, err := s.store.GetByID(ctx, settlementID)
		if err != nil {
			return TradingSettlement{}, err
		}
		if !ok || (userID != "" && settlement.UserID != userID) {
			return TradingSettlement{}, ErrTradingSettlementNotFound
		}
		return settlement, nil
	}
	if closeOrderID != "" && userID != "" {
		settlement, ok, err := s.store.GetByCloseOrderID(ctx, userID, closeOrderID)
		if err != nil {
			return TradingSettlement{}, err
		}
		if !ok {
			return TradingSettlement{}, ErrTradingSettlementNotFound
		}
		return settlement, nil
	}
	return TradingSettlement{}, ErrTradingSettlementNotFound
}

func (s *CloseSettlementService) applyManualPayoutFields(ctx context.Context, settlement TradingSettlement, request TradingSettlementManualPayoutRequest) (TradingSettlement, error) {
	if settlement.SettlementAsset == "" {
		settlement.SettlementAsset = normalizeAsset(s.conf.SettlementAsset)
	}
	if settlement.SettlementAsset == "" {
		settlement.SettlementAsset = defaultCloseSettlementAsset
	}

	settlementUSD := strings.TrimSpace(request.SettlementUSD)
	if settlementUSD == "" {
		settlementUSD = strings.TrimSpace(settlement.SettlementUSD)
	}
	if settlementUSD != "" {
		if looksLikeAtomicUSD(settlementUSD) {
			return TradingSettlement{}, ErrTradingSettlementManualReview
		}
		parsed, ok := parseDecimalRat(settlementUSD)
		if !ok || parsed.Sign() <= 0 {
			return TradingSettlement{}, ErrTradingSettlementInvalidAmount
		}
		settlement.SettlementUSD = settlementUSD
	}

	amount := strings.TrimSpace(request.SettlementAmount)
	if amount == "" {
		amount = strings.TrimSpace(settlement.SettlementAmount)
	}
	if amount == "" {
		if settlementUSD == "" {
			return TradingSettlement{}, ErrTradingSettlementInvalidAmount
		}
		price := strings.TrimSpace(settlement.AssetUSDPrice)
		if price == "" {
			snapshot, err := s.settlementAssetPrice(ctx, settlement.SettlementAsset)
			if err != nil {
				return TradingSettlement{}, err
			}
			price = snapshot.USDPrice
			settlement.AssetUSDPrice = snapshot.USDPrice
			settlement.PriceSnapshotID = snapshot.ID
		}
		var err error
		amount, err = settlementAssetAmountRaw(settlementUSD, price, s.settlementAssetDecimals(settlement.SettlementAsset))
		if err != nil {
			return TradingSettlement{}, err
		}
	}
	parsedAmount, ok := parseLedgerAmount(amount)
	if !ok || parsedAmount.Sign() <= 0 {
		return TradingSettlement{}, ErrTradingSettlementInvalidAmount
	}
	settlement.SettlementAmount = amount
	return settlement, nil
}

func firstNonEmptyTradingString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
