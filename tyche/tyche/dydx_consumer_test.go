package tyche

import (
	"context"
	"errors"
	"testing"
)

func TestDYDXProcessorMarksIntentSucceededAfterBroadcast(t *testing.T) {
	var markedID string
	processor := NewDYDXProcessor(
		DYDXBroadcasterFunc(func(ctx context.Context, intent DYDXIntent) (string, error) {
			if intent.ID != "order-1" {
				t.Fatalf("broadcast intent ID = %q, want order-1", intent.ID)
			}
			return "tx-hash-1", nil
		}),
		DYDXStatusStoreFunc(func(id, status, txHash, reason string) error {
			markedID = id
			if status != DYDXStatusSucceed {
				t.Fatalf("status = %q, want %q", status, DYDXStatusSucceed)
			}
			if txHash != "tx-hash-1" {
				t.Fatalf("txHash = %q, want tx-hash-1", txHash)
			}
			if reason != "" {
				t.Fatalf("reason = %q, want empty", reason)
			}
			return nil
		}),
	)

	err := processor.Process(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
	})

	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if markedID != "order-1" {
		t.Fatalf("markedID = %q, want order-1", markedID)
	}
}

func TestDYDXProcessorMarksIntentFailedAfterBroadcastError(t *testing.T) {
	want := errors.New("broadcast failed")
	processor := NewDYDXProcessor(
		DYDXBroadcasterFunc(func(ctx context.Context, intent DYDXIntent) (string, error) {
			return "", want
		}),
		DYDXStatusStoreFunc(func(id, status, txHash, reason string) error {
			if status != DYDXStatusFailed {
				t.Fatalf("status = %q, want %q", status, DYDXStatusFailed)
			}
			if txHash != "" {
				t.Fatalf("txHash = %q, want empty", txHash)
			}
			if reason != want.Error() {
				t.Fatalf("reason = %q, want %q", reason, want.Error())
			}
			return nil
		}),
	)

	err := processor.Process(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
	})

	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
}

func TestDYDXProcessorReturnsStatusStoreError(t *testing.T) {
	want := errors.New("redis down")
	processor := NewDYDXProcessor(
		DYDXBroadcasterFunc(func(ctx context.Context, intent DYDXIntent) (string, error) {
			return "tx-hash-1", nil
		}),
		DYDXStatusStoreFunc(func(id, status, txHash, reason string) error {
			return want
		}),
	)

	err := processor.Process(context.Background(), DYDXIntent{
		ID:     "order-1",
		Action: DYDXActionPlaceOrder,
	})

	if !errors.Is(err, want) {
		t.Fatalf("Process() error = %v, want %v", err, want)
	}
}
