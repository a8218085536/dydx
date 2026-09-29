package tyche

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	WalletBindingStatusPending = "pending"
	WalletBindingStatusActive  = "active"
)

var (
	ErrWalletBindingNotFound         = errors.New("wallet binding not found")
	ErrWalletBindingSignatureInvalid = errors.New("wallet binding signature invalid")
)

type WalletBinding struct {
	ID          string `json:"id" redis:"id"`
	UserID      string `json:"userId" redis:"userId"`
	NECOAddress string `json:"necoAddress" redis:"necoAddress"`
	DYDXAddress string `json:"dydxAddress" redis:"dydxAddress"`
	Nonce       string `json:"nonce" redis:"nonce"`
	Message     string `json:"message" redis:"message"`
	Status      string `json:"status" redis:"status"`
}

type WalletBindingCreate struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress"`
	DYDXAddress string `json:"dydxAddress"`
}

type WalletBindingVerify struct {
	BindingID     string `json:"bindingId"`
	NECOSignature string `json:"necoSignature"`
	DYDXSignature string `json:"dydxSignature"`
}

type WalletBindingStore interface {
	Save(context.Context, WalletBinding) error
	Get(context.Context, string) (WalletBinding, error)
	Activate(context.Context, string) (WalletBinding, error)
}

type WalletBindingOverview struct {
	UserID               string         `json:"userId"`
	HasActiveDYDXBinding bool           `json:"hasActiveDYDXBinding"`
	Binding              *WalletBinding `json:"binding,omitempty"`
}

type WalletBindingVerifier interface {
	VerifyNECO(context.Context, WalletBinding, string) error
	VerifyDYDX(context.Context, WalletBinding, string) error
}

type WalletBindingService struct {
	store    WalletBindingStore
	verifier WalletBindingVerifier
}

func NewWalletBindingService(store WalletBindingStore, verifier WalletBindingVerifier) *WalletBindingService {
	return &WalletBindingService{store: store, verifier: verifier}
}

func (s *WalletBindingService) Create(ctx context.Context, request WalletBindingCreate) (WalletBinding, error) {
	nonce := uuid.NewString()
	binding := WalletBinding{
		ID:          uuid.NewString(),
		UserID:      request.UserID,
		NECOAddress: request.NECOAddress,
		DYDXAddress: request.DYDXAddress,
		Nonce:       nonce,
		Status:      WalletBindingStatusPending,
	}
	binding.Message = BuildWalletBindingMessage(binding)

	if err := s.store.Save(ctx, binding); err != nil {
		return WalletBinding{}, err
	}
	return binding, nil
}

func (s *WalletBindingService) Verify(ctx context.Context, request WalletBindingVerify) (WalletBinding, error) {
	binding, err := s.store.Get(ctx, request.BindingID)
	if err != nil {
		return WalletBinding{}, err
	}
	if err := s.verifier.VerifyNECO(ctx, binding, request.NECOSignature); err != nil {
		return WalletBinding{}, err
	}
	if err := s.verifier.VerifyDYDX(ctx, binding, request.DYDXSignature); err != nil {
		return WalletBinding{}, err
	}
	return s.store.Activate(ctx, binding.ID)
}

