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

func TestWalletBindingCreateReturnsMessage(t *testing.T) {
	app := &App{walletBindings: tyche.NewWalletBindingService(newAppMemoryBindingStore(), appAcceptBindingVerifier{})}

	rec := postJSON(t, app, "/tyche/api/wallet-binding/create", map[string]interface{}{
		"userId":      "user-1",
		"necoAddress": "0x0000000000000000000000000000000000000001",
		"dydxAddress": "dydx1user",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte("Bind FlyDEX account")) {
		t.Fatalf("body=%s, want binding message", rec.Body.String())
	}
}

func TestWalletBindingVerifyActivatesBinding(t *testing.T) {
	store := newAppMemoryBindingStore()
	service := tyche.NewWalletBindingService(store, appAcceptBindingVerifier{})
	binding, err := service.Create(context.Background(), tyche.WalletBindingCreate{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	app := &App{walletBindings: service}
	rec := postJSON(t, app, "/tyche/api/wallet-binding/verify", map[string]interface{}{
		"bindingId":     binding.ID,
		"necoSignature": "neco-signature",
		"dydxSignature": "dydx-signature",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.WalletBindingStatusActive)) {
		t.Fatalf("body=%s, want active status", rec.Body.String())
	}
}

func TestWalletBindingGetAPI(t *testing.T) {
	store := tyche.NewMemoryWalletBindingStore()
	service := tyche.NewWalletBindingService(store, appAcceptBindingVerifier{})
	if err := store.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := &App{walletBindings: service}

	rec := postJSON(t, app, "/tyche/api/wallet-binding/get", map[string]interface{}{
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"dydxAddress":"dydx1user"`)) {
		t.Fatalf("body=%s, want dydx address", rec.Body.String())
	}
}

func TestWalletBindingGetAPIAcceptsUserID(t *testing.T) {
	store := tyche.NewMemoryWalletBindingStore()
	service := tyche.NewWalletBindingService(store, appAcceptBindingVerifier{})
	if err := store.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := &App{walletBindings: service}

	rec := postJSON(t, app, "/tyche/api/wallet-binding/get", map[string]interface{}{
		"userId": "user-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"dydxAddress":"dydx1user"`)) {
		t.Fatalf("body=%s, want dydx address", rec.Body.String())
	}
}

func TestWalletBindingGetByDYDXAPI(t *testing.T) {
	store := tyche.NewMemoryWalletBindingStore()
	service := tyche.NewWalletBindingService(store, appAcceptBindingVerifier{})
	if err := store.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := &App{walletBindings: service}

	rec := postJSON(t, app, "/tyche/api/wallet-binding/get-by-dydx", map[string]interface{}{
		"dydxAddress": "dydx1user",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"userId":"user-1"`)) {
		t.Fatalf("body=%s, want user id", rec.Body.String())
	}
}

func TestWalletBindingOverviewAPI(t *testing.T) {
	store := tyche.NewMemoryWalletBindingStore()
	service := tyche.NewWalletBindingService(store, appAcceptBindingVerifier{})
	if err := store.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := &App{walletBindings: service}

	rec := postJSON(t, app, "/tyche/api/wallet-binding/overview", map[string]interface{}{
		"userId": "user-1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"hasActiveDYDXBinding":true`)) {
		t.Fatalf("body=%s, want active binding", rec.Body.String())
	}
}

func TestWalletBindingOverviewAPIAcceptsNECOAddress(t *testing.T) {
	store := tyche.NewMemoryWalletBindingStore()
	service := tyche.NewWalletBindingService(store, appAcceptBindingVerifier{})
	if err := store.Save(context.Background(), tyche.WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
		Status:      tyche.WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := &App{walletBindings: service}

	rec := postJSON(t, app, "/tyche/api/wallet-binding/overview", map[string]interface{}{
		"necoAddress": "0x0000000000000000000000000000000000000001",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"userId":"user-1"`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte(`"hasActiveDYDXBinding":true`)) {
		t.Fatalf("body=%s, want active binding overview", rec.Body.String())
	}
}

func postJSON(t *testing.T, app *App, path string, body map[string]interface{}) *httptest.ResponseRecorder {
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

type appMemoryBindingStore struct {
	saved tyche.WalletBinding
}

func newAppMemoryBindingStore() *appMemoryBindingStore {
	return &appMemoryBindingStore{}
}

func (s *appMemoryBindingStore) Save(ctx context.Context, binding tyche.WalletBinding) error {
	s.saved = binding
	return nil
}

func (s *appMemoryBindingStore) Get(ctx context.Context, id string) (tyche.WalletBinding, error) {
	if s.saved.ID != id {
		return tyche.WalletBinding{}, tyche.ErrWalletBindingNotFound
	}
	return s.saved, nil
}

func (s *appMemoryBindingStore) Activate(ctx context.Context, id string) (tyche.WalletBinding, error) {
	if s.saved.ID != id {
		return tyche.WalletBinding{}, tyche.ErrWalletBindingNotFound
	}
	s.saved.Status = tyche.WalletBindingStatusActive
	return s.saved, nil
}

type appAcceptBindingVerifier struct{}

func (appAcceptBindingVerifier) VerifyNECO(context.Context, tyche.WalletBinding, string) error {
	return nil
}

func (appAcceptBindingVerifier) VerifyDYDX(context.Context, tyche.WalletBinding, string) error {
	return nil
}
