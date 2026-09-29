package tyche

import (
	"context"
	"errors"
	"strings"
)

const (
	TradingReconciliationSeverityInfo     = "info"
	TradingReconciliationSeverityWarning  = "warning"
	TradingReconciliationSeverityCritical = "critical"

	TradingReconciliationStatusOK       = "ok"
	TradingReconciliationStatusWarning  = "warning"
	TradingReconciliationStatusCritical = "critical"
)

type TradingReconciliationRequest struct {
	UserID       string `json:"userId"`
	SettlementID string `json:"settlementId"`
	OrderID      string `json:"orderId"`
	CloseOrderID string `json:"closeOrderId"`
	Status       string `json:"status"`
}

type TradingReconciliationIssue struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	UserID       string `json:"userId,omitempty"`
	Asset        string `json:"asset,omitempty"`
	SettlementID string `json:"settlementId,omitempty"`
	OrderID      string `json:"orderId,omitempty"`
	EventID      string `json:"eventId,omitempty"`
}

type TradingSettlementReconciliation struct {
	Settlement TradingSettlement            `json:"settlement"`
	Status     string                       `json:"status"`
	Issues     []TradingReconciliationIssue `json:"issues"`
}

type TradingReconciliationSummary struct {
	Total          int `json:"total"`
	OK             int `json:"ok"`
	Warning        int `json:"warning"`
	Critical       int `json:"critical"`
	Issues         int `json:"issues"`
	WarningIssues  int `json:"warningIssues"`
	CriticalIssues int `json:"criticalIssues"`
}

type TradingReconciliationReport struct {
	UserID       string                            `json:"userId,omitempty"`
	SettlementID string                            `json:"settlementId,omitempty"`
	Items        []TradingSettlementReconciliation `json:"items"`
	Issues       []TradingReconciliationIssue      `json:"issues"`
	Summary      TradingReconciliationSummary      `json:"summary"`
}

type TradingReconciliationService struct {
	settlements TradingSettlementStore
	ledger      *LedgerService
	fills       TradingFillLookup
	deposits    TradingReconciliationNECODepositSource
}

type TradingReconciliationNECODepositSource interface {
	ListDepositsByUser(context.Context, string) ([]NECODeposit, error)
}

func NewTradingReconciliationService(settlements TradingSettlementStore, ledger *LedgerService, fills TradingFillLookup) *TradingReconciliationService {
	if typed, ok := fills.(*TradingSyncService); ok && typed == nil {
		fills = nil
	}
	return &TradingReconciliationService{
		settlements: settlements,
		ledger:      ledger,
		fills:       fills,
	}
}

func (s *TradingReconciliationService) SetNECODepositSource(source TradingReconciliationNECODepositSource) {
	if s != nil {
		s.deposits = source
	}
}

func (s *TradingReconciliationService) Report(ctx context.Context, request TradingReconciliationRequest) (TradingReconciliationReport, error) {
	if s == nil || s.settlements == nil {
		return TradingReconciliationReport{}, ErrTradingSettlementNotFound
	}
	settlements, err := s.loadSettlements(ctx, request)
	if err != nil {
		return TradingReconciliationReport{}, err
	}
	report := TradingReconciliationReport{
		UserID:       request.UserID,
		SettlementID: request.SettlementID,
		Items:        make([]TradingSettlementReconciliation, 0, len(settlements)),
		Issues:       make([]TradingReconciliationIssue, 0),
	}
	for _, settlement := range settlements {
		item := s.reconcileSettlement(ctx, settlement)
		report.Items = append(report.Items, item)
		report.appendIssues(item.Issues)
		report.Summary.Total++
		switch item.Status {
		case TradingReconciliationStatusCritical:
			report.Summary.Critical++
		case TradingReconciliationStatusWarning:
			report.Summary.Warning++
		default:
			report.Summary.OK++
		}
	}
	depositIssues := s.reconcileNECODeposits(ctx, request.UserID)
	report.appendIssues(depositIssues)
	return report, nil
}