func (s *WalletBindingService) HasActiveDYDXBinding(ctx context.Context, necoAddress string) (bool, error) {
	lookup, ok := s.store.(interface {
		GetByNECOAddress(context.Context, string) (WalletBinding, error)
	})
	if !ok {
		return false, nil
	}
	binding, err := lookup.GetByNECOAddress(ctx, necoAddress)
	if errors.Is(err, ErrWalletBindingNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return binding.Status == WalletBindingStatusActive, nil
}

func (s *WalletBindingService) GetByNECOAddress(ctx context.Context, necoAddress string) (WalletBinding, error) {
	lookup, ok := s.store.(interface {
		GetByNECOAddress(context.Context, string) (WalletBinding, error)
	})
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return lookup.GetByNECOAddress(ctx, necoAddress)
}

func (s *WalletBindingService) GetByDYDXAddress(ctx context.Context, dydxAddress string) (WalletBinding, error) {
	lookup, ok := s.store.(interface {
		GetByDYDXAddress(context.Context, string) (WalletBinding, error)
	})
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return lookup.GetByDYDXAddress(ctx, dydxAddress)
}

func (s *WalletBindingService) GetByUserID(ctx context.Context, userID string) (WalletBinding, error) {
	lookup, ok := s.store.(interface {
		GetByUserID(context.Context, string) (WalletBinding, error)
	})
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return lookup.GetByUserID(ctx, userID)
}

func (s *WalletBindingService) DYDXAccountForUser(ctx context.Context, userID string) (string, error) {
	binding, err := s.GetByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if binding.Status != WalletBindingStatusActive || binding.DYDXAddress == "" {
		return "", ErrWalletBindingNotFound
	}
	return binding.DYDXAddress, nil
}

func (s *WalletBindingService) Overview(ctx context.Context, userID string) (WalletBindingOverview, error) {
	overview := WalletBindingOverview{UserID: userID}
	lookup, ok := s.store.(interface {
		GetByUserID(context.Context, string) (WalletBinding, error)
	})
	if !ok {
		return overview, nil
	}
	binding, err := lookup.GetByUserID(ctx, userID)
	if errors.Is(err, ErrWalletBindingNotFound) {
		return overview, nil
	}
	if err != nil {
		return WalletBindingOverview{}, err
	}
	overview.HasActiveDYDXBinding = binding.Status == WalletBindingStatusActive
	overview.Binding = &binding
	return overview, nil
}

type DYDXAddressProvider interface {
	CreateDYDXAddress(context.Context, AuthUser) (string, error)
}

type WalletBindingProvisioner struct {
	store    WalletBindingStore
	provider DYDXAddressProvider
}

func NewWalletBindingProvisioner(store WalletBindingStore, provider DYDXAddressProvider) *WalletBindingProvisioner {
	return &WalletBindingProvisioner{store: store, provider: provider}
}

func (p *WalletBindingProvisioner) ProvisionDYDXBinding(ctx context.Context, user AuthUser) (WalletBinding, error) {
	if lookup, ok := p.store.(interface {
		GetByUserID(context.Context, string) (WalletBinding, error)
	}); ok {
		binding, err := lookup.GetByUserID(ctx, user.UserID)
		if err == nil && binding.Status == WalletBindingStatusActive && binding.DYDXAddress != "" {
			return binding, nil
		}
		if err != nil && !errors.Is(err, ErrWalletBindingNotFound) {
			return WalletBinding{}, err
		}
	}
	address, err := p.provider.CreateDYDXAddress(ctx, user)
	if err != nil {
		return WalletBinding{}, err
	}
	binding := WalletBinding{
		ID:          uuid.NewString(),
		UserID:      user.UserID,
		NECOAddress: user.NECOAddress,
		DYDXAddress: address,
		Status:      WalletBindingStatusActive,
	}
	if err := p.store.Save(ctx, binding); err != nil {
		return WalletBinding{}, err
	}
	return binding, nil
}

type StaticDYDXAddressProvider struct {
	AddressPrefix string
}

// StaticDYDXAddressProvider is only for narrow unit tests. Production App uses
// the dydxtrading wallet creation endpoint and never this provider.
func (p StaticDYDXAddressProvider) CreateDYDXAddress(ctx context.Context, user AuthUser) (string, error) {
	prefix := p.AddressPrefix
	if prefix == "" {
		prefix = "dydx"
	}
	return prefix + user.UserID, nil
}

func BuildWalletBindingMessage(binding WalletBinding) string {
	return fmt.Sprintf("Bind FlyDEX account\nuserId:%s\nnecoAddress:%s\ndydxAddress:%s\nnonce:%s",
		binding.UserID,
		binding.NECOAddress,
		binding.DYDXAddress,
		binding.Nonce,
	)
}

type MemoryWalletBindingStore struct {
	byID   map[string]WalletBinding
	byNECO map[string]string
	byDYDX map[string]string
	byUser map[string]string
}

func NewMemoryWalletBindingStore() *MemoryWalletBindingStore {
	return &MemoryWalletBindingStore{
		byID:   map[string]WalletBinding{},
		byNECO: map[string]string{},
		byDYDX: map[string]string{},
		byUser: map[string]string{},
	}
}

func (s *MemoryWalletBindingStore) Save(ctx context.Context, binding WalletBinding) error {
	s.byID[binding.ID] = binding
	if binding.Status == WalletBindingStatusActive {
		s.index(binding)
	}
	return nil
}

func (s *MemoryWalletBindingStore) index(binding WalletBinding) {
	if strings.TrimSpace(binding.NECOAddress) != "" {
		s.byNECO[strings.ToLower(binding.NECOAddress)] = binding.ID
	}
	if strings.TrimSpace(binding.DYDXAddress) != "" {
		s.byDYDX[strings.ToLower(binding.DYDXAddress)] = binding.ID
	}
	if strings.TrimSpace(binding.UserID) != "" {
		s.byUser[binding.UserID] = binding.ID
	}
}

func (s *MemoryWalletBindingStore) Get(ctx context.Context, id string) (WalletBinding, error) {
	binding, ok := s.byID[id]
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return binding, nil
}

func (s *MemoryWalletBindingStore) Activate(ctx context.Context, id string) (WalletBinding, error) {
	binding, ok := s.byID[id]
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	binding.Status = WalletBindingStatusActive
	s.byID[id] = binding
	s.index(binding)
	return binding, nil
}

func (s *MemoryWalletBindingStore) GetByNECOAddress(ctx context.Context, address string) (WalletBinding, error) {
	id, ok := s.byNECO[strings.ToLower(address)]
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return s.Get(ctx, id)
}

func (s *MemoryWalletBindingStore) GetByDYDXAddress(ctx context.Context, address string) (WalletBinding, error) {
	id, ok := s.byDYDX[strings.ToLower(address)]
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return s.Get(ctx, id)
}

func (s *MemoryWalletBindingStore) GetByUserID(ctx context.Context, userID string) (WalletBinding, error) {
	id, ok := s.byUser[userID]
	if !ok {
		return WalletBinding{}, ErrWalletBindingNotFound
	}
	return s.Get(ctx, id)
}
