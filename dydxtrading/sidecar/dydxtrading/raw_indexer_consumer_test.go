package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRawIndexerConsumerWorkerPollsRawSourceThroughTycheResolver(t *testing.T) {
	var gotRawKey string
	var gotTycheKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/raw-indexer/events":
			gotRawKey = r.Header.Get("X-API-Key")
			_, _ = w.Write([]byte(`{
				"code": 200,
				"nextCursor": "cursor-2",
				"data": [{
					"eventId": "height-10-event-1",
					"subtype": "stateful_order",
					"data": {
						"order_removal": {
							"removed_order_id": {
								"subaccount_id": {"owner": "dydx1user", "number": 0},
								"client_id": 44,
								"order_flags": 64,
								"clob_pair_id": 7
							},
							"reason": "ORDER_REMOVAL_REASON_FULLY_FILLED"
						}
					}
				}]
			}`))
		case "/tyche/api/wallet-binding/get-by-dydx":
			gotTycheKey = r.Header.Get("X-Tyche-Internal-Key")
			_, _ = w.Write([]byte(`{
				"code": 200,
				"data": {
					"userId": "user-1",
					"dydxAddress": "dydx1user",
					"status": "active"
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	buffer := NewMemoryTradingEventBuffer()
	cursor := NewMemoryIndexerConsumerCursorStore("cursor-1")
	worker, enabled, err := NewRawIndexerConsumerWorker(RawIndexerConsumerWorkerConfig{
		RawSourceURL:        server.URL + "/raw-indexer/events",
		RawSourceAPIKey:     "raw-secret",
		TycheURL:            server.URL,
		TycheInternalAPIKey: "tyche-secret",
		Client:              server.Client(),
		Cursor:              cursor,
		Ingestor:            buffer,
	})
	require.NoError(t, err)
	require.True(t, enabled)

	err = worker.PollOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, "raw-secret", gotRawKey)
	require.Equal(t, "tyche-secret", gotTycheKey)
	require.Equal(t, "cursor-2", cursor.cursor)
	page, err := buffer.OrderStatuses(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, page.Data, 1)
	require.Equal(t, "user-1", page.Data[0].UserID)
	require.Equal(t, "filled", page.Data[0].Status)
	require.Equal(t, "dydx1user:0:44:7", page.Data[0].OrderID)
}

func TestNewRawIndexerConsumerWorkerDisabledWithoutRawSource(t *testing.T) {
	worker, enabled, err := NewRawIndexerConsumerWorker(RawIndexerConsumerWorkerConfig{
		Ingestor: NewMemoryTradingEventBuffer(),
	})

	require.NoError(t, err)
	require.False(t, enabled)
	require.Nil(t, worker)
}

func TestNewRawIndexerConsumerWorkerUsesProvidedNormalizedSource(t *testing.T) {
	var gotTycheKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tyche/api/wallet-binding/get-by-dydx":
			gotTycheKey = r.Header.Get("X-Tyche-Internal-Key")
			_, _ = w.Write([]byte(`{
				"code": 200,
				"data": {
					"userId": "user-1",
					"dydxAddress": "dydx1user",
					"status": "active"
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	buffer := NewMemoryTradingEventBuffer()
	cursor := NewMemoryIndexerConsumerCursorStore("cursor-1")
	source := &recordingNormalizedIndexerEventSource{
		cursor:     "cursor-1",
		nextCursor: "cursor-2",
		events: []NormalizedIndexerEvent{{
			EventID: "height-10-tx-0-event-1",
			Type:    NormalizedIndexerEventTypeStatefulOrderRemoval,
			StatefulOrderRemoval: &NormalizedStatefulOrderRemovalEvent{
				OrderID: NormalizedOrderID{
					SubaccountID: NormalizedSubaccountID{Owner: "dydx1user"},
					ClientID:     44,
					ClobPairID:   7,
				},
				Reason: NormalizedOrderRemovalReasonFullyFilled,
			},
		}},
	}
	worker, enabled, err := NewRawIndexerConsumerWorker(RawIndexerConsumerWorkerConfig{
		Source:              source,
		TycheURL:            server.URL,
		TycheInternalAPIKey: "tyche-secret",
		Client:              server.Client(),
		Cursor:              cursor,
		Ingestor:            buffer,
	})
	require.NoError(t, err)
	require.True(t, enabled)

	err = worker.PollOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, "tyche-secret", gotTycheKey)
	require.Equal(t, "cursor-1", source.gotCursor)
	require.Equal(t, "cursor-2", cursor.cursor)
	page, err := buffer.OrderStatuses(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, page.Data, 1)
	require.Equal(t, "user-1", page.Data[0].UserID)
	require.Equal(t, "filled", page.Data[0].Status)
}

func TestNewRawIndexerConsumerWorkerRequiresTycheURL(t *testing.T) {
	worker, enabled, err := NewRawIndexerConsumerWorker(RawIndexerConsumerWorkerConfig{
		RawSourceURL: "http://127.0.0.1/raw-indexer/events",
		Ingestor:     NewMemoryTradingEventBuffer(),
	})

	require.ErrorContains(t, err, "missing Tyche URL")
	require.False(t, enabled)
	require.Nil(t, worker)
}

type recordingNormalizedIndexerEventSource struct {
	cursor     string
	gotCursor  string
	nextCursor string
	events     []NormalizedIndexerEvent
}

func (s *recordingNormalizedIndexerEventSource) Fetch(ctx context.Context, cursor string) (NormalizedIndexerEventPage, error) {
	_ = ctx
	s.gotCursor = cursor
	return NormalizedIndexerEventPage{
		Events:     s.events,
		NextCursor: s.nextCursor,
	}, nil
}
