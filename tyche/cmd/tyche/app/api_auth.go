package app

import (
	"context"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"net/http"
)

func (app *App) AuthChallenge(c *ginx.Context) {
	var request tyche.AuthChallengeRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.auth == nil {
		c.Failure(http.StatusInternalServerError, "auth service is not configured", nil)
		return
	}
	challenge, err := app.auth.Challenge(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", challenge)
}

func (app *App) AuthLogin(c *ginx.Context) {
	var request tyche.AuthLoginRequest
	if err := c.BindJSONEx(&request); err != nil {
		c.Failure(http.StatusBadRequest, err.Error(), nil)
		return
	}
	if app.auth == nil {
		c.Failure(http.StatusInternalServerError, "auth service is not configured", nil)
		return
	}
	login, err := app.auth.Login(context.Background(), request)
	if err != nil {
		c.Failure(http.StatusUnauthorized, err.Error(), nil)
		return
	}
	c.Success(http.StatusOK, "succ", login)
}
