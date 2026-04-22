# Phase 2b: Python gRPC Client + Strategy Interface + Walk-Forward Validation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Python 策略层通过 gRPC 与 Go 引擎交互，支持完整回测流程和 Walk-Forward 验证防过拟合。

**Architecture:** Go 服务端新增 session 管理（通过 gRPC metadata `x-session-id` 标识），Python 客户端驱动逐 Bar 回测，本地追踪持仓，最终用 Walk-Forward 跨时间窗口验证策略稳健性。无需修改 proto 文件。

**Tech Stack:** Python 3.11+, grpcio 1.62+, grpcio-tools, protobuf 4.x, pytest, Go 1.22+ (server修改)

---

## 参考资料

- proto 文件：`backtest-engine/proto/backtest.proto`
- Go 引擎入口：`backtest-engine/cmd/server/main.go`
- Matcher：`backtest-engine/internal/matcher/matcher.go`（限涨跌停 + 佣金 + 印花税）
- Portfolio：`backtest-engine/internal/portfolio/portfolio.go`（T+1 lot 追踪）
- Analytics：`backtest-engine/internal/analytics/analytics.go`
- Python 目录：`python/`（目前为空）

---

## 文件结构

```
backtest-engine/
└── cmd/server/main.go          # 修改：添加 session 管理 + 实现 SubmitOrder

python/
├── pyproject.toml              # Python 项目配置
├── Makefile                    # proto 生成脚本
├── backtest/
│   ├── __init__.py
│   ├── proto/
│   │   ├── __init__.py
│   │   ├── backtest_pb2.py     # 生成，勿手改
│   │   └── backtest_pb2_grpc.py
│   ├── types.py                # Bar, PortfolioSnapshot, Order, Fill dataclass
│   ├── portfolio.py            # PortfolioTracker（本地持仓，处理 Fill）
│   ├── analytics.py            # Sharpe/DrawDown/WinRate 等指标计算
│   ├── client.py               # BacktestClient（gRPC 连接封装）
│   ├── strategy.py             # 抽象 Strategy 基类
│   ├── runner.py               # BacktestRunner（单次回测流程编排）
│   └── walk_forward.py         # WalkForwardValidator
├── strategies/
│   ├── __init__.py
│   └── ma_cross.py             # MA 双均线策略示例
└── tests/
    ├── __init__.py
    ├── test_portfolio.py
    ├── test_analytics.py
    ├── test_runner.py           # 使用 mock gRPC stub
    └── test_walk_forward.py
```

---

## Task 1: Go 服务端 Session 管理 + SubmitOrder 实现

**背景：** 当前 `SubmitOrder` 只返回 REJECTED。需要添加 session 状态，使 Python 可以在 `StreamBars` 会话中提交订单并得到真实撮合结果。Session ID 通过 gRPC metadata `x-session-id` 传递，日期通过 `x-date-unix` 传递。

**Files:**
- Modify: `backtest-engine/cmd/server/main.go`

- [ ] **Step 1: 运行现有引擎测试，确认基线通过**

```bash
cd backtest-engine
go test ./... 2>&1
```

Expected: all PASS（当前已有的测试均通过）

- [ ] **Step 2: 替换 main.go 为完整的 session 版本**

将 `backtest-engine/cmd/server/main.go` 整体替换为以下内容：

