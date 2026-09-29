package tyche

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestNECOLoginProvisionedDYDXWalletIsUsedForTrading(t *testing.T) {
	sidecar := NewMockDYDXSidecar(MockDYDXSidecarConfig{SubmitStatus: TradingOrderStatusSubmitted})
	sidecarServer := httptest.NewServer(sidecar.Handler())
	defer sidecarServer.Close()

	necoKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	necoAddress := crypto.PubkeyToAddress(necoKey.PublicKey).Hex()
	authStore := NewMemoryAuthStore()
	bindingStore := NewMemoryWalletBindingStore()
	bindings := NewWalletBindingService(bindingStore, nil)
	auth := NewAuthService(authStore, EVMWalletBindingVerifier{}, bindings)
	auth.SetDYDXBindingProvisioner(NewWalletBindingProvisioner(bindingStore, NewDYDXProxyWalletHTTPProvider(DYDXProxyWalletHTTPProviderConfig{
		URL: sidecarServer.URL + "/wallet/create",
	})))

	challenge, err := auth.Challenge(context.Background(), AuthChallengeRequest{NECOAddress: necoAddress})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	login, err := auth.Login(context.Background(), AuthLoginRequest{
		NECOAddress: necoAddress,
		Signature:   signEVMMessageForTest(t, necoKey, challenge.Message),
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.DYDXAddress == "" {
		t.Fatal("login DYDXAddress is empty")
	}

	conf := minimalConfig()
	conf.DYDXTradingURL = sidecarServer.URL + "/dydx/trading"
	conf.DYDXHeightURL = sidecarServer.URL + "/status"
	conf.DYDXChainID = "dydx-local-1"
	conf.DYDXGoodTilSeconds = 3600
	conf.DYDXMarkets = []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
	runner := NewDYDXRunner(conf)
	ledger := NewLedgerService(NewMemoryLedgerStore())
	creditAvailableBalance(t, ledger, login.UserID, "USDC", "100")
	trading := NewTradingService(NewMemoryTradingStore(), ledger, runner)
	trading.SetDYDXAccountResolver(bindings)

	order, err := trading.PlaceOrder(context.Background(), TradingPlaceOrderRequest{
		UserID:        login.UserID,
		ClientOrderID: "123",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDC",
		MarginAmount:  "25",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if order.DYDXAccount != login.DYDXAddress {
		t.Fatalf("order DYDXAccount = %q, want %q", order.DYDXAccount, login.DYDXAddress)
	}
	requests := sidecar.Requests()
	if len(requests) != 1 {
		t.Fatalf("sidecar requests = %d, want 1", len(requests))
	}
	if requests[0].Account != login.DYDXAddress {
		t.Fatalf("sidecar account = %q, want %q", requests[0].Account, login.DYDXAddress)
	}
	tx := decodeMockCosmosTx(t, requests[0])
	message := tx.Message.Value.(map[string]interface{})
	orderValue := message["order"].(map[string]interface{})
	orderID := orderValue["orderId"].(map[string]interface{})
	subaccount := orderID["subaccountId"].(map[string]interface{})
	if tx.Account != login.DYDXAddress || subaccount["owner"] != login.DYDXAddress {
		t.Fatalf("tx account=%q owner=%v, want %q", tx.Account, subaccount["owner"], login.DYDXAddress)
	}
}

func decodeMockCosmosTx(t *testing.T, request DYDXChainRequest) DYDXCosmosTxRequest {
	t.Helper()
	raw, err := json.Marshal(request.Params[DYDXCosmosTxParamKey])
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var tx DYDXCosmosTxRequest
	if err := json.Unmarshal(raw, &tx); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return tx
}
