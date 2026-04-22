# A股量化回测系统

私有 A 股量化回测系统，支持策略开发、历史回测、Walk-Forward 验证和结果可视化。

## 架构

```
stock/
├── backtest-engine/   # Go 回测引擎（gRPC 服务，端口 50051）
├── api-server/        # go-zero HTTP API 层（端口 8080）
├── python/            # Python 策略层（通过 gRPC 与引擎通信）
├── frontend/          # React 前端（端口 5173）
├── tools/             # 独立 CLI 工具（数据导入、复权因子、交易日历）
├── migrations/        # PostgreSQL 迁移 SQL
├── data/
│   ├── raw/           # 原始行情数据（只读，不提交）
│   └── normalized/    # 标准化 Parquet 文件（程序生成）
└── docker-compose.yml # PostgreSQL 16
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 回测引擎 | Go 1.22+，gRPC + protobuf3 |
| 策略层 | Python 3.11+，grpcio |
| HTTP API | go-zero v1.10+ |
| 前端 | React 18 + TailwindCSS v3 + ECharts 5 |
| 数据库 | PostgreSQL 16（Docker） |
| 行情存储 | Apache Parquet（per-stock 文件） |

## 快速启动

### 1. 启动 PostgreSQL

```bash
docker compose up -d
```

首次启动会自动执行 `migrations/` 下的所有 SQL。

### 2. 启动回测引擎（gRPC）

```bash
cd backtest-engine
go build -o backtest-engine ./cmd/server/
./backtest-engine -port 50051
```

### 3. 启动 API Server

```bash
cd api-server
go build -o api-server .
./api-server -f etc/api-server.yaml
```

### 4. 启动前端

```bash
cd frontend
npm install
npm run dev
# 浏览器访问 http://localhost:5173
```

### 5. 导入行情数据

```bash
cd tools/importer
go build -o importer .
./importer --src ../../data/raw --freq 1m
```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/health | 健康检查 |
| GET | /v1/strategies | 策略列表 |
| POST | /v1/strategies | 创建策略 |
| GET | /v1/strategies/:id | 策略详情 |
| PUT | /v1/strategies/:id | 更新策略 |
| DELETE | /v1/strategies/:id | 删除策略 |
| POST | /v1/backtests | 运行回测 |
| GET | /v1/backtests | 回测历史 |
| GET | /v1/backtests/:id | 回测结果 |
| GET | /v1/symbols | 可用股票列表 |

## A股规则

- **T+1**：当日买入次日才可卖出
- **涨跌停**：涨幅 ≥9.9% 拒绝买单，跌幅 ≥9.9% 拒绝卖单（ST 股 4.9%）
- **印花税**：卖出方向 0.1%
- **佣金**：双向 0.03%
- **复权**：通达信加法复权，`adj_price = raw_price + adj_factor`

## 编写策略

策略放在 `python/strategies/` 目录，继承 `Strategy` 基类：

```python
from backtest.types import Bar, Order, OrderSide, PortfolioSnapshot

class MyStrategy:
    def on_bar(self, bar: Bar, portfolio: PortfolioSnapshot) -> list[Order]:
        # bar.adj_close = bar.close + bar.adj_factor
        # 返回空列表表示不操作
        return []
```

运行单次回测：

```bash
cd python
.venv/bin/python run_backtest.py \
  --strategy MACrossStrategy \
  --params '{"fast":5,"slow":20}' \
  --symbols sz000001 \
  --start 2020-01-01 \
  --end 2023-12-31 \
  --capital 1000000
```

## 绩效指标

每次回测输出：年化收益率、最大回撤、夏普比率（无风险利率=0，×√242）、胜率、Calmar 比率、总收益率、净值曲线。

## 数据格式

行情数据存储在 `data/normalized/{freq}/{symbol}.parquet`，字段：

| 字段 | 说明 |
|------|------|
| symbol | 股票代码，如 sz000001 / sh600000 |
| date | 时间戳 |
| open/high/low/close | 未复权价格 |
| volume | 成交量（股） |
| amount | 成交额（元） |
| adj_factor | 通达信加法复权因子（默认 1.0） |

## 待完成

- [ ] Docker Desktop 安装后启动 PostgreSQL（当前 API 无 DB 可用）
- [ ] 将 `data/raw/` 中的行情数据导入 Parquet（运行 importer）
- [ ] Phase 4：LLM Agent 策略自动评审