```go
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/parsedong/stock/backtest-engine/internal/data"
	"github.com/parsedong/stock/backtest-engine/internal/matcher"
	"github.com/parsedong/stock/backtest-engine/internal/portfolio"
	pb "github.com/parsedong/stock/backtest-engine/proto"
)

var (
	port    = flag.String("port", "50051", "gRPC服务端口")
	dataDir = flag.String("data", "../../data/normalized", "标准化数据目录")
)

// session 持有单次回测的状态
type session struct {
	portfolio   *portfolio.Portfolio
	matcher     *matcher.Matcher
	currentBars map[string]data.Bar // symbol → 当天最新 bar（供撮合用）
	prevPrices  map[string]float64  // symbol → 前日收盘（供涨跌停判断）
	mu          sync.Mutex
}

type server struct {
	pb.UnimplementedBacktestEngineServer
	store    data.DataStore
	sessions sync.Map // string(sessionID) → *session
}

// sessionFromCtx 从 gRPC metadata 中取 x-session-id
func sessionFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("x-session-id")
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// dateFromCtx 从 gRPC metadata 中取 x-date-unix（Unix 秒）
func dateFromCtx(ctx context.Context) time.Time {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return time.Time{}
	}
	vals := md.Get("x-date-unix")
	if len(vals) == 0 {
		return time.Time{}
	}
	var unix int64
	if _, err := fmt.Sscanf(vals[0], "%d", &unix); err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

func (s *server) RunBacktest(_ context.Context, req *pb.BacktestRequest) (*pb.BacktestResult, error) {
	log.Printf("RunBacktest: symbols=%v start=%s end=%s capital=%.0f",
		req.Symbols, req.StartDate, req.EndDate, req.InitCapital)
	return &pb.BacktestResult{}, nil
}

func (s *server) StreamBars(req *pb.StreamRequest, stream pb.BacktestEngine_StreamBarsServer) error {
	sessionID := sessionFromCtx(stream.Context())
	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "x-session-id metadata required")
	}

	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid start_date: %v", err)
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid end_date: %v", err)
	}

	initCapital := req.InitCapital
	if initCapital == 0 {
		initCapital = 1_000_000
	}
	commission := req.Commission
	if commission == 0 {
		commission = 0.0003
	}

	sess := &session{
		portfolio:   portfolio.New(initCapital),
		matcher:     matcher.New(matcher.Config{Commission: commission}),
		currentBars: make(map[string]data.Bar),
		prevPrices:  make(map[string]float64),
	}
	s.sessions.Store(sessionID, sess)
	defer s.sessions.Delete(sessionID)

	days, err := s.store.TradingDays(start, end)
	if err != nil {
		return status.Errorf(codes.Internal, "trading days: %v", err)
	}

	var prevDay time.Time
	for _, day := range days {
		// 新的一天开始：将上一天的 currentBars 转为 prevPrices
		if !prevDay.IsZero() {
			sess.mu.Lock()
			for sym, bar := range sess.currentBars {
				sess.prevPrices[sym] = bar.Close
			}
			sess.currentBars = make(map[string]data.Bar)
			sess.mu.Unlock()
		}
		prevDay = day

		bars, err := s.store.LoadBarsByDate(day, req.Symbols)
		if err != nil {
			return status.Errorf(codes.Internal, "load bars %s: %v", day.Format("2006-01-02"), err)
		}
		for sym, bar := range bars {
			sess.mu.Lock()
			sess.currentBars[sym] = bar
			sess.mu.Unlock()

			if err := stream.Send(&pb.Bar{
				Symbol:    bar.Symbol,
				DateUnix:  bar.Date.Unix(),
				Open:      bar.Open,
				High:      bar.High,
				Low:       bar.Low,
				Close:     bar.Close,
				Volume:    bar.Volume,
				Amount:    bar.Amount,
				AdjFactor: bar.AdjFactor,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *server) SubmitOrder(ctx context.Context, req *pb.Order) (*pb.Fill, error) {
	sessionID := sessionFromCtx(ctx)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "x-session-id metadata required")
	}

	v, ok := s.sessions.Load(sessionID)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "session %q not found", sessionID)
	}
	sess := v.(*session)

	orderDate := dateFromCtx(ctx)
	if orderDate.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "x-date-unix metadata required")
	}

	sess.mu.Lock()
	bar, hasCurrent := sess.currentBars[req.Symbol]
	prevClose := sess.prevPrices[req.Symbol]
	sess.mu.Unlock()

	if !hasCurrent {
		return &pb.Fill{
			Symbol:       req.Symbol,
			Side:         req.Side,
			Status:       pb.OrderStatus_REJECTED,
			RejectReason: "no bar data for symbol on current day",
		}, nil
	}

	// 检查可卖数量（T+1）
	if req.Side == pb.OrderSide_SELL {
		pos := sess.portfolio.Position(req.Symbol)
		avail := pos.AvailableQty(orderDate)
		if avail < req.Quantity {
			return &pb.Fill{
				Symbol:       req.Symbol,
				Side:         req.Side,
				Status:       pb.OrderStatus_REJECTED,
				RejectReason: fmt.Sprintf("T+1: available=%.0f, want=%.0f", avail, req.Quantity),
			}, nil
		}
	}

	order := matcher.Order{
		Symbol:   req.Symbol,
		Side:     matcher.Side(req.Side),
		Quantity: req.Quantity,
		Price:    req.Price,
	}
	fill := sess.matcher.Match(order, bar, prevClose, false)

	if fill.Status == matcher.Filled {
		sess.mu.Lock()
		_ = sess.portfolio.ApplyFill(fill, orderDate)
		sess.mu.Unlock()
	}

	fillStatus := pb.OrderStatus_REJECTED
	if fill.Status == matcher.Filled {
		fillStatus = pb.OrderStatus_FILLED
	}
	return &pb.Fill{
		Symbol:       fill.Symbol,
		Side:         pb.OrderSide(fill.Side),
		FilledQty:    fill.FilledQty,
		FilledPrice:  fill.FilledPrice,
		Commission:   fill.Commission,
		StampDuty:    fill.StampDuty,
		Status:       fillStatus,
		RejectReason: fill.RejectReason,
	}, nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterBacktestEngineServer(s, &server{store: data.NewParquetStore(*dataDir)})
	log.Printf("backtest engine listening on :%s", *port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
```

**注意：** 上面代码用了 `fmt.Sscanf` 和 `fmt.Sprintf`，需要在 import 中添加 `"fmt"`。

- [ ] **Step 3: 添加 fmt import**

检查 Step 2 写入的文件，确认 `"fmt"` 在 import 列表中（`dateFromCtx` 和 `SubmitOrder` 都用到 `fmt`）。

- [ ] **Step 4: 编译确认**

```bash
cd backtest-engine
go build ./cmd/server/
```

Expected: 无报错

- [ ] **Step 5: 运行现有测试确认不回归**

```bash
cd backtest-engine
go test ./... 2>&1
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add backtest-engine/cmd/server/main.go
git commit -m "feat: add session management and implement SubmitOrder in gRPC server"
```

---

## Task 2: Python 项目初始化 + Proto Stubs 生成

**Files:**
- Create: `python/pyproject.toml`
- Create: `python/Makefile`
- Create: `python/backtest/__init__.py`
- Create: `python/backtest/proto/__init__.py`
- Create: `python/backtest/proto/backtest_pb2.py` (generated)
- Create: `python/backtest/proto/backtest_pb2_grpc.py` (generated)
- Create: `python/strategies/__init__.py`
- Create: `python/tests/__init__.py`

- [ ] **Step 1: 创建 pyproject.toml**

创建 `python/pyproject.toml`：

```toml
[build-system]
requires = ["setuptools>=68"]
build-backend = "setuptools.backends.legacy:build"

[project]
name = "stock-backtest"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = [
    "grpcio>=1.62.0",
    "grpcio-tools>=1.62.0",
    "protobuf>=4.25.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "pytest-mock>=3.12",
]
strategies = [
    "numpy>=1.26",
    "pandas>=2.1",
]

[tool.setuptools.packages.find]
where = ["."]
include = ["backtest*", "strategies*"]
```

- [ ] **Step 2: 安装依赖**

```bash
cd python
pip install -e ".[dev]"
```

Expected: grpcio, protobuf, pytest 均安装成功

- [ ] **Step 3: 创建 Makefile（proto 生成）**

创建 `python/Makefile`：

```makefile
PROTO_SRC = ../backtest-engine/proto/backtest.proto
OUT_DIR   = backtest/proto

.PHONY: proto clean

proto:
	python -m grpc_tools.protoc \
		-I../backtest-engine/proto \
		--python_out=$(OUT_DIR) \
		--grpc_python_out=$(OUT_DIR) \
		$(PROTO_SRC)
	@echo "Proto stubs generated in $(OUT_DIR)"

clean:
	rm -f $(OUT_DIR)/backtest_pb2*.py
```

