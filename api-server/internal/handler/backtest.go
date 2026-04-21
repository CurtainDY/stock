package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/parsedong/stock/api-server/internal/logic"
	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

func runBacktestHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RunBacktestReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewBacktestLogic(r.Context(), svcCtx)
		resp, err := l.Run(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func getBacktestHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathInt64(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		l := logic.NewBacktestLogic(r.Context(), svcCtx)
		resp, err := l.Get(id)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if resp == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func listBacktestsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewBacktestLogic(r.Context(), svcCtx)
		list, err := l.List()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if list == nil {
			list = []*types.BacktestRunResp{}
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"runs": list, "total": len(list)})
	}
}
