//go:build cgo

package dydxtrading

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/dydxprotocol/v4-chain/protocol/app/config"
	"github.com/stretchr/testify/require"
)

func TestEncryptedFileProxyWalletStoreCreatesStableEncryptedWallet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy-wallets.json")
	store, err := NewEncryptedFileProxyWalletStore(EncryptedFileProxyWalletStoreConfig{
		Path:          path,
		Secret:        strings.Repeat("s", 32),
		AddressPrefix: appconfig.Bech32PrefixAccAddr,
	})
	require.NoError(t, err)

	first, err := store.CreateOrGetProxyWallet(context.Background(), ProxyWalletCreateRequest{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
	})
	require.NoError(t, err)
	second, err := store.CreateOrGetProxyWallet(context.Background(), ProxyWalletCreateRequest{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
	})
	require.NoError(t, err)
	privateKey, err := store.PrivateKeyForAddress(context.Background(), first.DYDXAddress)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	require.Equal(t, first.DYDXAddress, second.DYDXAddress)
	require.True(t, strings.HasPrefix(first.DYDXAddress, appconfig.Bech32PrefixAccAddr+"1"))
	require.NotContains(t, string(raw), string(privateKey.Bytes()))
	require.NotContains(t, string(raw), first.DYDXAddress+" private")
}

func TestHTTPHandlerCreatesProxyWallet(t *testing.T) {
	store, err := NewEncryptedFileProxyWalletStore(EncryptedFileProxyWalletStoreConfig{
		Path:   filepath.Join(t.TempDir(), "proxy-wallets.json"),
		Secret: strings.Repeat("s", 32),
	})
	require.NoError(t, err)
	body, err := json.Marshal(ProxyWalletCreateRequest{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/wallet/create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token-with-at-least-32-chars")
	recorder := httptest.NewRecorder()
	handler := NewHTTPHandler(NewService(ServiceConfig{}), HTTPConfig{
		BearerToken:       "secret-token-with-at-least-32-chars",
		WalletProvisioner: store,
	})

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int         `json:"code"`
		Msg  string      `json:"msg"`
		Data ProxyWallet `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, "user-1", resp.Data.UserID)
	require.NotEmpty(t, resp.Data.DYDXAddress)
}