- [ ] **Step 4: 生成 proto stubs**

```bash
cd python
mkdir -p backtest/proto
make proto
```

Expected: 生成 `backtest/proto/backtest_pb2.py` 和 `backtest/proto/backtest_pb2_grpc.py`

- [ ] **Step 5: 创建 __init__.py 文件**

```bash
touch python/backtest/__init__.py
touch python/backtest/proto/__init__.py
touch python/strategies/__init__.py
touch python/tests/__init__.py
```

- [ ] **Step 6: 验证 proto import 正常**

```bash
cd python
python -c "from backtest.proto import backtest_pb2; print('ok:', backtest_pb2.Bar())"
```

Expected: `ok: `（空 Bar 消息）

- [ ] **Step 7: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add python/
git commit -m "feat: initialize Python project with gRPC proto stubs"
```

---

## Task 3: Python 核心类型 + PortfolioTracker + Analytics

**Files:**
- Create: `python/backtest/types.py`
- Create: `python/backtest/portfolio.py`
- Create: `python/backtest/analytics.py`
- Create: `python/tests/test_portfolio.py`
- Create: `python/tests/test_analytics.py`

- [ ] **Step 1: 写 test_portfolio.py（先写测试）**

创建 `python/tests/test_portfolio.py`：

```python
import pytest
from datetime import date
from backtest.types import Fill, FillStatus, OrderSide
from backtest.portfolio import PortfolioTracker


def make_fill(symbol, side, qty, price, commission=0.0, stamp_duty=0.0):
    return Fill(
        symbol=symbol,
        side=side,
        filled_qty=qty,
        filled_price=price,
        commission=commission,
        stamp_duty=stamp_duty,
        status=FillStatus.FILLED,
    )


def test_buy_increases_position():
    p = PortfolioTracker(init_cash=100_000)
    fill = make_fill("sz000001", OrderSide.BUY, 1000, 10.0, commission=3.0)
    p.apply_fill(fill, date(2020, 1, 2))
    assert p.position("sz000001").quantity == 1000
    assert abs(p.cash - (100_000 - 10_000 - 3.0)) < 0.01


def test_sell_decreases_position():
    p = PortfolioTracker(init_cash=100_000)
    p.apply_fill(make_fill("sz000001", OrderSide.BUY, 1000, 10.0), date(2020, 1, 2))
    p.apply_fill(make_fill("sz000001", OrderSide.SELL, 500, 11.0, stamp_duty=5.5), date(2020, 1, 3))
    assert p.position("sz000001").quantity == 500
    assert abs(p.cash - (100_000 - 10_000 + 5_500 - 5.5)) < 0.01


def test_total_value():
    p = PortfolioTracker(init_cash=100_000)
    p.apply_fill(make_fill("sz000001", OrderSide.BUY, 1000, 10.0), date(2020, 1, 2))
    val = p.total_value({"sz000001": 12.0})
    assert abs(val - (100_000 - 10_000 + 1000 * 12.0)) < 0.01


def test_snapshot():
    p = PortfolioTracker(init_cash=100_000)
    p.apply_fill(make_fill("sz000001", OrderSide.BUY, 1000, 10.0), date(2020, 1, 2))
    snap = p.snapshot({"sz000001": 11.0})
    assert snap.cash == p.cash
    assert len(snap.positions) == 1
    assert snap.positions["sz000001"].quantity == 1000
    assert abs(snap.total_value - (p.cash + 11_000)) < 0.01
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
cd python
pytest tests/test_portfolio.py -v 2>&1 | head -30
```

Expected: ImportError（`backtest.types` 不存在）

- [ ] **Step 3: 实现 types.py**

创建 `python/backtest/types.py`：

```python
from dataclasses import dataclass, field
from datetime import date
from enum import IntEnum
from typing import Optional


class OrderSide(IntEnum):
    BUY = 0
    SELL = 1


class FillStatus(IntEnum):
    PENDING = 0
    FILLED = 1
    REJECTED = 2


@dataclass
class Bar:
    symbol: str
    date: date
    open: float
    high: float
    low: float
    close: float
    volume: float
    amount: float
    adj_factor: float = 0.0

    @property
    def adj_close(self) -> float:
        """前复权价格（同花顺加法偏移）"""
        if self.adj_factor == 0:
            return self.close
        return self.close + self.adj_factor


@dataclass
class Order:
    symbol: str
    side: OrderSide
    quantity: float
    price: float = 0.0  # 0 = 市价单，以收盘价成交


@dataclass
class Fill:
    symbol: str
    side: OrderSide
    filled_qty: float
    filled_price: float
    commission: float
    stamp_duty: float
    status: FillStatus
    reject_reason: str = ""


@dataclass
class Position:
    symbol: str
    quantity: float
    cost_price: float


@dataclass
class PortfolioSnapshot:
    cash: float
    positions: dict[str, Position]  # symbol → Position
    total_value: float
```

- [ ] **Step 4: 实现 portfolio.py**

创建 `python/backtest/portfolio.py`：

```python
from datetime import date
from backtest.types import Fill, FillStatus, OrderSide, Position, PortfolioSnapshot


