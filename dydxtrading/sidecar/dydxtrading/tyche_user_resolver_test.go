package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTycheDYDXUserResolverReturnsActiveBindingUser(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/tyche/api/wallet-binding/get-by-dydx", r.URL.Path)
		gotKey = r.Header.Get("X-Tyche-Internal-Key")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"msg": "succ",
			"data": {
				"userId": "user-1",
				"dydxAddress": "dydx1user",
				"status": "active"
			}
		}`))
	}))
	defer server.Close()

	userID, ok, err := NewTycheDYDXUserResolver(TycheDYDXUserResolverConfig{
		URL:            server.URL,
		InternalAPIKey: "internal-secret",
		Client:         server.Client(),
	}).UserIDByDYDXAddress(context.Background(), "dydx1user")

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "internal-secret", gotKey)
	require.Equal(t, "user-1", userID)
}

func TestTycheDYDXUserResolverSkipsMissingBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code": 400, "msg": "wallet binding not found"}`))
	}))
	defer server.Close()

	userID, ok, err := NewTycheDYDXUserResolver(TycheDYDXUserResolverConfig{
		URL:    server.URL,
		Client: server.Client(),
	}).UserIDByDYDXAddress(context.Background(), "dydx1missing")

	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, userID)
}

func TestTycheDYDXUserResolverSkipsInactiveBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"code": 200,
			"data": {
				"userId": "user-1",
				"dydxAddress": "dydx1user",
				"status": "pending"
			}
		}`))
	}))
	defer server.Close()

	userID, ok, err := NewTycheDYDXUserResolver(TycheDYDXUserResolverConfig{
		URL:    server.URL,
		Client: server.Client(),
	}).UserIDByDYDXAddress(context.Background(), "dydx1user")

	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, userID)
}
