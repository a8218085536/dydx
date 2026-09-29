package dydxtrading

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	RawIndexerSubtypeOrderFill        = "order_fill"
	RawIndexerSubtypeStatefulOrder    = "stateful_order"
	RawIndexerSubtypeSubaccountUpdate = "subaccount_update"
)

type RawIndexerEvent struct {
	EventID string          `json:"eventId"`
	Subtype string          `json:"subtype"`
	Data    json.RawMessage `json:"data"`
}

func (e *RawIndexerEvent) UnmarshalJSON(raw []byte) error {
	var data struct {
		EventID      string          `json:"eventId"`
		EventIDUpper string          `json:"eventID"`
		EventIDSnake string          `json:"event_id"`
		Subtype      string          `json:"subtype"`
		Data         json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	e.EventID = firstString(data.EventID, data.EventIDUpper, data.EventIDSnake)
	e.Subtype = data.Subtype
	e.Data = data.Data
	return nil
}

type RawIndexerEventAdapter struct{}

func NewRawIndexerEventAdapter() *RawIndexerEventAdapter {
	return &RawIndexerEventAdapter{}
}

func (a *RawIndexerEventAdapter) ConvertJSON(raw []byte) (NormalizedIndexerEvent, error) {
	var event RawIndexerEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return NormalizedIndexerEvent{}, err
	}
	return a.Convert(event)
}

func (a *RawIndexerEventAdapter) Convert(event RawIndexerEvent) (NormalizedIndexerEvent, error) {
	if event.EventID == "" {
		return NormalizedIndexerEvent{}, errors.New("missing indexer event id")
	}
	switch event.Subtype {
	case RawIndexerSubtypeOrderFill:
		orderFill, err := convertRawOrderFill(event.Data)
		if err != nil {
			return NormalizedIndexerEvent{}, err
		}
		return NormalizedIndexerEvent{EventID: event.EventID, Type: NormalizedIndexerEventTypeOrderFill, OrderFill: &orderFill}, nil
	case RawIndexerSubtypeStatefulOrder:
		removal, err := convertRawStatefulOrderRemoval(event.Data)
		if err != nil {
			return NormalizedIndexerEvent{}, err
		}
		return NormalizedIndexerEvent{EventID: event.EventID, Type: NormalizedIndexerEventTypeStatefulOrderRemoval, StatefulOrderRemoval: &removal}, nil
	case RawIndexerSubtypeSubaccountUpdate:
		update, err := convertRawSubaccountUpdate(event.Data)
		if err != nil {
			return NormalizedIndexerEvent{}, err
		}
		return NormalizedIndexerEvent{EventID: event.EventID, Type: NormalizedIndexerEventTypeSubaccountUpdate, SubaccountUpdate: &update}, nil
	default:
		return NormalizedIndexerEvent{}, fmt.Errorf("unsupported indexer event subtype %q", event.Subtype)
	}
}

type rawOrderFillData struct {
	MakerOrder               rawIndexerOrder  `json:"makerOrder"`
	MakerOrderSnake          rawIndexerOrder  `json:"maker_order"`
	Order                    *rawIndexerOrder `json:"order"`
	FillAmount               uint64           `json:"fillAmount"`
	FillAmountSnake          uint64           `json:"fill_amount"`
	MakerReleasedMargin      string           `json:"makerReleasedMargin"`
	MakerReleasedMarginSnake string           `json:"maker_released_margin"`
	MakerFee                 int64            `json:"makerFee"`
	MakerFeeSnake            int64            `json:"maker_fee"`
	MakerRealizedPNL         string           `json:"makerRealizedPnl"`
	MakerRealizedPNLSnake    string           `json:"maker_realized_pnl"`
	TakerReleasedMargin      string           `json:"takerReleasedMargin"`
	TakerReleasedMarginSnake string           `json:"taker_released_margin"`
	TakerFee                 int64            `json:"takerFee"`
	TakerFeeSnake            int64            `json:"taker_fee"`
	TakerRealizedPNL         string           `json:"takerRealizedPnl"`
	TakerRealizedPNLSnake    string           `json:"taker_realized_pnl"`
}

type rawStatefulOrderData struct {
	OrderRemoval      *rawOrderRemoval `json:"orderRemoval"`
	OrderRemovalSnake *rawOrderRemoval `json:"order_removal"`
}

type rawOrderRemoval struct {
	RemovedOrderID      rawIndexerOrderID `json:"removedOrderId"`
	RemovedOrderIDSnake rawIndexerOrderID `json:"removed_order_id"`
	Reason              string            `json:"reason"`
}