class PortfolioTracker:
    """本地持仓追踪器，根据 Fill 更新现金和仓位。"""

    def __init__(self, init_cash: float):
        self._cash = init_cash
        self._positions: dict[str, _PositionDetail] = {}

    @property
    def cash(self) -> float:
        return self._cash

    def position(self, symbol: str) -> Position:
        if symbol not in self._positions:
            return Position(symbol=symbol, quantity=0.0, cost_price=0.0)
        p = self._positions[symbol]
        return Position(symbol=symbol, quantity=p.quantity, cost_price=p.cost_price)

    def apply_fill(self, fill: Fill, trade_date: date) -> None:
        if fill.status != FillStatus.FILLED:
            return

        if fill.side == OrderSide.BUY:
            cost = fill.filled_qty * fill.filled_price + fill.commission + fill.stamp_duty
            self._cash -= cost
            if fill.symbol not in self._positions:
                self._positions[fill.symbol] = _PositionDetail()
            self._positions[fill.symbol].add_lot(fill.filled_qty, fill.filled_price)

        elif fill.side == OrderSide.SELL:
            proceeds = fill.filled_qty * fill.filled_price - fill.commission - fill.stamp_duty
            self._cash += proceeds
            if fill.symbol in self._positions:
                self._positions[fill.symbol].reduce(fill.filled_qty)
                if self._positions[fill.symbol].quantity <= 0:
                    del self._positions[fill.symbol]

    def total_value(self, prices: dict[str, float]) -> float:
        total = self._cash
        for sym, pos in self._positions.items():
            total += pos.quantity * prices.get(sym, 0.0)
        return total

    def snapshot(self, prices: dict[str, float]) -> PortfolioSnapshot:
        positions = {
            sym: Position(symbol=sym, quantity=p.quantity, cost_price=p.cost_price)
            for sym, p in self._positions.items()
        }
        return PortfolioSnapshot(
            cash=self._cash,
            positions=positions,
            total_value=self.total_value(prices),
        )


class _PositionDetail:
    def __init__(self):
        self.quantity = 0.0
        self.cost_price = 0.0

    def add_lot(self, qty: float, price: float) -> None:
        total_cost = self.cost_price * self.quantity + price * qty
        self.quantity += qty
        self.cost_price = total_cost / self.quantity if self.quantity > 0 else 0.0

    def reduce(self, qty: float) -> None:
        self.quantity -= qty
```

- [ ] **Step 5: 运行 portfolio 测试**

```bash
cd python
pytest tests/test_portfolio.py -v
```

Expected: 4 tests PASSED

- [ ] **Step 6: 写 test_analytics.py（先写测试）**

创建 `python/tests/test_analytics.py`：

```python
import math
from backtest.analytics import compute_metrics


def test_flat_equity_returns_zero():
    equity = [1.0] * 10
    m = compute_metrics(equity)
    assert m.total_return == 0.0
    assert m.max_drawdown == 0.0


def test_positive_return():
    # 净值从1涨到2，总收益100%
    equity = [1.0 + i * 0.1 for i in range(11)]  # 1.0 → 2.0
    m = compute_metrics(equity)
    assert abs(m.total_return - 1.0) < 0.01


def test_max_drawdown():
    # 涨到2再跌到1，最大回撤50%
    equity = [1.0, 1.5, 2.0, 1.5, 1.0]
    m = compute_metrics(equity)
    assert abs(m.max_drawdown - 0.5) < 0.01


def test_sharpe_positive_trend():
    # 稳定上涨序列，Sharpe 应 > 0
    equity = [1.0 + i * 0.001 for i in range(242)]
    m = compute_metrics(equity)
    assert m.sharpe_ratio > 0


def test_win_rate():
    # 前5天涨，后5天跌
    equity = [1.0, 1.1, 1.2, 1.3, 1.4, 1.3, 1.2, 1.1, 1.0, 0.9]
    m = compute_metrics(equity)
    # 9个日收益：4正5负
    assert abs(m.win_rate - 4 / 9) < 0.01


def test_calmar_ratio():
    equity = [1.0, 1.5, 2.0, 1.5, 1.0]
    m = compute_metrics(equity)
    # calmar = annual_return / max_drawdown
    if m.max_drawdown > 0:
        assert abs(m.calmar_ratio - m.annual_return / m.max_drawdown) < 0.001
```

- [ ] **Step 7: 运行测试，确认失败**

```bash
cd python
pytest tests/test_analytics.py -v 2>&1 | head -20
```

Expected: ImportError（`backtest.analytics` 不存在）

- [ ] **Step 8: 实现 analytics.py**

创建 `python/backtest/analytics.py`：

```python
import math
from dataclasses import dataclass


@dataclass
class Metrics:
    total_return: float
    annual_return: float
    max_drawdown: float
    sharpe_ratio: float
    win_rate: float
    calmar_ratio: float
    equity_curve: list[float]


def compute_metrics(equity_curve: list[float], trading_days_per_year: int = 242) -> Metrics:
    """
    从净值曲线计算绩效指标。
    equity_curve: 每日净值，初始值通常为 1.0。
    """
    n = len(equity_curve)
    if n < 2:
        return Metrics(0.0, 0.0, 0.0, 0.0, 0.0, 0.0, equity_curve)

    # 日收益率序列
    daily_returns = [
        (equity_curve[i] - equity_curve[i - 1]) / equity_curve[i - 1]
        for i in range(1, n)
    ]

    # 总收益
    total_return = (equity_curve[-1] - equity_curve[0]) / equity_curve[0]

    # 年化收益（几何平均，基于实际天数）
    years = (n - 1) / trading_days_per_year
    if years > 0 and equity_curve[0] > 0:
        annual_return = (equity_curve[-1] / equity_curve[0]) ** (1.0 / years) - 1.0
    else:
        annual_return = 0.0

    # 最大回撤
    max_drawdown = _max_drawdown(equity_curve)

    # 夏普比率（无风险利率=0）
    sharpe_ratio = _sharpe(daily_returns, trading_days_per_year)

    # 胜率
    wins = sum(1 for r in daily_returns if r > 0)
    win_rate = wins / len(daily_returns) if daily_returns else 0.0

    # Calmar
    calmar_ratio = annual_return / max_drawdown if max_drawdown > 0 else 0.0

    return Metrics(
        total_return=total_return,
        annual_return=annual_return,
        max_drawdown=max_drawdown,
        sharpe_ratio=sharpe_ratio,
        win_rate=win_rate,
        calmar_ratio=calmar_ratio,
        equity_curve=list(equity_curve),
    )


def _max_drawdown(equity: list[float]) -> float:
    peak = equity[0]
    max_dd = 0.0
    for v in equity:
        if v > peak:
            peak = v
        dd = (peak - v) / peak if peak > 0 else 0.0
        if dd > max_dd:
            max_dd = dd
    return max_dd


