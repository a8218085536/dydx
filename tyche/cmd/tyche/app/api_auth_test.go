package app

import (
	"bytes"
	"context"
	"encoding/json"
	"ethgo/tyche"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
)

func TestAuthChallengeAPI(t *testing.T) {
	app := &App{auth: tyche.NewAuthService(tyche.NewMemoryAuthStore(), tyche.EVMWalletBindingVerifier{}, nil)}
	rec := postPublicJSON(t, app, "/tyche/api/auth/challenge", map[string]interface{}{
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte("Login to FlyDEX")) {
		t.Fatalf("body=%s, want login message", rec.Body.String())
	}
}

func TestAuthLoginAPI(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	auth := tyche.NewAuthService(tyche.NewMemoryAuthStore(), tyche.EVMWalletBindingVerifier{}, nil)
	challenge, err := auth.Challenge(context.Background(), tyche.AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	app := &App{auth: auth}

	rec := postPublicJSON(t, app, "/tyche/api/auth/login", map[string]interface{}{
		"necoAddress": address,
		"signature":   signEVMMessageForAppTest(t, key, challenge.Message),
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte("sessionToken")) {
		t.Fatalf("body=%s, want session token", rec.Body.String())
	}
}

func postPublicJSON(t *testing.T, app *App, path string, body map[string]interface{}) *httptest.ResponseRecorder {
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
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}
