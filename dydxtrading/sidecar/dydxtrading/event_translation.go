package dydxtrading

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	NormalizedOrderRemovalReasonUserCanceled     = "user_canceled"
	NormalizedOrderRemovalReasonFullyFilled      = "fully_filled"
	NormalizedOrderRemovalReasonExpired          = "expired"
	NormalizedOrderRemovalReasonReplaced         = "replaced"
	NormalizedOrderRemovalReasonInternalError    = "internal_error"
	NormalizedOrderRemovalReasonUndercollateral  = "undercollateralized"
	NormalizedOrderRemovalReasonPostOnlyRejected = "post_only_would_cross"
	NormalizedOrderRemovalReasonFOKRejected      = "fok_could_not_be_fully_filled"
)

type TradingMarketMapping struct {
	Market string
	Asset  string
}

type DYDXUserResolver interface {
	UserIDByDYDXAddress(context.Context, string) (string, bool, error)
}

type IndexerEventTranslatorConfig struct {
	UserResolver DYDXUserResolver
	Markets      map[uint32]TradingMarketMapping
	Perpetuals   map[uint32]TradingMarketMapping
}

type IndexerEventTranslator struct {
	resolver   DYDXUserResolver
	markets    map[uint32]TradingMarketMapping
	perpetuals map[uint32]TradingMarketMapping
}

type NormalizedSubaccountID struct {
	Owner  string
	Number uint32
}

type NormalizedOrderID struct {
	SubaccountID NormalizedSubaccountID
	ClientID     uint32
	OrderFlags   uint32
	ClobPairID   uint32
}

type NormalizedOrder struct {
	OrderID  NormalizedOrderID
	Side     string
	Quantums string
	Subticks string
}

type NormalizedOrderFillEvent struct {
	MakerOrder          NormalizedOrder
	TakerOrder          *NormalizedOrder
	FillAmount          uint64
	MakerReleasedMargin string
	MakerFee            int64
	MakerRealizedPNL    string
	TakerReleasedMargin string
	TakerFee            int64
	TakerRealizedPNL    string
}

type NormalizedStatefulOrderRemovalEvent struct {
	OrderID NormalizedOrderID
	Reason  string
}

type NormalizedSubaccountUpdateEvent struct {
	SubaccountID                NormalizedSubaccountID
	UpdatedPerpetualPositions   []NormalizedPerpetualPosition
	UpdatedTradingSettlements   []NormalizedTradingSettlement
	UpdatedPerpetualPositionPnl []NormalizedPerpetualPositionPNL
}

type NormalizedPerpetualPosition struct {
	PerpetualId uint32
	Quantums    string
}

type NormalizedTradingSettlement struct {
	Asset          string
	ReleasedMargin string
	Fee            string
	RealizedPNL    string
}

type NormalizedPerpetualPositionPNL struct {
	PerpetualId    uint32
	UnrealizedPNL  string
	Margin         string
	EntryPrice     string
	MarkPrice      string
	OverrideMarket string
	OverrideAsset  string
}

func NewIndexerEventTranslator(cfg IndexerEventTranslatorConfig) *IndexerEventTranslator {
	return &IndexerEventTranslator{
		resolver:   cfg.UserResolver,
		markets:    cfg.Markets,
		perpetuals: cfg.Perpetuals,
	}
}

func (t *IndexerEventTranslator) TranslateOrderFill(ctx context.Context, eventID string, fill NormalizedOrderFillEvent) ([]TradingFillEvent, error) {
	events := make([]TradingFillEvent, 0, 2)
	if event, ok, err := t.orderFillForOrder(ctx, eventID+":maker", eventID, "maker", fill, fill.MakerOrder, fill.MakerReleasedMargin, fill.MakerFee, fill.MakerRealizedPNL); err != nil {
		return nil, err
	} else if ok {
		events = append(events, event)
	}
	if fill.TakerOrder != nil {
		if event, ok, err := t.orderFillForOrder(ctx, eventID+":taker", eventID, "taker", fill, *fill.TakerOrder, fill.TakerReleasedMargin, fill.TakerFee, fill.TakerRealizedPNL); err != nil {
			return nil, err
		} else if ok {
			events = append(events, event)
		}
	}
	return events, nil
}

func (t *IndexerEventTranslator) TranslateStatefulOrderRemoval(ctx context.Context, eventID string, removal NormalizedStatefulOrderRemovalEvent) (TradingOrderStatusEvent, error) {
	userID, ok, err := t.userID(ctx, removal.OrderID.SubaccountID.Owner)
	if err != nil {
		return TradingOrderStatusEvent{}, err
	}
	if !ok {
		return TradingOrderStatusEvent{}, nil
	}
	return TradingOrderStatusEvent{
		EventID: eventID,
		UserID:  userID,
		OrderID: orderIDString(removal.OrderID),
		Status:  statusForRemovalReason(removal.Reason),
		Reason:  removal.Reason,
	}, nil
}

