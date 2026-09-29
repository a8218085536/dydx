package app

import (
	"context"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

type walletBindingGetRequest struct {
	UserID      string `json:"userId,omitempty"`
	NECOAddress string `json:"necoAddress"`
}

type walletBindingGetByDYDXRequest struct {
	DYDXAddress string `json:"dydxAddress"`
}

type walletBindingOverviewRequest struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress,omitempty"`
}

func (app *App) WalletBindingCreate(c *ginx.Context) {
	var request tyche.WalletBindingCreate
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if app.walletBindings == nil {
		c.Failure(http.StatusInternalServerError, "wallet binding service is not configured", nil)
		return
	}

	binding, err := app.walletBindings.Create(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", binding)
}

func (app *App) WalletBindingVerify(c *ginx.Context) {
	var request tyche.WalletBindingVerify
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if app.walletBindings == nil {
		c.Failure(http.StatusInternalServerError, "wallet binding service is not configured", nil)
		return
	}

	binding, err := app.walletBindings.Verify(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", binding)
}

func (app *App) WalletBindingGet(c *ginx.Context) {
	var request walletBindingGetRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.walletBindings == nil {
		c.Failure(http.StatusInternalServerError, "wallet binding service is not configured", nil)
		return
	}

	var binding tyche.WalletBinding
	var err error
	if request.UserID != "" {
		binding, err = app.walletBindings.GetByUserID(context.Background(), request.UserID)
	} else {
		binding, err = app.walletBindings.GetByNECOAddress(context.Background(), request.NECOAddress)
	}
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", binding)
}

func (app *App) WalletBindingGetByDYDX(c *ginx.Context) {
	var request walletBindingGetByDYDXRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.walletBindings == nil {
		c.Failure(http.StatusInternalServerError, "wallet binding service is not configured", nil)
		return
	}

	binding, err := app.walletBindings.GetByDYDXAddress(context.Background(), request.DYDXAddress)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", binding)
}

func (app *App) WalletBindingOverview(c *ginx.Context) {
	var request walletBindingOverviewRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.walletBindings == nil {
		c.Failure(http.StatusInternalServerError, "wallet binding service is not configured", nil)
		return
	}

	var overview tyche.WalletBindingOverview
	var err error
	if request.UserID != "" {
		overview, err = app.walletBindings.Overview(context.Background(), request.UserID)
	} else {
		var binding tyche.WalletBinding
		binding, err = app.walletBindings.GetByNECOAddress(context.Background(), request.NECOAddress)
		if err == nil {
			overview = tyche.WalletBindingOverview{
				UserID:               binding.UserID,
				HasActiveDYDXBinding: binding.Status == tyche.WalletBindingStatusActive,
				Binding:              &binding,
			}
		}
	}
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", overview)
}
