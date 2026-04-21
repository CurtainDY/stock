package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"github.com/parsedong/stock/api-server/internal/svc"
)

func RegisterHandlers(server *rest.Server, ctx *svc.ServiceContext) {
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/v1/health", Handler: healthHandler()},

		// Strategies
		{Method: http.MethodPost, Path: "/v1/strategies", Handler: createStrategyHandler(ctx)},
		{Method: http.MethodGet, Path: "/v1/strategies", Handler: listStrategiesHandler(ctx)},
		{Method: http.MethodGet, Path: "/v1/strategies/:id", Handler: getStrategyHandler(ctx)},
		{Method: http.MethodPut, Path: "/v1/strategies/:id", Handler: updateStrategyHandler(ctx)},
		{Method: http.MethodDelete, Path: "/v1/strategies/:id", Handler: deleteStrategyHandler(ctx)},

		// Backtests (added in Task 5)
		// Symbols (added in Task 5)
	})
}
