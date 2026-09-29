package tyche

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrAuthChallengeNotFound = errors.New("auth challenge not found")
	ErrAuthInvalidSignature  = errors.New("auth invalid signature")
)

type AuthChallengeRequest struct {
	NECOAddress string `json:"necoAddress"`
}

type AuthChallenge struct {
	NECOAddress string `json:"necoAddress" redis:"necoAddress"`
	Nonce       string `json:"nonce" redis:"nonce"`
	Message     string `json:"message" redis:"message"`
}

type AuthLoginRequest struct {
	NECOAddress string `json:"necoAddress"`
	Nonce       string `json:"nonce,omitempty"`
	Signature   string `json:"signature"`
}

type AuthLoginResponse struct {
	UserID         string `json:"userId"`
	SessionToken   string `json:"sessionToken"`
	NECOAddress    string `json:"necoAddress"`
	HasDYDXBinding bool   `json:"hasDydxBinding"`
	DYDXAddress    string `json:"dydxAddress,omitempty"`
}

type AuthUser struct {
	UserID      string `json:"userId" redis:"userId"`
	NECOAddress string `json:"necoAddress" redis:"necoAddress"`
}

type AuthStore interface {
	SaveChallenge(context.Context, AuthChallenge) error
	GetChallenge(context.Context, string) (AuthChallenge, error)
	GetOrCreateUser(context.Context, string) (AuthUser, error)
	SaveSession(context.Context, string, string) error
}

type AuthUserSaver interface {
	SaveUser(context.Context, AuthUser) error
}

type AuthChallengeNonceStore interface {
	GetChallengeByNonce(context.Context, string, string) (AuthChallenge, error)
}

type AuthBindingLookup interface {
	HasActiveDYDXBinding(context.Context, string) (bool, error)
}

type AuthDepositClaimer interface {
	ClaimPending(context.Context, string, string) ([]NECODeposit, error)
}

type AuthDYDXBindingProvisioner interface {
	ProvisionDYDXBinding(context.Context, AuthUser) (WalletBinding, error)
}

type AuthService struct {
	store                  AuthStore
	verifier               EVMWalletBindingVerifier
	bindings               AuthBindingLookup
	depositClaimer         AuthDepositClaimer
	dydxBindingProvisioner AuthDYDXBindingProvisioner
}

func NewAuthService(store AuthStore, verifier EVMWalletBindingVerifier, bindings AuthBindingLookup) *AuthService {
	return &AuthService{store: store, verifier: verifier, bindings: bindings}
}

func (s *AuthService) SetDepositClaimer(claimer AuthDepositClaimer) {
	s.depositClaimer = claimer
}

func (s *AuthService) SetDYDXBindingProvisioner(provisioner AuthDYDXBindingProvisioner) {
	s.dydxBindingProvisioner = provisioner
}

func (s *AuthService) Challenge(ctx context.Context, request AuthChallengeRequest) (AuthChallenge, error) {
	challenge := AuthChallenge{
		NECOAddress: request.NECOAddress,
		Nonce:       uuid.NewString(),
	}
	challenge.Message = BuildAuthLoginMessage(challenge)
	if err := s.store.SaveChallenge(ctx, challenge); err != nil {
		return AuthChallenge{}, err
	}
	return challenge, nil
}

func (s *AuthService) Login(ctx context.Context, request AuthLoginRequest) (AuthLoginResponse, error) {
	challenge, err := s.loginChallenge(ctx, request.NECOAddress, request.Nonce)
	if err != nil {
		return AuthLoginResponse{}, err
	}
	recovered, err := RecoverEVMMessageAddress(challenge.Message, request.Signature)
	if err != nil {
		return AuthLoginResponse{}, err
	}
	if !strings.EqualFold(recovered.Hex(), request.NECOAddress) {
		return AuthLoginResponse{}, ErrAuthInvalidSignature
	}

	user, err := s.store.GetOrCreateUser(ctx, request.NECOAddress)
	if err != nil {
		return AuthLoginResponse{}, err
	}

	hasBinding := false
	dydxAddress := ""
	canonicalUser := user
	if s.bindings != nil {
		hasBinding, err = s.bindings.HasActiveDYDXBinding(ctx, request.NECOAddress)
		if err != nil {
			return AuthLoginResponse{}, err
		}
		if hasBinding {
			if lookup, ok := s.bindings.(interface {
				GetByNECOAddress(context.Context, string) (WalletBinding, error)
			}); ok {
				binding, err := lookup.GetByNECOAddress(ctx, request.NECOAddress)
				if err != nil {
					return AuthLoginResponse{}, err
				}
				if binding.Status == WalletBindingStatusActive {
					dydxAddress = binding.DYDXAddress
					if strings.TrimSpace(binding.UserID) != "" {
						canonicalUser.UserID = binding.UserID
					}
					if strings.TrimSpace(binding.NECOAddress) != "" {
						canonicalUser.NECOAddress = binding.NECOAddress
					}
				}
			}
		}
	}
	if !hasBinding && s.dydxBindingProvisioner != nil {
		binding, err := s.dydxBindingProvisioner.ProvisionDYDXBinding(ctx, user)
		if err != nil {
			return AuthLoginResponse{}, err
		}
		hasBinding = binding.Status == WalletBindingStatusActive
		dydxAddress = binding.DYDXAddress
		if strings.TrimSpace(binding.UserID) != "" {
			canonicalUser.UserID = binding.UserID
		}
		if strings.TrimSpace(binding.NECOAddress) != "" {
			canonicalUser.NECOAddress = binding.NECOAddress
		}
	}
	if saver, ok := s.store.(AuthUserSaver); ok {
		if err := saver.SaveUser(ctx, canonicalUser); err != nil {
			return AuthLoginResponse{}, err
		}
	}

	sessionToken := uuid.NewString()
	if err := s.store.SaveSession(ctx, sessionToken, canonicalUser.UserID); err != nil {
		return AuthLoginResponse{}, err
	}
	if s.depositClaimer != nil {
		if _, err := s.depositClaimer.ClaimPending(ctx, canonicalUser.UserID, canonicalUser.NECOAddress); err != nil {
			return AuthLoginResponse{}, err
		}
	}
	return AuthLoginResponse{
		UserID:         canonicalUser.UserID,
		SessionToken:   sessionToken,
		NECOAddress:    request.NECOAddress,
		HasDYDXBinding: hasBinding,
		DYDXAddress:    dydxAddress,
	}, nil
}

