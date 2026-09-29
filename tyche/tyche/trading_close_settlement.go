package tyche

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

var ErrTradingSettlementNotConfirmed = errors.New("trading close settlement is not confirmed")

type CloseSettlementConfig struct {
	Mode                     string
	SettlementAsset          string
	SettlementAssetDecimals  uint8
	CollateralQuantumsPerUSD string
	SubaccountNumber         uint32
	Markets                  []DYDXMarketConfig
	AssetMappings            []AssetMapping
	TradeCloseCallbackURL    string
}

type CloseSettlementRequest struct {
	CloseOrder TradingOrder
	Position   TradingPosition
	Fill       TradingFill
	Reconcile  DYDXOrderReconcileResult
}

type CancelSettlementRequest struct {
	Order     TradingOrder
	Fill      TradingFill
	Reconcile DYDXOrderReconcileResult
}

type FailedOrderSettlementRequest struct {
	Order     TradingOrder
	Fill      TradingFill
	Reconcile DYDXOrderReconcileResult
}

type closeSettlementBindingLookup interface {
	GetByUserID(context.Context, string) (WalletBinding, error)
	GetByDYDXAddress(context.Context, string) (WalletBinding, error)
}

type CloseSettlementService struct {
	store      TradingSettlementStore
	ledger     *LedgerService
	withdrawer *NECOWithdrawalWorker
	oracle     AssetUSDOracle
	bindings   closeSettlementBindingLookup
	fills      TradingFillLookup
	conf       CloseSettlementConfig
	markets    map[string]DYDXMarketConfig
	assets     map[string]AssetMapping
	callback   TradingCloseCallbackSender
}

func NewCloseSettlementService(store TradingSettlementStore, ledger *LedgerService, withdrawer *NECOWithdrawalWorker, oracle AssetUSDOracle, bindings closeSettlementBindingLookup, conf CloseSettlementConfig) *CloseSettlementService {
	conf.Mode = NormalizeCloseSettlementMode(conf.Mode)
	if conf.SettlementAsset == "" {
		conf.SettlementAsset = defaultCloseSettlementAsset
	}
	if conf.SettlementAssetDecimals == 0 {
		conf.SettlementAssetDecimals = defaultCloseSettlementAssetDecimals
	}
	if conf.CollateralQuantumsPerUSD == "" {
		conf.CollateralQuantumsPerUSD = "1000000"
	}
	service := &CloseSettlementService{
		store:      store,
		ledger:     ledger,
		withdrawer: withdrawer,
		oracle:     oracle,
		bindings:   bindings,
		conf:       conf,
		markets:    map[string]DYDXMarketConfig{},
		assets:     map[string]AssetMapping{},
	}
	if strings.TrimSpace(conf.TradeCloseCallbackURL) != "" {
		service.callback = NewHTTPTradingCloseCallbackSender(conf.TradeCloseCallbackURL)
	}
	for _, market := range conf.Markets {
		service.markets[normalizeDYDXMarket(market.Market)] = market
	}
	for _, mapping := range conf.AssetMappings {
		asset := normalizeAsset(mapping.NECOAsset)
		if asset == "" {
			continue
		}
		service.assets[asset] = mapping
	}
	return service
}

func (s *CloseSettlementService) SetFillLookup(lookup TradingFillLookup) {
	if s != nil {
		s.fills = lookup
	}
}

func (s *CloseSettlementService) SetTradeCloseCallbackSender(sender TradingCloseCallbackSender) {
	if s != nil {
		s.callback = sender
	}
}

