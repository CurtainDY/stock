# Phase 3a: go-zero HTTP API Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 go-zero 构建 HTTP API 层，为前端提供策略管理、回测触发和结果查询接口，并通过 gRPC 与 backtest-engine 通信。

**Architecture:** `api-server/` 是独立 go module，使用 go-zero rest 框架处理 HTTP，通过 gRPC 客户端调用 backtest-engine，PostgreSQL 存储策略和回测记录。回测执行通过调用 Python 子进程（`python/run_backtest.py`）完成，结果异步写回 DB。

**Tech Stack:** Go 1.22+, go-zero v1.7+, lib/pq, google.golang.org/grpc v1.80+, PostgreSQL 16

---

## 参考资料

- gRPC proto: `backtest-engine/proto/backtest.proto`
- 已有迁移: `migrations/001_init.sql`（有 stocks/import_batches/import_checks 表）
- gRPC 服务端口: 50051（默认）
- Python 入口: `python/` 目录（Phase 2b 已实现）
- Spec: `docs/superpowers/specs/2026-04-16-system-spec.md`

---

## 文件结构

```
api-server/
├── go.mod
├── main.go                          # 入口：加载配置，注册路由，启动 HTTP 服务
├── etc/
│   └── api-server.yaml              # 配置：HTTP端口、gRPC地址、DB连接串
└── internal/
    ├── config/
    │   └── config.go                # Config 结构体（内嵌 rest.RestConf）
    ├── svc/
    │   └── servicecontext.go        # ServiceContext：持有 DB 连接和 gRPC stub
    ├── types/
    │   └── types.go                 # 请求/响应结构体（Strategy, BacktestRun 等）
    ├── handler/
    │   ├── routes.go                # RegisterHandlers：绑定路由
    │   ├── health.go                # GET /v1/health
    │   ├── strategy.go              # Strategy CRUD handlers
    │   ├── backtest.go              # POST /v1/backtests, GET /v1/backtests/:id
    │   └── symbol.go                # GET /v1/symbols
    └── logic/
        ├── strategy_logic.go        # Strategy 业务逻辑（操作 DB）
        └── backtest_logic.go        # Backtest 业务逻辑（DB + 触发 Python）

migrations/
└── 002_strategies.sql               # 新增 strategies + backtest_runs 表

python/
└── run_backtest.py                  # CLI 入口：读参数→构建 BacktestRunner→输出 JSON
```

---

## Task 1: DB 迁移 — strategies + backtest_runs 表

**Files:**
- Create: `migrations/002_strategies.sql`

- [ ] **Step 1: 创建迁移文件**

创建 `migrations/002_strategies.sql`：

```sql
-- 策略定义表
CREATE TABLE IF NOT EXISTS strategies (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    class_name  VARCHAR(100) NOT NULL,   -- Python 类名，如 MACrossStrategy
    params      JSONB DEFAULT '{}',      -- 策略参数，如 {"fast":5,"slow":20}
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 回测运行记录表
CREATE TABLE IF NOT EXISTS backtest_runs (
    id              SERIAL PRIMARY KEY,
    strategy_id     INT REFERENCES strategies(id) ON DELETE SET NULL,
    strategy_name   VARCHAR(100) NOT NULL,
    symbols         TEXT[]  NOT NULL,
    start_date      DATE    NOT NULL,
    end_date        DATE    NOT NULL,
    init_capital    DOUBLE PRECISION DEFAULT 1000000,
    status          VARCHAR(16) DEFAULT 'pending',  -- pending/running/done/failed
    annual_return   DOUBLE PRECISION,
    max_drawdown    DOUBLE PRECISION,
    sharpe_ratio    DOUBLE PRECISION,
    win_rate        DOUBLE PRECISION,
    calmar_ratio    DOUBLE PRECISION,
    total_return    DOUBLE PRECISION,
    equity_curve    DOUBLE PRECISION[],
    error_msg       TEXT DEFAULT '',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);
CREATE INDEX ON backtest_runs(strategy_id);
CREATE INDEX ON backtest_runs(status);
CREATE INDEX ON backtest_runs(created_at DESC);
```

- [ ] **Step 2: 应用迁移（需 Docker PostgreSQL 运行）**

```bash
# 若 Docker 可用：
docker exec -i stock-postgres psql -U stock -d stock < migrations/002_strategies.sql
```

