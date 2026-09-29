package dydxtrading

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPTradingEventIngestorPostsBatchWithBearerToken(t *testing.T) {
	var gotAuth string
	var gotBatch TradingEventBatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/dydx/events/ingest", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBatch))
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer server.Close()

	err := NewHTTPTradingEventIngestor(HTTPTradingEventIngestorConfig{
		URL:         server.URL,
		BearerToken: "ingest-secret",
		Client:      server.Client(),
	}).Ingest(context.Background(), TradingEventBatch{
		Fills: []TradingFillEvent{{EventID: "fill-1", UserID: "user-1"}},
	})

	require.NoError(t, err)
	require.Equal(t, "Bearer ingest-secret", gotAuth)
	require.Len(t, gotBatch.Fills, 1)
	require.Equal(t, "fill-1", gotBatch.Fills[0].EventID)
}

func TestHTTPTradingEventIngestorReturnsSidecarError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":400,"msg":"missing event id"}`))
	}))
	defer server.Close()

	err := NewHTTPTradingEventIngestor(HTTPTradingEventIngestorConfig{
		URL:    server.URL,
		Client: server.Client(),
	}).Ingest(context.Background(), TradingEventBatch{})

	require.ErrorContains(t, err, "missing event id")
}