func (s *CloseSettlementService) SettleClosedPosition(ctx context.Context, request CloseSettlementRequest) (TradingSettlement, error) {
	if s == nil || s.store == nil {
		return TradingSettlement{}, ErrTradingSettlementNotFound
	}
	order := request.CloseOrder
	if existing, ok, err := s.store.GetByCloseOrderID(ctx, order.UserID, order.ID); err != nil {
		return TradingSettlement{}, err
	} else if ok {
		return s.continueClosedPositionSettlement(ctx, existing, order, request.Fill)
	}
	if err := s.validateCloseSettlementRequest(request); err != nil {
		return TradingSettlement{}, err
	}
	necoAddress, err := s.resolveNECOAddress(ctx, order)
	if err != nil {
		return TradingSettlement{}, err
	}
	settlementUSD, err := s.settlementUSD(ctx, request)
	if err != nil {
		settlement := s.manualReviewSettlement(order, TradingSettlementKindClosePosition, necoAddress, err.Error())
		created, createErr := s.saveManualReview(ctx, settlement)
		if createErr != nil {
			return TradingSettlement{}, createErr
		}
		return created, ErrTradingSettlementManualReview
	}
	assetSpec := s.settlementAssetSpec(order, TradingSettlementKindClosePosition)
	snapshot, err := s.settlementAssetPrice(ctx, assetSpec.Asset)
	if err != nil {
		settlement := s.manualReviewSettlement(order, TradingSettlementKindClosePosition, necoAddress, err.Error())
		created, createErr := s.saveManualReview(ctx, settlement)
		if createErr != nil {
			return TradingSettlement{}, createErr
		}
		return created, ErrTradingSettlementManualReview
	}
	amount, err := settlementAssetAmountRaw(settlementUSD, snapshot.USDPrice, assetSpec.Decimals)
	if err != nil {
		settlement := s.manualReviewSettlement(order, TradingSettlementKindClosePosition, necoAddress, err.Error())
		settlement.SettlementUSD = settlementUSD
		settlement.AssetUSDPrice = snapshot.USDPrice
		settlement.PriceSnapshotID = snapshot.ID
		created, createErr := s.saveManualReview(ctx, settlement)
		if createErr != nil {
			return TradingSettlement{}, createErr
		}
		return created, ErrTradingSettlementManualReview
	}
	quantums := s.reservedQuantums(order, settlementUSD)
	settlement := TradingSettlement{
		UserID:                order.UserID,
		Kind:                  TradingSettlementKindClosePosition,
		NECOAddress:           necoAddress,
		DYDXAccount:           order.DYDXAccount,
		SubaccountNumber:      s.orderSubaccountNumber(order),
		Market:                order.Market,
		CloseOrderID:          order.ID,
		CloseClientOrderID:    order.ClientOrderID,
		SourcePositionID:      request.Position.EventID,
		SettlementUSD:         settlementUSD,
		SettlementAsset:       assetSpec.Asset,
		SettlementAmount:      amount,
		AssetUSDPrice:         snapshot.USDPrice,
		PriceSnapshotID:       snapshot.ID,
		SystemReserved:        true,
		DYDXReservedQuantums:  quantums,
		TradeCloseCallbackURL: s.conf.TradeCloseCallbackURL,
		Status:                TradingSettlementStatusPending,
	}
	created, _, err := s.store.CreateIfAbsent(ctx, settlement)
	if err != nil {
		return TradingSettlement{}, err
	}
	return s.continueClosedPositionSettlement(ctx, created, order, request.Fill)
}

func (s *CloseSettlementService) SettleCancelledOrder(ctx context.Context, request CancelSettlementRequest) (TradingSettlement, error) {
	if s == nil || s.store == nil {
		return TradingSettlement{}, ErrTradingSettlementNotFound
	}
	order := request.Order
	if existing, ok, err := s.store.GetByCloseOrderID(ctx, order.UserID, order.ID); err != nil {
		return TradingSettlement{}, err
	} else if ok {
		if existing.Status == TradingSettlementStatusManualReviewRequired && existing.SettlementAmount == "" {
			refreshed, err := s.buildCancelledOrderSettlement(ctx, request, existing)
			if err == nil {
				if err := s.store.Save(ctx, refreshed); err != nil {
					return TradingSettlement{}, err
				}
				return s.continueSettlement(ctx, refreshed)
			}
			if !errors.Is(err, ErrTradingSettlementManualReview) {
				return existing, err
			}
		}
		return s.continueSettlement(ctx, existing)
	}
	settlement, err := s.buildCancelledOrderSettlement(ctx, request, TradingSettlement{})
	if err != nil {
		if errors.Is(err, ErrTradingSettlementManualReview) {
			necoAddress, resolveErr := s.resolveNECOAddress(ctx, order)
			if resolveErr != nil {
				return TradingSettlement{}, resolveErr
			}
			settlement := s.manualReviewSettlement(order, TradingSettlementKindCancelOrder, necoAddress, err.Error())
			created, createErr := s.saveManualReview(ctx, settlement)
			if createErr != nil {
				return TradingSettlement{}, createErr
			}
			return created, ErrTradingSettlementManualReview
		}
		return TradingSettlement{}, err
	}
	created, _, err := s.store.CreateIfAbsent(ctx, settlement)
	if err != nil {
		return TradingSettlement{}, err
	}
	return s.continueSettlement(ctx, created)
}