若 Docker 未运行，跳过此步——迁移文件已就绪，后续有 DB 时执行即可。

- [ ] **Step 3: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add migrations/002_strategies.sql
git commit -m "feat: add strategies and backtest_runs tables migration"
```

---

## Task 2: Python CLI 入口 — run_backtest.py

**背景：** API 服务器通过 `python run_backtest.py --args...` 子进程调用 Python 策略运行回测，结果以 JSON 输出到 stdout。

**Files:**
- Create: `python/run_backtest.py`

- [ ] **Step 1: 创建 run_backtest.py**

创建 `python/run_backtest.py`：

```python
#!/usr/bin/env python3
"""
回测 CLI 入口。被 api-server 以子进程方式调用。

用法：
  python run_backtest.py \
    --strategy MACrossStrategy \
    --params '{"fast":5,"slow":20}' \
    --symbols sz000001,sh600000 \
    --start 2020-01-01 \
    --end 2023-12-31 \
    --capital 1000000 \
    --grpc-host localhost \
    --grpc-port 50051

输出到 stdout（JSON）：
  {"status":"done","annual_return":0.15,"max_drawdown":0.08,...,"equity_curve":[1.0,...]}

错误时输出：
  {"status":"failed","error":"..."}
"""
import argparse
import importlib
import json
import sys
import traceback
from datetime import date


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--strategy", required=True, help="策略类名")
    parser.add_argument("--params", default="{}", help="策略参数 JSON")
    parser.add_argument("--symbols", required=True, help="股票代码，逗号分隔")
    parser.add_argument("--start", required=True, help="开始日期 YYYY-MM-DD")
    parser.add_argument("--end", required=True, help="结束日期 YYYY-MM-DD")
    parser.add_argument("--capital", type=float, default=1_000_000)
    parser.add_argument("--grpc-host", default="localhost")
    parser.add_argument("--grpc-port", type=int, default=50051)
    args = parser.parse_args()

    try:
        result = run_backtest(args)
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"status": "failed", "error": traceback.format_exc()}))
        sys.exit(1)


def run_backtest(args) -> dict:
    import uuid
    from backtest.client import BacktestClient
    from backtest.runner import BacktestConfig, BacktestRunner

    # 动态加载策略类
    strategy_cls = _load_strategy(args.strategy)
    params = json.loads(args.params)

    symbols = [s.strip() for s in args.symbols.split(",")]
    start = date.fromisoformat(args.start)
    end = date.fromisoformat(args.end)

    config = BacktestConfig(
        symbols=symbols,
        start_date=start,
        end_date=end,
        init_capital=args.capital,
    )

    with BacktestClient(host=args.grpc_host, port=args.grpc_port) as client:
        session_id = client.new_session_id()
        runner = BacktestRunner(stub=client.stub, config=config, session_id=session_id)
        strategy = strategy_cls(**params)
        metrics = runner.run(strategy)

    return {
        "status": "done",
        "annual_return": metrics.annual_return,
        "max_drawdown": metrics.max_drawdown,
        "sharpe_ratio": metrics.sharpe_ratio,
        "win_rate": metrics.win_rate,
        "calmar_ratio": metrics.calmar_ratio,
        "total_return": metrics.total_return,
        "equity_curve": metrics.equity_curve,
    }


def _load_strategy(class_name: str):
    """
    按类名查找策略类，先在 strategies/ 包中搜索，再在 backtest/ 中搜索。
    支持 "MACrossStrategy" 或 "strategies.ma_cross.MACrossStrategy" 格式。
    """
    if "." in class_name:
        module_path, cls = class_name.rsplit(".", 1)
        mod = importlib.import_module(module_path)
        return getattr(mod, cls)

    # 简短名称：遍历 strategies 子模块
    import pkgutil
    import strategies as strat_pkg

    for importer, modname, ispkg in pkgutil.iter_modules(strat_pkg.__path__):
        mod = importlib.import_module(f"strategies.{modname}")
        if hasattr(mod, class_name):
            return getattr(mod, class_name)

    raise ValueError(f"Strategy class {class_name!r} not found in strategies/")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: 验证 CLI 可用（不连 gRPC，只验证导入）**

```bash
cd /Users/parsedong/workSpace/stock/python
.venv/bin/python3 run_backtest.py --help
```

