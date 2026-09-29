package app

import (
	"context"
	"errors"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
	"strings"
)

type tradingSyncEventGetRequest struct {
	EventID string `json:"eventId"`
}

type tradingSyncPositionGetRequest struct {
	UserID           string `json:"userId"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
}

type tradingOrderListRequest struct {
	UserID string `json:"userId"`
}

type tradingSyncFailureListRequest struct {
	UserID string `json:"userId"`
}

type tradingSyncRetryRequest struct {
	EventID string `json:"eventId"`
}

type tradingSettlementGetRequest struct {
	UserID       string `json:"userId"`
	SettlementID string `json:"settlementId"`
	OrderID      string `json:"orderId"`
	CloseOrderID string `json:"closeOrderId"`
}

type tradingSettlementListRequest struct {
	UserID string `json:"userId"`
}

type tradingSettlementManualReviewListRequest struct {
	UserID string `json:"userId"`
}

type tradingSettlementRetryRequest struct {
	UserID       string `json:"userId"`
	SettlementID string `json:"settlementId"`
	OrderID      string `json:"orderId"`
	CloseOrderID string `json:"closeOrderId"`
}

func (app *App) TradingMarkets(c *ginx.Context) {
	if app.conf == nil || app.conf.Tyche == nil {
		c.Failure(http.StatusInternalServerError, "tyche config is not configured", nil)
		return
	}
	c.Success(http.StatusOK, "succ", app.conf.Tyche.DYDXMarkets)
}

func (app *App) TradingAssets(c *ginx.Context) {
	if app.conf == nil || app.conf.Tyche == nil {
		c.Failure(http.StatusInternalServerError, "tyche config is not configured", nil)
		return
	}
	assets := make([]tyche.AssetMapping, 0)
	for _, mapping := range app.conf.Tyche.AssetMappings {
		if mapping.EnabledForTrading {
			assets = append(assets, mapping)
		}
	}
	c.Success(http.StatusOK, "succ", assets)
}

func (app *App) TradingQuote(c *ginx.Context) {
	var request tyche.TradingMarginQuoteRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading service is not configured", nil)
		return
	}
	quote, err := app.trading.QuoteMargin(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", quote)
}

func (app *App) TradingAccountSummary(c *ginx.Context) {
	var request tyche.TradingAccountSummaryRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingAccountSummary == nil {
		c.Failure(http.StatusInternalServerError, "trading account summary service is not configured", nil)
		return
	}
	ctx := context.Background()
	userID, err := app.resolveTradingAccountUserID(ctx, request.UserID, request.NECOAddress)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	request.UserID = userID
	summary, err := app.tradingAccountSummary.Summary(ctx, request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", summary)
}

func (app *App) TradingAccountValuation(c *ginx.Context) {
	var request tyche.TradingAccountValuationRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingAccountValuation == nil {
		c.Failure(http.StatusInternalServerError, "trading account valuation service is not configured", nil)
		return
	}
	ctx := context.Background()
	userID, err := app.resolveTradingAccountUserID(ctx, request.UserID, request.NECOAddress)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	request.UserID = userID
	valuation, err := app.tradingAccountValuation.Valuation(ctx, request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", valuation)
}

func (app *App) resolveTradingAccountUserID(ctx context.Context, userID, necoAddress string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID != "" || strings.TrimSpace(necoAddress) == "" || app.walletBindings == nil {
		return userID, nil
	}
	binding, err := app.walletBindings.GetByNECOAddress(ctx, necoAddress)
	if errors.Is(err, tyche.ErrWalletBindingNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if binding.Status != tyche.WalletBindingStatusActive {
		return "", nil
	}
	return binding.UserID, nil
}

func (app *App) TradingPlaceOrder(c *ginx.Context) {
	var request tyche.TradingPlaceOrderRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading service is not configured", nil)
		return
	}

	order, err := app.trading.PlaceOrder(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", order)
}

func (app *App) TradingClosePosition(c *ginx.Context) {
	var request tyche.TradingClosePositionRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading service is not configured", nil)
		return
	}

	order, err := app.trading.ClosePosition(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", order)
}

func (app *App) TradingCancelOrder(c *ginx.Context) {
	var request tyche.TradingCancelOrderRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading service is not configured", nil)
		return
	}

	order, err := app.trading.CancelOrder(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", order)
}

func (app *App) TradingTPSLCreate(c *ginx.Context) {
	var request tyche.TradingTPSLCreateRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingTPSL == nil {
		c.Failure(http.StatusInternalServerError, "trading tpsl service is not configured", nil)
		return
	}

	group, err := app.tradingTPSL.Create(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", group)
}

func (app *App) TradingTPSLGet(c *ginx.Context) {
	var request tyche.TradingTPSLGetRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingTPSL == nil {
		c.Failure(http.StatusInternalServerError, "trading tpsl service is not configured", nil)
		return
	}

	entry, err := app.tradingTPSL.Get(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", entry)
}

func (app *App) TradingTPSLList(c *ginx.Context) {
	var request tyche.TradingTPSLListRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingTPSL == nil {
		c.Failure(http.StatusInternalServerError, "trading tpsl service is not configured", nil)
		return
	}

	entries, err := app.tradingTPSL.List(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", entries)
}

func (app *App) TradingTPSLCancel(c *ginx.Context) {
	var request tyche.TradingTPSLCancelRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingTPSL == nil {
		c.Failure(http.StatusInternalServerError, "trading tpsl service is not configured", nil)
		return
	}

	entries, err := app.tradingTPSL.Cancel(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", entries)
}

func (app *App) TradingGetOrder(c *ginx.Context) {
	var request tyche.TradingCancelOrderRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading service is not configured", nil)
		return
	}

	order, err := app.trading.GetOrder(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", order)
}

func (app *App) TradingListOrders(c *ginx.Context) {
	var request tradingOrderListRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading service is not configured", nil)
		return
	}

	orders, err := app.trading.ListOrders(context.Background(), request.UserID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", orders)
}

func (app *App) TradingBalanceOverview(c *ginx.Context) {
	var request tyche.TradingBalanceOverviewRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingBalances == nil {
		c.Failure(http.StatusInternalServerError, "trading balance service is not configured", nil)
		return
	}

	overview, err := app.tradingBalances.Overview(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", overview)
}

func (app *App) TradingSettlementGet(c *ginx.Context) {
	var request tradingSettlementGetRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSettlements == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement store is not configured", nil)
		return
	}
	settlement, ok, err := app.getTradingSettlement(context.Background(), request.UserID, request.SettlementID, firstNonEmptyString(request.OrderID, request.CloseOrderID))
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !ok {
		c.Failure(http.StatusBadRequest, tyche.ErrTradingSettlementNotFound.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", settlement)
}

func (app *App) TradingSettlementList(c *ginx.Context) {
	var request tradingSettlementListRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSettlements == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement store is not configured", nil)
		return
	}
	settlements, err := app.tradingSettlements.ListByUser(context.Background(), request.UserID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", settlements)
}

func (app *App) TradingSettlementManualReviewList(c *ginx.Context) {
	var request tradingSettlementManualReviewListRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSettlements == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement store is not configured", nil)
		return
	}
	var settlements []tyche.TradingSettlement
	var err error
	if request.UserID != "" {
		settlements, err = app.tradingSettlements.ListByUser(context.Background(), request.UserID)
		settlements = filterAppSettlementsByStatus(settlements, tyche.TradingSettlementStatusManualReviewRequired)
	} else {
		settlements, err = app.tradingSettlements.ListByStatus(context.Background(), tyche.TradingSettlementStatusManualReviewRequired)
	}
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", settlements)
}

func (app *App) TradingSettlementManualPayout(c *ginx.Context) {
	var request tyche.TradingSettlementManualPayoutRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.closeSettlement == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement service is not configured", nil)
		return
	}
	settlement, err := app.closeSettlement.ManualPayout(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), settlement)
		return
	}
	c.Success(http.StatusOK, "succ", settlement)
}

func (app *App) TradingSettlementReject(c *ginx.Context) {
	var request tyche.TradingSettlementRejectRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.closeSettlement == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement service is not configured", nil)
		return
	}
	settlement, err := app.closeSettlement.RejectSettlement(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), settlement)
		return
	}
	c.Success(http.StatusOK, "succ", settlement)
}

func (app *App) TradingReconciliationReport(c *ginx.Context) {
	var request tyche.TradingReconciliationRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSettlements == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement store is not configured", nil)
		return
	}
	var fillLookup tyche.TradingFillLookup
	if app.tradingSync != nil {
		fillLookup = app.tradingSync
	}
	reconciler := tyche.NewTradingReconciliationService(app.tradingSettlements, app.ledger, fillLookup)
	if app.necoDeposits != nil {
		reconciler.SetNECODepositSource(app.necoDeposits)
	}
	report, err := reconciler.Report(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", report)
}

func (app *App) TradingSettlementRetry(c *ginx.Context) {
	var request tradingSettlementRetryRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.closeSettlement == nil || app.trading == nil {
		c.Failure(http.StatusInternalServerError, "trading settlement retry is not configured", nil)
		return
	}
	settlement, ok, err := app.getTradingSettlement(context.Background(), request.UserID, request.SettlementID, firstNonEmptyString(request.OrderID, request.CloseOrderID))
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !ok {
		c.Failure(http.StatusBadRequest, tyche.ErrTradingSettlementNotFound.Error(), nil)
		return
	}
	order, err := app.trading.GetOrder(context.Background(), tyche.TradingCancelOrderRequest{
		UserID:  settlement.UserID,
		OrderID: settlement.CloseOrderID,
	})
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	var next tyche.TradingSettlement
	switch settlement.Kind {
	case tyche.TradingSettlementKindCancelOrder:
		next, err = app.closeSettlement.SettleCancelledOrder(context.Background(), tyche.CancelSettlementRequest{Order: order})
	case tyche.TradingSettlementKindFailedOrder:
		next, err = app.closeSettlement.SettleFailedOrder(context.Background(), tyche.FailedOrderSettlementRequest{Order: order})
	default:
		next, err = app.closeSettlement.SettleClosedPosition(context.Background(), tyche.CloseSettlementRequest{CloseOrder: order})
	}
	if err != nil && !errors.Is(err, tyche.ErrTradingSettlementManualReview) {
		c.Failure(http.StatusBadRequest, err.Error(), next)
		return
	}
	c.Success(http.StatusOK, "succ", next)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (app *App) getTradingSettlement(ctx context.Context, userID, settlementID, closeOrderID string) (tyche.TradingSettlement, bool, error) {
	if settlementID != "" {
		settlement, ok, err := app.tradingSettlements.GetByID(ctx, settlementID)
		if err != nil || !ok {
			return settlement, ok, err
		}
		if userID != "" && settlement.UserID != userID {
			return tyche.TradingSettlement{}, false, nil
		}
		return settlement, true, nil
	}
	if closeOrderID != "" {
		return app.tradingSettlements.GetByCloseOrderID(ctx, userID, closeOrderID)
	}
	return tyche.TradingSettlement{}, false, nil
}

func filterAppSettlementsByStatus(settlements []tyche.TradingSettlement, status string) []tyche.TradingSettlement {
	filtered := make([]tyche.TradingSettlement, 0, len(settlements))
	for _, settlement := range settlements {
		if settlement.Status == status {
			filtered = append(filtered, settlement)
		}
	}
	return filtered
}

func (app *App) DYDXSyncFill(c *ginx.Context) {
	var request tyche.TradingFillSyncRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	fill, err := app.tradingSync.SyncFill(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", fill)
}

func (app *App) DYDXSyncPosition(c *ginx.Context) {
	var request tyche.TradingPositionSyncRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	position, err := app.tradingSync.SyncPosition(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", position)
}

func (app *App) DYDXSyncFailures(c *ginx.Context) {
	var request tradingSyncFailureListRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	failures, err := app.tradingSync.ListFailures(context.Background(), request.UserID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", failures)
}

func (app *App) DYDXSyncRetry(c *ginx.Context) {
	var request tradingSyncRetryRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	event, err := app.tradingSync.RetryFailure(context.Background(), request.EventID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", event)
}

func (app *App) DYDXSyncOrderStatus(c *ginx.Context) {
	var request tyche.TradingOrderStatusSyncRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	status, err := app.tradingSync.SyncOrderStatus(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", status)
}

func (app *App) DYDXSyncFillGet(c *ginx.Context) {
	var request tradingSyncEventGetRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	fill, err := app.tradingSync.GetFill(context.Background(), request.EventID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", fill)
}

func (app *App) DYDXSyncOrderStatusGet(c *ginx.Context) {
	var request tradingSyncEventGetRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	status, err := app.tradingSync.GetOrderStatus(context.Background(), request.EventID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", status)
}

func (app *App) DYDXSyncPositionGet(c *ginx.Context) {
	var request tradingSyncPositionGetRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.tradingSync == nil {
		c.Failure(http.StatusInternalServerError, "trading sync service is not configured", nil)
		return
	}

	subaccountNumber := request.SubaccountNumber
	if subaccountNumber == 0 && app.trading != nil {
		subaccountNumber = app.trading.SubaccountNumberForMarket(request.Market)
	}
	position, err := app.tradingSync.GetPositionForSubaccount(context.Background(), request.UserID, request.Market, subaccountNumber)
	if err != nil {
		if errors.Is(err, tyche.ErrTradingSyncEventNotFound) {
			c.Success(http.StatusOK, "succ", tyche.TradingPosition{
				UserID:           request.UserID,
				Market:           request.Market,
				SubaccountNumber: subaccountNumber,
				Size:             "0",
				EntryPrice:       "0",
				MarkPrice:        "0",
				UnrealizedPNL:    "0",
				Margin:           "0",
			})
			return
		}
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", position)
}