func (s *CloseSettlementService) SettleFailedOrder(ctx context.Context, request FailedOrderSettlementRequest) (TradingSettlement, error) {
	if s == nil || s.store == nil {
		return TradingSettlement{}, ErrTradingSettlementNotFound
	}
	order := request.Order
	if existing, ok, err := s.store.GetByCloseOrderID(ctx, order.UserID, order.ID); err != nil {
		return TradingSettlement{}, err
	} else if ok {
		return s.continueSettlement(ctx, existing)
	}
	settlement, err := s.buildFailedOrderSettlement(ctx, request)
	if err != nil {
		if errors.Is(err, ErrTradingSettlementManualReview) {
			necoAddress, resolveErr := s.resolveNECOAddress(ctx, order)
			if resolveErr != nil {
				return TradingSettlement{}, resolveErr
			}
			settlement := s.manualReviewSettlement(order, TradingSettlementKindFailedOrder, necoAddress, err.Error())
			created, createErr := s.saveManualReview(ctx, settlement)
			if createErr != nil {
				return TradingSettlement{}, createErr
			}
			return created, ErrTradingSettlementManualReview
		}
		return TradingSettlement{}, err
	}
	created, _, err := s.store.CreateIfAbsent(ctx, settlement)
	if err != nil {
		return TradingSettlement{}, err
	}
	return s.continueSettlement(ctx, created)
}

func (s *CloseSettlementService) buildFailedOrderSettlement(ctx context.Context, request FailedOrderSettlementRequest) (TradingSettlement, error) {
	order := request.Order
	necoAddress, err := s.resolveNECOAddress(ctx, order)
	if err != nil {
		return TradingSettlement{}, err
	}
	if err := s.validateFailedOrderSettlementRequest(request); err != nil {
		return TradingSettlement{}, err
	}
	settlementUSD, amount, assetPrice, snapshotID, err := s.failedOrderSettlementValues(ctx, order)
	if err != nil {
		return TradingSettlement{}, err
	}
	return TradingSettlement{
		UserID:               order.UserID,
		Kind:                 TradingSettlementKindFailedOrder,
		NECOAddress:          necoAddress,
		DYDXAccount:          order.DYDXAccount,
		SubaccountNumber:     s.orderSubaccountNumber(order),
		Market:               order.Market,
		CloseOrderID:         order.ID,
		CloseClientOrderID:   order.ClientOrderID,
		SettlementUSD:        settlementUSD,
		SettlementAsset:      normalizeAsset(order.MarginAsset),
		SettlementAmount:     amount,
		AssetUSDPrice:        assetPrice,
		PriceSnapshotID:      snapshotID,
		SystemReserved:       true,
		DYDXReservedQuantums: s.reservedQuantums(order, settlementUSD),
		Status:               TradingSettlementStatusPending,
	}, nil
}

func (s *CloseSettlementService) buildCancelledOrderSettlement(ctx context.Context, request CancelSettlementRequest, existing TradingSettlement) (TradingSettlement, error) {
	order := request.Order
	necoAddress, err := s.resolveNECOAddress(ctx, order)
	if err != nil {
		return TradingSettlement{}, err
	}
	if err := s.validateCancelSettlementRequest(request); err != nil {
		return TradingSettlement{}, err
	}
	settlementUSD, err := s.cancelSettlementUSD(ctx, request)
	if err != nil {
		return TradingSettlement{}, err
	}
	assetSpec := s.settlementAssetSpec(order, TradingSettlementKindCancelOrder)
	snapshot, err := s.settlementAssetPrice(ctx, assetSpec.Asset)
	if err != nil {
		return TradingSettlement{}, err
	}
	amount, err := settlementAssetAmountRaw(settlementUSD, snapshot.USDPrice, assetSpec.Decimals)
	if err != nil {
		return TradingSettlement{}, err
	}
	settlement := existing
	settlement.UserID = order.UserID
	settlement.Kind = TradingSettlementKindCancelOrder
	settlement.NECOAddress = necoAddress
	settlement.DYDXAccount = order.DYDXAccount
	settlement.SubaccountNumber = s.orderSubaccountNumber(order)
	settlement.Market = order.Market
	settlement.CloseOrderID = order.ID
	settlement.CloseClientOrderID = order.ClientOrderID
	settlement.SettlementUSD = settlementUSD
	settlement.SettlementAsset = assetSpec.Asset
	settlement.SettlementAmount = amount
	settlement.AssetUSDPrice = snapshot.USDPrice
	settlement.PriceSnapshotID = snapshot.ID
	settlement.SystemReserved = true
	settlement.DYDXReservedQuantums = s.reservedQuantums(order, settlementUSD)
	settlement.Status = TradingSettlementStatusPending
	settlement.Reason = ""
	return settlement, nil
}

