package dydxtrading

import (
	"context"
	"fmt"
	"strconv"
)

const (
	protobufIndexerSubtypeOrderFill        = "order_fill"
	protobufIndexerSubtypeStatefulOrder    = "stateful_order"
	protobufIndexerSubtypeSubaccountUpdate = "subaccount_update"
)

type IndexerBlockMessage struct {
	Offset string
	Value  []byte
}

type IndexerBlockMessageSource interface {
	FetchMessages(context.Context, string) ([]IndexerBlockMessage, string, error)
}

type ProtobufIndexerBlockSource struct {
	messages IndexerBlockMessageSource
}

func NewProtobufIndexerBlockSource(messages IndexerBlockMessageSource) *ProtobufIndexerBlockSource {
	return &ProtobufIndexerBlockSource{messages: messages}
}

func (s *ProtobufIndexerBlockSource) Fetch(ctx context.Context, cursor string) (NormalizedIndexerEventPage, error) {
	if s.messages == nil {
		return NormalizedIndexerEventPage{}, fmt.Errorf("missing indexer block message source")
	}
	messages, nextCursor, err := s.messages.FetchMessages(ctx, cursor)
	if err != nil {
		return NormalizedIndexerEventPage{}, err
	}
	var events []NormalizedIndexerEvent
	for _, message := range messages {
		blockEvents, err := decodeIndexerBlock(message.Value)
		if err != nil {
			return NormalizedIndexerEventPage{}, err
		}
		events = append(events, blockEvents...)
		if nextCursor == "" {
			nextCursor = message.Offset
		}
	}
	return NormalizedIndexerEventPage{Events: events, NextCursor: nextCursor}, nil
}

