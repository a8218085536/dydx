package tyche

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestNECOLoginBindingDYDXOrderAndEventSyncUpdatesLedger(t *testing.T) {
	ctx := context.Background()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	necoAddress := crypto.PubkeyToAddress(key.PublicKey).Hex()

	authStore := NewMemoryAuthStore()
	bindingStore := NewMemoryWalletBindingStore()
	auth := NewAuthService(authStore, EVMWalletBindingVerifier{}, NewWalletBindingService(bindingStore, nil))
	auth.SetDYDXBindingProvisioner(&recordingDYDXBindingProvisioner{
		address: "dydx1trader",
		store:   bindingStore,
	})
	challenge, err := auth.Challenge(ctx, AuthChallengeRequest{NECOAddress: necoAddress})
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	login, err := auth.Login(ctx, AuthLoginRequest{
		NECOAddress: necoAddress,
		Signature:   signEVMMessageForTest(t, key, challenge.Message),
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !login.HasDYDXBinding || login.DYDXAddress != "dydx1trader" {
		t.Fatalf("login binding = has %v address %q", login.HasDYDXBinding, login.DYDXAddress)
	}

	ledgerStore := NewMemoryLedgerStore()
	ledger := NewLedgerService(ledgerStore)
	creditAvailableBalance(t, ledger, login.UserID, "USDT", "1000")
	tradingStore := NewMemoryTradingStore()
	trading := NewTradingService(tradingStore, ledger, &recordingTradingSubmitter{})
	order, err := trading.PlaceOrder(ctx, TradingPlaceOrderRequest{
		UserID:        login.UserID,
		ClientOrderID: "33",
		Market:        "ETH-USD",
		Side:          "buy",
		OrderType:     "limit",
		Price:         "2500",
		Size:          "1",
		MarginAsset:   "USDT",
		MarginAmount:  "100",
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}

	tradingSync := NewTradingSyncService(NewMemoryTradingSyncStore(), tradingStore, ledger)
	cursor := NewMemoryTradingSyncCursorStore(TradingSyncCursor{})
	worker := NewTradingSyncWorker(&recordingTradingEventSource{
		nextFillCursor:   "fill-cursor-1",
		nextStatusCursor: "status-cursor-1",
		nextPosCursor:    "position-cursor-1",
		fills: []TradingFillSyncRequest{{
			EventID:        "height-10-tx-0-event-0:taker",
			UserID:         login.UserID,
			OrderID:        "dydx1trader:0:33:7",
			Market:         "ETH-USD",
			Asset:          "USDT",
			ReleasedMargin: "0",
			Fee:            "2",
			RealizedPNL:    "0",
			OrderStatus:    TradingOrderStatusPartialFilled,
		}},
		statuses: []TradingOrderStatusSyncRequest{{
			EventID: "height-10-tx-0-event-1",
			UserID:  login.UserID,
			OrderID: "dydx1trader:0:33:7",
			Status:  TradingOrderStatusFilled,
			Reason:  "fully_filled",
		}},
		positions: []TradingPositionSyncRequest{{
			EventID:     "height-10-tx-0-event-2:2",
			UserID:      login.UserID,
			Market:      "ETH-USD",
			Size:        "123",
			MarginAsset: "USDT",
			Margin:      "0",
		}},
	}, tradingSync, cursor)

	if err := worker.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	storedOrder, ok, err := tradingStore.GetOrderByID(ctx, login.UserID, order.ID)
	if err != nil || !ok {
		t.Fatalf("GetOrderByID() order=%+v ok=%v err=%v", storedOrder, ok, err)
	}
	if storedOrder.Status != TradingOrderStatusFilled {
		t.Fatalf("order status = %q, want %q", storedOrder.Status, TradingOrderStatusFilled)
	}
	balance, err := ledger.GetBalance(ctx, login.UserID, "USDT")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "898" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want available=898 frozen=0", balance)
	}
	savedCursor, err := cursor.GetTradingSyncCursor(ctx)
	if err != nil {
		t.Fatalf("GetTradingSyncCursor() error = %v", err)
	}
	if savedCursor.FillsCursor != "fill-cursor-1" || savedCursor.OrderStatusCursor != "status-cursor-1" || savedCursor.PositionsCursor != "position-cursor-1" {
		t.Fatalf("cursor = %+v", savedCursor)
	}
}
