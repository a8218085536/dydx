package app

import (
	"context"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

type ledgerBalanceRequest struct {
	UserID string `json:"userId"`
	Asset  string `json:"asset"`
}

func (app *App) LedgerMockDeposit(c *ginx.Context) {
	var request tyche.LedgerDepositRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.ledger == nil {
		c.Failure(http.StatusInternalServerError, "ledger service is not configured", nil)
		return
	}

	deposit, err := app.ledger.CreditMockDeposit(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", deposit)
}

func (app *App) LedgerFreezeWithdrawal(c *ginx.Context) {
	var request tyche.LedgerWithdrawalRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.ledger == nil {
		c.Failure(http.StatusInternalServerError, "ledger service is not configured", nil)
		return
	}

	withdrawal, err := app.ledger.FreezeWithdrawal(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", withdrawal)
}

func (app *App) LedgerMarkWithdrawalPaid(c *ginx.Context) {
	var request tyche.LedgerWithdrawalResult
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.ledger == nil {
		c.Failure(http.StatusInternalServerError, "ledger service is not configured", nil)
		return
	}

	withdrawal, err := app.ledger.MarkWithdrawalPaid(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", withdrawal)
}

func (app *App) LedgerMarkWithdrawalFailed(c *ginx.Context) {
	var request tyche.LedgerWithdrawalResult
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.ledger == nil {
		c.Failure(http.StatusInternalServerError, "ledger service is not configured", nil)
		return
	}

	withdrawal, err := app.ledger.MarkWithdrawalFailed(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", withdrawal)
}

func (app *App) LedgerBalance(c *ginx.Context) {
	var request ledgerBalanceRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.ledger == nil {
		c.Failure(http.StatusInternalServerError, "ledger service is not configured", nil)
		return
	}

	balance, err := app.ledger.GetBalance(context.Background(), request.UserID, request.Asset)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", balance)
}

func (app *App) LedgerRepairFrozenBalance(c *ginx.Context) {
	var request tyche.LedgerFrozenBalanceRepairRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.ledger == nil {
		c.Failure(http.StatusInternalServerError, "ledger service is not configured", nil)
		return
	}

	report, err := app.ledger.RepairFrozenBalance(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", report)
}
