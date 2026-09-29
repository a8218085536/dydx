package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXProxyWalletHTTPProviderCreatesAddress(t *testing.T) {
	var gotAuth string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"code":200,"msg":"succ","data":{"dydxAddress":"dydx1created"}}`))
	}))
	defer server.Close()
	provider := NewDYDXProxyWalletHTTPProvider(DYDXProxyWalletHTTPProviderConfig{
		URL:         server.URL,
		BearerToken: "secret-token",
	})

	address, err := provider.CreateDYDXAddress(context.Background(), AuthUser{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
	})

	if err != nil {
		t.Fatalf("CreateDYDXAddress() error = %v", err)
	}
	if address != "dydx1created" {
		t.Fatalf("address = %q, want dydx1created", address)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["userId"] != "user-1" || gotBody["necoAddress"] == "" {
		t.Fatalf("body = %+v", gotBody)
	}
}

func TestDYDXProxyWalletHTTPProviderReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":500,"msg":"keystore locked"}`))
	}))
	defer server.Close()
	provider := NewDYDXProxyWalletHTTPProvider(DYDXProxyWalletHTTPProviderConfig{URL: server.URL})

	_, err := provider.CreateDYDXAddress(context.Background(), AuthUser{UserID: "user-1"})

	if err == nil || err.Error() != "keystore locked" {
		t.Fatalf("CreateDYDXAddress() error = %v, want keystore locked", err)
	}
}
