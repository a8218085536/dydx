package app

import (
	"bytes"
	"net/http"
	"testing"

	"ethgo/tyche"
)

func TestDYDXModeRejectsLegacyEVMRoutes(t *testing.T) {
	app := &App{conf: &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)}}
	paths := []string{
		"/tyche/api/transact",
		"/tyche/api/call",
		"/tyche/api/wallet/create",
		"/tyche/api/wallet/sign",
		"/tyche/api/wallet/minter",
		"/tyche/api/wallet/GetGasPrice",
		"/rules/paused",
		"/rules/setAccount",
		"/rules/getAccount",
	}

	for _, path := range paths {
		rec := postJSON(t, app, path, map[string]interface{}{})

		assertResponseCode(t, rec, http.StatusBadRequest)
		if !bytes.Contains(rec.Body.Bytes(), []byte("only available in evm mode")) {
			t.Fatalf("%s body=%s, want evm-only failure", path, rec.Body.String())
		}
	}
}

func TestProtectedAPIRejectsMissingInternalAPIKeyInDYDXMode(t *testing.T) {
	conf := appTestConfig(tyche.ChainTypeDYDX)
	conf.InternalAPIKey = "12345678901234567890123456789012"
	app := &App{conf: &Config{Tyche: conf}, ledger: tyche.NewLedgerService(tyche.NewMemoryLedgerStore())}

	rec := postJSON(t, app, "/tyche/api/ledger/balance", map[string]interface{}{
		"userId": "user-1",
		"asset":  "DEDX",
	})

	assertResponseCode(t, rec, http.StatusUnauthorized)
}