def _sharpe(daily_returns: list[float], trading_days_per_year: int) -> float:
    if not daily_returns:
        return 0.0
    n = len(daily_returns)
    mean = sum(daily_returns) / n
    variance = sum((r - mean) ** 2 for r in daily_returns) / n
    std = math.sqrt(variance)
    if std == 0:
        return 0.0
    return mean / std * math.sqrt(trading_days_per_year)
```

- [ ] **Step 9: 运行所有测试**

```bash
cd python
pytest tests/test_portfolio.py tests/test_analytics.py -v
```

Expected: 10 tests PASSED

- [ ] **Step 10: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add python/backtest/types.py python/backtest/portfolio.py python/backtest/analytics.py
git add python/tests/test_portfolio.py python/tests/test_analytics.py
git commit -m "feat: add Python types, portfolio tracker, and analytics"
```

---

## Task 4: gRPC 客户端 + BacktestRunner

**Files:**
- Create: `python/backtest/client.py`
- Create: `python/backtest/strategy.py`
- Create: `python/backtest/runner.py`
- Create: `python/tests/test_runner.py`

- [ ] **Step 1: 写 test_runner.py（先写测试）**

创建 `python/tests/test_runner.py`：

```python
from datetime import date, datetime, timezone
from unittest.mock import MagicMock, patch
import pytest

from backtest.types import Bar, Order, OrderSide, Fill, FillStatus
from backtest.strategy import Strategy
from backtest.runner import BacktestRunner, BacktestConfig
from backtest.analytics import Metrics


class BuyAndHoldStrategy(Strategy):
    """每次收到第一根 Bar 时全仓买入，之后持有。"""

    def __init__(self):
        self.bought = False

    def on_bar(self, bar: Bar, portfolio) -> list[Order]:
        if self.bought:
            return []
        self.bought = True
        qty = int(portfolio.cash / bar.close / 100) * 100
        return [Order(symbol=bar.symbol, side=OrderSide.BUY, quantity=qty)]


def _make_pb_bar(symbol, date_obj, close=10.0):
    """创建一个 protobuf Bar mock。"""
    dt = datetime(date_obj.year, date_obj.month, date_obj.day, tzinfo=timezone.utc)
    bar = MagicMock()
    bar.symbol = symbol
    bar.date_unix = int(dt.timestamp())
    bar.open = close * 0.99
    bar.high = close * 1.01
    bar.low = close * 0.98
    bar.close = close
    bar.volume = 100_000
    bar.amount = 100_000 * close
    bar.adj_factor = 0.0
    return bar


def _make_pb_fill(symbol, side, qty, price):
    fill = MagicMock()
    fill.symbol = symbol
    fill.side = side
    fill.filled_qty = qty
    fill.filled_price = price
    fill.commission = price * qty * 0.0003
    fill.stamp_duty = 0.0
    from backtest.proto import backtest_pb2
    fill.status = backtest_pb2.FILLED
    fill.reject_reason = ""
    return fill


def test_runner_builds_equity_curve():
    """BacktestRunner 应该返回包含净值曲线的 Metrics。"""
    from backtest.proto import backtest_pb2

    # 构造 5 天 Bar 序列，价格从 10 涨到 11
    bars = [_make_pb_bar("sz000001", date(2020, 1, d), 10.0 + d * 0.2)
            for d in range(1, 6)]

    stub_mock = MagicMock()
    stub_mock.StreamBars.return_value = iter(bars)
    stub_mock.SubmitOrder.return_value = _make_pb_fill(
        "sz000001", backtest_pb2.BUY, 9000, 10.2
    )

    config = BacktestConfig(
        symbols=["sz000001"],
        start_date=date(2020, 1, 1),
        end_date=date(2020, 1, 5),
        init_capital=100_000,
    )
    runner = BacktestRunner(stub=stub_mock, config=config, session_id="test-session")
    metrics = runner.run(BuyAndHoldStrategy())

    assert isinstance(metrics, Metrics)
    assert len(metrics.equity_curve) > 0
    assert metrics.equity_curve[0] == pytest.approx(1.0)


def test_runner_rejected_order_does_not_crash():
    """被拒绝的订单不应使 runner 崩溃。"""
    from backtest.proto import backtest_pb2

    bars = [_make_pb_bar("sz000001", date(2020, 1, d), 10.0) for d in range(1, 4)]

    reject_fill = MagicMock()
    reject_fill.status = backtest_pb2.REJECTED
    reject_fill.reject_reason = "limit_up"

    stub_mock = MagicMock()
    stub_mock.StreamBars.return_value = iter(bars)
    stub_mock.SubmitOrder.return_value = reject_fill

    config = BacktestConfig(
        symbols=["sz000001"],
        start_date=date(2020, 1, 1),
        end_date=date(2020, 1, 3),
        init_capital=100_000,
    )
    runner = BacktestRunner(stub=stub_mock, config=config, session_id="test-session")
    metrics = runner.run(BuyAndHoldStrategy())
    assert metrics is not None
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
cd python
pytest tests/test_runner.py -v 2>&1 | head -20
```

Expected: ImportError（`backtest.client` / `backtest.runner` 不存在）

- [ ] **Step 3: 实现 strategy.py**

创建 `python/backtest/strategy.py`：

```python
from abc import ABC, abstractmethod
from backtest.types import Bar, Order, PortfolioSnapshot


class Strategy(ABC):
    """所有策略必须继承此类并实现 on_bar。"""

    @abstractmethod
    def on_bar(self, bar: Bar, portfolio: PortfolioSnapshot) -> list[Order]:
        """
        接收新 Bar，返回订单列表（空列表 = 不操作）。
        bar: 当前 Bar（只有当前及之前信息，禁止前视）
        portfolio: 当前持仓快照（只读）
        """
        ...

    def on_start(self) -> None:
        """回测开始时调用（可选覆盖）"""

    def on_end(self) -> None:
        """回测结束时调用（可选覆盖）"""
```

- [ ] **Step 4: 实现 client.py**

创建 `python/backtest/client.py`：

