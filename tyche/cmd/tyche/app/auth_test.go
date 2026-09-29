package app

import (
	"bytes"
	"encoding/json"
	"ethgo/tyche"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProtectedLedgerEndpointRejectsMissingAPIKey(t *testing.T) {
	app := &App{
		conf:   &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		ledger: tyche.NewLedgerService(tyche.NewMemoryLedgerStore()),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	rec := postJSONWithAPIKey(t, app, "/tyche/api/ledger/deposit/mock", "", map[string]interface{}{
		"userId": "user-1",
		"asset":  "USDC",
		"amount": "100",
		"refId":  "mock-deposit-1",
	})

	assertResponseCode(t, rec, http.StatusUnauthorized)
}

func TestProtectedLedgerEndpointAcceptsAPIKey(t *testing.T) {
	app := &App{
		conf:   &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		ledger: tyche.NewLedgerService(tyche.NewMemoryLedgerStore()),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	rec := postJSONWithAPIKey(t, app, "/tyche/api/ledger/deposit/mock", "secret", map[string]interface{}{
		"userId": "user-1",
		"asset":  "USDC",
		"amount": "100",
		"refId":  "mock-deposit-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
}

func postJSONWithAPIKey(t *testing.T, app *App, path, apiKey string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	app.Router(engine)

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-Tyche-Internal-Key", apiKey)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}