func (s *TradingReconciliationService) loadSettlements(ctx context.Context, request TradingReconciliationRequest) ([]TradingSettlement, error) {
	closeOrderID := firstNonEmptyTradingString(request.OrderID, request.CloseOrderID)
	if request.SettlementID != "" {
		settlement, ok, err := s.settlements.GetByID(ctx, request.SettlementID)
		if err != nil {
			return nil, err
		}
		if !ok || (request.UserID != "" && settlement.UserID != request.UserID) {
			return nil, ErrTradingSettlementNotFound
		}
		return []TradingSettlement{settlement}, nil
	}
	if closeOrderID != "" && request.UserID != "" {
		settlement, ok, err := s.settlements.GetByCloseOrderID(ctx, request.UserID, closeOrderID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrTradingSettlementNotFound
		}
		return []TradingSettlement{settlement}, nil
	}
	if request.UserID != "" {
		settlements, err := s.settlements.ListByUser(ctx, request.UserID)
		if err != nil {
			return nil, err
		}
		if request.Status == "" {
			return settlements, nil
		}
		return filterSettlementsByStatus(settlements, request.Status), nil
	}
	status := request.Status
	if status == "" {
		status = TradingSettlementStatusManualReviewRequired
	}
	return s.settlements.ListByStatus(ctx, status)
}

func (s *TradingReconciliationService) reconcileSettlement(ctx context.Context, settlement TradingSettlement) TradingSettlementReconciliation {
	issues := make([]TradingReconciliationIssue, 0)
	add := func(code, severity, message string) {
		issues = append(issues, TradingReconciliationIssue{
			Code:         code,
			Severity:     severity,
			Message:      message,
			UserID:       settlement.UserID,
			Asset:        settlement.SettlementAsset,
			SettlementID: settlement.ID,
			OrderID:      settlement.CloseOrderID,
		})
	}

	switch settlement.Status {
	case TradingSettlementStatusManualReviewRequired:
		add("manual_review_required", TradingReconciliationSeverityWarning, nonEmptyMessage(settlement.Reason, "settlement requires manual review"))
	case TradingSettlementStatusFailed:
		add("settlement_failed", TradingReconciliationSeverityCritical, nonEmptyMessage(settlement.Reason, "settlement failed"))
	case TradingSettlementStatusRejected:
		add("settlement_rejected", TradingReconciliationSeverityInfo, nonEmptyMessage(settlement.Reason, "settlement rejected"))
	}
	if settlement.SystemReserved && settlement.DYDXReservedQuantums == "" &&
		settlement.Status != TradingSettlementStatusManualReviewRequired &&
		settlement.Status != TradingSettlementStatusRejected {
		add("missing_dydx_reserved_quantums", TradingReconciliationSeverityWarning, "settlement is marked system reserved but has no dYdX reserved quantums")
	}
	if settlement.SettlementUSD != "" {
		usd, ok := parseDecimalRat(settlement.SettlementUSD)
		if !ok || usd.Sign() <= 0 || looksLikeAtomicUSD(settlement.SettlementUSD) {
			add("invalid_settlement_usd", TradingReconciliationSeverityCritical, "settlement USD is invalid or non-positive")
		}
	}
	if settlementAmountRequired(settlement.Status) {
		amount, ok := parseLedgerAmount(settlement.SettlementAmount)
		if !ok || amount.Sign() <= 0 {
			add("invalid_settlement_amount", TradingReconciliationSeverityCritical, "settlement amount is invalid or non-positive")
		}
	}
	if settlementWithdrawalRequired(settlement.Status) {
		if settlement.LedgerWithdrawalID == "" {
			add("ledger_withdrawal_missing", TradingReconciliationSeverityCritical, "settlement payout status requires a ledger withdrawal")
		} else if s.ledger == nil {
			add("ledger_not_configured", TradingReconciliationSeverityCritical, "ledger service is not configured")
		} else {
			withdrawal, err := s.ledger.GetWithdrawal(ctx, settlement.LedgerWithdrawalID)
			if errors.Is(err, ErrLedgerWithdrawalNotFound) {
				add("ledger_withdrawal_missing", TradingReconciliationSeverityCritical, "ledger withdrawal referenced by settlement was not found")
			} else if err != nil {
				add("ledger_withdrawal_lookup_failed", TradingReconciliationSeverityCritical, err.Error())
			} else {
				issues = append(issues, reconcileSettlementWithdrawal(settlement, withdrawal)...)
			}
		}
	}
	issues = append(issues, s.reconcileSettlementFills(ctx, settlement)...)
	return TradingSettlementReconciliation{
		Settlement: settlement,
		Status:     highestReconciliationStatus(issues),
		Issues:     issues,
	}
}