Expected: 输出 usage 帮助信息，无报错

- [ ] **Step 3: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add python/run_backtest.py
git commit -m "feat: add run_backtest.py CLI for API server subprocess invocation"
```

---

## Task 3: api-server 项目初始化

**Files:**
- Create: `api-server/go.mod`
- Create: `api-server/etc/api-server.yaml`
- Create: `api-server/internal/config/config.go`
- Create: `api-server/internal/svc/servicecontext.go`
- Create: `api-server/internal/types/types.go`
- Create: `api-server/main.go`

- [ ] **Step 1: 初始化 Go 模块**

```bash
mkdir -p api-server/etc api-server/internal/config api-server/internal/svc
mkdir -p api-server/internal/types api-server/internal/handler api-server/internal/logic
cd api-server
go mod init github.com/parsedong/stock/api-server
go get github.com/zeromicro/go-zero@latest
go get github.com/lib/pq@latest
go get google.golang.org/grpc@v1.80.0
go get google.golang.org/protobuf@v1.36.11
```

- [ ] **Step 2: 创建配置文件 `api-server/etc/api-server.yaml`**

```yaml
Name: api-server
Host: 0.0.0.0
Port: 8080

BacktestEngine:
  Host: localhost
  Port: 50051

Database:
  DSN: postgres://stock:stock123@localhost:5432/stock?sslmode=disable

Python:
  Executable: python3         # Python 可执行文件路径
  WorkDir: ../python          # run_backtest.py 所在目录
```

- [ ] **Step 3: 创建 `api-server/internal/config/config.go`**

```go
package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	BacktestEngine struct {
		Host string
		Port int
	}

	Database struct {
		DSN string
	}

	Python struct {
		Executable string
		WorkDir    string
	}
}
```

- [ ] **Step 4: 创建 `api-server/internal/types/types.go`**

```go
package types

// ---- 策略 ----

type CreateStrategyReq struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ClassName   string                 `json:"class_name"`
	Params      map[string]interface{} `json:"params"`
}

type UpdateStrategyReq struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ClassName   string                 `json:"class_name"`
	Params      map[string]interface{} `json:"params"`
}

type StrategyResp struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ClassName   string                 `json:"class_name"`
	Params      map[string]interface{} `json:"params"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// ---- 回测 ----

type RunBacktestReq struct {
	StrategyID  *int64   `json:"strategy_id"`   // 可选：指定已保存策略
	StrategyName string  `json:"strategy_name"` // 策略类名（未保存时使用）
	Params      map[string]interface{} `json:"params"`
	Symbols     []string `json:"symbols"`
	StartDate   string   `json:"start_date"`   // YYYY-MM-DD
	EndDate     string   `json:"end_date"`
	InitCapital float64  `json:"init_capital"`
}

type BacktestRunResp struct {
	ID           int64     `json:"id"`
	StrategyName string    `json:"strategy_name"`
	Symbols      []string  `json:"symbols"`
	StartDate    string    `json:"start_date"`
	EndDate      string    `json:"end_date"`
	InitCapital  float64   `json:"init_capital"`
	Status       string    `json:"status"`
	AnnualReturn *float64  `json:"annual_return,omitempty"`
	MaxDrawdown  *float64  `json:"max_drawdown,omitempty"`
	SharpeRatio  *float64  `json:"sharpe_ratio,omitempty"`
	WinRate      *float64  `json:"win_rate,omitempty"`
	CalmarRatio  *float64  `json:"calmar_ratio,omitempty"`
	TotalReturn  *float64  `json:"total_return,omitempty"`
	EquityCurve  []float64 `json:"equity_curve,omitempty"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
	CreatedAt    string    `json:"created_at"`
	FinishedAt   *string   `json:"finished_at,omitempty"`
}

// ---- Symbol ----

