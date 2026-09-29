package tyche

import (
	"context"
	"testing"
)

func TestReconciliationServiceReportsLedgerAndPositionMismatch(t *testing.T) {
	source := &memoryReconciliationSource{
		ledger: []LedgerBalance{
			{UserID: "user-1", Asset: "USDC", Available: "100", Frozen: "0"},
		},
		positions: []TradingPosition{
			{UserID: "user-1", Market: "ETH-USD", MarginAsset: "USDC", Margin: "25"},
		},
	}
	service := NewReconciliationService(source)

	report, err := service.RunOnce(context.Background())

	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(report.Issues))
	}
	if report.Issues[0].Type != ReconciliationIssueLedgerPositionMismatch {
		t.Fatalf("issue = %+v, want ledger position mismatch", report.Issues[0])
	}
}

func TestReconciliationServiceReportsActiveOrderWithoutDYDXPosition(t *testing.T) {
	source := &memoryReconciliationSource{
		ledger: []LedgerBalance{
			{UserID: "user-1", Asset: "USDC", Available: "75", Frozen: "0"},
		},
		orders: []TradingOrder{
			{ID: "order-1", UserID: "user-1", Market: "ETH-USD", MarginAsset: "USDC", Status: TradingOrderStatusSubmitted},
		},
	}
	service := NewReconciliationService(source)

	report, err := service.RunOnce(context.Background())

	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(report.Issues))
	}
	if report.Issues[0].Type != ReconciliationIssueMissingDYDXPosition {
		t.Fatalf("issue = %+v, want missing dydx position", report.Issues[0])
	}
}

func TestReconciliationServiceReportsDYDXSyncFailure(t *testing.T) {
	source := &memoryReconciliationSource{
		failures: []TradingOrderStatusEvent{
			{EventID: "status-1", UserID: "user-1", OrderID: "order-1", Status: TradingOrderStatusRejected, Reason: "mock rejected"},
		},
	}
	service := NewReconciliationService(source)

	report, err := service.RunOnce(context.Background())

	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(report.Issues))
	}
	if report.Issues[0].Type != ReconciliationIssueDYDXSyncFailure {
		t.Fatalf("issue = %+v, want dydx sync failure", report.Issues[0])
	}
}

func TestReconciliationServiceDoesNotReportHealthyActiveOrderWithPosition(t *testing.T) {
	source := &memoryReconciliationSource{
		orders: []TradingOrder{
			{ID: "order-1", UserID: "user-1", Market: "ETH-USD", MarginAsset: "USDC", Status: TradingOrderStatusPartialFilled},
		},
		positions: []TradingPosition{
			{UserID: "user-1", Market: "ETH-USD", MarginAsset: "USDC", Margin: "25"},
		},
	}
	service := NewReconciliationService(source)

	report, err := service.RunOnce(context.Background())

	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("issues = %+v, want none", report.Issues)
	}
}

type memoryReconciliationSource struct {
	ledger    []LedgerBalance
	positions []TradingPosition
	orders    []TradingOrder
	failures  []TradingOrderStatusEvent
}

func (s *memoryReconciliationSource) LedgerBalances(context.Context) ([]LedgerBalance, error) {
	return s.ledger, nil
}

func (s *memoryReconciliationSource) TradingPositions(context.Context) ([]TradingPosition, error) {
	return s.positions, nil
}

func (s *memoryReconciliationSource) TradingOrders(context.Context) ([]TradingOrder, error) {
	return s.orders, nil
}

func (s *memoryReconciliationSource) TradingSyncFailures(context.Context) ([]TradingOrderStatusEvent, error) {
	return s.failures, nil
}
