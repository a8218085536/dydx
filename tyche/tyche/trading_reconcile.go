package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultTradingReconcileLimit = 100
)

type DYDXOrderReconcileRequest struct {
	TxHash           string `json:"txHash"`
	UserID           string `json:"userId"`
	OrderID          string `json:"orderId"`
	DYDXAccount      string `json:"dydxAccount"`
	ClientOrderID    string `json:"clientOrderId"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber"`
	ClobPairID       uint32 `json:"clobPairId"`
	OrderFlags       uint32 `json:"orderFlags"`
}

type DYDXOrderReconcileResult struct {
	Tx            DYDXTxReconcileSnapshot            `json:"tx"`
	StatefulOrder DYDXStatefulOrderReconcileSnapshot `json:"statefulOrder"`
	Subaccount    DYDXSubaccountReconcileSnapshot    `json:"subaccount"`
}

type DYDXTxReconcileSnapshot struct {
	Found  bool   `json:"found"`
	TxHash string `json:"txHash"`
	Height string `json:"height"`
	Code   uint32 `json:"code"`
	RawLog string `json:"rawLog"`
}

type DYDXStatefulOrderReconcileSnapshot struct {
	Checked    bool   `json:"checked"`
	Found      bool   `json:"found"`
	FillAmount string `json:"fillAmount"`
}

type DYDXSubaccountReconcileSnapshot struct {
	Found              bool                                     `json:"found"`
	AssetPositions     []DYDXAssetPositionReconcileSnapshot     `json:"assetPositions"`
	PerpetualPositions []DYDXPerpetualPositionReconcileSnapshot `json:"perpetualPositions"`
}

type DYDXAssetPositionReconcileSnapshot struct {
	AssetID  uint32 `json:"assetId"`
	Quantums string `json:"quantums"`
}

type DYDXPerpetualPositionReconcileSnapshot struct {
	PerpetualID  uint32 `json:"perpetualId"`
	Quantums     string `json:"quantums"`
	FundingIndex string `json:"fundingIndex,omitempty"`
	QuoteBalance string `json:"quoteBalance,omitempty"`
}

type DYDXOrderReconcileSource interface {
	ReconcileOrder(context.Context, DYDXOrderReconcileRequest) (DYDXOrderReconcileResult, error)
}

type DYDXOrderReconcileHTTPSourceConfig struct {
	URL    string
	APIKey string
}

type DYDXOrderReconcileHTTPSource struct {
	conf   DYDXOrderReconcileHTTPSourceConfig
	client *http.Client
}

func NewDYDXOrderReconcileHTTPSource(conf DYDXOrderReconcileHTTPSourceConfig) *DYDXOrderReconcileHTTPSource {
	return &DYDXOrderReconcileHTTPSource{conf: conf, client: http.DefaultClient}
}

