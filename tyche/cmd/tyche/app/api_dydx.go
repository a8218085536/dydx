package app

import (
	"context"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

func (app *App) DYDXSubmit(c *ginx.Context) {
	var request tyche.DYDXIntent
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	submitter, err := app.dydxSubmitter()
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}

	if err := submitter.Submit(context.Background(), request); err != nil {
		c.Failure(http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.Success(http.StatusOK, "succ", nil)
}
