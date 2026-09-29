package dydxtrading

import (
	"context"
	"testing"
)

func TestProtobufIndexerBlockSourceDecodesIndexerTendermintBlock(t *testing.T) {
	orderFill := testProtoMessage(
		testProtoBytes(1, testIndexerOrder("dydx1maker", 0, 22, 0, 7, 2, 1000000000, 1818000000)),
		testProtoBytes(2, testIndexerOrder("dydx1taker", 0, 33, 0, 7, 1, 2000000000, 1819000000)),
		testProtoVarint(3, 55),
		testProtoZigZag64(5, -1),
		testProtoZigZag64(6, 2),
	)
	removal := testProtoMessage(
		testProtoBytes(4, testProtoMessage(
			testProtoBytes(1, testIndexerOrderID("dydx1taker", 0, 33, 0, 7)),
			testProtoVarint(2, 12),
		)),
	)
	subaccountUpdate := testProtoMessage(
		testProtoBytes(1, testIndexerSubaccountID("dydx1taker", 0)),
		testProtoBytes(3, testProtoMessage(
			testProtoVarint(1, 2),
			testProtoBytes(2, []byte{2, 123}),
		)),
	)
	block := testProtoMessage(
		testProtoVarint(1, 10),
		testProtoBytes(3, testIndexerTendermintEvent("order_fill", 0, 0, orderFill)),
		testProtoBytes(3, testIndexerTendermintEvent("stateful_order", 0, 1, removal)),
		testProtoBytes(3, testIndexerTendermintEvent("subaccount_update", 0, 2, subaccountUpdate)),
	)
	source := NewProtobufIndexerBlockSource(&recordingIndexerBlockMessageSource{
		messages: []IndexerBlockMessage{{Offset: "offset-10", Value: block}},
	})

	page, err := source.Fetch(context.Background(), "")

	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if page.NextCursor != "offset-10" {
		t.Fatalf("NextCursor = %q, want offset-10", page.NextCursor)
	}
	if len(page.Events) != 3 {
		t.Fatalf("events len = %d, want 3: %+v", len(page.Events), page.Events)
	}
	if page.Events[0].EventID != "height-10-tx-0-event-0" || page.Events[0].Type != NormalizedIndexerEventTypeOrderFill {
		t.Fatalf("first event = %+v", page.Events[0])
	}
	if page.Events[0].OrderFill.FillAmount != 55 || page.Events[0].OrderFill.MakerFee != -1 || page.Events[0].OrderFill.TakerFee != 2 {
		t.Fatalf("order fill = %+v", page.Events[0].OrderFill)
	}
	if page.Events[0].OrderFill.MakerOrder.OrderID.SubaccountID.Owner != "dydx1maker" {
		t.Fatalf("maker owner = %q, want dydx1maker", page.Events[0].OrderFill.MakerOrder.OrderID.SubaccountID.Owner)
	}
	if page.Events[0].OrderFill.MakerOrder.Side != "sell" || page.Events[0].OrderFill.MakerOrder.Quantums != "1000000000" || page.Events[0].OrderFill.MakerOrder.Subticks != "1818000000" {
		t.Fatalf("maker order details = %+v", page.Events[0].OrderFill.MakerOrder)
	}
	if page.Events[0].OrderFill.TakerOrder == nil || page.Events[0].OrderFill.TakerOrder.Side != "buy" || page.Events[0].OrderFill.TakerOrder.Quantums != "2000000000" || page.Events[0].OrderFill.TakerOrder.Subticks != "1819000000" {
		t.Fatalf("taker order details = %+v", page.Events[0].OrderFill.TakerOrder)
	}
	if page.Events[1].Type != NormalizedIndexerEventTypeStatefulOrderRemoval {
		t.Fatalf("second type = %q", page.Events[1].Type)
	}
	if page.Events[1].StatefulOrderRemoval.Reason != NormalizedOrderRemovalReasonFullyFilled {
		t.Fatalf("removal reason = %q", page.Events[1].StatefulOrderRemoval.Reason)
	}
	if page.Events[2].Type != NormalizedIndexerEventTypeSubaccountUpdate {
		t.Fatalf("third type = %q", page.Events[2].Type)
	}
	if got := page.Events[2].SubaccountUpdate.UpdatedPerpetualPositions[0].Quantums; got != "123" {
		t.Fatalf("position quantums = %q, want 123", got)
	}
}

func TestProtobufIndexerBlockSourceSkipsUnsupportedSubtypes(t *testing.T) {
	block := testProtoMessage(
		testProtoVarint(1, 11),
		testProtoBytes(3, testIndexerTendermintEvent("market", 0, 0, []byte("ignored"))),
	)
	source := NewProtobufIndexerBlockSource(&recordingIndexerBlockMessageSource{
		messages: []IndexerBlockMessage{{Offset: "offset-11", Value: block}},
	})

	page, err := source.Fetch(context.Background(), "")

	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(page.Events) != 0 {
		t.Fatalf("events len = %d, want 0", len(page.Events))
	}
	if page.NextCursor != "offset-11" {
		t.Fatalf("NextCursor = %q, want offset-11", page.NextCursor)
	}
}

