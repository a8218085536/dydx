package tyche

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrTradingSyncDuplicateEvent = errors.New("trading sync duplicate event")
var ErrTradingSyncEventNotFound = errors.New("trading sync event not found")

type TradingFillSyncRequest struct {
	EventID            string `json:"eventId"`
	TradeHash          string `json:"tradeHash"`
	UserID             string `json:"userId"`
	OrderID            string `json:"orderId"`
	Market             string `json:"market"`
	Asset              string `json:"asset"`
	ReleasedMargin     string `json:"releasedMargin"`
	Fee                string `json:"fee"`
	RealizedPNL        string `json:"realizedPnl"`
	OrderStatus        string `json:"orderStatus"`
	LiquidityRole      string `json:"liquidityRole"`
	MakerAddress       string `json:"makerAddress"`
	MakerNECOAddress   string `json:"makerNecoAddress"`
	TakerAddress       string `json:"takerAddress"`
	TakerNECOAddress   string `json:"takerNecoAddress"`
	MakerOrderID       string `json:"makerOrderId"`
	TakerOrderID       string `json:"takerOrderId"`
	ClobPairID         uint32 `json:"clobPairId"`
	FillAmount         string `json:"fillAmount"`
	MakerFee           string `json:"makerFee"`
	TakerFee           string `json:"takerFee"`
	MakerOrderSide     string `json:"makerOrderSide"`
	TakerOrderSide     string `json:"takerOrderSide"`
	MakerOrderQuantums string `json:"makerOrderQuantums"`
	MakerOrderSubticks string `json:"makerOrderSubticks"`
	TakerOrderQuantums string `json:"takerOrderQuantums"`
	TakerOrderSubticks string `json:"takerOrderSubticks"`
	CreatedAt          int64  `json:"createdAt"`
}

type TradingFill struct {
	EventID            string `json:"eventId" redis:"eventId"`
	TradeHash          string `json:"tradeHash,omitempty" redis:"tradeHash"`
	UserID             string `json:"userId" redis:"userId"`
	OrderID            string `json:"orderId" redis:"orderId"`
	Market             string `json:"market" redis:"market"`
	Asset              string `json:"asset" redis:"asset"`
	ReleasedMargin     string `json:"releasedMargin" redis:"releasedMargin"`
	Fee                string `json:"fee" redis:"fee"`
	RealizedPNL        string `json:"realizedPnl" redis:"realizedPnl"`
	OrderStatus        string `json:"orderStatus" redis:"orderStatus"`
	LiquidityRole      string `json:"liquidityRole,omitempty" redis:"liquidityRole"`
	MakerAddress       string `json:"makerAddress,omitempty" redis:"makerAddress"`
	MakerNECOAddress   string `json:"makerNecoAddress,omitempty" redis:"makerNecoAddress"`
	TakerAddress       string `json:"takerAddress,omitempty" redis:"takerAddress"`
	TakerNECOAddress   string `json:"takerNecoAddress,omitempty" redis:"takerNecoAddress"`
	MakerOrderID       string `json:"makerOrderId,omitempty" redis:"makerOrderId"`
	TakerOrderID       string `json:"takerOrderId,omitempty" redis:"takerOrderId"`
	ClobPairID         uint32 `json:"clobPairId,omitempty" redis:"clobPairId"`
	FillAmount         string `json:"fillAmount,omitempty" redis:"fillAmount"`
	MakerFee           string `json:"makerFee,omitempty" redis:"makerFee"`
	TakerFee           string `json:"takerFee,omitempty" redis:"takerFee"`
	MakerOrderSide     string `json:"makerOrderSide,omitempty" redis:"makerOrderSide"`
	TakerOrderSide     string `json:"takerOrderSide,omitempty" redis:"takerOrderSide"`
	MakerOrderQuantums string `json:"makerOrderQuantums,omitempty" redis:"makerOrderQuantums"`
	MakerOrderSubticks string `json:"makerOrderSubticks,omitempty" redis:"makerOrderSubticks"`
	TakerOrderQuantums string `json:"takerOrderQuantums,omitempty" redis:"takerOrderQuantums"`
	TakerOrderSubticks string `json:"takerOrderSubticks,omitempty" redis:"takerOrderSubticks"`
	CreatedAt          int64  `json:"createdAt,omitempty" redis:"createdAt"`
}