func reconcileSettlementWithdrawal(settlement TradingSettlement, withdrawal LedgerWithdrawal) []TradingReconciliationIssue {
	issues := make([]TradingReconciliationIssue, 0)
	add := func(code, severity, message string) {
		issues = append(issues, TradingReconciliationIssue{
			Code:         code,
			Severity:     severity,
			Message:      message,
			UserID:       settlement.UserID,
			Asset:        settlement.SettlementAsset,
			SettlementID: settlement.ID,
			OrderID:      settlement.CloseOrderID,
		})
	}
	if withdrawal.UserID != settlement.UserID ||
		normalizeAsset(withdrawal.Asset) != normalizeAsset(settlement.SettlementAsset) ||
		withdrawal.Amount != settlement.SettlementAmount ||
		!strings.EqualFold(withdrawal.ToAddress, settlement.NECOAddress) {
		add("ledger_withdrawal_mismatch", TradingReconciliationSeverityCritical, "ledger withdrawal does not match settlement user, asset, amount, or address")
	}
	if settlement.Status == TradingSettlementStatusNECOPayoutPaid && withdrawal.Status != LedgerWithdrawalStatusPaid {
		add("ledger_withdrawal_not_paid", TradingReconciliationSeverityCritical, "settlement is paid but ledger withdrawal is not paid")
	}
	if withdrawal.Status == LedgerWithdrawalStatusPaid && settlement.NECOTxHash == "" {
		add("missing_neco_tx_hash", TradingReconciliationSeverityWarning, "ledger withdrawal is paid but settlement has no NECO tx hash")
	}
	return issues
}

func (s *TradingReconciliationService) reconcileSettlementFills(ctx context.Context, settlement TradingSettlement) []TradingReconciliationIssue {
	if s.fills == nil || settlement.UserID == "" || settlement.CloseOrderID == "" {
		return nil
	}
	fills, err := s.fills.ListFillsByOrder(ctx, settlement.UserID, settlement.CloseOrderID)
	if err != nil {
		return []TradingReconciliationIssue{{
			Code:         "fill_lookup_failed",
			Severity:     TradingReconciliationSeverityCritical,
			Message:      err.Error(),
			UserID:       settlement.UserID,
			Asset:        settlement.SettlementAsset,
			SettlementID: settlement.ID,
			OrderID:      settlement.CloseOrderID,
		}}
	}
	issues := make([]TradingReconciliationIssue, 0)
	seenEvents := map[string]bool{}
	for _, fill := range fills {
		add := func(code, severity, message string) {
			issues = append(issues, TradingReconciliationIssue{
				Code:         code,
				Severity:     severity,
				Message:      message,
				UserID:       settlement.UserID,
				Asset:        settlement.SettlementAsset,
				SettlementID: settlement.ID,
				OrderID:      settlement.CloseOrderID,
				EventID:      fill.EventID,
			})
		}
		if fill.EventID == "" {
			add("fill_event_id_missing", TradingReconciliationSeverityWarning, "fill event id is empty")
		} else if seenEvents[fill.EventID] {
			add("duplicate_fill", TradingReconciliationSeverityCritical, "duplicate fill event found for settlement order")
		}
		seenEvents[fill.EventID] = true
		released, releasedOK := parseDecimalRat(defaultLedgerAmount(fill.ReleasedMargin))
		fee, feeOK := parseDecimalRat(defaultLedgerAmount(fill.Fee))
		pnl, pnlOK := parseDecimalRat(defaultLedgerAmount(fill.RealizedPNL))
		if !releasedOK || released.Sign() < 0 || !feeOK || fee.Sign() < 0 || !pnlOK {
			add("invalid_fill_accounting", TradingReconciliationSeverityCritical, "fill accounting fields are invalid")
			continue
		}
		if pnl.Sign() < 0 {
			add("negative_realized_pnl", TradingReconciliationSeverityWarning, "fill realized PnL is negative")
		}
	}
	return issues
}

