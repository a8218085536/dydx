package tyche

import "context"

const (
	ReconciliationIssueLedgerPositionMismatch = "ledger_position_mismatch"
	ReconciliationIssueMissingDYDXPosition    = "missing_dydx_position"
	ReconciliationIssueDYDXSyncFailure        = "dydx_sync_failure"
)

type ReconciliationIssue struct {
	Type   string `json:"type"`
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
	Detail string `json:"detail"`
}

type ReconciliationReport struct {
	Issues []ReconciliationIssue `json:"issues"`
}

type ReconciliationSource interface {
	LedgerBalances(context.Context) ([]LedgerBalance, error)
	TradingPositions(context.Context) ([]TradingPosition, error)
}

type ReconciliationTradingOrderSource interface {
	TradingOrders(context.Context) ([]TradingOrder, error)
}

type ReconciliationTradingFailureSource interface {
	TradingSyncFailures(context.Context) ([]TradingOrderStatusEvent, error)
}

type ReconciliationService struct {
	source ReconciliationSource
}

func NewReconciliationService(source ReconciliationSource) *ReconciliationService {
	return &ReconciliationService{source: source}
}

func (s *ReconciliationService) RunOnce(ctx context.Context) (ReconciliationReport, error) {
	balances, err := s.source.LedgerBalances(ctx)
	if err != nil {
		return ReconciliationReport{}, err
	}
	positions, err := s.source.TradingPositions(ctx)
	if err != nil {
		return ReconciliationReport{}, err
	}

	var report ReconciliationReport
	seen := map[string]bool{}
	for _, balance := range balances {
		if balance.Available != "0" || balance.Frozen != "0" {
			seen[balance.UserID+":"+balance.Asset] = true
		}
	}
	for _, position := range positions {
		key := position.UserID + ":" + position.MarginAsset
		if seen[key] {
			report.Issues = append(report.Issues, ReconciliationIssue{
				Type:   ReconciliationIssueLedgerPositionMismatch,
				UserID: position.UserID,
				Asset:  position.MarginAsset,
				Detail: "ledger balance and dydx position both require review",
			})
		}
	}
	if orderSource, ok := s.source.(ReconciliationTradingOrderSource); ok {
		orders, err := orderSource.TradingOrders(ctx)
		if err != nil {
			return ReconciliationReport{}, err
		}
		positionByUserMarket := map[string]bool{}
		for _, position := range positions {
			positionByUserMarket[position.UserID+":"+position.Market] = true
		}
		for _, order := range orders {
			if isActiveTradingOrderStatus(order.Status) && !positionByUserMarket[order.UserID+":"+order.Market] {
				report.Issues = append(report.Issues, ReconciliationIssue{
					Type:   ReconciliationIssueMissingDYDXPosition,
					UserID: order.UserID,
					Asset:  order.MarginAsset,
					Detail: "active trading order has no dydx position snapshot",
				})
			}
		}
	}
	if failureSource, ok := s.source.(ReconciliationTradingFailureSource); ok {
		failures, err := failureSource.TradingSyncFailures(ctx)
		if err != nil {
			return ReconciliationReport{}, err
		}
		for _, failure := range failures {
			report.Issues = append(report.Issues, ReconciliationIssue{
				Type:   ReconciliationIssueDYDXSyncFailure,
				UserID: failure.UserID,
				Asset:  failure.RefundAsset,
				Detail: failure.Reason,
			})
		}
	}
	return report, nil
}

func isActiveTradingOrderStatus(status string) bool {
	switch status {
	case TradingOrderStatusPendingSubmit,
		TradingOrderStatusBroadcasted,
		TradingOrderStatusSubmitted,
		TradingOrderStatusDYDXFundingFailed,
		TradingOrderStatusSubmitFailed,
		TradingOrderStatusPartialFilled,
		TradingOrderStatusCancelPending,
		TradingOrderStatusClosePending,
		TradingOrderStatusCloseBroadcasted,
		TradingOrderStatusCloseSubmitted,
		TradingOrderStatusClosePartialFilled:
		return true
	default:
		return false
	}
}