func (s *CloseSettlementService) continueSettlement(ctx context.Context, settlement TradingSettlement) (TradingSettlement, error) {
	switch settlement.Status {
	case TradingSettlementStatusNECOPayoutBroadcasted,
		TradingSettlementStatusNECOPayoutPaid,
		TradingSettlementStatusManualReviewRequired,
		TradingSettlementStatusRejected:
		return settlement, nil
	}
	if s.ledger == nil {
		settlement.Status = TradingSettlementStatusFailed
		settlement.Reason = "ledger service is not configured"
		_ = s.store.Save(ctx, settlement)
		return settlement, errors.New(settlement.Reason)
	}
	if settlement.LedgerCreditRefID == "" {
		settlement.LedgerCreditRefID = settlementCreditRefID(settlement)
	}
	if _, err := s.ledger.CreditMockDeposit(ctx, LedgerDepositRequest{
		UserID: settlement.UserID,
		Asset:  settlement.SettlementAsset,
		Amount: settlement.SettlementAmount,
		RefID:  settlement.LedgerCreditRefID,
	}); err != nil && !errors.Is(err, ErrLedgerDuplicateReference) {
		settlement.Status = TradingSettlementStatusFailed
		settlement.Reason = err.Error()
		_ = s.store.Save(ctx, settlement)
		return settlement, err
	}
	settlement.Status = TradingSettlementStatusDEDXCredited
	if err := s.store.Save(ctx, settlement); err != nil {
		return TradingSettlement{}, err
	}
	settlement, err := s.refreshFailedLedgerWithdrawal(ctx, settlement)
	if err != nil {
		return TradingSettlement{}, err
	}
	if settlement.LedgerWithdrawalID == "" {
		withdrawal, err := s.ledger.FreezeWithdrawal(ctx, LedgerWithdrawalRequest{
			UserID:    settlement.UserID,
			Asset:     settlement.SettlementAsset,
			Amount:    settlement.SettlementAmount,
			ToAddress: settlement.NECOAddress,
		})
		if err != nil {
			settlement.Status = TradingSettlementStatusFailed
			settlement.Reason = err.Error()
			_ = s.store.Save(ctx, settlement)
			return settlement, err
		}
		settlement.LedgerWithdrawalID = withdrawal.ID
		if err := s.store.Save(ctx, settlement); err != nil {
			return TradingSettlement{}, err
		}
	}
	if s.withdrawer == nil {
		return settlement, nil
	}
	record, err := s.withdrawer.ProcessWithdrawal(ctx, settlement.LedgerWithdrawalID)
	if err != nil {
		settlement.Status = TradingSettlementStatusFailed
		settlement.Reason = err.Error()
		_ = s.store.Save(ctx, settlement)
		return settlement, err
	}
	if record.Status == NECOWithdrawalStatusFailed {
		settlement.Status = TradingSettlementStatusFailed
		settlement.Reason = firstNonEmptyTradingString(record.Reason, "neco withdrawal failed")
		_ = s.store.Save(ctx, settlement)
		return settlement, errors.New(settlement.Reason)
	}
	if record.Status == NECOWithdrawalStatusPaid {
		settlement.Status = TradingSettlementStatusNECOPayoutPaid
		settlement.NECOTxHash = record.TxHash
		settlement.Reason = ""
		if err := s.store.Save(ctx, settlement); err != nil {
			return TradingSettlement{}, err
		}
		return settlement, nil
	}
	settlement.Status = TradingSettlementStatusNECOPayoutBroadcasted
	settlement.NECOTxHash = record.TxHash
	settlement.Reason = ""
	if err := s.store.Save(ctx, settlement); err != nil {
		return TradingSettlement{}, err
	}
	return settlement, nil
}

func (s *CloseSettlementService) refreshFailedLedgerWithdrawal(ctx context.Context, settlement TradingSettlement) (TradingSettlement, error) {
	if settlement.LedgerWithdrawalID == "" || s.ledger == nil {
		return settlement, nil
	}
	withdrawal, err := s.ledger.GetWithdrawal(ctx, settlement.LedgerWithdrawalID)
	if err != nil {
		return settlement, nil
	}
	if withdrawal.Status != LedgerWithdrawalStatusFailed {
		return settlement, nil
	}
	settlement.LedgerWithdrawalID = ""
	settlement.NECOTxHash = ""
	settlement.Reason = ""
	if err := s.store.Save(ctx, settlement); err != nil {
		return TradingSettlement{}, err
	}
	return settlement, nil
}

func (s *CloseSettlementService) continueClosedPositionSettlement(ctx context.Context, settlement TradingSettlement, order TradingOrder, fill TradingFill) (TradingSettlement, error) {
	settlement, err := s.continueSettlement(ctx, settlement)
	if err != nil {
		return settlement, err
	}
	return s.sendTradeCloseCallback(ctx, settlement, order, fill)
}

