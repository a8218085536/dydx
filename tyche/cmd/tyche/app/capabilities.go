package app

import (
	"errors"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

func (app *App) evmTransactor() (tyche.EVMTransactor, error) {
	runner, ok := app.base.(tyche.EVMTransactor)
	if !ok {
		return nil, errors.New("this API is only available in evm mode")
	}
	return runner, nil
}

func (app *App) dydxSubmitter() (tyche.DYDXSubmitter, error) {
	runner, ok := app.base.(tyche.DYDXSubmitter)
	if !ok {
		return nil, errors.New("this API is only available in dydx mode")
	}
	return runner, nil
}

func (app *App) evmOnly(handler func(*ginx.Context)) func(*ginx.Context) {
	return func(c *ginx.Context) {
		if app.conf != nil && app.conf.Tyche != nil && app.conf.Tyche.ChainType == tyche.ChainTypeDYDX {
			c.Failure(http.StatusBadRequest, "this API is only available in evm mode", nil)
			return
		}
		handler(c)
	}
}