func TestProtobufIndexerConsumerWorkerIngestsEventsForTyche(t *testing.T) {
	orderFill := testProtoMessage(
		testProtoBytes(1, testIndexerOrder("dydx1maker", 0, 22, 0, 7, 2, 1000000000, 1818000000)),
		testProtoBytes(2, testIndexerOrder("dydx1taker", 0, 33, 0, 7, 1, 2000000000, 1819000000)),
		testProtoVarint(3, 55),
		testProtoZigZag64(5, -1),
		testProtoZigZag64(6, 2),
	)
	removal := testProtoMessage(
		testProtoBytes(4, testProtoMessage(
			testProtoBytes(1, testIndexerOrderID("dydx1taker", 0, 33, 0, 7)),
			testProtoVarint(2, 12),
		)),
	)
	block := testProtoMessage(
		testProtoVarint(1, 10),
		testProtoBytes(3, testIndexerTendermintEvent("order_fill", 0, 0, orderFill)),
		testProtoBytes(3, testIndexerTendermintEvent("stateful_order", 0, 1, removal)),
	)
	source := NewProtobufIndexerBlockSource(&recordingIndexerBlockMessageSource{
		messages: []IndexerBlockMessage{{Offset: "11", Value: block}},
	})
	buffer := NewMemoryTradingEventBuffer()
	cursor := NewMemoryIndexerConsumerCursorStore("10")
	worker := NewIndexerConsumerWorker(IndexerConsumerWorkerConfig{
		Source:   source,
		Cursor:   cursor,
		Ingestor: buffer,
		Translator: NewIndexerEventTranslator(IndexerEventTranslatorConfig{
			UserResolver: staticUserResolver{"dydx1taker": "user-1"},
		}),
	})

	if err := worker.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	fillPage, err := buffer.Fills(context.Background(), "")
	if err != nil {
		t.Fatalf("Fills() error = %v", err)
	}
	if len(fillPage.Data) != 1 {
		t.Fatalf("fills len = %d, want 1: %+v", len(fillPage.Data), fillPage.Data)
	}
	if fillPage.Data[0].UserID != "user-1" || fillPage.Data[0].OrderID != "dydx1taker:0:33:7" || fillPage.Data[0].Fee != "2" {
		t.Fatalf("fill = %+v", fillPage.Data[0])
	}
	if fillPage.Data[0].LiquidityRole != "taker" || fillPage.Data[0].MakerAddress != "dydx1maker" || fillPage.Data[0].TakerAddress != "dydx1taker" || fillPage.Data[0].FillAmount != "55" {
		t.Fatalf("fill trade details = %+v", fillPage.Data[0])
	}
	statusPage, err := buffer.OrderStatuses(context.Background(), "")
	if err != nil {
		t.Fatalf("OrderStatuses() error = %v", err)
	}
	if len(statusPage.Data) != 1 {
		t.Fatalf("statuses len = %d, want 1: %+v", len(statusPage.Data), statusPage.Data)
	}
	if statusPage.Data[0].Status != "filled" || statusPage.Data[0].OrderID != "dydx1taker:0:33:7" {
		t.Fatalf("status = %+v", statusPage.Data[0])
	}
	if cursor.cursor != "11" {
		t.Fatalf("cursor = %q, want 11", cursor.cursor)
	}
}

type recordingIndexerBlockMessageSource struct {
	cursor   string
	messages []IndexerBlockMessage
}

func (s *recordingIndexerBlockMessageSource) FetchMessages(ctx context.Context, cursor string) ([]IndexerBlockMessage, string, error) {
	_ = ctx
	s.cursor = cursor
	nextCursor := ""
	if len(s.messages) > 0 {
		nextCursor = s.messages[len(s.messages)-1].Offset
	}
	return s.messages, nextCursor, nil
}

func testIndexerTendermintEvent(subtype string, txIndex, eventIndex uint32, data []byte) []byte {
	return testProtoMessage(
		testProtoString(1, subtype),
		testProtoVarint(3, uint64(txIndex)),
		testProtoVarint(5, uint64(eventIndex)),
		testProtoVarint(6, 1),
		testProtoBytes(7, data),
	)
}

func testIndexerOrder(owner string, number, clientID, orderFlags, clobPairID uint32, side, quantums, subticks uint64) []byte {
	return testProtoMessage(
		testProtoBytes(1, testIndexerOrderID(owner, number, clientID, orderFlags, clobPairID)),
		testProtoVarint(2, side),
		testProtoVarint(3, quantums),
		testProtoVarint(4, subticks),
	)
}

func testIndexerOrderID(owner string, number, clientID, orderFlags, clobPairID uint32) []byte {
	return testProtoMessage(
		testProtoBytes(1, testIndexerSubaccountID(owner, number)),
		testProtoFixed32(2, clientID),
		testProtoVarint(3, uint64(orderFlags)),
		testProtoVarint(4, uint64(clobPairID)),
	)
}

func testIndexerSubaccountID(owner string, number uint32) []byte {
	return testProtoMessage(
		testProtoString(1, owner),
		testProtoVarint(2, uint64(number)),
	)
}

func testProtoMessage(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func testProtoString(field uint64, value string) []byte {
	return testProtoBytes(field, []byte(value))
}

func testProtoBytes(field uint64, value []byte) []byte {
	out := testProtoTag(field, 2)
	out = append(out, testProtoUvarint(uint64(len(value)))...)
	out = append(out, value...)
	return out
}

func testProtoVarint(field uint64, value uint64) []byte {
	out := testProtoTag(field, 0)
	out = append(out, testProtoUvarint(value)...)
	return out
}

func testProtoZigZag64(field uint64, value int64) []byte {
	encoded := uint64(value<<1) ^ uint64(value>>63)
	return testProtoVarint(field, encoded)
}

func testProtoFixed32(field uint64, value uint32) []byte {
	out := testProtoTag(field, 5)
	out = append(out, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
	return out
}

func testProtoTag(field uint64, wire uint64) []byte {
	return testProtoUvarint(field<<3 | wire)
}

func testProtoUvarint(value uint64) []byte {
	var out []byte
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}
