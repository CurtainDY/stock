package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"status": "ok"})
	}
}