func (s *AuthService) loginChallenge(ctx context.Context, necoAddress, nonce string) (AuthChallenge, error) {
	if nonce != "" {
		if store, ok := s.store.(AuthChallengeNonceStore); ok {
			return store.GetChallengeByNonce(ctx, necoAddress, nonce)
		}
		challenge, err := s.store.GetChallenge(ctx, necoAddress)
		if err != nil {
			return AuthChallenge{}, err
		}
		if challenge.Nonce != nonce {
			return AuthChallenge{}, ErrAuthChallengeNotFound
		}
		return challenge, nil
	}
	return s.store.GetChallenge(ctx, necoAddress)
}

func BuildAuthLoginMessage(challenge AuthChallenge) string {
	return fmt.Sprintf("Login to FlyDEX\nnecoAddress:%s\nnonce:%s", challenge.NECOAddress, challenge.Nonce)
}

type MemoryAuthStore struct {
	challenges map[string]AuthChallenge
	byNonce    map[string]AuthChallenge
	users      map[string]AuthUser
	sessions   map[string]string
}

func NewMemoryAuthStore() *MemoryAuthStore {
	return &MemoryAuthStore{
		challenges: map[string]AuthChallenge{},
		byNonce:    map[string]AuthChallenge{},
		users:      map[string]AuthUser{},
		sessions:   map[string]string{},
	}
}

func (s *MemoryAuthStore) SaveChallenge(ctx context.Context, challenge AuthChallenge) error {
	key := strings.ToLower(challenge.NECOAddress)
	s.challenges[key] = challenge
	s.byNonce[authChallengeNonceKey(challenge.NECOAddress, challenge.Nonce)] = challenge
	return nil
}

func (s *MemoryAuthStore) GetChallenge(ctx context.Context, necoAddress string) (AuthChallenge, error) {
	challenge, ok := s.challenges[strings.ToLower(necoAddress)]
	if !ok {
		return AuthChallenge{}, ErrAuthChallengeNotFound
	}
	return challenge, nil
}

func (s *MemoryAuthStore) GetChallengeByNonce(ctx context.Context, necoAddress, nonce string) (AuthChallenge, error) {
	challenge, ok := s.byNonce[authChallengeNonceKey(necoAddress, nonce)]
	if !ok {
		return AuthChallenge{}, ErrAuthChallengeNotFound
	}
	return challenge, nil
}

func (s *MemoryAuthStore) GetOrCreateUser(ctx context.Context, necoAddress string) (AuthUser, error) {
	key := strings.ToLower(necoAddress)
	if user, ok := s.users[key]; ok {
		return user, nil
	}
	user := AuthUser{UserID: uuid.NewString(), NECOAddress: necoAddress}
	s.users[key] = user
	return user, nil
}

func (s *MemoryAuthStore) SaveUser(ctx context.Context, user AuthUser) error {
	s.users[strings.ToLower(user.NECOAddress)] = user
	return nil
}

func authChallengeNonceKey(necoAddress, nonce string) string {
	return strings.ToLower(necoAddress) + ":" + nonce
}

func (s *MemoryAuthStore) GetUserByNECOAddress(ctx context.Context, necoAddress string) (AuthUser, bool, error) {
	user, ok := s.users[strings.ToLower(necoAddress)]
	return user, ok, nil
}

func (s *MemoryAuthStore) SaveSession(ctx context.Context, token, userID string) error {
	s.sessions[token] = userID
	return nil
}
