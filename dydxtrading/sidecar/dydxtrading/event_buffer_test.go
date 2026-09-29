package dydxtrading

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryTradingEventBufferIngestsIdempotentlyAndPages(t *testing.T) {
	buffer := NewMemoryTradingEventBuffer()

	err := buffer.Ingest(context.Background(), TradingEventBatch{
		Fills: []TradingFillEvent{
			{EventID: "fill-1", UserID: "user-1", OrderID: "order-1"},
			{EventID: "fill-2", UserID: "user-1", OrderID: "order-2"},
			{EventID: "fill-1", UserID: "user-1", OrderID: "order-1-duplicate"},
		},
		OrderStatuses: []TradingOrderStatusEvent{
			{EventID: "status-1", UserID: "user-1", OrderID: "order-1", Status: "cancelled"},
		},
		Positions: []TradingPositionEvent{
			{EventID: "position-1", UserID: "user-1", Market: "ETH-USD", Size: "1"},
		},
	})

	require.NoError(t, err)
	fills, err := buffer.FillsPage(context.Background(), "", 1)
	require.NoError(t, err)
	require.Equal(t, "1", fills.NextCursor)
	require.Len(t, fills.Data, 1)
	require.Equal(t, "fill-1", fills.Data[0].EventID)

	nextFills, err := buffer.FillsPage(context.Background(), fills.NextCursor, 10)
	require.NoError(t, err)
	require.Equal(t, "2", nextFills.NextCursor)
	require.Len(t, nextFills.Data, 1)
	require.Equal(t, "fill-2", nextFills.Data[0].EventID)

	emptyFills, err := buffer.FillsPage(context.Background(), nextFills.NextCursor, 10)
	require.NoError(t, err)
	require.Equal(t, "2", emptyFills.NextCursor)
	require.Empty(t, emptyFills.Data)

	statuses, err := buffer.OrderStatusesPage(context.Background(), "", 10)
	require.NoError(t, err)
	require.Equal(t, "1", statuses.NextCursor)
	require.Len(t, statuses.Data, 1)
	require.Equal(t, "status-1", statuses.Data[0].EventID)

	positions, err := buffer.PositionsPage(context.Background(), "", 10)
	require.NoError(t, err)
	require.Equal(t, "1", positions.NextCursor)
	require.Len(t, positions.Data, 1)
	require.Equal(t, "position-1", positions.Data[0].EventID)
}

func TestMemoryTradingEventBufferRejectsMissingEventID(t *testing.T) {
	buffer := NewMemoryTradingEventBuffer()

	err := buffer.Ingest(context.Background(), TradingEventBatch{
		Fills: []TradingFillEvent{{UserID: "user-1"}},
	})

	require.ErrorContains(t, err, "missing event id")
}

func TestMemoryTradingEventBufferImplementsProviderAndIngestor(t *testing.T) {
	var provider TradingEventProvider = NewMemoryTradingEventBuffer()
	ingestor, ok := provider.(TradingEventIngestor)
	require.True(t, ok)

	err := ingestor.Ingest(context.Background(), TradingEventBatch{
		OrderStatuses: []TradingOrderStatusEvent{{EventID: "status-1", UserID: "user-1", Status: "rejected"}},
	})
	require.NoError(t, err)

	statuses, err := provider.OrderStatuses(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, statuses.Data, 1)
	require.Equal(t, "status-1", statuses.Data[0].EventID)
}