func (s *CloseSettlementService) sendTradeCloseCallback(ctx context.Context, settlement TradingSettlement, order TradingOrder, fill TradingFill) (TradingSettlement, error) {
	if s == nil || s.callback == nil || settlement.Kind != TradingSettlementKindClosePosition {
		return settlement, nil
	}
	if settlement.TradeCloseCallbackStatus == TradingCloseCallbackStatusSent {
		return settlement, nil
	}
	settlement.TradeCloseCallbackURL = s.conf.TradeCloseCallbackURL
	settlement.TradeCloseCallbackAt = time.Now().Unix()
	callbackFill, err := s.callbackFill(ctx, order, fill)
	if err != nil {
		settlement.TradeCloseCallbackStatus = TradingCloseCallbackStatusFailed
		settlement.TradeCloseCallbackReason = err.Error()
		_ = s.store.Save(ctx, settlement)
		return settlement, nil
	}
	s.enrichCallbackFillPartyAddresses(ctx, &callbackFill)
	if err := s.callback.SendTradeClose(ctx, NewTradingCloseCallbackPayload(settlement, order, callbackFill, s.markets)); err != nil {
		settlement.TradeCloseCallbackStatus = TradingCloseCallbackStatusFailed
		settlement.TradeCloseCallbackReason = err.Error()
		_ = s.store.Save(ctx, settlement)
		return settlement, nil
	}
	settlement.TradeCloseCallbackStatus = TradingCloseCallbackStatusSent
	settlement.TradeCloseCallbackReason = ""
	if err := s.store.Save(ctx, settlement); err != nil {
		return TradingSettlement{}, err
	}
	return settlement, nil
}

func (s *CloseSettlementService) callbackFill(ctx context.Context, order TradingOrder, fill TradingFill) (TradingFill, error) {
	if !tradingCloseCallbackFillMissing(fill) {
		return fill, nil
	}
	if s == nil || s.fills == nil {
		return TradingFill{}, ErrTradingCloseCallbackFillMissing
	}
	fills, err := s.fills.ListFillsByOrder(ctx, order.UserID, order.ID)
	if err != nil {
		return TradingFill{}, err
	}
	if len(fills) == 0 {
		return TradingFill{}, ErrTradingCloseCallbackFillMissing
	}
	return fills[len(fills)-1], nil
}

func tradingCloseCallbackFillMissing(fill TradingFill) bool {
	return strings.TrimSpace(fill.EventID) == "" &&
		strings.TrimSpace(fill.TradeHash) == "" &&
		strings.TrimSpace(fill.FillAmount) == ""
}

func (s *CloseSettlementService) enrichCallbackFillPartyAddresses(ctx context.Context, fill *TradingFill) {
	if s == nil || s.bindings == nil || fill == nil {
		return
	}
	if fill.MakerNECOAddress == "" {
		fill.MakerNECOAddress = s.necoAddressForDYDX(ctx, fill.MakerAddress)
	}
	if fill.TakerNECOAddress == "" {
		fill.TakerNECOAddress = s.necoAddressForDYDX(ctx, fill.TakerAddress)
	}
}

func (s *CloseSettlementService) necoAddressForDYDX(ctx context.Context, dydxAddress string) string {
	if strings.TrimSpace(dydxAddress) == "" {
		return ""
	}
	binding, err := s.bindings.GetByDYDXAddress(ctx, dydxAddress)
	if err != nil || binding.Status != WalletBindingStatusActive {
		return ""
	}
	return binding.NECOAddress
}

func (s *CloseSettlementService) validateCancelSettlementRequest(request CancelSettlementRequest) error {
	order := request.Order
	if order.ClosePosition || order.Status != TradingOrderStatusCancelled || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return ErrTradingSettlementNotConfirmed
	}
	if request.Reconcile.StatefulOrder.Checked && request.Reconcile.StatefulOrder.Found {
		return ErrTradingSettlementNotConfirmed
	}
	return nil
}

func (s *CloseSettlementService) validateFailedOrderSettlementRequest(request FailedOrderSettlementRequest) error {
	order := request.Order
	if order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return ErrTradingSettlementNotConfirmed
	}
	switch order.Status {
	case TradingOrderStatusRejected,
		TradingOrderStatusSubmitFailed,
		TradingOrderStatusEquityTierLimit:
	default:
		return ErrTradingSettlementNotConfirmed
	}
	if request.Reconcile.StatefulOrder.Checked && request.Reconcile.StatefulOrder.Found {
		return ErrTradingSettlementNotConfirmed
	}
	return nil
}

