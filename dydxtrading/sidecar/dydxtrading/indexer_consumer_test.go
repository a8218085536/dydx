package dydxtrading

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

var errIndexerConsumerTestIngestFailed = errors.New("ingest failed")

func TestIndexerConsumerWorkerPollOnceTranslatesAndIngestsBatch(t *testing.T) {
	source := &recordingNormalizedEventSource{
		page: NormalizedIndexerEventPage{
			NextCursor: "cursor-2",
			Events: []NormalizedIndexerEvent{
				{
					EventID: "height-10-event-1",
					Type:    NormalizedIndexerEventTypeOrderFill,
					OrderFill: &NormalizedOrderFillEvent{
						MakerOrder: NormalizedOrder{OrderID: normalizedOrderID("dydx1maker", 22, 7)},
						TakerOrder: &NormalizedOrder{OrderID: normalizedOrderID("dydx1taker", 33, 7)},
						MakerFee:   1,
						TakerFee:   2,
					},
				},
				{
					EventID: "height-10-event-2",
					Type:    NormalizedIndexerEventTypeStatefulOrderRemoval,
					StatefulOrderRemoval: &NormalizedStatefulOrderRemovalEvent{
						OrderID: normalizedOrderID("dydx1taker", 33, 7),
						Reason:  NormalizedOrderRemovalReasonUserCanceled,
					},
				},
				{
					EventID: "height-10-event-3",
					Type:    NormalizedIndexerEventTypeSubaccountUpdate,
					SubaccountUpdate: &NormalizedSubaccountUpdateEvent{
						SubaccountID: NormalizedSubaccountID{Owner: "dydx1taker", Number: 0},
						UpdatedPerpetualPositions: []NormalizedPerpetualPosition{
							{PerpetualId: 2, Quantums: "123"},
						},
					},
				},
			},
		},
	}
	cursor := NewMemoryIndexerConsumerCursorStore("cursor-1")
	ingestor := &recordingTradingEventIngestor{}
	worker := NewIndexerConsumerWorker(IndexerConsumerWorkerConfig{
		Source:   source,
		Cursor:   cursor,
		Ingestor: ingestor,
		Translator: NewIndexerEventTranslator(IndexerEventTranslatorConfig{
			UserResolver: staticUserResolver{
				"dydx1maker": "maker-user",
				"dydx1taker": "taker-user",
			},
			Markets:    map[uint32]TradingMarketMapping{7: {Market: "ETH-USD", Asset: "USDT"}},
			Perpetuals: map[uint32]TradingMarketMapping{2: {Market: "ETH-USD", Asset: "USDT"}},
		}),
	})

	err := worker.PollOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, "cursor-1", source.cursor)
	require.Equal(t, "cursor-2", cursor.cursor)
	require.Len(t, ingestor.batch.Fills, 2)
	require.Equal(t, "height-10-event-1:maker", ingestor.batch.Fills[0].EventID)
	require.Equal(t, "height-10-event-1:taker", ingestor.batch.Fills[1].EventID)
	require.Len(t, ingestor.batch.OrderStatuses, 1)
	require.Equal(t, "cancelled", ingestor.batch.OrderStatuses[0].Status)
	require.Len(t, ingestor.batch.Positions, 1)
	require.Equal(t, "123", ingestor.batch.Positions[0].Size)
}

func TestIndexerConsumerWorkerDoesNotAdvanceCursorWhenIngestFails(t *testing.T) {
	source := &recordingNormalizedEventSource{
		page: NormalizedIndexerEventPage{
			NextCursor: "cursor-2",
			Events: []NormalizedIndexerEvent{
				{
					EventID: "height-10-event-1",
					Type:    NormalizedIndexerEventTypeOrderFill,
					OrderFill: &NormalizedOrderFillEvent{
						TakerOrder: &NormalizedOrder{OrderID: normalizedOrderID("dydx1taker", 33, 7)},
					},
				},
			},
		},
	}
	cursor := NewMemoryIndexerConsumerCursorStore("cursor-1")
	worker := NewIndexerConsumerWorker(IndexerConsumerWorkerConfig{
		Source:   source,
		Cursor:   cursor,
		Ingestor: failingTradingEventIngestor{},
		Translator: NewIndexerEventTranslator(IndexerEventTranslatorConfig{
			UserResolver: staticUserResolver{"dydx1taker": "taker-user"},
			Markets:      map[uint32]TradingMarketMapping{7: {Market: "ETH-USD", Asset: "USDT"}},
		}),
	})

	err := worker.PollOnce(context.Background())

	require.ErrorContains(t, err, "ingest failed")
	require.Equal(t, "cursor-1", cursor.cursor)
}

type recordingNormalizedEventSource struct {
	cursor string
	page   NormalizedIndexerEventPage
}

func (s *recordingNormalizedEventSource) Fetch(ctx context.Context, cursor string) (NormalizedIndexerEventPage, error) {
	_ = ctx
	s.cursor = cursor
	return s.page, nil
}

type recordingTradingEventIngestor struct {
	batch TradingEventBatch
}

func (i *recordingTradingEventIngestor) Ingest(ctx context.Context, batch TradingEventBatch) error {
	_ = ctx
	i.batch = batch
	return nil
}

type failingTradingEventIngestor struct{}

func (failingTradingEventIngestor) Ingest(context.Context, TradingEventBatch) error {
	return errIndexerConsumerTestIngestFailed
}
