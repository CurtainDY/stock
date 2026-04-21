package handler

import (
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/parsedong/stock/api-server/internal/logic"
	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

func createStrategyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateStrategyReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewStrategyLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func listStrategiesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewStrategyLogic(r.Context(), svcCtx)
		list, err := l.List()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if list == nil {
			list = []*types.StrategyResp{}
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"strategies": list, "total": len(list)})
	}
}

func getStrategyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathInt64(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		l := logic.NewStrategyLogic(r.Context(), svcCtx)
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

func updateStrategyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathInt64(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var req types.UpdateStrategyReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewStrategyLogic(r.Context(), svcCtx)
		resp, err := l.Update(id, &req)
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

func deleteStrategyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathInt64(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		l := logic.NewStrategyLogic(r.Context(), svcCtx)
		if err := l.Delete(id); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// pathInt64 extracts an int64 from URL path parameters (go-zero uses r.PathValue)
func pathInt64(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}