func (s *CloseSettlementService) validateCloseSettlementRequest(request CloseSettlementRequest) error {
	order := request.CloseOrder
	if !order.ClosePosition {
		return ErrTradingSettlementNotConfirmed
	}
	if order.Status != TradingOrderStatusClosed && order.Status != TradingOrderStatusFilled {
		return ErrTradingSettlementNotConfirmed
	}
	if request.Reconcile.Subaccount.Found {
		if market, ok := s.markets[normalizeDYDXMarket(order.Market)]; ok {
			if closeOrderHasRemainingMarketPosition(order, market, request.Reconcile.Subaccount.PerpetualPositions) {
				return ErrTradingSettlementNotConfirmed
			}
		} else if hasAnyNonZeroPerpetualPosition(request.Reconcile.Subaccount.PerpetualPositions) {
			return ErrTradingSettlementNotConfirmed
		}
	}
	return nil
}

func (s *CloseSettlementService) cancelSettlementUSD(ctx context.Context, request CancelSettlementRequest) (string, error) {
	switch s.conf.Mode {
	case CloseSettlementModeFullAccounting:
		allowUnfilledMargin := !cancelSettlementHasKnownFill(request)
		return s.fullAccountingSettlementUSDForOrder(ctx, request.Order, request.Fill, allowUnfilledMargin)
	default:
		return marginOnlySettlementUSD(request.Order)
	}
}

func cancelSettlementHasKnownFill(request CancelSettlementRequest) bool {
	if tradingFillHasAccountingFields(request.Fill) {
		return true
	}
	fillAmount, ok := parseUint64String(request.Reconcile.StatefulOrder.FillAmount)
	return ok && fillAmount > 0
}

func (s *CloseSettlementService) settlementUSD(ctx context.Context, request CloseSettlementRequest) (string, error) {
	switch s.conf.Mode {
	case CloseSettlementModeFullAccounting:
		return s.fullAccountingSettlementUSDForOrder(ctx, request.CloseOrder, request.Fill, false)
	default:
		return marginOnlySettlementUSD(request.CloseOrder)
	}
}

func (s *CloseSettlementService) failedOrderSettlementValues(ctx context.Context, order TradingOrder) (settlementUSD, amount, assetPrice, snapshotID string, err error) {
	settlementUSD, err = marginOnlySettlementUSD(order)
	if err != nil {
		return "", "", "", "", err
	}
	asset := normalizeAsset(order.MarginAsset)
	if asset == "" {
		return "", "", "", "", ErrTradingSettlementManualReview
	}
	amount = strings.TrimSpace(order.MarginAmount)
	if parsed, ok := parseLedgerAmount(amount); !ok || parsed.Sign() <= 0 {
		return "", "", "", "", ErrTradingSettlementInvalidAmount
	}
	assetPrice = firstNonEmpty(strings.TrimSpace(order.AssetUSDPrice), strings.TrimSpace(order.DEDXUSDPrice))
	snapshotID = strings.TrimSpace(order.PriceSnapshotID)
	if assetPrice == "" || snapshotID == "" {
		if snapshot, priceErr := s.settlementAssetPrice(ctx, asset); priceErr == nil {
			if assetPrice == "" {
				assetPrice = snapshot.USDPrice
			}
			if snapshotID == "" {
				snapshotID = snapshot.ID
			}
		}
	}
	return settlementUSD, amount, assetPrice, snapshotID, nil
}

func marginOnlySettlementUSD(order TradingOrder) (string, error) {
	value := strings.TrimSpace(order.MarginUSD)
	if value == "" {
		return "", ErrTradingSettlementManualReview
	}
	if looksLikeAtomicUSD(value) {
		return "", ErrTradingSettlementManualReview
	}
	if parsed, ok := parseDecimalRat(value); !ok || parsed.Sign() <= 0 {
		return "", ErrTradingSettlementInvalidAmount
	}
	return value, nil
}

func (s *CloseSettlementService) fullAccountingSettlementUSDForOrder(ctx context.Context, order TradingOrder, fill TradingFill, allowUnfilledMargin bool) (string, error) {
	fills := make([]TradingFill, 0, 1)
	if tradingFillHasAccountingFields(fill) {
		fills = append(fills, fill)
	}
	if len(fills) == 0 && s.fills != nil {
		list, err := s.listAccountingFills(ctx, order)
		if err != nil {
			return "", err
		}
		fills = list
	}
	if len(fills) == 0 {
		if allowUnfilledMargin {
			return marginOnlySettlementUSD(order)
		}
		return "", ErrTradingSettlementManualReview
	}
	return fullAccountingSettlementUSD(fills)
}

