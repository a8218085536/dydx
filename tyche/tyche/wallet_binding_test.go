package tyche

import (
	"context"
	"errors"
	"testing"

	"github.com/garyburd/redigo/redis"
)

func TestWalletBindingServiceCreatesPendingBinding(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	service := NewWalletBindingService(store, acceptAllBindingVerifier{})

	binding, err := service.Create(context.Background(), WalletBindingCreate{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
	})

	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if binding.Status != WalletBindingStatusPending {
		t.Fatalf("Status = %q, want %q", binding.Status, WalletBindingStatusPending)
	}
	if binding.Nonce == "" {
		t.Fatal("Nonce is empty")
	}
	saved, err := store.Get(context.Background(), binding.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if saved.UserID != "user-1" {
		t.Fatalf("saved UserID = %q", saved.UserID)
	}
}

func TestWalletBindingServiceActivatesAfterBothSignaturesVerify(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	service := NewWalletBindingService(store, acceptAllBindingVerifier{})
	binding, err := service.Create(context.Background(), WalletBindingCreate{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	active, err := service.Verify(context.Background(), WalletBindingVerify{
		BindingID:     binding.ID,
		NECOSignature: "neco-signature",
		DYDXSignature: "dydx-signature",
	})

	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if active.Status != WalletBindingStatusActive {
		t.Fatalf("Status = %q, want %q", active.Status, WalletBindingStatusActive)
	}
}

func TestWalletBindingServiceRejectsBadSignature(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	service := NewWalletBindingService(store, rejectingBindingVerifier{})
	binding, err := service.Create(context.Background(), WalletBindingCreate{
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = service.Verify(context.Background(), WalletBindingVerify{
		BindingID:     binding.ID,
		NECOSignature: "bad",
		DYDXSignature: "bad",
	})

	if !errors.Is(err, ErrWalletBindingSignatureInvalid) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrWalletBindingSignatureInvalid)
	}
}

func TestWalletBindingProvisionerReusesExistingActiveBinding(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	existing := WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1existing",
		Status:      WalletBindingStatusActive,
	}
	if err := store.Save(context.Background(), existing); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	provider := &countingDYDXAddressProvider{address: "dydx1new"}
	provisioner := NewWalletBindingProvisioner(store, provider)

	binding, err := provisioner.ProvisionDYDXBinding(context.Background(), AuthUser{
		UserID:      "user-1",
		NECOAddress: existing.NECOAddress,
	})

	if err != nil {
		t.Fatalf("ProvisionDYDXBinding() error = %v", err)
	}
	if binding.DYDXAddress != "dydx1existing" {
		t.Fatalf("DYDXAddress = %q, want existing", binding.DYDXAddress)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestWalletBindingCreatePendingDoesNotMaskExistingActiveBinding(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	service := NewWalletBindingService(store, acceptAllBindingVerifier{})
	active := WalletBinding{
		ID:          "binding-active",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1active",
		Status:      WalletBindingStatusActive,
	}
	if err := store.Save(context.Background(), active); err != nil {
		t.Fatalf("Save() active error = %v", err)
	}

	pending, err := service.Create(context.Background(), WalletBindingCreate{
		UserID:      "user-1",
		NECOAddress: active.NECOAddress,
		DYDXAddress: "dydx1pending",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if saved, err := store.Get(context.Background(), pending.ID); err != nil || saved.Status != WalletBindingStatusPending {
		t.Fatalf("pending saved=%+v err=%v, want pending retrievable by id", saved, err)
	}
	byUser, err := store.GetByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	byNECO, err := store.GetByNECOAddress(context.Background(), active.NECOAddress)
	if err != nil {
		t.Fatalf("GetByNECOAddress() error = %v", err)
	}
	if byUser.ID != active.ID || byNECO.ID != active.ID {
		t.Fatalf("byUser=%+v byNECO=%+v, want existing active binding", byUser, byNECO)
	}
}

func TestWalletBindingVerifyPromotesPendingBindingToActiveLookup(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	service := NewWalletBindingService(store, acceptAllBindingVerifier{})
	active := WalletBinding{
		ID:          "binding-active",
		UserID:      "user-1",
		NECOAddress: "0x0000000000000000000000000000000000000001",
		DYDXAddress: "dydx1active",
		Status:      WalletBindingStatusActive,
	}
	if err := store.Save(context.Background(), active); err != nil {
		t.Fatalf("Save() active error = %v", err)
	}
	pending, err := service.Create(context.Background(), WalletBindingCreate{
		UserID:      "user-1",
		NECOAddress: active.NECOAddress,
		DYDXAddress: "dydx1pending",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	verified, err := service.Verify(context.Background(), WalletBindingVerify{
		BindingID:     pending.ID,
		NECOSignature: "neco-signature",
		DYDXSignature: "dydx-signature",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	byUser, err := store.GetByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	if verified.ID != pending.ID || byUser.ID != pending.ID || byUser.Status != WalletBindingStatusActive {
		t.Fatalf("verified=%+v byUser=%+v, want pending promoted", verified, byUser)
	}
}

func TestMemoryWalletBindingStoreAddressLookupsAreCaseInsensitive(t *testing.T) {
	store := NewMemoryWalletBindingStore()
	binding := WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: "0x00000000000000000000000000000000000000AA",
		DYDXAddress: "DYDX1USER",
		Status:      WalletBindingStatusActive,
	}
	if err := store.Save(context.Background(), binding); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	byNECO, err := store.GetByNECOAddress(context.Background(), "0x00000000000000000000000000000000000000aa")
	if err != nil {
		t.Fatalf("GetByNECOAddress() error = %v", err)
	}
	byDYDX, err := store.GetByDYDXAddress(context.Background(), "dydx1user")
	if err != nil {
		t.Fatalf("GetByDYDXAddress() error = %v", err)
	}
	if byNECO.ID != binding.ID || byDYDX.ID != binding.ID {
		t.Fatalf("byNECO=%+v byDYDX=%+v, want binding %s", byNECO, byDYDX, binding.ID)
	}
}

func TestWalletBindingScansRedisFields(t *testing.T) {
	values := []interface{}{
		[]byte("id"), []byte("binding-1"),
		[]byte("userId"), []byte("user-1"),
		[]byte("necoAddress"), []byte("0x0000000000000000000000000000000000000001"),
		[]byte("dydxAddress"), []byte("dydx1user"),
		[]byte("nonce"), []byte("nonce-1"),
		[]byte("message"), []byte("message-1"),
		[]byte("status"), []byte(WalletBindingStatusActive),
	}
	var binding WalletBinding

	if err := redis.ScanStruct(values, &binding); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if binding.UserID != "user-1" || binding.DYDXAddress != "dydx1user" || binding.Status != WalletBindingStatusActive {
		t.Fatalf("binding = %+v, want redis fields populated", binding)
	}
}

type acceptAllBindingVerifier struct{}

func (acceptAllBindingVerifier) VerifyNECO(context.Context, WalletBinding, string) error {
	return nil
}

func (acceptAllBindingVerifier) VerifyDYDX(context.Context, WalletBinding, string) error {
	return nil
}

type rejectingBindingVerifier struct{}

func (rejectingBindingVerifier) VerifyNECO(context.Context, WalletBinding, string) error {
	return ErrWalletBindingSignatureInvalid
}

func (rejectingBindingVerifier) VerifyDYDX(context.Context, WalletBinding, string) error {
	return nil
}

type countingDYDXAddressProvider struct {
	address string
	calls   int
}

func (p *countingDYDXAddressProvider) CreateDYDXAddress(context.Context, AuthUser) (string, error) {
	p.calls++
	return p.address, nil
}
