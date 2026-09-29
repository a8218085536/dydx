package app

import (
	"context"
	"errors"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

type necoWithdrawalRequest struct {
	WithdrawalID string `json:"withdrawalId"`
}

type necoDepositReceiptRequest struct {
	TxHash string `json:"txHash"`
}

func (app *App) NECOPriceQuote(c *ginx.Context) {
	var request tyche.NECOPriceQuoteRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.necoPriceQuotes == nil {
		c.Failure(http.StatusInternalServerError, "neco price quote service is not configured", nil)
		return
	}

	quote, err := app.necoPriceQuotes.Quote(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", quote)
}

func (app *App) NECOSyncDepositReceipt(c *ginx.Context) {
	var request necoDepositReceiptRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.necoDepositReceiptSync == nil {
		c.Failure(http.StatusInternalServerError, "neco deposit receipt syncer is not configured", nil)
		return
	}

	deposits, err := app.necoDepositReceiptSync.SyncReceipt(context.Background(), request.TxHash)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", deposits)
}

func (app *App) NECOProcessWithdrawal(c *ginx.Context) {
	var request necoWithdrawalRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.necoWithdrawal == nil {
		c.Failure(http.StatusInternalServerError, "neco withdrawal worker is not configured", nil)
		return
	}

	record, err := app.necoWithdrawal.ProcessWithdrawal(context.Background(), request.WithdrawalID)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", record)
}

func (app *App) NECOSyncWithdrawalReceipt(c *ginx.Context) {
	var request necoWithdrawalRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.necoWithdrawal == nil {
		c.Failure(http.StatusInternalServerError, "neco withdrawal worker is not configured", nil)
		return
	}

	record, err := app.necoWithdrawal.SyncReceipt(context.Background(), request.WithdrawalID)
	if errors.Is(err, tyche.ErrNECOWithdrawalNotFound) {
		record, err = app.necoWithdrawal.ProcessWithdrawal(context.Background(), request.WithdrawalID)
	}
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := app.updateTradingSettlementFromNECOWithdrawal(context.Background(), record); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", record)
}

func (app *App) updateTradingSettlementFromNECOWithdrawal(ctx context.Context, record tyche.NECOWithdrawalRecord) error {
	if app.tradingSettlements == nil || record.WithdrawalID == "" {
		return nil
	}
	settlement, ok, err := app.tradingSettlements.GetByLedgerWithdrawalID(ctx, record.WithdrawalID)
	if err != nil || !ok {
		return err
	}
	switch record.Status {
	case tyche.NECOWithdrawalStatusPaid:
		settlement.Status = tyche.TradingSettlementStatusNECOPayoutPaid
		settlement.NECOTxHash = record.TxHash
		settlement.Reason = ""
	case tyche.NECOWithdrawalStatusFailed:
		settlement.Status = tyche.TradingSettlementStatusFailed
		settlement.Reason = record.Reason
	default:
		return nil
	}
	return app.tradingSettlements.Save(ctx, settlement)
}
