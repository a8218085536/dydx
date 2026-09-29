//go:build cgo

package dydxtrading

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPHandlerServesReconcileOrderEndpoint(t *testing.T) {
	provider := &recordingReconcileProvider{
		result: OrderReconcileResult{
			Tx:            TxReconcileSnapshot{Found: true, TxHash: "HASH", Code: 0},
			StatefulOrder: StatefulOrderReconcileSnapshot{Checked: true, Found: true, FillAmount: "0"},
			Subaccount: SubaccountReconcileSnapshot{
				Found: true,
				PerpetualPositions: []PerpetualPositionReconcileSnapshot{{
					PerpetualID: 1,
					Quantums:    "-1000000",
				}},
			},
		},
	}
	handler := NewHTTPHandler(NewService(ServiceConfig{}), HTTPConfig{
		EventAPIKey:       "event-secret",
		ReconcileProvider: provider,
	})
	raw := []byte(`{"txHash":"HASH","userId":"user-1","orderId":"order-1","dydxAccount":"dydx1abc","clientOrderId":"123","market":"ETH-USD","subaccountNumber":0,"clobPairId":1,"orderFlags":64}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reconcile/order", bytes.NewReader(raw))
	req.Header.Set("X-API-Key", "event-secret")

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int                  `json:"code"`
		Msg  string               `json:"msg"`
		Data OrderReconcileResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, "succ", resp.Msg)
	require.Equal(t, "HASH", provider.request.TxHash)
	require.Equal(t, uint32(1), provider.request.ClobPairID)
	require.Equal(t, "-1000000", resp.Data.Subaccount.PerpetualPositions[0].Quantums)
}

func TestHTTPHandlerRejectsReconcileOrderWithoutAPIKey(t *testing.T) {
	handler := NewHTTPHandler(NewService(ServiceConfig{}), HTTPConfig{
		EventAPIKey:       "event-secret",
		ReconcileProvider: &recordingReconcileProvider{},
	})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/reconcile/order", bytes.NewReader([]byte(`{}`))))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

type recordingReconcileProvider struct {
	request OrderReconcileRequest
	result  OrderReconcileResult
}

func (p *recordingReconcileProvider) ReconcileOrder(_ context.Context, request OrderReconcileRequest) (OrderReconcileResult, error) {
	p.request = request
	return p.result, nil
}
