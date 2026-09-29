package app

import (
	"context"
	"encoding/json"
	"ethgo/tyche"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewWalletBindingVerifierUsesDYDXSignatureVerifyURL(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["address"] != "dydx1user" {
			t.Fatalf("address = %v", body["address"])
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"valid":true},"msg":"succ"}`))
	}))
	defer server.Close()

	conf := appTestConfig(tyche.ChainTypeDYDX)
	conf.DYDXSignatureVerifyURL = server.URL
	app := &App{conf: &Config{Tyche: conf}}

	verifier := app.newWalletBindingVerifier()
	err := verifier.VerifyDYDX(context.Background(), tyche.WalletBinding{
		DYDXAddress: "dydx1user",
		Message:     "Bind FlyDEX account",
	}, "signature")

	if err != nil {
		t.Fatalf("VerifyDYDX() error = %v", err)
	}
	if !called {
		t.Fatal("expected verifier upstream to be called")
	}
}
