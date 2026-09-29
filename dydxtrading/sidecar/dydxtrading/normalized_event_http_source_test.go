package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPNormalizedIndexerEventSourceFetchesEventsWithCursorAndAPIKey(t *testing.T) {
	var gotCursor string
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/indexer/events", r.URL.Path)
		gotCursor = r.URL.Query().Get("cursor")
		gotAPIKey = r.Header.Get("X-API-Key")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"nextCursor": "cursor-2",
			"data": [{
				"eventId": "height-10-event-1",
				"type": "order_fill",
				"orderFill": {
					"takerOrder": {
						"orderId": {
							"subaccountId": {"owner": "dydx1taker", "number": 0},
							"clientId": 33,
							"clobPairId": 7
						}
					},
					"takerFee": 2
				}
			}]
		}`))
	}))
	defer server.Close()

	page, err := NewHTTPNormalizedIndexerEventSource(HTTPNormalizedIndexerEventSourceConfig{
		URL:    server.URL + "/indexer/events",
		APIKey: "source-secret",
		Client: server.Client(),
	}).Fetch(context.Background(), "cursor-1")

	require.NoError(t, err)
	require.Equal(t, "cursor-1", gotCursor)
	require.Equal(t, "source-secret", gotAPIKey)
	require.Equal(t, "cursor-2", page.NextCursor)
	require.Len(t, page.Events, 1)
	require.Equal(t, NormalizedIndexerEventTypeOrderFill, page.Events[0].Type)
	require.Equal(t, "dydx1taker", page.Events[0].OrderFill.TakerOrder.OrderID.SubaccountID.Owner)
}