func (s *DYDXOrderReconcileHTTPSource) ReconcileOrder(ctx context.Context, request DYDXOrderReconcileRequest) (DYDXOrderReconcileResult, error) {
	if s.conf.URL == "" {
		return DYDXOrderReconcileResult{}, ErrDYDXChainClientNotConfigured
	}
	endpoint, err := url.Parse(strings.TrimRight(s.conf.URL, "/") + "/reconcile/order")
	if err != nil {
		return DYDXOrderReconcileResult{}, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return DYDXOrderReconcileResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(raw))
	if err != nil {
		return DYDXOrderReconcileResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if s.conf.APIKey != "" {
		httpReq.Header.Set("X-API-Key", s.conf.APIKey)
	}
	httpResp, err := s.client.Do(httpReq)
	if err != nil {
		return DYDXOrderReconcileResult{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return DYDXOrderReconcileResult{}, fmt.Errorf("dydx reconcile source returned http %d", httpResp.StatusCode)
	}
	var result struct {
		Code int                      `json:"code"`
		Msg  string                   `json:"msg"`
		Data DYDXOrderReconcileResult `json:"data"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return DYDXOrderReconcileResult{}, err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("dydx reconcile source returned code %d", result.Code)
		}
		return DYDXOrderReconcileResult{}, errors.New(result.Msg)
	}
	return result.Data, nil
}

type TradingReconcileOrderStore interface {
	ListReconcilableOrders(context.Context, int) ([]TradingOrder, error)
}

type TradingReconcileConfig struct {
	SubaccountNumber uint32
	OrderFlags       uint32
	Markets          []DYDXMarketConfig
}

type TradingReconcileService struct {
	source           DYDXOrderReconcileSource
	sync             *TradingSyncService
	closeSettlement  closeSettlementHandler
	subaccountNumber uint32
	orderFlags       uint32
	markets          map[string]DYDXMarketConfig
}

type closeSettlementHandler interface {
	SettleClosedPosition(context.Context, CloseSettlementRequest) (TradingSettlement, error)
	SettleCancelledOrder(context.Context, CancelSettlementRequest) (TradingSettlement, error)
	SettleFailedOrder(context.Context, FailedOrderSettlementRequest) (TradingSettlement, error)
}

func NewTradingReconcileService(source DYDXOrderReconcileSource, sync *TradingSyncService, conf TradingReconcileConfig) *TradingReconcileService {
	if conf.OrderFlags == 0 {
		conf.OrderFlags = DYDXOrderFlagsLongTerm
	}
	service := &TradingReconcileService{
		source:           source,
		sync:             sync,
		subaccountNumber: conf.SubaccountNumber,
		orderFlags:       conf.OrderFlags,
		markets:          map[string]DYDXMarketConfig{},
	}
	for _, market := range conf.Markets {
		service.markets[normalizeDYDXMarket(market.Market)] = market
	}
	return service
}

func (s *TradingReconcileService) SetCloseSettlementService(service closeSettlementHandler) {
	s.closeSettlement = service
}

func (s *TradingReconcileService) ReconcileOrder(ctx context.Context, order TradingOrder) (DYDXOrderReconcileResult, error) {
	if s == nil || s.source == nil {
		return DYDXOrderReconcileResult{}, ErrDYDXChainClientNotConfigured
	}
	market, ok := s.markets[normalizeDYDXMarket(order.Market)]
	if !ok {
		return DYDXOrderReconcileResult{}, ErrDYDXMarketNotConfigured
	}
	subaccountNumber := s.orderSubaccountNumber(order, market)
	txHash := tradingOrderReconcileTxHash(order)
	request := DYDXOrderReconcileRequest{
		TxHash:           txHash,
		UserID:           order.UserID,
		OrderID:          order.ID,
		DYDXAccount:      order.DYDXAccount,
		ClientOrderID:    order.ClientOrderID,
		Market:           order.Market,
		SubaccountNumber: subaccountNumber,
		ClobPairID:       market.ClobPairID,
		OrderFlags:       s.orderFlags,
	}
	result, err := s.source.ReconcileOrder(ctx, request)
	if err != nil {
		return DYDXOrderReconcileResult{}, err
	}
	if result.Tx.Found && result.Tx.Code != 0 {
		if err := s.syncStatus(ctx, order, TradingOrderStatusRejected, reconcileEventID("tx-rejected", order, result.Tx.TxHash), result.Tx.RawLog); err != nil {
			return result, err
		}
		order.Status = orderStatusForDYDXResult(order, TradingOrderStatusRejected)
		return result, s.settleFailedFundedOrder(ctx, order, result)
	}
	if result.Tx.Found && result.Tx.Code == 0 && isBroadcastedTradingOrderStatus(order.Status) {
		if err := s.syncStatus(ctx, order, TradingOrderStatusSubmitted, reconcileEventID("tx-submitted", order, result.Tx.TxHash), ""); err != nil {
			return result, err
		}
		order.Status = orderStatusForDYDXResult(order, TradingOrderStatusSubmitted)
	}

	fillAmount, hasFillAmount := parseUint64String(result.StatefulOrder.FillAmount)
	if result.StatefulOrder.Found && hasFillAmount && fillAmount > 0 {
		status := TradingOrderStatusPartialFilled
		if total, err := parseScaledUint64(order.Size, market.QuantumsPerUnit); err == nil && fillAmount >= total {
			status = TradingOrderStatusFilled
		}
		return result, s.syncStatus(ctx, order, status, reconcileEventID("stateful-fill:"+strconv.FormatUint(fillAmount, 10), order, result.Tx.TxHash), "")
	}
	if result.StatefulOrder.Checked && !result.StatefulOrder.Found {
		if order.ClosePosition {
			if result.Subaccount.Found {
				closeStatus := closeOrderStatusFromRemainingPosition(order, market, result.Subaccount.PerpetualPositions)
				switch closeStatus {
				case TradingOrderStatusCloseCancelled:
					return result, s.syncStatus(ctx, order, TradingOrderStatusCancelled, reconcileEventID("close-cancelled", order, result.Tx.TxHash), "")
				case TradingOrderStatusClosePartialFilled:
					return result, s.syncStatus(ctx, order, TradingOrderStatusPartialFilled, reconcileEventID("close-partial-filled", order, result.Tx.TxHash), "")
				}
			}
			if err := s.syncStatus(ctx, order, TradingOrderStatusFilled, reconcileEventID("close-filled", order, result.Tx.TxHash), ""); err != nil {
				return result, err
			}
			order.Status = orderStatusForDYDXResult(order, TradingOrderStatusFilled)
			if err := s.syncClearedPosition(ctx, order, market, result); err != nil {
				return result, err
			}
			if err := s.settleClosedPosition(ctx, order, result); err != nil {
				return result, err
			}
			return result, nil
		}
		if order.Status == TradingOrderStatusCancelPending {
			if err := s.syncStatus(ctx, order, TradingOrderStatusCancelled, reconcileEventID("cancelled", order, result.Tx.TxHash), ""); err != nil {
				return result, err
			}
			order.Status = TradingOrderStatusCancelled
			if err := s.settleCancelledOrder(ctx, order, result); err != nil {
				return result, err
			}
			return result, nil
		}
		if position, ok := matchingReconcilePosition(order, market, result.Subaccount.PerpetualPositions); ok {
			if err := s.syncPosition(ctx, order, market, position); err != nil {
				return result, err
			}
			if err := s.syncStatus(ctx, order, TradingOrderStatusFilled, reconcileEventID("position-filled", order, position.Quantums), ""); err != nil {
				return result, err
			}
			return result, nil
		}
		if isUnfilledIOCTradingOrder(order) {
			if err := s.syncStatus(ctx, order, TradingOrderStatusCancelled, reconcileEventID("ioc-cancelled", order, result.Tx.TxHash), ""); err != nil {
				return result, err
			}
			order.Status = TradingOrderStatusCancelled
			if err := s.settleCancelledOrder(ctx, order, result); err != nil {
				return result, err
			}
			return result, nil
		}
	}
	return result, nil
}

func (s *TradingReconcileService) orderSubaccountNumber(order TradingOrder, market DYDXMarketConfig) uint32 {
	fallback := defaultDYDXTradingSubaccountNumber
	if s != nil {
		fallback = s.subaccountNumber
	}
	return explicitOrBuiltInDYDXSubaccountNumber(order.SubaccountNumber, order.Market, market.ClobPairID, fallback)
}

func (s *TradingReconcileService) settleClosedPosition(ctx context.Context, order TradingOrder, result DYDXOrderReconcileResult) error {
	if s.closeSettlement == nil {
		return nil
	}
	_, err := s.closeSettlement.SettleClosedPosition(ctx, CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  result,
	})
	if errors.Is(err, ErrTradingSettlementManualReview) {
		return nil
	}
	return err
}

func (s *TradingReconcileService) settleCancelledOrder(ctx context.Context, order TradingOrder, result DYDXOrderReconcileResult) error {
	if s.closeSettlement == nil || order.DYDXFundingTxHash == "" {
		return nil
	}
	_, err := s.closeSettlement.SettleCancelledOrder(ctx, CancelSettlementRequest{
		Order:     order,
		Reconcile: result,
	})
	if errors.Is(err, ErrTradingSettlementManualReview) {
		return nil
	}
	return err
}

func (s *TradingReconcileService) settleFailedFundedOrder(ctx context.Context, order TradingOrder, result DYDXOrderReconcileResult) error {
	if s.closeSettlement == nil || order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return nil
	}
	_, err := s.closeSettlement.SettleFailedOrder(ctx, FailedOrderSettlementRequest{
		Order:     order,
		Reconcile: result,
	})
	if errors.Is(err, ErrTradingSettlementManualReview) {
		return nil
	}
	return err
}

func (s *TradingReconcileService) syncStatus(ctx context.Context, order TradingOrder, status, eventID, reason string) error {
	_, err := s.sync.SyncOrderStatus(ctx, TradingOrderStatusSyncRequest{
		EventID: eventID,
		UserID:  order.UserID,
		OrderID: order.ID,
		Status:  status,
		Reason:  reason,
	})
	if errors.Is(err, ErrTradingSyncDuplicateEvent) {
		return nil
	}
	return err
}

func (s *TradingReconcileService) syncPosition(ctx context.Context, order TradingOrder, market DYDXMarketConfig, position DYDXPerpetualPositionReconcileSnapshot) error {
	size, ok := quantumsToUnitString(position.Quantums, market.QuantumsPerUnit)
	if !ok {
		size = position.Quantums
	}
	_, err := s.sync.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:          reconcileEventID("position", order, position.Quantums),
		UserID:           order.UserID,
		Market:           order.Market,
		SubaccountNumber: s.orderSubaccountNumber(order, market),
		Size:             size,
		MarginAsset:      order.MarginAsset,
		Margin:           order.MarginUSD,
	})
	if errors.Is(err, ErrTradingSyncDuplicateEvent) {
		return nil
	}
	return err
}

func (s *TradingReconcileService) syncClearedPosition(ctx context.Context, order TradingOrder, market DYDXMarketConfig, result DYDXOrderReconcileResult) error {
	if closeOrderHasRemainingMarketPosition(order, market, result.Subaccount.PerpetualPositions) {
		return nil
	}
	_, err := s.sync.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:          reconcileEventID("position-cleared", order, result.Tx.TxHash),
		UserID:           order.UserID,
		Market:           order.Market,
		SubaccountNumber: s.orderSubaccountNumber(order, market),
		Size:             "0",
		MarginAsset:      order.MarginAsset,
		Margin:           "0",
	})
	if errors.Is(err, ErrTradingSyncDuplicateEvent) {
		return nil
	}
	return err
}

type TradingReconcileWorker struct {
	store   TradingReconcileOrderStore
	service *TradingReconcileService
	limit   int
}

func NewTradingReconcileWorker(store TradingReconcileOrderStore, service *TradingReconcileService, limit int) *TradingReconcileWorker {
	if limit <= 0 {
		limit = defaultTradingReconcileLimit
	}
	return &TradingReconcileWorker{store: store, service: service, limit: limit}
}

func (w *TradingReconcileWorker) SetCloseSettlementService(service closeSettlementHandler) {
	if w != nil && w.service != nil {
		w.service.SetCloseSettlementService(service)
	}
}

func (w *TradingReconcileWorker) PollOnce(ctx context.Context) error {
	if w == nil || w.store == nil || w.service == nil {
		return nil
	}
	orders, err := w.store.ListReconcilableOrders(ctx, w.limit)
	if err != nil {
		return err
	}
	var firstErr error
	for _, order := range orders {
		if isFailedFundedTradingOrder(order) {
			if err := w.service.settleFailedFundedOrder(ctx, order, DYDXOrderReconcileResult{}); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if order.DYDXOrderTxHash == "" || !isReconcilableTradingOrderStatus(order.Status) {
			continue
		}
		if _, err := w.service.ReconcileOrder(ctx, order); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func isReconcilableTradingOrderStatus(status string) bool {
	switch status {
	case TradingOrderStatusBroadcasted,
		TradingOrderStatusSubmitted,
		TradingOrderStatusPartialFilled,
		TradingOrderStatusCancelPending,
		TradingOrderStatusCloseBroadcasted,
		TradingOrderStatusCloseSubmitted,
		TradingOrderStatusClosePartialFilled:
		return true
	default:
		return false
	}
}

func isFailedFundedTradingOrder(order TradingOrder) bool {
	if order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return false
	}
	switch order.Status {
	case TradingOrderStatusRejected,
		TradingOrderStatusSubmitFailed,
		TradingOrderStatusEquityTierLimit:
		return true
	default:
		return false
	}
}

func isBroadcastedTradingOrderStatus(status string) bool {
	return status == TradingOrderStatusBroadcasted || status == TradingOrderStatusCloseBroadcasted
}

func tradingOrderReconcileTxHash(order TradingOrder) string {
	if order.Status == TradingOrderStatusCancelPending && strings.TrimSpace(order.DYDXCancelTxHash) != "" {
		return order.DYDXCancelTxHash
	}
	return order.DYDXOrderTxHash
}

func reconcileEventID(prefix string, order TradingOrder, suffix string) string {
	return "dydx-reconcile:" + prefix + ":" + order.UserID + ":" + order.ID + ":" + suffix
}

func isUnfilledIOCTradingOrder(order TradingOrder) bool {
	return strings.EqualFold(order.ExecutionOrderType, TradingOrderTypeIOC) ||
		(strings.EqualFold(order.OrderType, TradingOrderTypeMarket) && strings.TrimSpace(order.ExecutionOrderType) == "")
}

func matchingReconcilePosition(order TradingOrder, market DYDXMarketConfig, positions []DYDXPerpetualPositionReconcileSnapshot) (DYDXPerpetualPositionReconcileSnapshot, bool) {
	for _, position := range positions {
		if position.PerpetualID != market.ClobPairID {
			continue
		}
		quantums, ok := new(big.Int).SetString(strings.TrimSpace(position.Quantums), 10)
		if !ok || quantums.Sign() == 0 {
			continue
		}
		switch strings.ToLower(order.Side) {
		case "buy", "bid":
			if quantums.Sign() > 0 {
				return position, true
			}
		case "sell", "ask":
			if quantums.Sign() < 0 {
				return position, true
			}
		}
	}
	return DYDXPerpetualPositionReconcileSnapshot{}, false
}

func closeOrderHasRemainingMarketPosition(order TradingOrder, market DYDXMarketConfig, positions []DYDXPerpetualPositionReconcileSnapshot) bool {
	_, ok := marketPositionFromReconcile(market, positions)
	return ok
}

func marketPositionFromReconcile(market DYDXMarketConfig, positions []DYDXPerpetualPositionReconcileSnapshot) (DYDXPerpetualPositionReconcileSnapshot, bool) {
	for _, position := range positions {
		if position.PerpetualID != market.ClobPairID {
			continue
		}
		quantums, ok := new(big.Int).SetString(strings.TrimSpace(position.Quantums), 10)
		if ok && quantums.Sign() != 0 {
			return position, true
		}
	}
	return DYDXPerpetualPositionReconcileSnapshot{}, false
}

func closeOrderStatusFromRemainingPosition(order TradingOrder, market DYDXMarketConfig, positions []DYDXPerpetualPositionReconcileSnapshot) string {
	remaining, ok := remainingPositionAbsQuantums(market, positions)
	if !ok || remaining.Sign() == 0 {
		return TradingOrderStatusClosed
	}
	orderQuantums, err := parseScaledUint64(order.Size, market.QuantumsPerUnit)
	if err != nil || orderQuantums == 0 {
		return ""
	}
	if remaining.Cmp(new(big.Int).SetUint64(orderQuantums)) >= 0 {
		return TradingOrderStatusCloseCancelled
	}
	return TradingOrderStatusClosePartialFilled
}

func remainingPositionAbsQuantums(market DYDXMarketConfig, positions []DYDXPerpetualPositionReconcileSnapshot) (*big.Int, bool) {
	for _, position := range positions {
		if position.PerpetualID != market.ClobPairID {
			continue
		}
		quantums, ok := new(big.Int).SetString(strings.TrimSpace(position.Quantums), 10)
		if !ok {
			return nil, false
		}
		return new(big.Int).Abs(quantums), true
	}
	return big.NewInt(0), true
}

func parseUint64String(value string) (uint64, bool) {
	if strings.TrimSpace(value) == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil
}

func quantumsToUnitString(quantums, quantumsPerUnit string) (string, bool) {
	value, ok := new(big.Int).SetString(strings.TrimSpace(quantums), 10)
	if !ok {
		return "", false
	}
	scale, ok := parseDecimalRat(quantumsPerUnit)
	if !ok || scale.Sign() <= 0 {
		return "", false
	}
	out := new(big.Rat).SetInt(value)
	out.Quo(out, scale)
	return ratToDecimalString(out, 18), true
}
