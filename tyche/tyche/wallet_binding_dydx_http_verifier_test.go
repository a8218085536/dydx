package tyche

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDYDXSignatureHTTPVerifierAcceptsValidSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["address"] != "dydx1user" {
			t.Fatalf("address = %v", body["address"])
		}
		if body["message"] == "" {
			t.Fatal("message is empty")
		}
		if body["signature"] != "dydx-signature" {
			t.Fatalf("signature = %v", body["signature"])
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"valid":true},"msg":"succ"}`))
	}))
	defer server.Close()

	verifier := NewDYDXSignatureHTTPVerifier(DYDXSignatureHTTPVerifierConfig{URL: server.URL})
	err := verifier.VerifyDYDX(context.Background(), WalletBinding{
		DYDXAddress: "dydx1user",
		Message:     "Bind FlyDEX account",
	}, "dydx-signature")

	if err != nil {
		t.Fatalf("VerifyDYDX() error = %v", err)
	}
}

func TestDYDXSignatureHTTPVerifierRejectsInvalidSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"valid":false},"msg":"succ"}`))
	}))
	defer server.Close()

	verifier := NewDYDXSignatureHTTPVerifier(DYDXSignatureHTTPVerifierConfig{URL: server.URL})
	err := verifier.VerifyDYDX(context.Background(), WalletBinding{
		DYDXAddress: "dydx1user",
		Message:     "Bind FlyDEX account",
	}, "bad-signature")

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDYDXSignatureHTTPVerifierReturnsGatewayError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":500,"msg":"verifier unavailable"}`))
	}))
	defer server.Close()

	verifier := NewDYDXSignatureHTTPVerifier(DYDXSignatureHTTPVerifierConfig{URL: server.URL})
	err := verifier.VerifyDYDX(context.Background(), WalletBinding{
		DYDXAddress: "dydx1user",
		Message:     "Bind FlyDEX account",
	}, "dydx-signature")

	if err == nil {
		t.Fatal("expected error")
	}
}