func (s *CloseSettlementService) orderSubaccountNumber(order TradingOrder) uint32 {
	fallback := defaultDYDXTradingSubaccountNumber
	var markets map[string]DYDXMarketConfig
	if s != nil {
		fallback = s.conf.SubaccountNumber
		markets = s.markets
	}
	return explicitOrBuiltInDYDXSubaccountNumber(
		order.SubaccountNumber,
		order.Market,
		clobPairIDForMarket(markets, order.Market),
		fallback,
	)
}

func (s *CloseSettlementService) listAccountingFills(ctx context.Context, order TradingOrder) ([]TradingFill, error) {
	orderIDs := s.accountingFillOrderIDs(order)
	fills := make([]TradingFill, 0)
	seen := map[string]bool{}
	for _, orderID := range orderIDs {
		list, err := s.fills.ListFillsByOrder(ctx, order.UserID, orderID)
		if err != nil {
			return nil, err
		}
		for _, fill := range list {
			if fill.EventID != "" && seen[fill.EventID] {
				continue
			}
			if fill.EventID != "" {
				seen[fill.EventID] = true
			}
			fills = append(fills, fill)
		}
	}
	return fills, nil
}

func (s *CloseSettlementService) accountingFillOrderIDs(order TradingOrder) []string {
	ids := make([]string, 0, 2)
	if strings.TrimSpace(order.ID) != "" {
		ids = append(ids, order.ID)
	}
	dydxOrderID := s.dydxFillOrderID(order)
	if dydxOrderID != "" && dydxOrderID != order.ID {
		ids = append(ids, dydxOrderID)
	}
	return ids
}

func (s *CloseSettlementService) dydxFillOrderID(order TradingOrder) string {
	if strings.TrimSpace(order.DYDXAccount) == "" || strings.TrimSpace(order.ClientOrderID) == "" {
		return ""
	}
	market, ok := s.markets[normalizeDYDXMarket(order.Market)]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d:%s:%d", order.DYDXAccount, s.orderSubaccountNumber(order), order.ClientOrderID, market.ClobPairID)
}

func tradingFillHasAccountingFields(fill TradingFill) bool {
	return strings.TrimSpace(fill.ReleasedMargin) != "" &&
		strings.TrimSpace(fill.Fee) != "" &&
		strings.TrimSpace(fill.RealizedPNL) != ""
}

func fullAccountingSettlementUSD(fills []TradingFill) (string, error) {
	if len(fills) == 0 {
		return "", ErrTradingSettlementManualReview
	}
	releasedTotal := big.NewRat(0, 1)
	feeTotal := big.NewRat(0, 1)
	pnlTotal := big.NewRat(0, 1)
	for _, fill := range fills {
		if !tradingFillHasAccountingFields(fill) {
			return "", ErrTradingSettlementManualReview
		}
		released, ok := parseDecimalRat(fill.ReleasedMargin)
		if !ok {
			return "", ErrTradingSettlementInvalidAmount
		}
		fee, ok := parseDecimalRat(fill.Fee)
		if !ok {
			return "", ErrTradingSettlementInvalidAmount
		}
		pnl, ok := parseDecimalRat(fill.RealizedPNL)
		if !ok {
			return "", ErrTradingSettlementInvalidAmount
		}
		releasedTotal.Add(releasedTotal, released)
		feeTotal.Add(feeTotal, fee)
		pnlTotal.Add(pnlTotal, pnl)
	}
	if releasedTotal.Sign() <= 0 {
		return "", ErrTradingSettlementManualReview
	}
	total := new(big.Rat).Add(releasedTotal, pnlTotal)
	total.Sub(total, feeTotal)
	if total.Sign() <= 0 {
		return "", ErrTradingSettlementInvalidAmount
	}
	return ratToDecimalString(total, 18), nil
}

func (s *CloseSettlementService) resolveNECOAddress(ctx context.Context, order TradingOrder) (string, error) {
	if common.IsHexAddress(order.NECOAddress) {
		return order.NECOAddress, nil
	}
	if s.bindings == nil {
		return "", ErrWalletBindingNotFound
	}
	binding, err := s.bindings.GetByUserID(ctx, order.UserID)
	if err != nil {
		return "", err
	}
	if binding.Status != WalletBindingStatusActive || !common.IsHexAddress(binding.NECOAddress) {
		return "", ErrWalletBindingNotFound
	}
	return binding.NECOAddress, nil
}

