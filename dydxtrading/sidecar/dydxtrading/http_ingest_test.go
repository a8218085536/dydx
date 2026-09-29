package dydxtrading

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPHandlerIngestsTradingEventsAndServesThemToTyche(t *testing.T) {
	buffer := NewMemoryTradingEventBuffer()
	handler := NewTradingEventHTTPHandler(HTTPConfig{
		EventAPIKey:    "event-secret",
		EventProvider:  buffer,
		EventIngestor:  buffer,
		IngestEventKey: "ingest-secret",
	})
	body, err := json.Marshal(TradingEventBatch{
		Fills: []TradingFillEvent{
			{EventID: "fill-1", UserID: "user-1", OrderID: "order-1", Asset: "USDT", Fee: "1"},
		},
	})
	require.NoError(t, err)

	ingestRecorder := httptest.NewRecorder()
	ingestReq := httptest.NewRequest(http.MethodPost, "/dydx/events/ingest", bytes.NewReader(body))
	ingestReq.Header.Set("Authorization", "Bearer ingest-secret")
	handler.ServeHTTP(ingestRecorder, ingestReq)

	require.Equal(t, http.StatusOK, ingestRecorder.Code)
	var ingestResp tradingEventResponse
	require.NoError(t, json.Unmarshal(ingestRecorder.Body.Bytes(), &ingestResp))
	require.Equal(t, 200, ingestResp.Code)

	fillRecorder := httptest.NewRecorder()
	fillReq := httptest.NewRequest(http.MethodGet, "/fills", nil)
	fillReq.Header.Set("X-API-Key", "event-secret")
	handler.ServeHTTP(fillRecorder, fillReq)

	require.Equal(t, http.StatusOK, fillRecorder.Code)
	var fillResp struct {
		Code       int                `json:"code"`
		NextCursor string             `json:"nextCursor"`
		Data       []TradingFillEvent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(fillRecorder.Body.Bytes(), &fillResp))
	require.Equal(t, 200, fillResp.Code)
	require.Equal(t, "1", fillResp.NextCursor)
	require.Len(t, fillResp.Data, 1)
	require.Equal(t, "fill-1", fillResp.Data[0].EventID)
}

func TestHTTPHandlerRejectsEventIngestWithoutIngestKey(t *testing.T) {
	buffer := NewMemoryTradingEventBuffer()
	handler := NewTradingEventHTTPHandler(HTTPConfig{
		EventIngestor:  buffer,
		IngestEventKey: "ingest-secret",
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/dydx/events/ingest", bytes.NewReader([]byte(`{}`))))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
