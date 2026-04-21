package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

func listSymbolsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		freq := r.URL.Query().Get("freq")
		if freq == "" {
			freq = "1m"
		}
		dataDir := "../data/normalized"
		dir := filepath.Join(dataDir, freq)

		entries, err := os.ReadDir(dir)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, &types.SymbolsResp{Symbols: []string{}, Total: 0})
			return
		}

		var symbols []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".parquet") {
				symbols = append(symbols, strings.TrimSuffix(e.Name(), ".parquet"))
			}
		}
		sort.Strings(symbols)
		httpx.OkJsonCtx(r.Context(), w, &types.SymbolsResp{Symbols: symbols, Total: len(symbols)})
	}
}