func (s *CloseSettlementService) reservedQuantums(order TradingOrder, settlementUSD string) string {
	if strings.TrimSpace(order.DYDXOrderMarginQuantums) != "" {
		return order.DYDXOrderMarginQuantums
	}
	if strings.TrimSpace(order.DYDXFundingQuantums) != "" {
		return order.DYDXFundingQuantums
	}
	quantums, err := parseScaledUint64(settlementUSD, s.conf.CollateralQuantumsPerUSD)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(quantums, 10)
}

type closeSettlementAssetSpec struct {
	Asset    string
	Decimals uint8
}

func (s *CloseSettlementService) settlementAssetSpec(order TradingOrder, kind string) closeSettlementAssetSpec {
	entryAsset := normalizeAsset(order.MarginAsset)
	if entryAsset == "" {
		entryAsset = normalizeAsset(s.conf.SettlementAsset)
	}
	if entryAsset == "" {
		entryAsset = defaultCloseSettlementAsset
	}
	asset := entryAsset
	if kind == TradingSettlementKindClosePosition && normalizeDYDXMarket(order.Market) == "FDEX-USD" {
		switch entryAsset {
		case "DEDX", "FDEX":
			asset = "USDT"
		case "USDT":
			asset = s.dedxUSDSettlementBaseAsset()
		}
	}
	return closeSettlementAssetSpec{
		Asset:    asset,
		Decimals: s.settlementAssetDecimals(asset),
	}
}

func (s *CloseSettlementService) dedxUSDSettlementBaseAsset() string {
	if s != nil {
		if _, ok := s.assets["FDEX"]; ok {
			return "FDEX"
		}
		if normalizeAsset(s.conf.SettlementAsset) == "FDEX" {
			return "FDEX"
		}
	}
	return defaultCloseSettlementAsset
}

func (s *CloseSettlementService) settlementAssetDecimals(asset string) uint8 {
	asset = normalizeAsset(asset)
	if mapping, ok := s.assets[asset]; ok && mapping.Decimals != 0 {
		return mapping.Decimals
	}
	if asset == normalizeAsset(s.conf.SettlementAsset) && s.conf.SettlementAssetDecimals != 0 {
		return s.conf.SettlementAssetDecimals
	}
	if asset == "USDT" {
		return defaultCloseSettlementAssetDecimals
	}
	return defaultCloseSettlementAssetDecimals
}

func (s *CloseSettlementService) settlementAssetPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	asset = normalizeAsset(asset)
	if asset == "" {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	if mapping, ok := s.assets[asset]; ok && normalizeAsset(mapping.PriceRoute) == "USDT-USD" {
		return stableUSDRoutePrice(PriceRouteConfig{
			Name:     defaultString(mapping.PriceRoute, "USDT-USD"),
			Asset:    asset,
			USDAsset: "USD",
		})
	}
	if asset == "USDT" && s.oracle == nil {
		return stableUSDRoutePrice(PriceRouteConfig{
			Name:     "USDT-USD",
			Asset:    asset,
			USDAsset: "USD",
		})
	}
	if s.oracle == nil {
		return AssetPriceSnapshot{}, errors.New("settlement asset price oracle is not configured")
	}
	return s.oracle.AssetUSDPrice(ctx, asset)
}

func (s *CloseSettlementService) manualReviewSettlement(order TradingOrder, kind, necoAddress, reason string) TradingSettlement {
	assetSpec := s.settlementAssetSpec(order, kind)
	return TradingSettlement{
		UserID:             order.UserID,
		Kind:               kind,
		NECOAddress:        necoAddress,
		DYDXAccount:        order.DYDXAccount,
		SubaccountNumber:   s.orderSubaccountNumber(order),
		Market:             order.Market,
		CloseOrderID:       order.ID,
		CloseClientOrderID: order.ClientOrderID,
		SettlementAsset:    assetSpec.Asset,
		SystemReserved:     true,
		Status:             TradingSettlementStatusManualReviewRequired,
		Reason:             reason,
	}
}

func (s *CloseSettlementService) saveManualReview(ctx context.Context, settlement TradingSettlement) (TradingSettlement, error) {
	existing, _, err := s.store.CreateIfAbsent(ctx, settlement)
	return existing, err
}

func hasAnyNonZeroPerpetualPosition(positions []DYDXPerpetualPositionReconcileSnapshot) bool {
	for _, position := range positions {
		quantums, ok := new(big.Int).SetString(strings.TrimSpace(position.Quantums), 10)
		if ok && quantums.Sign() != 0 {
			return true
		}
	}
	return false
}

func settlementCreditRefID(settlement TradingSettlement) string {
	kind := settlement.Kind
	if kind == "" {
		kind = TradingSettlementKindClosePosition
	}
	return "dydx-" + kind + "-settlement:" + settlement.UserID + ":" + settlement.CloseOrderID + ":credit"
}
