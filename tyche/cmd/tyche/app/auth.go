package app

import (
	"ethgo/util/ginx"
	"net/http"
)

const internalAPIKeyHeader = "X-Tyche-Internal-Key"

func (app *App) protected(handler func(*ginx.Context)) func(*ginx.Context) {
	return func(c *ginx.Context) {
		if app.conf != nil && app.conf.Tyche != nil && app.conf.Tyche.InternalAPIKey != "" {
			if c.GetHeader(internalAPIKeyHeader) != app.conf.Tyche.InternalAPIKey {
				c.Failure(http.StatusUnauthorized, "invalid internal api key", nil)
				return
			}
		}
		handler(c)
	}
}