func decodeIndexerBlock(data []byte) ([]NormalizedIndexerEvent, error) {
	height, _, err := protoUvarint(data, 1)
	if err != nil {
		return nil, err
	}
	var events []NormalizedIndexerEvent
	if err := protoEachField(data, func(field protoField) error {
		if field.Number != 3 {
			return nil
		}
		if field.Wire != protoWireBytes {
			return fmt.Errorf("indexer block events field has wire %d, want bytes", field.Wire)
		}
		event, ok, err := decodeIndexerTendermintEvent(uint32(height), field.Value)
		if err != nil {
			return err
		}
		if ok {
			events = append(events, event)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return events, nil
}

func decodeIndexerTendermintEvent(height uint32, data []byte) (NormalizedIndexerEvent, bool, error) {
	subtype, ok, err := protoString(data, 1)
	if err != nil {
		return NormalizedIndexerEvent{}, false, err
	}
	if !ok {
		return NormalizedIndexerEvent{}, false, fmt.Errorf("indexer event missing subtype")
	}
	eventIndex, _, err := protoUvarint(data, 5)
	if err != nil {
		return NormalizedIndexerEvent{}, false, err
	}
	dataBytes, ok, err := protoBytes(data, 7)
	if err != nil {
		return NormalizedIndexerEvent{}, false, err
	}
	if !ok {
		return NormalizedIndexerEvent{}, false, fmt.Errorf("indexer event missing data bytes")
	}
	eventID, err := protobufIndexerEventID(height, data, uint32(eventIndex))
	if err != nil {
		return NormalizedIndexerEvent{}, false, err
	}
	switch subtype {
	case protobufIndexerSubtypeOrderFill:
		orderFill, err := decodeProtobufOrderFill(dataBytes)
		if err != nil {
			return NormalizedIndexerEvent{}, false, err
		}
		return NormalizedIndexerEvent{EventID: eventID, Type: NormalizedIndexerEventTypeOrderFill, OrderFill: &orderFill}, true, nil
	case protobufIndexerSubtypeStatefulOrder:
		removal, ok, err := decodeProtobufStatefulOrderRemoval(dataBytes)
		if err != nil || !ok {
			return NormalizedIndexerEvent{}, false, err
		}
		return NormalizedIndexerEvent{EventID: eventID, Type: NormalizedIndexerEventTypeStatefulOrderRemoval, StatefulOrderRemoval: &removal}, true, nil
	case protobufIndexerSubtypeSubaccountUpdate:
		update, err := decodeProtobufSubaccountUpdate(dataBytes)
		if err != nil {
			return NormalizedIndexerEvent{}, false, err
		}
		return NormalizedIndexerEvent{EventID: eventID, Type: NormalizedIndexerEventTypeSubaccountUpdate, SubaccountUpdate: &update}, true, nil
	default:
		return NormalizedIndexerEvent{}, false, nil
	}
}

func protobufIndexerEventID(height uint32, eventData []byte, eventIndex uint32) (string, error) {
	if txIndex, ok, err := protoUvarint(eventData, 3); err != nil {
		return "", err
	} else if ok {
		return fmt.Sprintf("height-%d-tx-%d-event-%d", height, txIndex, eventIndex), nil
	}
	if blockEvent, ok, err := protoUvarint(eventData, 4); err != nil {
		return "", err
	} else if ok {
		return fmt.Sprintf("height-%d-block-%d-event-%d", height, blockEvent, eventIndex), nil
	}
	return fmt.Sprintf("height-%d-event-%d", height, eventIndex), nil
}

func decodeProtobufOrderFill(data []byte) (NormalizedOrderFillEvent, error) {
	makerOrderBytes, ok, err := protoBytes(data, 1)
	if err != nil {
		return NormalizedOrderFillEvent{}, err
	}
	if !ok {
		return NormalizedOrderFillEvent{}, fmt.Errorf("order fill missing maker order")
	}
	makerOrder, err := decodeProtobufOrder(makerOrderBytes)
	if err != nil {
		return NormalizedOrderFillEvent{}, err
	}
	event := NormalizedOrderFillEvent{MakerOrder: makerOrder}
	if takerOrderBytes, ok, err := protoBytes(data, 2); err != nil {
		return NormalizedOrderFillEvent{}, err
	} else if ok {
		takerOrder, err := decodeProtobufOrder(takerOrderBytes)
		if err != nil {
			return NormalizedOrderFillEvent{}, err
		}
		event.TakerOrder = &takerOrder
	}
	if fillAmount, ok, err := protoUvarint(data, 3); err != nil {
		return NormalizedOrderFillEvent{}, err
	} else if ok {
		event.FillAmount = fillAmount
	}
	if makerFee, ok, err := protoZigZag64(data, 5); err != nil {
		return NormalizedOrderFillEvent{}, err
	} else if ok {
		event.MakerFee = makerFee
	}
	if takerFee, ok, err := protoZigZag64(data, 6); err != nil {
		return NormalizedOrderFillEvent{}, err
	} else if ok {
		event.TakerFee = takerFee
	}
	return event, nil
}

func decodeProtobufStatefulOrderRemoval(data []byte) (NormalizedStatefulOrderRemovalEvent, bool, error) {
	removalBytes, ok, err := protoBytes(data, 4)
	if err != nil || !ok {
		return NormalizedStatefulOrderRemovalEvent{}, false, err
	}
	orderIDBytes, ok, err := protoBytes(removalBytes, 1)
	if err != nil {
		return NormalizedStatefulOrderRemovalEvent{}, false, err
	}
	if !ok {
		return NormalizedStatefulOrderRemovalEvent{}, false, fmt.Errorf("stateful order removal missing order id")
	}
	orderID, err := decodeProtobufOrderID(orderIDBytes)
	if err != nil {
		return NormalizedStatefulOrderRemovalEvent{}, false, err
	}
	reasonValue, _, err := protoUvarint(removalBytes, 2)
	if err != nil {
		return NormalizedStatefulOrderRemovalEvent{}, false, err
	}
	return NormalizedStatefulOrderRemovalEvent{
		OrderID: orderID,
		Reason:  normalizeRemovalReason(protobufRemovalReasonName(reasonValue)),
	}, true, nil
}

func decodeProtobufSubaccountUpdate(data []byte) (NormalizedSubaccountUpdateEvent, error) {
	subaccountBytes, ok, err := protoBytes(data, 1)
	if err != nil {
		return NormalizedSubaccountUpdateEvent{}, err
	}
	if !ok {
		return NormalizedSubaccountUpdateEvent{}, fmt.Errorf("subaccount update missing subaccount id")
	}
	subaccountID, err := decodeProtobufSubaccountID(subaccountBytes)
	if err != nil {
		return NormalizedSubaccountUpdateEvent{}, err
	}
	update := NormalizedSubaccountUpdateEvent{SubaccountID: subaccountID}
	if err := protoEachField(data, func(field protoField) error {
		if field.Number != 3 {
			return nil
		}
		if field.Wire != protoWireBytes {
			return fmt.Errorf("updated perpetual positions field has wire %d, want bytes", field.Wire)
		}
		position, err := decodeProtobufPerpetualPosition(field.Value)
		if err != nil {
			return err
		}
		update.UpdatedPerpetualPositions = append(update.UpdatedPerpetualPositions, position)
		return nil
	}); err != nil {
		return NormalizedSubaccountUpdateEvent{}, err
	}
	return update, nil
}

func decodeProtobufOrder(data []byte) (NormalizedOrder, error) {
	orderIDBytes, ok, err := protoBytes(data, 1)
	if err != nil {
		return NormalizedOrder{}, err
	}
	if !ok {
		return NormalizedOrder{}, fmt.Errorf("order missing order id")
	}
	orderID, err := decodeProtobufOrderID(orderIDBytes)
	if err != nil {
		return NormalizedOrder{}, err
	}
	order := NormalizedOrder{OrderID: orderID}
	if side, ok, err := protoUvarint(data, 2); err != nil {
		return NormalizedOrder{}, err
	} else if ok {
		order.Side = normalizeOrderSideValue(side)
	}
	if quantums, ok, err := protoUvarint(data, 3); err != nil {
		return NormalizedOrder{}, err
	} else if ok {
		order.Quantums = strconv.FormatUint(quantums, 10)
	}
	if subticks, ok, err := protoUvarint(data, 4); err != nil {
		return NormalizedOrder{}, err
	} else if ok {
		order.Subticks = strconv.FormatUint(subticks, 10)
	}
	return order, nil
}

func decodeProtobufOrderID(data []byte) (NormalizedOrderID, error) {
	subaccountBytes, ok, err := protoBytes(data, 1)
	if err != nil {
		return NormalizedOrderID{}, err
	}
	if !ok {
		return NormalizedOrderID{}, fmt.Errorf("order id missing subaccount")
	}
	subaccountID, err := decodeProtobufSubaccountID(subaccountBytes)
	if err != nil {
		return NormalizedOrderID{}, err
	}
	orderID := NormalizedOrderID{SubaccountID: subaccountID}
	if clientID, ok, err := protoFixed32(data, 2); err != nil {
		return NormalizedOrderID{}, err
	} else if ok {
		orderID.ClientID = clientID
	}
	if orderFlags, ok, err := protoUvarint(data, 3); err != nil {
		return NormalizedOrderID{}, err
	} else if ok {
		orderID.OrderFlags = uint32(orderFlags)
	}
	if clobPairID, ok, err := protoUvarint(data, 4); err != nil {
		return NormalizedOrderID{}, err
	} else if ok {
		orderID.ClobPairID = uint32(clobPairID)
	}
	return orderID, nil
}

func decodeProtobufSubaccountID(data []byte) (NormalizedSubaccountID, error) {
	owner, _, err := protoString(data, 1)
	if err != nil {
		return NormalizedSubaccountID{}, err
	}
	number, _, err := protoUvarint(data, 2)
	if err != nil {
		return NormalizedSubaccountID{}, err
	}
	return NormalizedSubaccountID{Owner: owner, Number: uint32(number)}, nil
}

func decodeProtobufPerpetualPosition(data []byte) (NormalizedPerpetualPosition, error) {
	perpetualID, _, err := protoUvarint(data, 1)
	if err != nil {
		return NormalizedPerpetualPosition{}, err
	}
	quantums, ok, err := protoSerializableIntString(data, 2)
	if err != nil {
		return NormalizedPerpetualPosition{}, err
	}
	if !ok {
		quantums = "0"
	}
	return NormalizedPerpetualPosition{PerpetualId: uint32(perpetualID), Quantums: quantums}, nil
}

func protobufRemovalReasonName(reason uint64) string {
	switch reason {
	case 1:
		return "ORDER_REMOVAL_REASON_EXPIRED"
	case 2:
		return "ORDER_REMOVAL_REASON_USER_CANCELED"
	case 3:
		return "ORDER_REMOVAL_REASON_UNDERCOLLATERALIZED"
	case 4:
		return "ORDER_REMOVAL_REASON_INTERNAL_ERROR"
	case 6:
		return "ORDER_REMOVAL_REASON_POST_ONLY_WOULD_CROSS_MAKER_ORDER"
	case 8:
		return "ORDER_REMOVAL_REASON_FOK_ORDER_COULD_NOT_BE_FULLY_FULLED"
	case 10:
		return "ORDER_REMOVAL_REASON_INDEXER_EXPIRED"
	case 11:
		return "ORDER_REMOVAL_REASON_REPLACED"
	case 12:
		return "ORDER_REMOVAL_REASON_FULLY_FILLED"
	default:
		return "ORDER_REMOVAL_REASON_" + strconv.FormatUint(reason, 10)
	}
}
