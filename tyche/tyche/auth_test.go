package tyche

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestAuthServiceCreatesNECOLoginChallenge(t *testing.T) {
	service := NewAuthService(NewMemoryAuthStore(), EVMWalletBindingVerifier{}, nil)

	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{
		NECOAddress: "0x0000000000000000000000000000000000000001",
	})

	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	if challenge.Nonce == "" {
		t.Fatal("Nonce is empty")
	}
	if challenge.Message == "" {
		t.Fatal("Message is empty")
	}
}

func TestAuthServiceLoginCreatesUserAndReportsNoDYDXBinding(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	service := NewAuthService(NewMemoryAuthStore(), EVMWalletBindingVerifier{}, nil)

	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	signature := signEVMMessageForTest(t, key, challenge.Message)

	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Signature:   signature,
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.UserID == "" {
		t.Fatal("UserID is empty")
	}
	if login.SessionToken == "" {
		t.Fatal("SessionToken is empty")
	}
	if login.HasDYDXBinding {
		t.Fatal("HasDYDXBinding = true, want false")
	}
}

func TestAuthServiceLoginAcceptsEarlierOutstandingChallenge(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	service := NewAuthService(NewMemoryAuthStore(), EVMWalletBindingVerifier{}, nil)

	firstChallenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge(first) error = %v", err)
	}
	if _, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address}); err != nil {
		t.Fatalf("Challenge(second) error = %v", err)
	}

	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Nonce:       firstChallenge.Nonce,
		Signature:   signEVMMessageForTest(t, key, firstChallenge.Message),
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.UserID == "" {
		t.Fatal("UserID is empty")
	}
}

func TestAuthServiceLoginReportsExistingDYDXBinding(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	bindings := &memoryAuthBindingLookup{activeByNECO: map[string]bool{address: true}}
	service := NewAuthService(NewMemoryAuthStore(), EVMWalletBindingVerifier{}, bindings)

	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	signature := signEVMMessageForTest(t, key, challenge.Message)

	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Signature:   signature,
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !login.HasDYDXBinding {
		t.Fatal("HasDYDXBinding = false, want true")
	}
}

