package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRESTTradingEventProviderFetchesFillsWithCursorAndAPIKey(t *testing.T) {
	var gotPath string
	var gotCursor string
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCursor = r.URL.Query().Get("cursor")
		gotAPIKey = r.Header.Get("X-API-Key")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"nextCursor": "next-1",
			"data": [{
				"eventId": "fill-1",
				"userId": "user-1",
				"orderId": "order-1",
				"market": "ETH-USD",
				"asset": "USDT",
				"releasedMargin": "0",
				"fee": "1",
				"realizedPnl": "2",
				"orderStatus": "filled"
			}]
		}`))
	}))
	defer server.Close()

	page, err := NewRESTTradingEventProvider(RESTTradingEventProviderConfig{
		URL:    server.URL,
		APIKey: "upstream-secret",
		Client: server.Client(),
	}).Fills(context.Background(), "cursor-1")

	require.NoError(t, err)
	require.Equal(t, "/fills", gotPath)
	require.Equal(t, "cursor-1", gotCursor)
	require.Equal(t, "upstream-secret", gotAPIKey)
	require.Equal(t, "next-1", page.NextCursor)
	require.Len(t, page.Data, 1)
	require.Equal(t, "fill-1", page.Data[0].EventID)
}

func TestRESTTradingEventProviderReturnsUpstreamBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code": 500, "msg": "indexer unavailable", "data": []}`))
	}))
	defer server.Close()

	_, err := NewRESTTradingEventProvider(RESTTradingEventProviderConfig{
		URL:    server.URL,
		Client: server.Client(),
	}).OrderStatuses(context.Background(), "")

	require.ErrorContains(t, err, "indexer unavailable")
}