type SymbolsResp struct {
	Symbols []string `json:"symbols"`
	Total   int      `json:"total"`
}
```

- [ ] **Step 5: 创建 `api-server/internal/svc/servicecontext.go`**

```go
package svc

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/parsedong/stock/api-server/internal/config"
	pb "github.com/parsedong/stock/api-server/proto"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Engine pb.BacktestEngineClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := sql.Open("postgres", c.Database.DSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Printf("WARNING: db ping failed: %v (running without DB)", err)
	}

	addr := fmt.Sprintf("%s:%d", c.BacktestEngine.Host, c.BacktestEngine.Port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("WARNING: grpc dial failed: %v (running without engine)", err)
	}

	var engineClient pb.BacktestEngineClient
	if conn != nil {
		engineClient = pb.NewBacktestEngineClient(conn)
	}

	return &ServiceContext{
		Config: c,
		DB:     db,
		Engine: engineClient,
	}
}
```

**注意：** `api-server` 需要访问 proto 生成的 Go 代码。由于 proto 文件在 `backtest-engine/proto/`，我们有两个选择：
a) 复制生成的 pb.go 文件到 `api-server/proto/`
b) 使用 `replace` 指令引用 `backtest-engine` 模块

选 **a**（复制，避免 replace 复杂性）：

```bash
mkdir -p api-server/proto
cp backtest-engine/proto/backtest.pb.go api-server/proto/
cp backtest-engine/proto/backtest_grpc.pb.go api-server/proto/
# 修改 package 声明保持一致（已经是 package backtest）
```

- [ ] **Step 6: 复制 proto 生成文件到 api-server/proto/**

```bash
cd /Users/parsedong/workSpace/stock
mkdir -p api-server/proto
cp backtest-engine/proto/backtest.pb.go api-server/proto/
cp backtest-engine/proto/backtest_grpc.pb.go api-server/proto/
```

验证文件存在：
```bash
ls api-server/proto/
```
Expected: `backtest.pb.go  backtest_grpc.pb.go`

- [ ] **Step 7: 创建 `api-server/main.go`（骨架）**

```go
package main

import (
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"

	"github.com/parsedong/stock/api-server/internal/config"
	"github.com/parsedong/stock/api-server/internal/handler"
	"github.com/parsedong/stock/api-server/internal/svc"
)

var configFile = flag.String("f", "etc/api-server.yaml", "config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	server := rest.MustNewServer(c.RestConf,
		rest.WithCors("*"),
	)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting api-server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
```

- [ ] **Step 8: 创建占位 handler/routes.go**

创建 `api-server/internal/handler/routes.go`：

```go
package handler

import (
	"github.com/zeromicro/go-zero/rest"
	"github.com/parsedong/stock/api-server/internal/svc"
)

func RegisterHandlers(server *rest.Server, ctx *svc.ServiceContext) {
	// handlers registered in subsequent tasks
}
```

- [ ] **Step 9: 编译**

```bash
cd api-server
go mod tidy
go build .
```

Expected: 编译成功（二进制 `api-server` 或 `api-server.exe`）

- [ ] **Step 10: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add api-server/
git commit -m "feat: initialize api-server with go-zero framework"
```

---

## Task 4: Strategy CRUD

**Files:**
- Create: `api-server/internal/logic/strategy_logic.go`
- Create: `api-server/internal/handler/strategy.go`
- Modify: `api-server/internal/handler/routes.go`

- [ ] **Step 1: 实现 `api-server/internal/logic/strategy_logic.go`**

```go
package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

type StrategyLogic struct {
	ctx context.Context
	svc *svc.ServiceContext
}

func NewStrategyLogic(ctx context.Context, svc *svc.ServiceContext) *StrategyLogic {
	return &StrategyLogic{ctx: ctx, svc: svc}
}

func (l *StrategyLogic) Create(req *types.CreateStrategyReq) (*types.StrategyResp, error) {
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	var id int64
	var createdAt, updatedAt time.Time
	err = l.svc.DB.QueryRowContext(l.ctx, `
		INSERT INTO strategies (name, description, class_name, params)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		req.Name, req.Description, req.ClassName, string(paramsJSON),
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert strategy: %w", err)
	}
	return &types.StrategyResp{
		ID: id, Name: req.Name, Description: req.Description,
		ClassName: req.ClassName, Params: req.Params,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}, nil
}