```python
import uuid
import grpc
from backtest.proto import backtest_pb2_grpc


class BacktestClient:
    """gRPC 连接封装，自动管理 channel 生命周期。"""

    def __init__(self, host: str = "localhost", port: int = 50051):
        self._channel = grpc.insecure_channel(f"{host}:{port}")
        self.stub = backtest_pb2_grpc.BacktestEngineStub(self._channel)

    def new_session_id(self) -> str:
        return str(uuid.uuid4())

    def close(self) -> None:
        self._channel.close()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()
```

- [ ] **Step 5: 实现 runner.py**

创建 `python/backtest/runner.py`：

```python
from dataclasses import dataclass, field
from datetime import date, datetime, timezone
from typing import Any

import grpc

from backtest.analytics import Metrics, compute_metrics
from backtest.portfolio import PortfolioTracker
from backtest.proto import backtest_pb2
from backtest.strategy import Strategy
from backtest.types import Bar, Fill, FillStatus, Order, OrderSide


@dataclass
class BacktestConfig:
    symbols: list[str]
    start_date: date
    end_date: date
    init_capital: float = 1_000_000.0
    commission: float = 0.0003


class BacktestRunner:
    """
    驱动单次回测：
    1. 通过 StreamBars 从 Go 服务器接收 Bar
    2. 调用策略的 on_bar
    3. 通过 SubmitOrder 提交订单
    4. 本地追踪持仓和净值
    5. 返回 Metrics
    """

    def __init__(self, stub: Any, config: BacktestConfig, session_id: str):
        self._stub = stub
        self._config = config
        self._session_id = session_id

    def run(self, strategy: Strategy) -> Metrics:
        portfolio = PortfolioTracker(self._config.init_capital)
        equity_curve: list[float] = [1.0]

        strategy.on_start()

        stream_req = backtest_pb2.StreamRequest(
            symbols=self._config.symbols,
            start_date=self._config.start_date.isoformat(),
            end_date=self._config.end_date.isoformat(),
        )
        metadata = [("x-session-id", self._session_id)]

        current_day: date | None = None
        day_prices: dict[str, float] = {}

        for pb_bar in self._stub.StreamBars(stream_req, metadata=metadata):
            bar = _pb_to_bar(pb_bar)

            # 新的一天：记录净值
            if current_day is not None and bar.date != current_day:
                nav = portfolio.total_value(day_prices)
                equity_curve.append(nav / self._config.init_capital)
                day_prices = {}

            current_day = bar.date
            day_prices[bar.symbol] = bar.close

            snapshot = portfolio.snapshot(day_prices)
            orders = strategy.on_bar(bar, snapshot)

            for order in orders:
                fill = self._submit_order(order, bar.date)
                portfolio.apply_fill(fill, bar.date)

        # 记录最后一天净值
        if current_day is not None and day_prices:
            nav = portfolio.total_value(day_prices)
            equity_curve.append(nav / self._config.init_capital)

        strategy.on_end()

        return compute_metrics(equity_curve)

    def _submit_order(self, order: Order, trade_date: date) -> Fill:
        dt = datetime(trade_date.year, trade_date.month, trade_date.day, tzinfo=timezone.utc)
        metadata = [
            ("x-session-id", self._session_id),
            ("x-date-unix", str(int(dt.timestamp()))),
        ]
        pb_order = backtest_pb2.Order(
            symbol=order.symbol,
            side=backtest_pb2.OrderSide.Value(order.side.name),
            quantity=order.quantity,
            price=order.price,
        )
        pb_fill = self._stub.SubmitOrder(pb_order, metadata=metadata)
        return _pb_to_fill(pb_fill)


def _pb_to_bar(pb: Any) -> Bar:
    dt = datetime.fromtimestamp(pb.date_unix, tz=timezone.utc).date()
    return Bar(
        symbol=pb.symbol,
        date=dt,
        open=pb.open,
        high=pb.high,
        low=pb.low,
        close=pb.close,
        volume=pb.volume,
        amount=pb.amount,
        adj_factor=pb.adj_factor,
    )


def _pb_to_fill(pb: Any) -> Fill:
    status = FillStatus.FILLED if pb.status == backtest_pb2.FILLED else FillStatus.REJECTED
    return Fill(
        symbol=pb.symbol,
        side=OrderSide(pb.side),
        filled_qty=pb.filled_qty,
        filled_price=pb.filled_price,
        commission=pb.commission,
        stamp_duty=pb.stamp_duty,
        status=status,
        reject_reason=pb.reject_reason,
    )
```

- [ ] **Step 6: 运行 runner 测试**

```bash
cd python
pytest tests/test_runner.py -v
```

Expected: 2 tests PASSED

- [ ] **Step 7: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add python/backtest/strategy.py python/backtest/client.py python/backtest/runner.py
git add python/tests/test_runner.py
git commit -m "feat: add gRPC client, Strategy base class, and BacktestRunner"
```

---

## Task 5: Walk-Forward 验证器

**背景：** Walk-Forward 将完整历史分为多个 [训练, 测试] 窗口，防止策略在单一样本外测试上过拟合。支持 expanding（扩展式）和 rolling（滚动式）两种模式。

**Files:**
- Create: `python/backtest/walk_forward.py`
- Create: `python/tests/test_walk_forward.py`

- [ ] **Step 1: 写 test_walk_forward.py（先写测试）**

创建 `python/tests/test_walk_forward.py`：

```python
from datetime import date
from unittest.mock import MagicMock, patch
import pytest

from backtest.walk_forward import WalkForwardValidator, WalkForwardConfig, WFWindow
from backtest.analytics import Metrics


def _make_metrics(annual_return=0.15, max_drawdown=0.10, sharpe=1.2, win_rate=0.55):
    return Metrics(
        total_return=annual_return,
        annual_return=annual_return,
        max_drawdown=max_drawdown,
        sharpe_ratio=sharpe,
        win_rate=win_rate,
        calmar_ratio=annual_return / max_drawdown if max_drawdown else 0,
        equity_curve=[1.0],
    )


