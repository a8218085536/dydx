package app

import (
	"bytes"
	"context"
	"encoding/json"
	"ethgo/tyche"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDYDXBroadcastRejectsUnknownAction(t *testing.T) {
	app := &App{chainGateway: tyche.NewDYDXChainGateway(nil)}
	rec := postDYDXBroadcast(t, app, map[string]interface{}{
		"id":      "broadcast-1",
		"action":  "stake",
		"account": "dydx1sender",
	})

	assertResponseCode(t, rec, http.StatusBadRequest)
}

func TestDYDXBroadcastReturnsUnconfiguredClientError(t *testing.T) {
	app := &App{chainGateway: tyche.NewDYDXChainGateway(nil)}
	rec := postDYDXBroadcast(t, app, map[string]interface{}{
		"id":      "broadcast-1",
		"action":  tyche.DYDXActionPlaceOrder,
		"account": "dydx1sender",
		"params": map[string]interface{}{
			"market": "ETH-USD",
		},
	})

	assertResponseCode(t, rec, http.StatusInternalServerError)
	if !bytes.Contains(rec.Body.Bytes(), []byte("dydx chain client is not configured")) {
		t.Fatalf("body=%s, want chain client error", rec.Body.String())
	}
}

func TestDYDXBroadcastReturnsTxHash(t *testing.T) {
	app := &App{chainGateway: tyche.NewDYDXChainGateway(fakeChainClient{})}
	rec := postDYDXBroadcast(t, app, map[string]interface{}{
		"id":      "broadcast-1",
		"action":  tyche.DYDXActionPlaceOrder,
		"account": "dydx1sender",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte("chain-tx-1")) {
		t.Fatalf("body=%s, want tx hash", rec.Body.String())
	}
}

func TestDYDXBroadcastRejectsWhenTradingSidecarIsConfigured(t *testing.T) {
	conf := appTestConfig(tyche.ChainTypeDYDX)
	app := &App{conf: &Config{Tyche: conf}}
	app.chainGateway = tyche.NewDYDXChainGateway(app.newDYDXChainClient())

	rec := postDYDXBroadcast(t, app, map[string]interface{}{
		"id":      "broadcast-1",
		"action":  tyche.DYDXActionPlaceOrder,
		"account": "dydx1sender",
	})

	assertResponseCode(t, rec, http.StatusBadRequest)
	if !bytes.Contains(rec.Body.Bytes(), []byte("legacy dydx broadcast is disabled")) {
		t.Fatalf("body=%s, want legacy broadcast disabled", rec.Body.String())
	}
}

func postDYDXBroadcast(t *testing.T, app *App, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	app.Router(engine)

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/dydx/broadcast", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if app.conf != nil && app.conf.Tyche != nil && app.conf.Tyche.InternalAPIKey != "" {
		req.Header.Set("X-Tyche-Internal-Key", app.conf.Tyche.InternalAPIKey)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

type fakeChainClient struct{}

func (fakeChainClient) Broadcast(_ context.Context, _ tyche.DYDXChainRequest) (string, error) {
	return "chain-tx-1", nil
}
