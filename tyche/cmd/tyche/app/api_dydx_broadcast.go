package app

import (
	"context"
	"errors"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

func (app *App) DYDXBroadcast(c *ginx.Context) {
	if app.conf != nil && app.conf.Tyche != nil && app.conf.Tyche.DYDXTradingURL != "" {
		c.Failure(http.StatusBadRequest, "legacy dydx broadcast is disabled; use dydxTradingURL", nil)
		return
	}

	var request tyche.DYDXChainRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if app.chainGateway == nil {
		app.chainGateway = tyche.NewDYDXChainGateway(nil)
	}

	txHash, err := app.chainGateway.Broadcast(context.Background(), request)
	if err != nil {
		if errors.Is(err, tyche.ErrUnsupportedDYDXAction) {
			c.Failure(http.StatusBadRequest, err.Error(), nil)
			return
		}
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", map[string]string{"txHash": txHash})
}