def test_expanding_window_split():
    """Expanding 模式：训练窗口随迭代增长，测试窗口固定。"""
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        mode="expanding",
    )
    wf = WalkForwardValidator(config)
    windows = wf.windows()

    assert len(windows) == 3  # 2015-2017 train/2017-2018 test, 2015-2018/2018-2019, 2015-2019/2019-2020
    # 每个窗口的测试开始日 = 上一个训练结束日
    for w in windows:
        assert w.test_start > w.train_start
        assert w.test_end > w.test_start
    # Expanding：训练窗口起始日固定
    assert all(w.train_start == date(2015, 1, 1) for w in windows)


def test_rolling_window_split():
    """Rolling 模式：训练窗口大小固定，整体向前滑动。"""
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        mode="rolling",
    )
    wf = WalkForwardValidator(config)
    windows = wf.windows()

    assert len(windows) == 3
    # Rolling：训练窗口起始日递增
    train_starts = [w.train_start for w in windows]
    assert train_starts == sorted(set(train_starts))
    assert len(set(train_starts)) == 3


def test_passes_filter_with_good_metrics():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        min_annual_return=0.10,
        min_sharpe=0.8,
        max_drawdown=0.30,
        min_win_rate=0.45,
    )
    wf = WalkForwardValidator(config)
    good = _make_metrics(annual_return=0.20, max_drawdown=0.10, sharpe=1.5, win_rate=0.55)
    assert wf.passes_filter(good) is True


def test_fails_filter_with_bad_drawdown():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        max_drawdown=0.30,
    )
    wf = WalkForwardValidator(config)
    bad = _make_metrics(max_drawdown=0.50)
    assert wf.passes_filter(bad) is False


def test_run_aggregates_test_metrics():
    """run() 应返回所有测试窗口指标的汇总。"""
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2018, 1, 1),
        train_years=1,
        test_years=1,
        mode="expanding",
    )
    wf = WalkForwardValidator(config)

    mock_runner_factory = MagicMock()
    mock_runner = MagicMock()
    mock_runner.run.return_value = _make_metrics()
    mock_runner_factory.return_value = mock_runner

    from backtest.strategy import Strategy

    class DummyStrategy(Strategy):
        def on_bar(self, bar, portfolio):
            return []

    result = wf.run(DummyStrategy, mock_runner_factory)
    assert result.num_windows == 2
    assert len(result.test_metrics) == 2
    assert result.avg_annual_return > 0
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
cd python
pytest tests/test_walk_forward.py -v 2>&1 | head -20
```

Expected: ImportError（`backtest.walk_forward` 不存在）

- [ ] **Step 3: 实现 walk_forward.py**

创建 `python/backtest/walk_forward.py`：

```python
from dataclasses import dataclass, field
from datetime import date
from typing import Callable, Type
import math

from backtest.analytics import Metrics
from backtest.strategy import Strategy


@dataclass
class WalkForwardConfig:
    start_date: date
    end_date: date
    train_years: int
    test_years: int
    mode: str = "expanding"          # "expanding" 或 "rolling"
    min_annual_return: float = 0.10  # 最低年化收益
    min_sharpe: float = 0.80         # 最低 Sharpe
    max_drawdown: float = 0.30       # 最大允许回撤（上限）
    min_win_rate: float = 0.45       # 最低胜率


@dataclass
class WFWindow:
    window_id: int
    train_start: date
    train_end: date
    test_start: date
    test_end: date


@dataclass
class WalkForwardResult:
    num_windows: int
    test_metrics: list[Metrics]
    avg_annual_return: float
    avg_max_drawdown: float
    avg_sharpe: float
    passes_filter: bool


class WalkForwardValidator:
    def __init__(self, config: WalkForwardConfig):
        self._cfg = config

    def windows(self) -> list[WFWindow]:
        """生成 Walk-Forward 窗口列表。"""
        from dateutil.relativedelta import relativedelta

        result = []
        cfg = self._cfg
        test_delta = relativedelta(years=cfg.test_years)
        train_delta = relativedelta(years=cfg.train_years)

        window_id = 0
        test_start = cfg.start_date + train_delta
        while True:
            test_end = test_start + test_delta
            if test_end > cfg.end_date:
                break

            if cfg.mode == "expanding":
                train_start = cfg.start_date
            else:  # rolling
                train_start = test_start - train_delta

            result.append(WFWindow(
                window_id=window_id,
                train_start=train_start,
                train_end=test_start,
                test_start=test_start,
                test_end=test_end,
            ))
            window_id += 1
            test_start = test_end  # 下一窗口测试期从此处开始

        return result

    def passes_filter(self, metrics: Metrics) -> bool:
        """检查单个窗口指标是否通过准入门槛。"""
        cfg = self._cfg
        return (
            metrics.annual_return >= cfg.min_annual_return
            and metrics.sharpe_ratio >= cfg.min_sharpe
            and metrics.max_drawdown <= cfg.max_drawdown
            and metrics.win_rate >= cfg.min_win_rate
        )

    def run(
        self,
        strategy_cls: Type[Strategy],
        runner_factory: Callable[[WFWindow, str], object],  # (window, phase) → BacktestRunner
    ) -> WalkForwardResult:
        """
        对每个窗口运行训练期和测试期回测，返回测试期汇总指标。

        runner_factory(window, phase) 需返回配置好对应日期的 BacktestRunner。
        phase 为 "train" 或 "test"。
        """
        windows = self.windows()
        test_metrics: list[Metrics] = []

        for window in windows:
            # 训练期：可在此处做参数优化（当前版本直接用默认参数）
            train_runner = runner_factory(window, "train")
            train_runner.run(strategy_cls())

            # 测试期：评估真实样本外表现
            test_runner = runner_factory(window, "test")
            metrics = test_runner.run(strategy_cls())
            test_metrics.append(metrics)

        if not test_metrics:
            return WalkForwardResult(
                num_windows=0,
                test_metrics=[],
                avg_annual_return=0.0,
                avg_max_drawdown=0.0,
                avg_sharpe=0.0,
                passes_filter=False,
            )

        avg_annual = sum(m.annual_return for m in test_metrics) / len(test_metrics)
        avg_dd = sum(m.max_drawdown for m in test_metrics) / len(test_metrics)
        avg_sharpe = sum(m.sharpe_ratio for m in test_metrics) / len(test_metrics)

        summary_metrics = Metrics(
            total_return=avg_annual,
            annual_return=avg_annual,
            max_drawdown=avg_dd,
            sharpe_ratio=avg_sharpe,
            win_rate=sum(m.win_rate for m in test_metrics) / len(test_metrics),
            calmar_ratio=avg_annual / avg_dd if avg_dd > 0 else 0.0,
            equity_curve=[1.0],
        )

        return WalkForwardResult(
            num_windows=len(test_metrics),
            test_metrics=test_metrics,
            avg_annual_return=avg_annual,
            avg_max_drawdown=avg_dd,
            avg_sharpe=avg_sharpe,
            passes_filter=self.passes_filter(summary_metrics),
        )