func (t *IndexerEventTranslator) TranslateSubaccountUpdate(ctx context.Context, eventID string, update NormalizedSubaccountUpdateEvent) ([]TradingPositionEvent, error) {
	userID, ok, err := t.userID(ctx, update.SubaccountID.Owner)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	events := make([]TradingPositionEvent, 0, len(update.UpdatedPerpetualPositions))
	for _, position := range update.UpdatedPerpetualPositions {
		mapping := t.perpetualMapping(position.PerpetualId)
		events = append(events, TradingPositionEvent{
			EventID:          eventID + ":" + strconv.FormatUint(uint64(position.PerpetualId), 10),
			UserID:           userID,
			Market:           mapping.Market,
			SubaccountNumber: update.SubaccountID.Number,
			Size:             position.Quantums,
			MarginAsset:      mapping.Asset,
		})
	}
	return events, nil
}

func (t *IndexerEventTranslator) orderFillForOrder(ctx context.Context, eventID string, tradeHash string, liquidityRole string, fill NormalizedOrderFillEvent, order NormalizedOrder, releasedMargin string, fee int64, realizedPNL string) (TradingFillEvent, bool, error) {
	userID, ok, err := t.userID(ctx, order.OrderID.SubaccountID.Owner)
	if err != nil {
		return TradingFillEvent{}, false, err
	}
	if !ok {
		return TradingFillEvent{}, false, nil
	}
	mapping := t.marketMapping(order.OrderID.ClobPairID)
	makerOrderID := orderIDString(fill.MakerOrder.OrderID)
	takerAddress := ""
	takerOrderID := ""
	takerOrderSide := ""
	takerOrderQuantums := ""
	takerOrderSubticks := ""
	if fill.TakerOrder != nil {
		takerAddress = fill.TakerOrder.OrderID.SubaccountID.Owner
		takerOrderID = orderIDString(fill.TakerOrder.OrderID)
		takerOrderSide = fill.TakerOrder.Side
		takerOrderQuantums = fill.TakerOrder.Quantums
		takerOrderSubticks = fill.TakerOrder.Subticks
	}
	return TradingFillEvent{
		EventID:            eventID,
		TradeHash:          tradeHash,
		UserID:             userID,
		OrderID:            orderIDString(order.OrderID),
		Market:             mapping.Market,
		Asset:              mapping.Asset,
		ReleasedMargin:     firstString(releasedMargin, "0"),
		Fee:                strconv.FormatInt(fee, 10),
		RealizedPNL:        firstString(realizedPNL, "0"),
		OrderStatus:        "partial_filled",
		LiquidityRole:      liquidityRole,
		MakerAddress:       fill.MakerOrder.OrderID.SubaccountID.Owner,
		TakerAddress:       takerAddress,
		MakerOrderID:       makerOrderID,
		TakerOrderID:       takerOrderID,
		ClobPairID:         order.OrderID.ClobPairID,
		FillAmount:         strconv.FormatUint(fill.FillAmount, 10),
		MakerFee:           strconv.FormatInt(fill.MakerFee, 10),
		TakerFee:           strconv.FormatInt(fill.TakerFee, 10),
		MakerOrderSide:     fill.MakerOrder.Side,
		TakerOrderSide:     takerOrderSide,
		MakerOrderQuantums: fill.MakerOrder.Quantums,
		MakerOrderSubticks: fill.MakerOrder.Subticks,
		TakerOrderQuantums: takerOrderQuantums,
		TakerOrderSubticks: takerOrderSubticks,
	}, true, nil
}

func (t *IndexerEventTranslator) userID(ctx context.Context, dydxAddress string) (string, bool, error) {
	if t.resolver == nil {
		return "", false, nil
	}
	return t.resolver.UserIDByDYDXAddress(ctx, dydxAddress)
}

func (t *IndexerEventTranslator) marketMapping(clobPairID uint32) TradingMarketMapping {
	if mapping, ok := t.markets[clobPairID]; ok {
		return mapping
	}
	return TradingMarketMapping{
		Market: "clob:" + strconv.FormatUint(uint64(clobPairID), 10),
		Asset:  "USDT",
	}
}

func (t *IndexerEventTranslator) perpetualMapping(perpetualID uint32) TradingMarketMapping {
	if mapping, ok := t.perpetuals[perpetualID]; ok {
		return mapping
	}
	return TradingMarketMapping{
		Market: "perpetual:" + strconv.FormatUint(uint64(perpetualID), 10),
		Asset:  "USDT",
	}
}

func orderIDString(orderID NormalizedOrderID) string {
	return fmt.Sprintf(
		"%s:%d:%d:%d",
		orderID.SubaccountID.Owner,
		orderID.SubaccountID.Number,
		orderID.ClientID,
		orderID.ClobPairID,
	)
}

func statusForRemovalReason(reason string) string {
	switch reason {
	case NormalizedOrderRemovalReasonUserCanceled, NormalizedOrderRemovalReasonExpired, NormalizedOrderRemovalReasonReplaced:
		return "cancelled"
	case NormalizedOrderRemovalReasonFullyFilled:
		return "filled"
	default:
		return "rejected"
	}
}

func normalizeOrderSide(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "1", "BUY", "SIDE_BUY":
		return "buy"
	case "2", "SELL", "SIDE_SELL":
		return "sell"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeOrderSideValue(value uint64) string {
	switch value {
	case 1:
		return "buy"
	case 2:
		return "sell"
	default:
		return ""
	}
}