type rawSubaccountUpdateData struct {
	SubaccountID                NormalizedSubaccountID           `json:"subaccountId"`
	SubaccountIDSnake           NormalizedSubaccountID           `json:"subaccount_id"`
	UpdatedPerpetualPositions   []rawPerpetualPosition           `json:"updatedPerpetualPositions"`
	UpdatedPerpetualPosSnake    []rawPerpetualPosition           `json:"updated_perpetual_positions"`
	UpdatedTradingSettlements   []NormalizedTradingSettlement    `json:"updatedTradingSettlements"`
	UpdatedPerpetualPositionPnl []NormalizedPerpetualPositionPNL `json:"updatedPerpetualPositionPnl"`
}

type rawIndexerOrder struct {
	OrderID      rawIndexerOrderID `json:"orderId"`
	OrderIDSnake rawIndexerOrderID `json:"order_id"`
	Side         rawOrderSide      `json:"side"`
	Quantums     rawUint64String   `json:"quantums"`
	Subticks     rawUint64String   `json:"subticks"`
}

type rawIndexerOrderID struct {
	SubaccountID      NormalizedSubaccountID `json:"subaccountId"`
	SubaccountIDSnake NormalizedSubaccountID `json:"subaccount_id"`
	ClientID          uint32                 `json:"clientId"`
	ClientIDSnake     uint32                 `json:"client_id"`
	OrderFlags        uint32                 `json:"orderFlags"`
	OrderFlagsSnake   uint32                 `json:"order_flags"`
	ClobPairID        uint32                 `json:"clobPairId"`
	ClobPairIDSnake   uint32                 `json:"clob_pair_id"`
}

type rawPerpetualPosition struct {
	PerpetualId      uint32 `json:"perpetualId"`
	PerpetualIdSnake uint32 `json:"perpetual_id"`
	Quantums         string `json:"quantums"`
}

func convertRawOrderFill(raw json.RawMessage) (NormalizedOrderFillEvent, error) {
	var data rawOrderFillData
	if err := json.Unmarshal(raw, &data); err != nil {
		return NormalizedOrderFillEvent{}, err
	}
	event := NormalizedOrderFillEvent{
		MakerOrder:          normalizedOrderFromRaw(data.makerOrder()),
		FillAmount:          firstUint64(data.FillAmount, data.FillAmountSnake),
		MakerReleasedMargin: firstString(data.MakerReleasedMargin, data.MakerReleasedMarginSnake),
		MakerFee:            firstInt64(data.MakerFee, data.MakerFeeSnake),
		MakerRealizedPNL:    firstString(data.MakerRealizedPNL, data.MakerRealizedPNLSnake),
		TakerReleasedMargin: firstString(data.TakerReleasedMargin, data.TakerReleasedMarginSnake),
		TakerFee:            firstInt64(data.TakerFee, data.TakerFeeSnake),
		TakerRealizedPNL:    firstString(data.TakerRealizedPNL, data.TakerRealizedPNLSnake),
	}
	if data.Order != nil {
		order := normalizedOrderFromRaw(*data.Order)
		event.TakerOrder = &order
	}
	return event, nil
}

func convertRawStatefulOrderRemoval(raw json.RawMessage) (NormalizedStatefulOrderRemovalEvent, error) {
	var data rawStatefulOrderData
	if err := json.Unmarshal(raw, &data); err != nil {
		return NormalizedStatefulOrderRemovalEvent{}, err
	}
	orderRemoval := data.orderRemoval()
	if orderRemoval == nil {
		return NormalizedStatefulOrderRemovalEvent{}, errors.New("missing stateful order removal")
	}
	return NormalizedStatefulOrderRemovalEvent{
		OrderID: normalizedOrderIDFromRaw(orderRemoval.removedOrderID()),
		Reason:  normalizeRemovalReason(orderRemoval.Reason),
	}, nil
}

func convertRawSubaccountUpdate(raw json.RawMessage) (NormalizedSubaccountUpdateEvent, error) {
	var data rawSubaccountUpdateData
	if err := json.Unmarshal(raw, &data); err != nil {
		return NormalizedSubaccountUpdateEvent{}, err
	}
	return NormalizedSubaccountUpdateEvent{
		SubaccountID:                data.subaccountID(),
		UpdatedPerpetualPositions:   data.updatedPerpetualPositions(),
		UpdatedTradingSettlements:   data.UpdatedTradingSettlements,
		UpdatedPerpetualPositionPnl: data.UpdatedPerpetualPositionPnl,
	}, nil
}

func normalizedOrderIDFromRaw(orderID rawIndexerOrderID) NormalizedOrderID {
	return NormalizedOrderID{
		SubaccountID: orderID.subaccountID(),
		ClientID:     firstUint32(orderID.ClientID, orderID.ClientIDSnake),
		OrderFlags:   firstUint32(orderID.OrderFlags, orderID.OrderFlagsSnake),
		ClobPairID:   firstUint32(orderID.ClobPairID, orderID.ClobPairIDSnake),
	}
}

func normalizedOrderFromRaw(order rawIndexerOrder) NormalizedOrder {
	return NormalizedOrder{
		OrderID:  normalizedOrderIDFromRaw(order.orderID()),
		Side:     string(order.Side),
		Quantums: string(order.Quantums),
		Subticks: string(order.Subticks),
	}
}