func (l *StrategyLogic) List() ([]*types.StrategyResp, error) {
	rows, err := l.svc.DB.QueryContext(l.ctx, `
		SELECT id, name, description, class_name, params, created_at, updated_at
		FROM strategies ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query strategies: %w", err)
	}
	defer rows.Close()

	var result []*types.StrategyResp
	for rows.Next() {
		var s types.StrategyResp
		var paramsRaw string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.ClassName,
			&paramsRaw, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(paramsRaw), &s.Params) //nolint:errcheck
		s.CreatedAt = createdAt.Format(time.RFC3339)
		s.UpdatedAt = updatedAt.Format(time.RFC3339)
		result = append(result, &s)
	}
	return result, rows.Err()
}

func (l *StrategyLogic) Get(id int64) (*types.StrategyResp, error) {
	var s types.StrategyResp
	var paramsRaw string
	var createdAt, updatedAt time.Time
	err := l.svc.DB.QueryRowContext(l.ctx, `
		SELECT id, name, description, class_name, params, created_at, updated_at
		FROM strategies WHERE id=$1`, id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.ClassName,
		&paramsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get strategy: %w", err)
	}
	json.Unmarshal([]byte(paramsRaw), &s.Params) //nolint:errcheck
	s.CreatedAt = createdAt.Format(time.RFC3339)
	s.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &s, nil
}

func (l *StrategyLogic) Update(id int64, req *types.UpdateStrategyReq) (*types.StrategyResp, error) {
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	var updatedAt time.Time
	err = l.svc.DB.QueryRowContext(l.ctx, `
		UPDATE strategies SET name=$1, description=$2, class_name=$3, params=$4, updated_at=NOW()
		WHERE id=$5 RETURNING updated_at`,
		req.Name, req.Description, req.ClassName, string(paramsJSON), id,
	).Scan(&updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update strategy: %w", err)
	}
	return l.Get(id)
}

func (l *StrategyLogic) Delete(id int64) error {
	_, err := l.svc.DB.ExecContext(l.ctx, `DELETE FROM strategies WHERE id=$1`, id)
	return err
}
```

- [ ] **Step 2: 实现 `api-server/internal/handler/strategy.go`**

```go
package handler

import (
	"encoding/json"
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

// pathInt64 从 URL path 参数中取 int64（go-zero 通过 r.PathValue 提供）
func pathInt64(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}
```

- [ ] **Step 3: 更新 `api-server/internal/handler/routes.go`**

```go
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
```

- [ ] **Step 4: 创建 `api-server/internal/handler/health.go`**

```go
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
```

- [ ] **Step 5: 编译**

```bash
cd api-server
go build .
```

Expected: 编译成功

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add api-server/
git commit -m "feat: add strategy CRUD handlers and routes"
```

---

## Task 5: Backtest + Symbol 接口

**Files:**
- Create: `api-server/internal/logic/backtest_logic.go`
- Create: `api-server/internal/handler/backtest.go`
- Create: `api-server/internal/handler/symbol.go`
- Modify: `api-server/internal/handler/routes.go`

- [ ] **Step 1: 实现 `api-server/internal/logic/backtest_logic.go`**

```go
package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

type BacktestLogic struct {
	ctx context.Context
	svc *svc.ServiceContext
}

func NewBacktestLogic(ctx context.Context, svc *svc.ServiceContext) *BacktestLogic {
	return &BacktestLogic{ctx: ctx, svc: svc}
}

// Run 创建回测记录，同步执行 Python 子进程，将结果写回 DB
func (l *BacktestLogic) Run(req *types.RunBacktestReq) (*types.BacktestRunResp, error) {
	strategyName := req.StrategyName
	if req.StrategyID != nil {
		s, err := NewStrategyLogic(l.ctx, l.svc).Get(*req.StrategyID)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, fmt.Errorf("strategy %d not found", *req.StrategyID)
		}
		strategyName = s.ClassName
		if req.Params == nil {
			req.Params = s.Params
		}
	}

	paramsJSON, _ := json.Marshal(req.Params)
	initCapital := req.InitCapital
	if initCapital == 0 {
		initCapital = 1_000_000
	}

	// 创建待运行记录
	var runID int64
	var createdAt time.Time
	err := l.svc.DB.QueryRowContext(l.ctx, `
		INSERT INTO backtest_runs
		(strategy_id, strategy_name, symbols, start_date, end_date, init_capital, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'running')
		RETURNING id, created_at`,
		req.StrategyID, strategyName,
		pq.Array(req.Symbols),
		req.StartDate, req.EndDate, initCapital,
	).Scan(&runID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert backtest_run: %w", err)
	}

	// 执行 Python 子进程
	result := l.executePython(strategyName, string(paramsJSON), req.Symbols,
		req.StartDate, req.EndDate, initCapital)

	// 更新 DB 结果
	if result["status"] == "done" {
		annualReturn := floatVal(result, "annual_return")
		maxDrawdown := floatVal(result, "max_drawdown")
		sharpe := floatVal(result, "sharpe_ratio")
		winRate := floatVal(result, "win_rate")
		calmar := floatVal(result, "calmar_ratio")
		totalReturn := floatVal(result, "total_return")

		equityCurveRaw, _ := json.Marshal(result["equity_curve"])
		l.svc.DB.ExecContext(l.ctx, `
			UPDATE backtest_runs SET
				status='done', annual_return=$1, max_drawdown=$2, sharpe_ratio=$3,
				win_rate=$4, calmar_ratio=$5, total_return=$6, equity_curve=$7,
				finished_at=NOW()
			WHERE id=$8`,
			annualReturn, maxDrawdown, sharpe, winRate, calmar, totalReturn,
			equityCurveRaw, runID)
	} else {
		errMsg, _ := result["error"].(string)
		l.svc.DB.ExecContext(l.ctx, `
			UPDATE backtest_runs SET status='failed', error_msg=$1, finished_at=NOW()
			WHERE id=$2`, errMsg, runID)
	}

	return l.Get(runID)
}

func (l *BacktestLogic) executePython(
	strategyName, paramsJSON string,
	symbols []string, startDate, endDate string,
	initCapital float64,
) map[string]interface{} {
	c := l.svc.Config.Python
	pythonExe := c.Executable
	if pythonExe == "" {
		pythonExe = "python3"
	}
	workDir := c.WorkDir
	if workDir == "" {
		workDir = "../python"
	}
	// 解析为绝对路径
	if !filepath.IsAbs(workDir) {
		if abs, err := filepath.Abs(workDir); err == nil {
			workDir = abs
		}
	}

	args := []string{
		"run_backtest.py",
		"--strategy", strategyName,
		"--params", paramsJSON,
		"--symbols", strings.Join(symbols, ","),
		"--start", startDate,
		"--end", endDate,
		"--capital", fmt.Sprintf("%.0f", initCapital),
		"--grpc-host", l.svc.Config.BacktestEngine.Host,
		"--grpc-port", fmt.Sprintf("%d", l.svc.Config.BacktestEngine.Port),
	}
	cmd := exec.CommandContext(l.ctx, pythonExe, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PYTHONPATH="+workDir)

	out, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{"status": "failed", "error": err.Error()}
	}

	var result map[string]interface{}
	if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
		return map[string]interface{}{"status": "failed", "error": string(out)}
	}
	return result
}

