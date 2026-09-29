package tyche

import (
	"context"
	"testing"
)

func TestTradingReconciliationServiceFlagsManualReviewAndFillAlerts(t *testing.T) {
	settlements := NewMemoryTradingSettlementStore()
	if _, _, err := settlements.CreateIfAbsent(context.Background(), TradingSettlement{
		ID:            "settlement-1",
		UserID:        "user-1",
		CloseOrderID:  "close-order-1",
		SettlementUSD: "-1",
		Status:        TradingSettlementStatusManualReviewRequired,
		Reason:        "negative settlement after fees",
	}); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	service := NewTradingReconciliationService(settlements, nil, staticFillLookup{
		fills: []TradingFill{
			{EventID: "fill-1", UserID: "user-1", OrderID: "close-order-1", ReleasedMargin: "10", Fee: "1", RealizedPNL: "-2"},
			{EventID: "fill-1", UserID: "user-1", OrderID: "close-order-1", ReleasedMargin: "10", Fee: "1", RealizedPNL: "-2"},
		},
	})

	report, err := service.Report(context.Background(), TradingReconciliationRequest{
		UserID:       "user-1",
		SettlementID: "settlement-1",
	})

	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	wantCodes := map[string]bool{
		"manual_review_required": false,
		"invalid_settlement_usd": false,
		"duplicate_fill":         false,
		"negative_realized_pnl":  false,
	}
	for _, issue := range report.Issues {
		if _, ok := wantCodes[issue.Code]; ok {
			wantCodes[issue.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Fatalf("issue %s not found in report: %+v", code, report.Issues)
		}
	}
	hasCritical := false
	hasWarning := false
	for _, issue := range report.Issues {
		hasCritical = hasCritical || issue.Severity == TradingReconciliationSeverityCritical
		hasWarning = hasWarning || issue.Severity == TradingReconciliationSeverityWarning
	}
	if !hasCritical || !hasWarning || report.Summary.Critical == 0 {
		t.Fatalf("summary=%+v issues=%+v, want critical settlement with both critical and warning issues", report.Summary, report.Issues)
	}
}

func TestTradingReconciliationServiceFlagsCreditedNECODepositMissingLedgerRef(t *testing.T) {
	service := NewTradingReconciliationService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil)
	service.SetNECODepositSource(staticNECODepositSource{
		deposits: []NECODeposit{
			{EventID: "deposit-1", UserID: "user-1", Asset: "DEDX", Amount: "100", Status: NECODepositStatusCredited},
		},
	})

	report, err := service.Report(context.Background(), TradingReconciliationRequest{UserID: "user-1"})

	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	found := false
	for _, issue := range report.Issues {
		found = found || issue.Code == "neco_deposit_missing_ledger_ref"
	}
	if !found {
		t.Fatalf("issues=%+v, want missing ledger ref", report.Issues)
	}
	if report.Summary.CriticalIssues == 0 {
		t.Fatalf("summary=%+v, want critical issue count", report.Summary)
	}
}

type staticFillLookup struct {
	fills []TradingFill
}

func (s staticFillLookup) ListFillsByOrder(context.Context, string, string) ([]TradingFill, error) {
	return append([]TradingFill{}, s.fills...), nil
}

type staticNECODepositSource struct {
	deposits []NECODeposit
}

func (s staticNECODepositSource) ListDepositsByUser(context.Context, string) ([]NECODeposit, error) {
	return append([]NECODeposit{}, s.deposits...), nil
}