func TestAuthServiceLoginReportsActiveWalletBindingServiceState(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	bindingStore := NewMemoryWalletBindingStore()
	if err := bindingStore.Save(context.Background(), WalletBinding{
		ID:          "binding-1",
		UserID:      "user-1",
		NECOAddress: address,
		DYDXAddress: "dydx1user",
		Status:      WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bindings := NewWalletBindingService(bindingStore, nil)
	service := NewAuthService(NewMemoryAuthStore(), EVMWalletBindingVerifier{}, bindings)

	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	signature := signEVMMessageForTest(t, key, challenge.Message)

	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Signature:   signature,
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !login.HasDYDXBinding {
		t.Fatal("HasDYDXBinding = false, want true")
	}
	if login.DYDXAddress != "dydx1user" {
		t.Fatalf("DYDXAddress = %q, want dydx1user", login.DYDXAddress)
	}
}

func TestAuthServiceLoginUsesExistingBindingUserIDWhenAuthUserWasRecreated(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	authStore := NewMemoryAuthStore()
	bindingStore := NewMemoryWalletBindingStore()
	if err := bindingStore.Save(context.Background(), WalletBinding{
		ID:          "binding-1",
		UserID:      "bound-user",
		NECOAddress: address,
		DYDXAddress: "dydx1bound",
		Status:      WalletBindingStatusActive,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	service := NewAuthService(authStore, EVMWalletBindingVerifier{}, NewWalletBindingService(bindingStore, nil))

	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Signature:   signEVMMessageForTest(t, key, challenge.Message),
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.UserID != "bound-user" {
		t.Fatalf("UserID = %q, want existing binding user", login.UserID)
	}
	if authStore.sessions[login.SessionToken] != "bound-user" {
		t.Fatalf("session user = %q, want existing binding user", authStore.sessions[login.SessionToken])
	}
	user, ok, err := authStore.GetUserByNECOAddress(context.Background(), address)
	if err != nil || !ok {
		t.Fatalf("GetUserByNECOAddress() user=%+v ok=%v err=%v", user, ok, err)
	}
	if user.UserID != "bound-user" {
		t.Fatalf("auth user = %+v, want repaired binding user", user)
	}
}

func TestAuthServiceLoginClaimsPendingNECODeposits(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	authStore := NewMemoryAuthStore()
	ledgerStore := NewMemoryLedgerStore()
	deposits := NewNECODepositService(NewMemoryNECODepositStore(), NewLedgerService(ledgerStore), authStore, NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	if _, err := deposits.ProcessTransfer(context.Background(), NECOTokenTransfer{
		ChainID:       "9001",
		TokenAddress:  "0x0000000000000000000000000000000000000ded",
		From:          address,
		To:            "0x0000000000000000000000000000000000000abc",
		Amount:        "50",
		TxHash:        "0xtx1",
		LogIndex:      0,
		Confirmations: 12,
	}); err != nil {
		t.Fatalf("ProcessTransfer() error = %v", err)
	}
	service := NewAuthService(authStore, EVMWalletBindingVerifier{}, nil)
	service.SetDepositClaimer(deposits)
	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}

	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Signature:   signEVMMessageForTest(t, key, challenge.Message),
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), login.UserID, "DEDX")
	if balance.Available != "50" {
		t.Fatalf("available = %q, want 50", balance.Available)
	}
}

func TestAuthServiceLoginAutoCreatesDYDXBindingWhenMissing(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	authStore := NewMemoryAuthStore()
	bindingStore := NewMemoryWalletBindingStore()
	bindings := NewWalletBindingService(bindingStore, nil)
	provisioner := &recordingDYDXBindingProvisioner{address: "dydx1created", store: bindingStore}
	service := NewAuthService(authStore, EVMWalletBindingVerifier{}, bindings)
	service.SetDYDXBindingProvisioner(provisioner)
	challenge, err := service.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: address})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}

	login, err := service.Login(context.Background(), AuthLoginRequest{
		NECOAddress: address,
		Signature:   signEVMMessageForTest(t, key, challenge.Message),
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !login.HasDYDXBinding {
		t.Fatal("HasDYDXBinding = false, want true")
	}
	if login.DYDXAddress != "dydx1created" {
		t.Fatalf("DYDXAddress = %q, want dydx1created", login.DYDXAddress)
	}
	if provisioner.userID != login.UserID || provisioner.necoAddress != address {
		t.Fatalf("provisioner userID=%q neco=%q", provisioner.userID, provisioner.necoAddress)
	}
	binding, err := bindingStore.GetByNECOAddress(context.Background(), address)
	if err != nil {
		t.Fatalf("GetByNECOAddress() error = %v", err)
	}
	if binding.Status != WalletBindingStatusActive || binding.DYDXAddress != "dydx1created" {
		t.Fatalf("binding = %+v, want active dydx1created", binding)
	}
}

type memoryAuthBindingLookup struct {
	activeByNECO map[string]bool
}

func (l *memoryAuthBindingLookup) HasActiveDYDXBinding(ctx context.Context, necoAddress string) (bool, error) {
	return l.activeByNECO[necoAddress], nil
}

type recordingDYDXBindingProvisioner struct {
	address     string
	store       *MemoryWalletBindingStore
	userID      string
	necoAddress string
}

func (p *recordingDYDXBindingProvisioner) ProvisionDYDXBinding(ctx context.Context, user AuthUser) (WalletBinding, error) {
	p.userID = user.UserID
	p.necoAddress = user.NECOAddress
	binding := WalletBinding{
		ID:          "binding-created",
		UserID:      user.UserID,
		NECOAddress: user.NECOAddress,
		DYDXAddress: p.address,
		Status:      WalletBindingStatusActive,
	}
	if p.store != nil {
		if err := p.store.Save(ctx, binding); err != nil {
			return WalletBinding{}, err
		}
	}
	return binding, nil
}