func (l *BacktestLogic) Get(id int64) (*types.BacktestRunResp, error) {
	row := l.svc.DB.QueryRowContext(l.ctx, `
		SELECT id, strategy_name, symbols, start_date, end_date, init_capital,
		       status, annual_return, max_drawdown, sharpe_ratio, win_rate,
		       calmar_ratio, total_return, equity_curve, error_msg, created_at, finished_at
		FROM backtest_runs WHERE id=$1`, id)
	return scanBacktestRun(row)
}

func (l *BacktestLogic) List() ([]*types.BacktestRunResp, error) {
	rows, err := l.svc.DB.QueryContext(l.ctx, `
		SELECT id, strategy_name, symbols, start_date, end_date, init_capital,
		       status, annual_return, max_drawdown, sharpe_ratio, win_rate,
		       calmar_ratio, total_return, equity_curve, error_msg, created_at, finished_at
		FROM backtest_runs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*types.BacktestRunResp
	for rows.Next() {
		r, err := scanBacktestRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// scanBacktestRun 从 sql.Row 或 sql.Rows 扫描一条回测记录
type scanner interface {
	Scan(dest ...any) error
}

func scanBacktestRun(s scanner) (*types.BacktestRunResp, error) {
	var r types.BacktestRunResp
	var symbols pq.StringArray
	var startDate, endDate time.Time
	var annualReturn, maxDrawdown, sharpe, winRate, calmar, totalReturn *float64
	var equityCurveRaw []byte
	var finishedAt *time.Time

	err := s.Scan(
		&r.ID, &r.StrategyName, &symbols, &startDate, &endDate, &r.InitCapital,
		&r.Status, &annualReturn, &maxDrawdown, &sharpe, &winRate,
		&calmar, &totalReturn, &equityCurveRaw, &r.ErrorMsg,
		&r.CreatedAt, &finishedAt,
	)
	if err != nil {
		return nil, err
	}
	r.Symbols = []string(symbols)
	r.StartDate = startDate.Format("2006-01-02")
	r.EndDate = endDate.Format("2006-01-02")
	r.AnnualReturn = annualReturn
	r.MaxDrawdown = maxDrawdown
	r.SharpeRatio = sharpe
	r.WinRate = winRate
	r.CalmarRatio = calmar
	r.TotalReturn = totalReturn
	if len(equityCurveRaw) > 0 {
		json.Unmarshal(equityCurveRaw, &r.EquityCurve) //nolint:errcheck
	}
	if finishedAt != nil {
		s := finishedAt.Format(time.RFC3339)
		r.FinishedAt = &s
	}
	return &r, nil
}

func floatVal(m map[string]interface{}, key string) *float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return &f
		}
	}
	return nil
}
```

- [ ] **Step 2: 实现 `api-server/internal/handler/backtest.go`**

```go
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
```

- [ ] **Step 3: 实现 `api-server/internal/handler/symbol.go`**

```go
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
```

- [ ] **Step 4: 更新 routes.go**

替换 `api-server/internal/handler/routes.go` 全文：

```go
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

		// Backtests
		{Method: http.MethodPost, Path: "/v1/backtests", Handler: runBacktestHandler(ctx)},
		{Method: http.MethodGet, Path: "/v1/backtests", Handler: listBacktestsHandler(ctx)},
		{Method: http.MethodGet, Path: "/v1/backtests/:id", Handler: getBacktestHandler(ctx)},

		// Symbols
		{Method: http.MethodGet, Path: "/v1/symbols", Handler: listSymbolsHandler(ctx)},
	})
}
```

- [ ] **Step 5: 编译**

```bash
cd api-server
go mod tidy
go build .
```

Expected: 编译成功

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add api-server/
git commit -m "feat: add backtest run and symbol list handlers"
```

---

## Task 6: 集成测试（httptest，不依赖真实 DB/gRPC）

**Files:**
- Create: `api-server/handler_test.go`

- [ ] **Step 1: 编写集成测试**

创建 `api-server/handler_test.go`：

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/rest"

	"github.com/parsedong/stock/api-server/internal/handler"
	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/config"
)

// newTestServer 创建一个测试用 HTTP 服务器（无真实 DB/gRPC）
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	srv := rest.MustNewServer(rest.RestConf{
		Host: "localhost",
		Port: 0,
	})
	// Use nil DB/Engine — handlers that don't need them will work fine
	ctx := &svc.ServiceContext{Config: config.Config{}}
	handler.RegisterHandlers(srv, ctx)
	return srv
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("health: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Errorf("health: body %q does not contain 'ok'", rr.Body.String())
	}
}

func TestSymbolsEndpointReturnsEmptyWhenNoData(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/symbols?freq=1m", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	// Should return 200 even when data dir doesn't exist
	if rr.Code != http.StatusOK {
		t.Errorf("symbols: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "symbols") {
		t.Errorf("symbols: body %q does not contain 'symbols'", rr.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试**

```bash
cd api-server
go test ./... -v -run TestHealth
go test ./... -v -run TestSymbols
```

Expected: 两个测试 PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add api-server/handler_test.go
git commit -m "test: add integration tests for health and symbols endpoints"
```

---

## 自检：Spec 覆盖确认

| Spec 要求 | 对应任务 |
|----------|---------|
| HTTP API 层使用 go-zero | Task 3 |
| api-server 独立 go module | Task 3 |
| api-server 不直接操作数据 | Task 4/5 — 通过 logic 层调用 DB |
| 策略管理（CRUD）| Task 4 |
| 回测触发（调用 Python 策略）| Task 5 |
| gRPC client 连接 backtest-engine | Task 3 svc/servicecontext.go |
| CORS 支持前端跨域 | Task 3 main.go `rest.WithCors("*")` |
| 禁止硬编码连接信息 | Task 3 — 通过 etc/api-server.yaml 注入 |
| 回测结果存 PostgreSQL | Task 5 backtest_logic.go |
| 原始行情 Bar 不存 PostgreSQL | ✓ 不在 API 层存储 Bar |