type TradingPositionSyncRequest struct {
	EventID          string `json:"eventId"`
	UserID           string `json:"userId"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
	Size             string `json:"size"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnrealizedPNL    string `json:"unrealizedPnl"`
	MarginAsset      string `json:"marginAsset"`
	Margin           string `json:"margin"`
}

type TradingPosition struct {
	EventID          string `json:"eventId" redis:"eventId"`
	UserID           string `json:"userId" redis:"userId"`
	Market           string `json:"market" redis:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty" redis:"subaccountNumber"`
	Size             string `json:"size" redis:"size"`
	EntryPrice       string `json:"entryPrice" redis:"entryPrice"`
	MarkPrice        string `json:"markPrice" redis:"markPrice"`
	UnrealizedPNL    string `json:"unrealizedPnl" redis:"unrealizedPnl"`
	MarginAsset      string `json:"marginAsset" redis:"marginAsset"`
	Margin           string `json:"margin" redis:"margin"`
}

type TradingSyncCursor struct {
	FillsCursor       string `json:"fillsCursor" redis:"fillsCursor"`
	OrderStatusCursor string `json:"orderStatusCursor" redis:"orderStatusCursor"`
	PositionsCursor   string `json:"positionsCursor" redis:"positionsCursor"`
}

type DYDXTradingEventSource interface {
	FetchFills(context.Context, string) ([]TradingFillSyncRequest, string, error)
	FetchPositions(context.Context, string) ([]TradingPositionSyncRequest, string, error)
	FetchOrderStatuses(context.Context, string) ([]TradingOrderStatusSyncRequest, string, error)
}

type TradingSyncCursorStore interface {
	GetTradingSyncCursor(context.Context) (TradingSyncCursor, error)
	SaveTradingSyncCursor(context.Context, TradingSyncCursor) error
}

