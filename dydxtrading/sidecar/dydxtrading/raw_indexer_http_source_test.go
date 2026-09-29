package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPRawIndexerEventSourceFetchesAndConvertsEvents(t *testing.T) {
	var gotCursor string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/raw-indexer/events", r.URL.Path)
		gotCursor = r.URL.Query().Get("cursor")
		require.Equal(t, "raw-secret", r.Header.Get("X-API-Key"))
		_, _ = w.Write([]byte(`{
			"code": 200,
			"nextCursor": "cursor-2",
			"data": [{
				"eventId": "height-10-event-1",
				"subtype": "stateful_order",
				"data": {
					"orderRemoval": {
						"removedOrderId": {
							"subaccountId": {"owner": "dydx1owner", "number": 0},
							"clientId": 44,
							"orderFlags": 64,
							"clobPairId": 7
						},
						"reason": "ORDER_REMOVAL_REASON_USER_CANCELED"
					}
				}
			}]
		}`))
	}))
	defer server.Close()

	page, err := NewHTTPRawIndexerEventSource(HTTPRawIndexerEventSourceConfig{
		URL:    server.URL + "/raw-indexer/events",
		APIKey: "raw-secret",
		Client: server.Client(),
	}).Fetch(context.Background(), "cursor-1")

	require.NoError(t, err)
	require.Equal(t, "cursor-1", gotCursor)
	require.Equal(t, "cursor-2", page.NextCursor)
	require.Len(t, page.Events, 1)
	require.Equal(t, NormalizedIndexerEventTypeStatefulOrderRemoval, page.Events[0].Type)
	require.Equal(t, "user_canceled", page.Events[0].StatefulOrderRemoval.Reason)
}
