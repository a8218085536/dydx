package tyche

import "testing"

func TestDYDXIntentFromMessageData(t *testing.T) {
	intent, err := DYDXIntentFromMessageData(map[string]string{
		"id":     "order-1",
		"action": DYDXActionPlaceOrder,
		"params": `{"market":"ETH-USD","side":"buy"}`,
	})

	if err != nil {
		t.Fatalf("DYDXIntentFromMessageData() error = %v", err)
	}
	if intent.ID != "order-1" {
		t.Fatalf("ID = %q", intent.ID)
	}
	if intent.Params["market"] != "ETH-USD" {
		t.Fatalf("market = %v", intent.Params["market"])
	}
}

func TestDYDXIntentFromMessageDataRejectsBadParams(t *testing.T) {
	_, err := DYDXIntentFromMessageData(map[string]string{
		"id":     "order-1",
		"action": DYDXActionPlaceOrder,
		"params": `{bad-json`,
	})

	if err == nil {
		t.Fatal("expected error")
	}
}