type TradingOrderStatusSyncRequest struct {
	EventID      string `json:"eventId"`
	UserID       string `json:"userId"`
	OrderID      string `json:"orderId"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	RefundAsset  string `json:"refundAsset"`
	RefundAmount string `json:"refundAmount"`
}

type TradingOrderStatusEvent struct {
	EventID      string `json:"eventId" redis:"eventId"`
	UserID       string `json:"userId" redis:"userId"`
	OrderID      string `json:"orderId" redis:"orderId"`
	Status       string `json:"status" redis:"status"`
	Reason       string `json:"reason" redis:"reason"`
	RefundAsset  string `json:"refundAsset" redis:"refundAsset"`
	RefundAmount string `json:"refundAmount" redis:"refundAmount"`
}

type TradingSyncStore interface {
	SaveFill(ctx context.Context, fill TradingFill) error
	SavePosition(ctx context.Context, position TradingPosition) error
	SaveOrderStatus(ctx context.Context, status TradingOrderStatusEvent) error
	GetFill(ctx context.Context, eventID string) (TradingFill, bool, error)
	GetOrderStatus(ctx context.Context, eventID string) (TradingOrderStatusEvent, bool, error)
	GetPosition(ctx context.Context, userID, market string) (TradingPosition, bool, error)
	GetPositionForSubaccount(ctx context.Context, userID, market string, subaccountNumber uint32) (TradingPosition, bool, error)
}

type TradingFillLookup interface {
	ListFillsByOrder(context.Context, string, string) ([]TradingFill, error)
}

type TradingPositionLookup interface {
	ListPositionsByUser(context.Context, string) ([]TradingPosition, error)
}

type TradingOrderSettlementService interface {
	TradingFailedOrderSettlementService
	SettleCancelledOrder(context.Context, CancelSettlementRequest) (TradingSettlement, error)
}

type TradingSyncService struct {
	store      TradingSyncStore
	orders     TradingStore
	ledger     *LedgerService
	settlement TradingOrderSettlementService
	bindings   TradingFillBindingLookup
}

func NewTradingSyncService(store TradingSyncStore, orders TradingStore, ledger *LedgerService) *TradingSyncService {
	return &TradingSyncService{store: store, orders: orders, ledger: ledger}
}

func (s *TradingSyncService) SetOrderSettlementService(service TradingOrderSettlementService) {
	s.settlement = service
}

type TradingFillBindingLookup interface {
	GetByDYDXAddress(context.Context, string) (WalletBinding, error)
}

func (s *TradingSyncService) SetWalletBindingLookup(lookup TradingFillBindingLookup) {
	if s != nil {
		s.bindings = lookup
	}
}

func (s *TradingSyncService) SyncFill(ctx context.Context, request TradingFillSyncRequest) (TradingFill, error) {
	createdAt := request.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	fill := TradingFill{
		EventID:            request.EventID,
		TradeHash:          request.TradeHash,
		UserID:             request.UserID,
		OrderID:            request.OrderID,
		Market:             request.Market,
		Asset:              request.Asset,
		ReleasedMargin:     request.ReleasedMargin,
		Fee:                request.Fee,
		RealizedPNL:        request.RealizedPNL,
		OrderStatus:        request.OrderStatus,
		LiquidityRole:      request.LiquidityRole,
		MakerAddress:       request.MakerAddress,
		MakerNECOAddress:   request.MakerNECOAddress,
		TakerAddress:       request.TakerAddress,
		TakerNECOAddress:   request.TakerNECOAddress,
		MakerOrderID:       request.MakerOrderID,
		TakerOrderID:       request.TakerOrderID,
		ClobPairID:         request.ClobPairID,
		FillAmount:         request.FillAmount,
		MakerFee:           request.MakerFee,
		TakerFee:           request.TakerFee,
		MakerOrderSide:     request.MakerOrderSide,
		TakerOrderSide:     request.TakerOrderSide,
		MakerOrderQuantums: request.MakerOrderQuantums,
		MakerOrderSubticks: request.MakerOrderSubticks,
		TakerOrderQuantums: request.TakerOrderQuantums,
		TakerOrderSubticks: request.TakerOrderSubticks,
		CreatedAt:          createdAt,
	}
	s.enrichFillPartyAddresses(ctx, &fill)
	if err := s.store.SaveFill(ctx, fill); err != nil {
		return TradingFill{}, err
	}
	if order, ok, err := s.findOrderForSync(ctx, request.UserID, request.OrderID); err != nil {
		return TradingFill{}, err
	} else if ok {
		status := request.OrderStatus
		if status == "" {
			status = TradingOrderStatusFilled
		}
		order.Status = orderStatusForDYDXResult(order, status)
		if err := s.orders.SaveOrder(ctx, order); err != nil {
			return TradingFill{}, err
		}
		if _, err := s.ledger.ApplyTradingSettlement(ctx, LedgerTradingSettlementRequest{
			UserID:         request.UserID,
			Asset:          request.Asset,
			ReleasedMargin: request.ReleasedMargin,
			Fee:            request.Fee,
			RealizedPNL:    request.RealizedPNL,
			RefID:          request.EventID,
		}); err != nil {
			return TradingFill{}, err
		}
	}
	return fill, nil
}

func (s *TradingSyncService) enrichFillPartyAddresses(ctx context.Context, fill *TradingFill) {
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

func (s *TradingSyncService) necoAddressForDYDX(ctx context.Context, dydxAddress string) string {
	if strings.TrimSpace(dydxAddress) == "" {
		return ""
	}
	binding, err := s.bindings.GetByDYDXAddress(ctx, dydxAddress)
	if err != nil || binding.Status != WalletBindingStatusActive {
		return ""
	}
	return binding.NECOAddress
}

func (s *TradingSyncService) SyncPosition(ctx context.Context, request TradingPositionSyncRequest) (TradingPosition, error) {
	position := TradingPosition{
		EventID:          request.EventID,
		UserID:           request.UserID,
		Market:           request.Market,
		SubaccountNumber: request.SubaccountNumber,
		Size:             request.Size,
		EntryPrice:       request.EntryPrice,
		MarkPrice:        request.MarkPrice,
		UnrealizedPNL:    request.UnrealizedPNL,
		MarginAsset:      request.MarginAsset,
		Margin:           request.Margin,
	}
	if err := s.store.SavePosition(ctx, position); err != nil {
		return TradingPosition{}, err
	}
	return position, nil
}

func (s *TradingSyncService) SyncOrderStatus(ctx context.Context, request TradingOrderStatusSyncRequest) (TradingOrderStatusEvent, error) {
	event := TradingOrderStatusEvent{
		EventID:      request.EventID,
		UserID:       request.UserID,
		OrderID:      request.OrderID,
		Status:       request.Status,
		Reason:       request.Reason,
		RefundAsset:  request.RefundAsset,
		RefundAmount: request.RefundAmount,
	}
	if err := s.store.SaveOrderStatus(ctx, event); err != nil {
		return TradingOrderStatusEvent{}, err
	}
	order, ok, err := s.findOrderForSync(ctx, request.UserID, request.OrderID)
	if err != nil {
		return TradingOrderStatusEvent{}, err
	}
	if !ok {
		return event, nil
	}
	hadDYDXFundingRisk := orderHasDYDXFundingRisk(order)
	order.SubaccountNumber = explicitOrBuiltInDYDXSubaccountNumber(order.SubaccountNumber, order.Market, 0, order.SubaccountNumber)
	order.Status = orderStatusForDYDXResult(order, request.Status)
	if err := s.orders.SaveOrder(ctx, order); err != nil {
		return TradingOrderStatusEvent{}, err
	}
	switch order.Status {
	case TradingOrderStatusRejected:
		if hadDYDXFundingRisk {
			if err := s.settleFailedFundedOrder(ctx, order, event); err != nil {
				return TradingOrderStatusEvent{}, err
			}
			return event, nil
		}
		asset := order.MarginAsset
		amount := order.MarginAmount
		if request.RefundAsset != "" {
			asset = request.RefundAsset
		}
		if request.RefundAmount != "" {
			amount = request.RefundAmount
		}
		if _, err := s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
			UserID: request.UserID,
			Asset:  asset,
			Amount: amount,
			RefID:  request.EventID,
		}); err != nil {
			return TradingOrderStatusEvent{}, err
		}
	case TradingOrderStatusCancelled:
		if hadDYDXFundingRisk {
			if err := s.settleCancelledFundedOrder(ctx, order, event); err != nil {
				return TradingOrderStatusEvent{}, err
			}
			return event, nil
		}
		if request.RefundAmount != "" && request.RefundAsset != "" && !hadDYDXFundingRisk {
			if _, err := s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
				UserID: request.UserID,
				Asset:  request.RefundAsset,
				Amount: request.RefundAmount,
				RefID:  request.EventID,
			}); err != nil {
				return TradingOrderStatusEvent{}, err
			}
		}
	}
	return event, nil
}

func (s *TradingSyncService) settleFailedFundedOrder(ctx context.Context, order TradingOrder, event TradingOrderStatusEvent) error {
	if s == nil || s.settlement == nil || order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return nil
	}
	_, err := s.settlement.SettleFailedOrder(ctx, FailedOrderSettlementRequest{
		Order: order,
		Reconcile: DYDXOrderReconcileResult{
			Tx: DYDXTxReconcileSnapshot{
				Found:  true,
				TxHash: order.DYDXOrderTxHash,
				Code:   1,
				RawLog: event.Reason,
			},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
		},
	})
	if errors.Is(err, ErrTradingSettlementManualReview) {
		return nil
	}
	return err
}

func (s *TradingSyncService) settleCancelledFundedOrder(ctx context.Context, order TradingOrder, event TradingOrderStatusEvent) error {
	if s == nil || s.settlement == nil || order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return nil
	}
	txHash := firstNonEmpty(order.DYDXCancelTxHash, order.DYDXOrderTxHash)
	_, err := s.settlement.SettleCancelledOrder(ctx, CancelSettlementRequest{
		Order: order,
		Reconcile: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, TxHash: txHash, Code: 0, RawLog: event.Reason},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
		},
	})
	if errors.Is(err, ErrTradingSettlementManualReview) {
		return nil
	}
	return err
}

func orderHasDYDXFundingRisk(order TradingOrder) bool {
	return order.DYDXFundingTxHash != "" ||
		order.DYDXFundingStatus != "" ||
		order.DYDXFundingQuantums != "" ||
		order.Status == TradingOrderStatusDYDXFundingFailed ||
		order.Status == TradingOrderStatusSubmitFailed
}

func (s *TradingSyncService) GetFill(ctx context.Context, eventID string) (TradingFill, error) {
	fill, ok, err := s.store.GetFill(ctx, eventID)
	if err != nil {
		return TradingFill{}, err
	}
	if !ok {
		return TradingFill{}, ErrTradingSyncEventNotFound
	}
	return fill, nil
}

func (s *TradingSyncService) ListFillsByOrder(ctx context.Context, userID, orderID string) ([]TradingFill, error) {
	lister, ok := s.store.(TradingFillLookup)
	if !ok {
		return []TradingFill{}, nil
	}
	return lister.ListFillsByOrder(ctx, userID, orderID)
}

func (s *TradingSyncService) ListPositionsByUser(ctx context.Context, userID string) ([]TradingPosition, error) {
	lister, ok := s.store.(TradingPositionLookup)
	if !ok {
		return []TradingPosition{}, nil
	}
	return lister.ListPositionsByUser(ctx, userID)
}

func (s *TradingSyncService) GetOrderStatus(ctx context.Context, eventID string) (TradingOrderStatusEvent, error) {
	status, ok, err := s.store.GetOrderStatus(ctx, eventID)
	if err != nil {
		return TradingOrderStatusEvent{}, err
	}
	if !ok {
		return TradingOrderStatusEvent{}, ErrTradingSyncEventNotFound
	}
	return status, nil
}

func (s *TradingSyncService) GetPosition(ctx context.Context, userID, market string) (TradingPosition, error) {
	position, ok, err := s.store.GetPosition(ctx, userID, market)
	if err != nil {
		return TradingPosition{}, err
	}
	if !ok {
		return TradingPosition{}, ErrTradingSyncEventNotFound
	}
	return position, nil
}

func (s *TradingSyncService) GetPositionForSubaccount(ctx context.Context, userID, market string, subaccountNumber uint32) (TradingPosition, error) {
	position, ok, err := s.store.GetPositionForSubaccount(ctx, userID, market, subaccountNumber)
	if err != nil {
		return TradingPosition{}, err
	}
	if !ok {
		return TradingPosition{}, ErrTradingSyncEventNotFound
	}
	return position, nil
}

func (s *TradingSyncService) ListFailures(ctx context.Context, userID string) ([]TradingOrderStatusEvent, error) {
	lister, ok := s.store.(interface {
		ListOrderStatuses(context.Context, string) ([]TradingOrderStatusEvent, error)
	})
	if !ok {
		return []TradingOrderStatusEvent{}, nil
	}
	statuses, err := lister.ListOrderStatuses(ctx, userID)
	if err != nil {
		return nil, err
	}
	failures := make([]TradingOrderStatusEvent, 0)
	for _, status := range statuses {
		if status.Status == TradingOrderStatusRejected || status.Status == TradingOrderStatusCloseFailed || status.Reason != "" {
			failures = append(failures, status)
		}
	}
	return failures, nil
}

func (s *TradingSyncService) RetryFailure(ctx context.Context, eventID string) (TradingOrderStatusEvent, error) {
	event, err := s.GetOrderStatus(ctx, eventID)
	if err != nil {
		return TradingOrderStatusEvent{}, err
	}
	if event.Status != TradingOrderStatusRejected && event.Reason == "" {
		return TradingOrderStatusEvent{}, ErrTradingSyncEventNotFound
	}
	return event, nil
}

func (s *TradingSyncService) findOrderForSync(ctx context.Context, userID, orderID string) (TradingOrder, bool, error) {
	order, ok, err := s.orders.GetOrderByID(ctx, userID, orderID)
	if err != nil || ok {
		return order, ok, err
	}
	if clientOrderID := clientOrderIDFromDYDXOrderID(orderID); clientOrderID != "" {
		return s.orders.GetOrderByClientID(ctx, userID, clientOrderID)
	}
	return TradingOrder{}, false, nil
}

func clientOrderIDFromDYDXOrderID(orderID string) string {
	parts := strings.Split(orderID, ":")
	if len(parts) != 4 {
		return ""
	}
	return parts[2]
}

type MemoryTradingSyncStore struct {
	events        map[string]bool
	fills         map[string]TradingFill
	orderStatuses map[string]TradingOrderStatusEvent
	positions     map[string]TradingPosition
}

func NewMemoryTradingSyncStore() *MemoryTradingSyncStore {
	return &MemoryTradingSyncStore{
		events:        map[string]bool{},
		fills:         map[string]TradingFill{},
		orderStatuses: map[string]TradingOrderStatusEvent{},
		positions:     map[string]TradingPosition{},
	}
}

func (s *MemoryTradingSyncStore) SaveFill(ctx context.Context, fill TradingFill) error {
	if s.events[fill.EventID] {
		return ErrTradingSyncDuplicateEvent
	}
	s.events[fill.EventID] = true
	s.fills[fill.EventID] = fill
	return nil
}

func (s *MemoryTradingSyncStore) SavePosition(ctx context.Context, position TradingPosition) error {
	if s.events[position.EventID] {
		return ErrTradingSyncDuplicateEvent
	}
	s.events[position.EventID] = true
	key := positionSubaccountKey(position.UserID, position.Market, position.SubaccountNumber)
	if tradingPositionSizeIsZero(position.Size) {
		delete(s.positions, key)
		return nil
	}
	s.positions[key] = position
	return nil
}

func (s *MemoryTradingSyncStore) SaveOrderStatus(ctx context.Context, status TradingOrderStatusEvent) error {
	if s.events[status.EventID] {
		return ErrTradingSyncDuplicateEvent
	}
	s.events[status.EventID] = true
	s.orderStatuses[status.EventID] = status
	return nil
}

func (s *MemoryTradingSyncStore) GetFill(ctx context.Context, eventID string) (TradingFill, bool, error) {
	fill, ok := s.fills[eventID]
	return fill, ok, nil
}

func (s *MemoryTradingSyncStore) ListFillsByOrder(ctx context.Context, userID, orderID string) ([]TradingFill, error) {
	fills := make([]TradingFill, 0)
	for _, fill := range s.fills {
		if fill.UserID == userID && fill.OrderID == orderID {
			fills = append(fills, fill)
		}
	}
	sort.Slice(fills, func(i, j int) bool {
		return fills[i].EventID < fills[j].EventID
	})
	return fills, nil
}

func (s *MemoryTradingSyncStore) GetOrderStatus(ctx context.Context, eventID string) (TradingOrderStatusEvent, bool, error) {
	status, ok := s.orderStatuses[eventID]
	return status, ok, nil
}

func (s *MemoryTradingSyncStore) GetPosition(ctx context.Context, userID, market string) (TradingPosition, bool, error) {
	position, ok := s.positions[positionKey(userID, market)]
	if ok && tradingPositionSizeIsZero(position.Size) {
		return TradingPosition{}, false, nil
	}
	return position, ok, nil
}

func (s *MemoryTradingSyncStore) GetPositionForSubaccount(ctx context.Context, userID, market string, subaccountNumber uint32) (TradingPosition, bool, error) {
	position, ok := s.positions[positionSubaccountKey(userID, market, subaccountNumber)]
	if ok {
		if tradingPositionSizeIsZero(position.Size) {
			return TradingPosition{}, false, nil
		}
		return position, true, nil
	}
	position, ok = s.positions[positionKey(userID, market)]
	if !ok {
		return TradingPosition{}, false, nil
	}
	if position.SubaccountNumber != 0 && position.SubaccountNumber != subaccountNumber {
		return TradingPosition{}, false, nil
	}
	position.SubaccountNumber = subaccountNumber
	if tradingPositionSizeIsZero(position.Size) {
		return TradingPosition{}, false, nil
	}
	return position, true, nil
}

func (s *MemoryTradingSyncStore) ListPositionsByUser(ctx context.Context, userID string) ([]TradingPosition, error) {
	positions := make([]TradingPosition, 0)
	for _, position := range s.positions {
		if position.UserID == userID && !tradingPositionSizeIsZero(position.Size) {
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

func (s *MemoryTradingSyncStore) ListOrderStatuses(ctx context.Context, userID string) ([]TradingOrderStatusEvent, error) {
	statuses := make([]TradingOrderStatusEvent, 0)
	for _, status := range s.orderStatuses {
		if status.UserID == userID {
			statuses = append(statuses, status)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].EventID < statuses[j].EventID
	})
	return statuses, nil
}

func positionKey(userID, market string) string {
	return userID + ":" + market
}

func positionSubaccountKey(userID, market string, subaccountNumber uint32) string {
	if subaccountNumber == defaultDYDXTradingSubaccountNumber {
		return positionKey(userID, market)
	}
	return userID + ":" + market + ":" + strconv.FormatUint(uint64(subaccountNumber), 10)
}

func tradingPositionSizeIsZero(size string) bool {
	value, ok := parseDecimalRat(size)
	return ok && value.Sign() == 0
}

type TradingSyncWorker struct {
	source  DYDXTradingEventSource
	service *TradingSyncService
	cursor  TradingSyncCursorStore
}

func NewTradingSyncWorker(source DYDXTradingEventSource, service *TradingSyncService, cursor TradingSyncCursorStore) *TradingSyncWorker {
	return &TradingSyncWorker{source: source, service: service, cursor: cursor}
}

func (w *TradingSyncWorker) PollOnce(ctx context.Context) error {
	cursor, err := w.cursor.GetTradingSyncCursor(ctx)
	if err != nil {
		return err
	}
	fills, nextFills, err := w.source.FetchFills(ctx, cursor.FillsCursor)
	if err != nil {
		return err
	}
	for _, fill := range fills {
		if _, err := w.service.SyncFill(ctx, fill); err != nil && !errors.Is(err, ErrTradingSyncDuplicateEvent) {
			return err
		}
	}
	statuses, nextStatuses, err := w.source.FetchOrderStatuses(ctx, cursor.OrderStatusCursor)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if _, err := w.service.SyncOrderStatus(ctx, status); err != nil && !errors.Is(err, ErrTradingSyncDuplicateEvent) {
			return err
		}
	}
	positions, nextPositions, err := w.source.FetchPositions(ctx, cursor.PositionsCursor)
	if err != nil {
		return err
	}
	for _, position := range positions {
		if _, err := w.service.SyncPosition(ctx, position); err != nil && !errors.Is(err, ErrTradingSyncDuplicateEvent) {
			return err
		}
	}
	cursor.FillsCursor = coalesceCursor(nextFills, cursor.FillsCursor)
	cursor.OrderStatusCursor = coalesceCursor(nextStatuses, cursor.OrderStatusCursor)
	cursor.PositionsCursor = coalesceCursor(nextPositions, cursor.PositionsCursor)
	return w.cursor.SaveTradingSyncCursor(ctx, cursor)
}

type MemoryTradingSyncCursorStore struct {
	cursor TradingSyncCursor
}

func NewMemoryTradingSyncCursorStore(cursor TradingSyncCursor) *MemoryTradingSyncCursorStore {
	return &MemoryTradingSyncCursorStore{cursor: cursor}
}

func (s *MemoryTradingSyncCursorStore) GetTradingSyncCursor(context.Context) (TradingSyncCursor, error) {
	return s.cursor, nil
}

func (s *MemoryTradingSyncCursorStore) SaveTradingSyncCursor(_ context.Context, cursor TradingSyncCursor) error {
	s.cursor = cursor
	return nil
}

func coalesceCursor(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
