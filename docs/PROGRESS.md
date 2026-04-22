# 项目进展与后续计划

**最后更新：** 2026-04-22

---

## 整体思路

做一个私有 A 股量化回测系统，最终目标是**实盘交易**。路径是：

1. 先把回测基础设施搭好（引擎 + 数据 + 策略层）
2. 加 HTTP API 和前端，方便管理策略和查看结果
3. 引入 LLM 自动评审策略，过滤掉过拟合的策略
4. 对接券商 API 实现实盘

---

## 已完成

### Phase 1 — Go 回测引擎 ✅

**目录：** `backtest-engine/`

核心实现：
- gRPC 服务（端口 50051），三个接口：`StreamBars`、`SubmitOrder`、`RunBacktest`
- A股撮合规则：T+1、涨跌停（普通股 9.9%/ST 4.9%）、印花税 0.1%、佣金 0.03%
- 持仓管理：多 lot 记录，T+1 通过 `AvailableQty(date)` 控制可卖数量
- Session 管理：`sync.Map` 隔离并发回测会话，全天 bar 原子替换防竞态
- Parquet 数据读取：`data/normalized/{freq}/{symbol}.parquet`

关键设计决策：
- 复权方式：**通达信加法复权**，`adj_price = raw_price + adj_factor`（factor 为负数）
- Fill 枚举映射：matcher.Filled=0 → pb.FILLED=1，显式 if/else 避免直接强转
- Bar 推送：按交易日升序，策略不能看到未来数据

### Phase 2a — 数据管道 ✅

**目录：** `tools/`

- `tools/importer/`：CSV → Parquet，自动检测 UTF-8/GBK 编码，填充复权因子
- `tools/adjfactor/`：解析通达信复权因子文件，写入 PostgreSQL `adj_factors` 表
- `tools/calendar/`：通过 tushare 获取真实交易日历，缓存到 `data/calendar.csv`
- 数据校验：完整度 ≥80%、价格合理性、复权因子 >0，结果写入 `import_checks` 表

数据格式：`data/raw/` 是淘宝买的原始 CSV，不提交 git。

### Phase 2b — Python 策略层 ✅

**目录：** `python/`

- `backtest/types.py`：Bar（含 `adj_close` 属性）、Order、Fill、Position、PortfolioSnapshot
- `backtest/portfolio.py`：PortfolioTracker，管理持仓和资金，`apply_fill` / `total_value`
- `backtest/analytics.py`：计算 6 项绩效指标，Sharpe = mean/std × √242，无风险利率=0
- `backtest/client.py`：gRPC 客户端封装，context manager
- `backtest/runner.py`：BacktestRunner，流式接收 Bar，调用策略 `on_bar`，提交订单
- `backtest/walk_forward.py`：Walk-Forward 验证，支持扩展窗口/滚动窗口，防过拟合
- `strategies/ma_cross.py`：MA 双均线示例策略，使用 adj_close

gRPC metadata：`x-session-id`（会话隔离）+ `x-date-unix`（T+1 判断）

### Phase 3a — HTTP API Server ✅

**目录：** `api-server/`

- go-zero v1.10，独立 go module
- 9 个 HTTP 端点（见 README）
- 策略 CRUD：存 PostgreSQL `strategies` 表
- 回测触发：插 `backtest_runs` 记录 → 启动 Python 子进程 → 结果写回 DB
- Symbol 列表：扫描 `data/normalized/{freq}/` 目录
- 配置文件：`etc/api-server.yaml`（DB DSN、gRPC 地址、Python 路径）

### Phase 3b — React 前端 ✅

**目录：** `frontend/`

- Vite 5 + React 18 + TailwindCSS v3 + ECharts 5 + React Router v6
- 策略管理页：表格展示 + 新建/编辑/删除 Modal
- 回测运行页：选策略 + 填参数 + 提交，左侧历史记录列表
- 回测结果页：6 项指标卡片 + ECharts 净值曲线折线图
- 19 个单元测试全部通过
- Vite 代理：`/v1/*` → `localhost:8080`

---

## 当前状态

服务已启动（本地开发环境）：
- ✅ backtest-engine：`:50051`（进程 68920）
- ✅ api-server：`:8080`（进程 68992，DB 连接失败，功能受限）
- ✅ frontend：`http://localhost:5173`（进程 69098）
- ❌ PostgreSQL：**未启动**，需要 Docker Desktop

**当前限制：**
- 策略管理、回测历史需要 PostgreSQL，现在会报 `connection refused`
- `data/normalized/` 为空，需要先跑 importer 才能回测

---

## 待完成

### 立即可做（解锁当前功能）

**1. 安装 Docker Desktop**
```bash
brew install --cask docker
# 打开 Docker Desktop，等待启动
docker compose up -d   # 在 stock/ 目录下
```
PostgreSQL 启动后重启 api-server，策略管理就可以用了。

**2. 导入行情数据**
```bash
cd tools/importer
go build -o importer .
./importer --src ../../data/raw --freq 1m
```
导入完成后 `data/normalized/1m/` 会有 Parquet 文件，才能真正跑回测。

---

### Phase 4 — LLM 策略评审（未开始）

**目标：** 用 Claude API 自动评审回测结果，判断策略是否过拟合、是否值得实盘。

**思路：**
- 每次回测完成后，把绩效指标 + Walk-Forward 结果发给 Claude
- Claude 输出评审报告：风险点、过拟合迹象、是否推荐实盘
- 评审结果存 PostgreSQL `agent_reviews` 表
- 前端结果页增加"AI 评审"标签页

**需要新增：**
- `migrations/003_agent_reviews.sql`
- `api-server/internal/logic/review_logic.go`（调用 Claude API）
- 前端 ResultPage 增加评审展示

---

### Phase 5 — 实盘对接（远期）

**目标：** 策略通过评审后，接入券商 API 执行真实交易。

**候选方案：**
- XTP（兴业证券）：Go SDK，低延迟
- QMT（迅投）：Python SDK，简单易用
- 富途/老虎：港美股，A 股不支持

**需要新增：**
- 实盘撮合模块（替换回测撮合器）
- 风控模块：单日最大亏损限制、持仓上限
- 监控告警：持仓变化推送（微信/钉钉）

---

## 数据库表结构

```sql
-- 已有（migrations/001_init.sql）
stocks           -- 股票元数据
import_batches   -- 导入记录
import_checks    -- 数据校验结果

-- 已有（migrations/002_strategies.sql）
strategies       -- 策略定义（name, class_name, params JSONB）
backtest_runs    -- 回测记录（metrics, equity_curve[], status）

-- 待建（Phase 4）
agent_reviews    -- LLM 评审结果
```

---

## 关键配置

**api-server/etc/api-server.yaml**
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
  Executable: python3
  WorkDir: ../python
```

**docker-compose.yml**
- 镜像：postgres:16
- 用户/密码/库：stock / stock123 / stock
- 端口：5432
- 数据卷：./data/pg
- 迁移：自动加载 ./migrations/*.sql

---

## 代码仓库

https://github.com/CurtainDY/stock
