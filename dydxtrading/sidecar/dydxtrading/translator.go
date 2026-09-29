//go:build cgo

package dydxtrading

import (
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"
	satypes "github.com/dydxprotocol/v4-chain/protocol/x/subaccounts/types"
)

type placeOrderValue struct {
	Order orderValue `json:"order"`
}

type orderValue struct {
	OrderID                    orderIDValue `json:"orderId"`
	Side                       uint32       `json:"side"`
	Quantums                   uint64       `json:"quantums"`
	Subticks                   uint64       `json:"subticks"`
	GoodTilBlock               uint32       `json:"goodTilBlock"`
	GoodTilBlockTime           uint32       `json:"goodTilBlockTime"`
	TimeInForce                uint32       `json:"timeInForce"`
	ReduceOnly                 bool         `json:"reduceOnly"`
	ClientMetadata             uint32       `json:"clientMetadata"`
	ConditionType              uint32       `json:"conditionType"`
	ConditionalTriggerSubticks uint64       `json:"conditionalOrderTriggerSubticks"`
}

type cancelOrderValue struct {
	OrderID          orderIDValue `json:"orderId"`
	GoodTilBlock     uint32       `json:"goodTilBlock"`
	GoodTilBlockTime uint32       `json:"goodTilBlockTime"`
}

type orderIDValue struct {
	SubaccountID subaccountIDValue `json:"subaccountId"`
	ClientID     uint32            `json:"clientId"`
	OrderFlags   uint32            `json:"orderFlags"`
	ClobPairID   uint32            `json:"clobPairId"`
}

type subaccountIDValue struct {
	Owner  string `json:"owner"`
	Number uint32 `json:"number"`
}

func BuildMessage(req TycheTradingRequest) (sdk.Msg, MessageMeta, error) {
	tx := req.Params.CosmosTx
	feeAmount, err := parseOptionalTxFeeAmount(tx.FeeAmount)
	if err != nil {
		return nil, MessageMeta{}, err
	}
	meta := MessageMeta{
		ID:            firstNonEmpty(tx.ID, req.ID),
		Action:        firstNonEmpty(tx.Action, req.Action),
		Account:       firstNonEmpty(tx.Account, req.Account),
		ChainID:       tx.ChainID,
		BroadcastMode: firstNonEmpty(tx.BroadcastMode, BroadcastModeSync),
		GasLimit:      tx.GasLimit,
		FeeAmount:     feeAmount,
	}
	if meta.Action == "" {
		return nil, meta, fmt.Errorf("missing action")
	}
	if meta.Account == "" {
		return nil, meta, fmt.Errorf("missing account")
	}
	if tx.SignMode != "" && tx.SignMode != SignModeDirect {
		return nil, meta, fmt.Errorf("unsupported sign mode %q", tx.SignMode)
	}
	if meta.ChainID == "" {
		return nil, meta, fmt.Errorf("missing chainId")
	}

	switch tx.Message.TypeURL {
	case TypeURLPlaceOrder:
		if meta.Action != ActionPlaceOrder {
			return nil, meta, fmt.Errorf("action %q does not match %s", meta.Action, TypeURLPlaceOrder)
		}
		msg, err := buildPlaceOrder(tx.Message.Value)
		return msg, meta, err
	case TypeURLCancelOrder:
		if meta.Action != ActionCancelOrder {
			return nil, meta, fmt.Errorf("action %q does not match %s", meta.Action, TypeURLCancelOrder)
		}
		msg, err := buildCancelOrder(tx.Message.Value)
		return msg, meta, err
	default:
		return nil, meta, fmt.Errorf("unsupported message typeUrl %q", tx.Message.TypeURL)
	}
}

func MessageOwner(msg sdk.Msg) string {
	switch typed := msg.(type) {
	case *clobtypes.MsgPlaceOrder:
		return typed.Order.OrderId.SubaccountId.Owner
	case *clobtypes.MsgCancelOrder:
		return typed.OrderId.SubaccountId.Owner
	default:
		return ""
	}
}

func buildPlaceOrder(raw json.RawMessage) (*clobtypes.MsgPlaceOrder, error) {
	var value placeOrderValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode place order: %w", err)
	}
	orderID := clobtypes.OrderId{
		SubaccountId: toSubaccountID(value.Order.OrderID.SubaccountID),
		ClientId:     value.Order.OrderID.ClientID,
		OrderFlags:   value.Order.OrderID.OrderFlags,
		ClobPairId:   value.Order.OrderID.ClobPairID,
	}
	order := clobtypes.Order{
		OrderId:                         orderID,
		Side:                            clobtypes.Order_Side(value.Order.Side),
		Quantums:                        value.Order.Quantums,
		Subticks:                        value.Order.Subticks,
		TimeInForce:                     clobtypes.Order_TimeInForce(value.Order.TimeInForce),
		ReduceOnly:                      value.Order.ReduceOnly,
		ClientMetadata:                  value.Order.ClientMetadata,
		ConditionType:                   clobtypes.Order_ConditionType(value.Order.ConditionType),
		ConditionalOrderTriggerSubticks: value.Order.ConditionalTriggerSubticks,
	}
	if orderID.IsStatefulOrder() {
		order.GoodTilOneof = &clobtypes.Order_GoodTilBlockTime{GoodTilBlockTime: value.Order.GoodTilBlockTime}
	} else {
		order.GoodTilOneof = &clobtypes.Order_GoodTilBlock{GoodTilBlock: value.Order.GoodTilBlock}
	}
	msg := clobtypes.NewMsgPlaceOrder(order)
	if err := msg.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid place order: %w", err)
	}
	return msg, nil
}

func buildCancelOrder(raw json.RawMessage) (*clobtypes.MsgCancelOrder, error) {
	var value cancelOrderValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode cancel order: %w", err)
	}
	orderID := clobtypes.OrderId{
		SubaccountId: toSubaccountID(value.OrderID.SubaccountID),
		ClientId:     value.OrderID.ClientID,
		OrderFlags:   value.OrderID.OrderFlags,
		ClobPairId:   value.OrderID.ClobPairID,
	}
	var msg *clobtypes.MsgCancelOrder
	switch {
	case orderID.IsShortTermOrder():
		msg = clobtypes.NewMsgCancelOrderShortTerm(orderID, value.GoodTilBlock)
	case orderID.IsStatefulOrder():
		msg = clobtypes.NewMsgCancelOrderStateful(orderID, value.GoodTilBlockTime)
	default:
		return nil, fmt.Errorf("unsupported cancel order flags %d", orderID.OrderFlags)
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid cancel order: %w", err)
	}
	return msg, nil
}

func toSubaccountID(value subaccountIDValue) satypes.SubaccountId {
	return satypes.SubaccountId{
		Owner:  value.Owner,
		Number: value.Number,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultTxGasLimit(value uint64) uint64 {
	if value != 0 {
		return value
	}
	return DefaultTxGasLimit
}

func firstNonZeroUint64(values ...uint64) uint64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
