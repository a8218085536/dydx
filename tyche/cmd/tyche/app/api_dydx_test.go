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

func TestDYDXSubmitRejectsEVMMode(t *testing.T) {
	app := &App{base: tyche.New(nil, appTestConfig(tyche.ChainTypeEVM))}
	rec := postDYDXSubmit(t, app, map[string]interface{}{
		"id":     "order-1",
		"action": tyche.DYDXActionPlaceOrder,
	})

	assertResponseCode(t, rec, http.StatusBadRequest)
	if !bytes.Contains(rec.Body.Bytes(), []byte("this API is only available in dydx mode")) {
		t.Fatalf("body=%s, want dydx mode error", rec.Body.String())
	}
}

func TestDYDXSubmitRoutesToDYDXRunner(t *testing.T) {
	app := &App{base: fakeDYDXSubmitter{}}

	rec := postDYDXSubmit(t, app, map[string]interface{}{
		"id":     "order-1",
		"action": tyche.DYDXActionPlaceOrder,
		"params": map[string]interface{}{
			"market": "ETH-USD",
			"side":   "buy",
		},
	})

	assertResponseCode(t, rec, http.StatusOK)
}

func postDYDXSubmit(t *testing.T, app *App, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	app.Router(engine)

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tyche/api/dydx/submit", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func assertResponseCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal() error = %v; body=%s", err, rec.Body.String())
	}
	if body.Code != want {
		t.Fatalf("response code = %d, want %d; body=%s", body.Code, want, rec.Body.String())
	}
}

func appTestConfig(chainType string) *tyche.Config {
	conf := &tyche.Config{
		ChainType:                 chainType,
		Account:                   "0x0000000000000000000000000000000000000001",
		PrivateKey:                "0000000000000000000000000000000000000000000000000000000000000001",
		EstimatorJS:               "./js/gasprice.js",
		ExpectedMD5Password:       "password-md5",
		AllowedIP:                 "127.0.0.1",
		ErrorNumberOfConcurrent:   1,
		FailedNumberOfConcurrent:  1,
		PendingNumberOfConcurrent: 1,
		SentNumberOfConcurrent:    1,
		SucceedNumberOfConcurrent: 1,
	}
	if chainType == tyche.ChainTypeDYDX {
		conf.Account = ""
		conf.PrivateKey = ""
		conf.EstimatorJS = ""
		conf.InternalAPIKey = "12345678901234567890123456789012"
		conf.DYDXTradingURL = "http://127.0.0.1:8090/dydx/trading"
		conf.DYDXWalletCreateURL = "http://127.0.0.1:8090/wallet/create"
		conf.DYDXChainID = "dydx-local-1"
		conf.DYDXHeightURL = "http://127.0.0.1:26657/status"
		conf.DYDXGoodTilSeconds = 3600
		conf.DYDXMarkets = []tyche.DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
	}
	return conf
}

type fakeDYDXSubmitter struct{}

func (fakeDYDXSubmitter) Init(_ context.Context) error {
	return nil
}

func (fakeDYDXSubmitter) Run(_ context.Context) error {
	return nil
}

func (fakeDYDXSubmitter) Submit(_ context.Context, _ tyche.DYDXIntent) error {
	return nil
}
