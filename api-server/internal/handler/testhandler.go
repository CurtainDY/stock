package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/router"

	"github.com/parsedong/stock/api-server/internal/svc"
)

// NewHTTPHandler builds a plain http.Handler wired with all routes.
// It is intended for use in tests where a full go-zero server is not needed.
func NewHTTPHandler(ctx *svc.ServiceContext) http.Handler {
	r := router.NewRouter()
	routes := []struct {
		method  string
		path    string
		handler http.Handler
	}{
		{http.MethodGet, "/v1/health", healthHandler()},
		{http.MethodPost, "/v1/strategies", createStrategyHandler(ctx)},
		{http.MethodGet, "/v1/strategies", listStrategiesHandler(ctx)},
		{http.MethodGet, "/v1/strategies/:id", getStrategyHandler(ctx)},
		{http.MethodPut, "/v1/strategies/:id", updateStrategyHandler(ctx)},
		{http.MethodDelete, "/v1/strategies/:id", deleteStrategyHandler(ctx)},
		{http.MethodPost, "/v1/backtests", runBacktestHandler(ctx)},
		{http.MethodGet, "/v1/backtests", listBacktestsHandler(ctx)},
		{http.MethodGet, "/v1/backtests/:id", getBacktestHandler(ctx)},
		{http.MethodGet, "/v1/symbols", listSymbolsHandler(ctx)},
	}
	for _, rt := range routes {
		if err := r.Handle(rt.method, rt.path, rt.handler); err != nil {
			panic(err)
		}
	}
	return r
}