func (s *TradingReconciliationService) reconcileNECODeposits(ctx context.Context, userID string) []TradingReconciliationIssue {
	if s.deposits == nil || userID == "" {
		return nil
	}
	deposits, err := s.deposits.ListDepositsByUser(ctx, userID)
	if err != nil {
		return []TradingReconciliationIssue{{
			Code:     "neco_deposit_lookup_failed",
			Severity: TradingReconciliationSeverityCritical,
			Message:  err.Error(),
			UserID:   userID,
		}}
	}
	issues := make([]TradingReconciliationIssue, 0)
	for _, deposit := range deposits {
		add := func(code, severity, message string) {
			issues = append(issues, TradingReconciliationIssue{
				Code:     code,
				Severity: severity,
				Message:  message,
				UserID:   deposit.UserID,
				Asset:    deposit.Asset,
				EventID:  deposit.EventID,
			})
		}
		amount, ok := parseLedgerAmount(deposit.Amount)
		if !ok || amount.Sign() <= 0 {
			add("neco_deposit_invalid_amount", TradingReconciliationSeverityCritical, "NECO deposit amount is invalid or non-positive")
		}
		switch deposit.Status {
		case NECODepositStatusPending:
			add("neco_deposit_pending", TradingReconciliationSeverityWarning, "NECO deposit is still pending user claim")
		case NECODepositStatusCredited:
			if s.ledger == nil {
				add("ledger_not_configured", TradingReconciliationSeverityCritical, "ledger service is not configured")
				continue
			}
			hasRef, err := s.ledger.HasReference(ctx, deposit.EventID)
			if err != nil {
				add("ledger_ref_lookup_failed", TradingReconciliationSeverityCritical, err.Error())
				continue
			}
			if !hasRef {
				add("neco_deposit_missing_ledger_ref", TradingReconciliationSeverityCritical, "credited NECO deposit has no matching ledger reference")
			}
		}
	}
	return issues
}

func settlementAmountRequired(status string) bool {
	switch status {
	case TradingSettlementStatusDEDXCredited, TradingSettlementStatusNECOPayoutBroadcasted, TradingSettlementStatusNECOPayoutPaid:
		return true
	default:
		return false
	}
}

func settlementWithdrawalRequired(status string) bool {
	switch status {
	case TradingSettlementStatusDEDXCredited, TradingSettlementStatusNECOPayoutBroadcasted, TradingSettlementStatusNECOPayoutPaid:
		return true
	default:
		return false
	}
}

func highestReconciliationStatus(issues []TradingReconciliationIssue) string {
	status := TradingReconciliationStatusOK
	for _, issue := range issues {
		if issue.Severity == TradingReconciliationSeverityCritical {
			return TradingReconciliationStatusCritical
		}
		if issue.Severity == TradingReconciliationSeverityWarning {
			status = TradingReconciliationStatusWarning
		}
	}
	return status
}

func filterSettlementsByStatus(settlements []TradingSettlement, status string) []TradingSettlement {
	if status == "" {
		return settlements
	}
	filtered := make([]TradingSettlement, 0, len(settlements))
	for _, settlement := range settlements {
		if settlement.Status == status {
			filtered = append(filtered, settlement)
		}
	}
	return filtered
}

func nonEmptyMessage(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func (r *TradingReconciliationReport) appendIssues(issues []TradingReconciliationIssue) {
	r.Issues = append(r.Issues, issues...)
	for _, issue := range issues {
		r.Summary.Issues++
		switch issue.Severity {
		case TradingReconciliationSeverityCritical:
			r.Summary.CriticalIssues++
		case TradingReconciliationSeverityWarning:
			r.Summary.WarningIssues++
		}
	}
}