```

**注意：** `walk_forward.py` 用到了 `python-dateutil`，需要在 `pyproject.toml` dependencies 中加入 `"python-dateutil>=2.9"`，然后 `pip install -e .`。

- [ ] **Step 4: 添加 dateutil 依赖**

编辑 `python/pyproject.toml`，在 `dependencies` 列表中添加 `"python-dateutil>=2.9"`:

```toml
dependencies = [
    "grpcio>=1.62.0",
    "grpcio-tools>=1.62.0",
    "protobuf>=4.25.0",
    "python-dateutil>=2.9",
]
```

```bash
cd python && pip install -e ".[dev]"
```

- [ ] **Step 5: 运行所有测试**

```bash
cd python
pytest tests/ -v
```

Expected: 全部 PASS（test_portfolio + test_analytics + test_runner + test_walk_forward）

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add python/backtest/walk_forward.py python/tests/test_walk_forward.py python/pyproject.toml
git commit -m "feat: add Walk-Forward validator with expanding/rolling window support"
```

---

## Task 6: 示例策略 + 最终验证

**Files:**
- Create: `python/strategies/ma_cross.py`

- [ ] **Step 1: 实现 MA 双均线策略**

创建 `python/strategies/ma_cross.py`：

```python
from collections import deque
from backtest.types import Bar, Order, OrderSide, PortfolioSnapshot
from backtest.strategy import Strategy


class MACrossStrategy(Strategy):
    """
    双均线金叉/死叉策略（使用复权价格）。
    - 快线上穿慢线（金叉）→ 全仓买入
    - 快线下穿慢线（死叉）→ 全部卖出
    """

    def __init__(self, fast: int = 5, slow: int = 20, position_pct: float = 0.95):
        self.fast = fast
        self.slow = slow
        self.position_pct = position_pct
        self._prices: dict[str, deque] = {}
        self._prev_fast: dict[str, float] = {}
        self._prev_slow: dict[str, float] = {}

    def on_bar(self, bar: Bar, portfolio: PortfolioSnapshot) -> list[Order]:
        sym = bar.symbol
        price = bar.adj_close  # 使用复权价格

        if sym not in self._prices:
            self._prices[sym] = deque(maxlen=self.slow)

        self._prices[sym].append(price)

        prices = list(self._prices[sym])
        if len(prices) < self.slow:
            return []  # 数据不足，等待

        fast_ma = sum(prices[-self.fast :]) / self.fast
        slow_ma = sum(prices) / self.slow

        prev_fast = self._prev_fast.get(sym, fast_ma)
        prev_slow = self._prev_slow.get(sym, slow_ma)
        self._prev_fast[sym] = fast_ma
        self._prev_slow[sym] = slow_ma

        orders = []
        pos = portfolio.positions.get(sym)

        # 金叉：快线从下穿过慢线
        if prev_fast <= prev_slow and fast_ma > slow_ma:
            if not pos or pos.quantity == 0:
                qty = int(portfolio.cash * self.position_pct / price / 100) * 100
                if qty >= 100:
                    orders.append(Order(symbol=sym, side=OrderSide.BUY, quantity=qty))

        # 死叉：快线从上穿过慢线
        elif prev_fast >= prev_slow and fast_ma < slow_ma:
            if pos and pos.quantity > 0:
                orders.append(Order(symbol=sym, side=OrderSide.SELL, quantity=pos.quantity))

        return orders
```

- [ ] **Step 2: 运行全部测试**

```bash
cd python
pytest tests/ -v --tb=short
```

Expected: 所有测试 PASS

- [ ] **Step 3: 验证 MA 策略可以被正确实例化**

```bash
cd python
python -c "
from strategies.ma_cross import MACrossStrategy
from backtest.walk_forward import WalkForwardValidator, WalkForwardConfig
from datetime import date
s = MACrossStrategy(fast=5, slow=20)
cfg = WalkForwardConfig(date(2015,1,1), date(2020,1,1), train_years=2, test_years=1)
wf = WalkForwardValidator(cfg)
print('windows:', len(wf.windows()))
print('ok')
"
```

Expected: `windows: 3` 和 `ok`

- [ ] **Step 4: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add python/strategies/ma_cross.py
git commit -m "feat: add MA crossover example strategy"
```

---

## 自检：Spec 覆盖确认

| Spec 要求 | 对应任务 |
|----------|---------|
| Python 策略通过 gRPC client 获取数据，禁止直接读文件 | Task 4 runner.py |
| 策略必须实现统一接口 on_bar(bar, portfolio) | Task 4 strategy.py |
| gRPC 强类型接口（不用 HTTP/JSON） | Task 2 proto stubs |
| Session 隔离（每次回测独立上下文） | Task 1 session management |
| T+1 规则在撮合层强制执行 | Task 1 SubmitOrder |
| 涨跌停规则 | Task 1 → matcher.Match |
| 复权价格使用 adj_close | Task 3 types.py + Task 6 策略 |
| Walk-Forward 防过拟合 | Task 5 |
| 绩效指标（Sharpe/DrawDown/WinRate/Calmar） | Task 3 analytics.py |
| 禁止在测试中使用真实数据文件 | Task 3/4/5 所有测试用 mock |