func (d rawOrderFillData) makerOrder() rawIndexerOrder {
	if d.MakerOrder.hasOrderID() {
		return d.MakerOrder
	}
	return d.MakerOrderSnake
}

func (d rawStatefulOrderData) orderRemoval() *rawOrderRemoval {
	if d.OrderRemoval != nil {
		return d.OrderRemoval
	}
	return d.OrderRemovalSnake
}

func (r rawOrderRemoval) removedOrderID() rawIndexerOrderID {
	if r.RemovedOrderID.hasValue() {
		return r.RemovedOrderID
	}
	return r.RemovedOrderIDSnake
}

func (d rawSubaccountUpdateData) subaccountID() NormalizedSubaccountID {
	if hasSubaccountID(d.SubaccountID) {
		return d.SubaccountID
	}
	return d.SubaccountIDSnake
}

func (d rawSubaccountUpdateData) updatedPerpetualPositions() []NormalizedPerpetualPosition {
	positions := d.UpdatedPerpetualPositions
	if len(positions) == 0 {
		positions = d.UpdatedPerpetualPosSnake
	}
	normalized := make([]NormalizedPerpetualPosition, 0, len(positions))
	for _, position := range positions {
		normalized = append(normalized, NormalizedPerpetualPosition{
			PerpetualId: firstUint32(position.PerpetualId, position.PerpetualIdSnake),
			Quantums:    position.Quantums,
		})
	}
	return normalized
}

func (o rawIndexerOrder) orderID() rawIndexerOrderID {
	if o.OrderID.hasValue() {
		return o.OrderID
	}
	return o.OrderIDSnake
}

func (o rawIndexerOrder) hasOrderID() bool {
	return o.OrderID.hasValue() || o.OrderIDSnake.hasValue()
}

func (id rawIndexerOrderID) hasValue() bool {
	return hasSubaccountID(id.SubaccountID) ||
		hasSubaccountID(id.SubaccountIDSnake) ||
		id.ClientID != 0 ||
		id.ClientIDSnake != 0 ||
		id.OrderFlags != 0 ||
		id.OrderFlagsSnake != 0 ||
		id.ClobPairID != 0 ||
		id.ClobPairIDSnake != 0
}

func (id rawIndexerOrderID) subaccountID() NormalizedSubaccountID {
	if hasSubaccountID(id.SubaccountID) {
		return id.SubaccountID
	}
	return id.SubaccountIDSnake
}

func hasSubaccountID(id NormalizedSubaccountID) bool {
	return id.Owner != "" || id.Number != 0
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstUint32(values ...uint32) uint32 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstUint64(values ...uint64) uint64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

type rawOrderSide string

func (s *rawOrderSide) UnmarshalJSON(raw []byte) error {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return nil
	}
	if strings.HasPrefix(value, "\"") {
		var parsed string
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return err
		}
		*s = rawOrderSide(normalizeOrderSide(parsed))
		return nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return err
	}
	*s = rawOrderSide(normalizeOrderSideValue(parsed))
	return nil
}

type rawUint64String string

func (v *rawUint64String) UnmarshalJSON(raw []byte) error {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return nil
	}
	if strings.HasPrefix(value, "\"") {
		var parsed string
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return err
		}
		*v = rawUint64String(parsed)
		return nil
	}
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return err
	}
	*v = rawUint64String(value)
	return nil
}

func normalizeRemovalReason(reason string) string {
	switch strings.ToUpper(reason) {
	case "ORDER_REMOVAL_REASON_USER_CANCELED":
		return NormalizedOrderRemovalReasonUserCanceled
	case "ORDER_REMOVAL_REASON_FULLY_FILLED":
		return NormalizedOrderRemovalReasonFullyFilled
	case "ORDER_REMOVAL_REASON_EXPIRED", "ORDER_REMOVAL_REASON_INDEXER_EXPIRED":
		return NormalizedOrderRemovalReasonExpired
	case "ORDER_REMOVAL_REASON_REPLACED":
		return NormalizedOrderRemovalReasonReplaced
	case "ORDER_REMOVAL_REASON_INTERNAL_ERROR":
		return NormalizedOrderRemovalReasonInternalError
	case "ORDER_REMOVAL_REASON_UNDERCOLLATERALIZED":
		return NormalizedOrderRemovalReasonUndercollateral
	case "ORDER_REMOVAL_REASON_POST_ONLY_WOULD_CROSS_MAKER_ORDER":
		return NormalizedOrderRemovalReasonPostOnlyRejected
	case "ORDER_REMOVAL_REASON_FOK_ORDER_COULD_NOT_BE_FULLY_FULLED":
		return NormalizedOrderRemovalReasonFOKRejected
	default:
		return strings.ToLower(strings.TrimPrefix(reason, "ORDER_REMOVAL_REASON_"))
	}
}
