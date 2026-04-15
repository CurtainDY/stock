# A股回测系统 Phase 1：数据层 + Go回测引擎 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建可运行单策略回测的数据层和Go回测引擎，通过gRPC暴露接口供Python策略调用。

**Architecture:** 事件驱动模型（参考vnpy CTA引擎设计）——引擎按时间顺序推送Bar事件，Python策略响应并提交Order，Go撮合引擎处理成交并更新持仓，严格防止前视偏差。数据层参考QUANTAXIS的Parquet分片存储方案，按年分片日频数据，读取性能优秀。

**Tech Stack:** Go 1.22+, Protocol Buffers 3, gRPC, Apache Parquet (parquet-go), Python 3.11+, grpcio, pandas, pytest

---

## 参考资料

- **vnpy CTA引擎：** https://github.com/vnpy/vnpy — 参考 `vnpy/app/cta_backtester/` 的事件驱动回测结构
- **QUANTAXIS数据层：** https://github.com/QUANTAXIS/QUANTAXIS — 参考 `QUANTAXIS/QAFetch/` 数据获取和 `QUANTAXIS/QAData/` 数据结构
- **A股规则参考：** 涨跌停10%（ST股5%），T+1，印花税0.1%单向，佣金双向0.03%

---

## 文件结构

```
stock/
├── backtest-engine/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go              # gRPC服务启动入口
│   ├── internal/
│   │   ├── data/
│   │   │   ├── bar.go               # Bar数据结构定义
│   │   │   ├── store.go             # DataStore接口定义
│   │   │   ├── parquet_store.go     # Parquet文件读取实现
│   │   │   └── store_test.go        # DataStore测试
│   │   ├── engine/
│   │   │   ├── engine.go            # 主回测循环
│   │   │   ├── engine_test.go       # 引擎集成测试
│   │   │   └── eventbus.go          # 事件总线（Bar/Order/Fill事件）
│   │   ├── matcher/
│   │   │   ├── matcher.go           # 撮合引擎（含A股规则）
│   │   │   └── matcher_test.go      # 撮合规则单元测试
│   │   ├── portfolio/
│   │   │   ├── portfolio.go         # 持仓、资金、手续费
│   │   │   └── portfolio_test.go
│   │   └── analytics/
│   │       ├── analytics.go         # 绩效指标计算
│   │       └── analytics_test.go
│   ├── proto/
│   │   └── backtest.proto           # gRPC服务定义
│   ├── go.mod
│   └── go.sum
├── data/
│   ├── raw/                         # 原始数据（不修改）
│   ├── normalized/
│   │   └── daily/                   # 按年分片的Parquet文件
│   └── testdata/                    # 测试用小样本数据
├── tools/
│   └── normalize/
│       ├── main.go                  # 数据标准化CLI工具
│       └── main_test.go
└── python/
    ├── client/
    │   ├── backtest_client.py       # gRPC客户端封装
    │   └── test_backtest_client.py  # Python端集成测试
    └── strategies/
        └── example_ma_strategy.py  # 示例均线策略（用于验证）
```

---

## Task 1: 项目初始化

**Files:**
- Create: `backtest-engine/go.mod`
- Create: `backtest-engine/proto/backtest.proto`

- [ ] **Step 1: 初始化Go模块**

```bash
cd /Users/parsedong/workSpace/stock
mkdir -p backtest-engine/cmd/server
mkdir -p backtest-engine/internal/{data,engine,matcher,portfolio,analytics}
mkdir -p backtest-engine/proto
mkdir -p data/{raw,normalized/daily,testdata}
mkdir -p tools/normalize
mkdir -p python/{client,strategies}
cd backtest-engine
go mod init github.com/parsedong/stock/backtest-engine
```

- [ ] **Step 2: 安装依赖**

```bash
cd backtest-engine
go get github.com/parquet-go/parquet-go@latest
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get github.com/shopspring/decimal@latest
```

- [ ] **Step 3: 安装protoc和Go插件**

```bash
brew install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

- [ ] **Step 4: 写 backtest.proto**

```protobuf
syntax = "proto3";
package backtest;
option go_package = "github.com/parsedong/stock/backtest-engine/proto";

// 单根K线
message Bar {
  string symbol     = 1;  // 股票代码，如 "000001.SZ"
  int64  date_unix  = 2;  // Unix时间戳（秒）
  double open       = 3;
  double high       = 4;
  double low        = 5;
  double close      = 6;
  double volume     = 7;  // 成交量（股）
  double amount     = 8;  // 成交额（元）
  double adj_factor = 9;  // 复权因子
}

// 订单方向
enum OrderSide {
  BUY  = 0;
  SELL = 1;
}

// 订单状态
enum OrderStatus {
  PENDING   = 0;
  FILLED    = 1;
  REJECTED  = 2;  // 涨跌停、停牌、资金不足等
}

// 订单请求
message Order {
  string     symbol   = 1;
  OrderSide  side     = 2;
  double     quantity = 3;  // 手数（100股/手）
  double     price    = 4;  // 0表示市价单
}

// 成交回报
message Fill {
  string    symbol        = 1;
  OrderSide side          = 2;
  double    filled_qty    = 3;
  double    filled_price  = 4;
  double    commission    = 5;  // 手续费
  double    stamp_duty    = 6;  // 印花税（仅卖出）
  OrderStatus status      = 7;
  string    reject_reason = 8;  // REJECTED时说明原因
}

// 回测请求
message BacktestRequest {
  repeated string symbols      = 1;  // 标的列表
  string          start_date   = 2;  // "2020-01-01"
  string          end_date     = 3;  // "2023-12-31"
  double          init_capital = 4;  // 初始资金（元）
  double          commission   = 5;  // 佣金率，默认0.0003
}

// 绩效结果
message BacktestResult {
  double annual_return   = 1;  // 年化收益率
  double max_drawdown    = 2;  // 最大回撤（正数，如0.15表示15%）
  double sharpe_ratio    = 3;
  double win_rate        = 4;
  double avg_hold_days   = 5;
  double turnover_rate   = 6;  // 年换手率
  double calmar_ratio    = 7;
  double total_return    = 8;  // 总收益率
  repeated double equity_curve = 9;  // 每日净值曲线
}

// 流式Bar请求
message StreamRequest {
  repeated string symbols    = 1;
  string          start_date = 2;
  string          end_date   = 3;
}

service BacktestEngine {
  rpc RunBacktest(BacktestRequest)   returns (BacktestResult);
  rpc StreamBars(StreamRequest)      returns (stream Bar);
  rpc SubmitOrder(Order)             returns (Fill);
}
```

- [ ] **Step 5: 生成Go代码**

```bash
cd backtest-engine
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/backtest.proto
```

Expected: 生成 `proto/backtest.pb.go` 和 `proto/backtest_grpc.pb.go`

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git init
git add backtest-engine/ data/ tools/ python/
git commit -m "chore: initialize project structure and proto definitions"
```

---

## Task 2: Bar数据结构和DataStore接口

**Files:**
- Create: `backtest-engine/internal/data/bar.go`
- Create: `backtest-engine/internal/data/store.go`
- Create: `backtest-engine/internal/data/store_test.go`

- [ ] **Step 1: 写 bar.go**

```go
package data

import "time"

// Bar 代表一根K线（日频或分钟频）
type Bar struct {
    Symbol    string
    Date      time.Time
    Open      float64
    High      float64
    Low       float64
    Close     float64
    Volume    float64 // 成交量（股）
    Amount    float64 // 成交额（元）
    AdjFactor float64 // 复权因子，未复权时为1.0
}

// AdjClose 返回前复权收盘价
func (b Bar) AdjClose() float64 {
    if b.AdjFactor == 0 {
        return b.Close
    }
    return b.Close * b.AdjFactor
}

// IsLimit 判断是否涨跌停（涨跌幅超过9.9%视为涨跌停，ST股为4.9%）
func (b Bar) IsLimitUp(prevClose float64, isST bool) bool {
    if prevClose == 0 {
        return false
    }
    limit := 0.099
    if isST {
        limit = 0.049
    }
    return (b.Close-prevClose)/prevClose >= limit
}

func (b Bar) IsLimitDown(prevClose float64, isST bool) bool {
    if prevClose == 0 {
        return false
    }
    limit := 0.099
    if isST {
        limit = 0.049
    }
    return (prevClose-b.Close)/prevClose >= limit
}
```

- [ ] **Step 2: 写 store.go（接口定义）**

```go
package data

import "time"

// DataStore 定义数据访问接口，支持多种后端（Parquet、数据库等）
type DataStore interface {
    // LoadBars 加载指定标的在时间范围内的所有Bar，按日期升序返回
    LoadBars(symbol string, start, end time.Time) ([]Bar, error)

    // LoadBarsByDate 加载指定日期所有标的的Bar（截面数据）
    LoadBarsByDate(date time.Time, symbols []string) (map[string]Bar, error)

    // Symbols 返回所有可用标的列表
    Symbols() ([]string, error)

    // TradingDays 返回时间范围内的交易日列表
    TradingDays(start, end time.Time) ([]time.Time, error)
}
```

- [ ] **Step 3: 写失败测试**

```go
// store_test.go
package data_test

import (
    "testing"
    "time"
    "github.com/parsedong/stock/backtest-engine/internal/data"
)

// MockStore 用于测试
type MockStore struct {
    bars map[string][]data.Bar
}

func (m *MockStore) LoadBars(symbol string, start, end time.Time) ([]data.Bar, error) {
    return m.bars[symbol], nil
}

func (m *MockStore) LoadBarsByDate(date time.Time, symbols []string) (map[string]data.Bar, error) {
    result := make(map[string]data.Bar)
    for _, sym := range symbols {
        for _, b := range m.bars[sym] {
            if b.Date.Equal(date) {
                result[sym] = b
            }
        }
    }
    return result, nil
}

func (m *MockStore) Symbols() ([]string, error) {
    keys := make([]string, 0, len(m.bars))
    for k := range m.bars {
        keys = append(keys, k)
    }
    return keys, nil
}

func (m *MockStore) TradingDays(start, end time.Time) ([]time.Time, error) {
    seen := make(map[string]time.Time)
    for _, bars := range m.bars {
        for _, b := range bars {
            if !b.Date.Before(start) && !b.Date.After(end) {
                seen[b.Date.Format("2006-01-02")] = b.Date
            }
        }
    }
    days := make([]time.Time, 0, len(seen))
    for _, d := range seen {
        days = append(days, d)
    }
    return days, nil
}

func TestBarAdjClose(t *testing.T) {
    b := data.Bar{Close: 10.0, AdjFactor: 0.9}
    want := 9.0
    got := b.AdjClose()
    if got != want {
        t.Errorf("AdjClose() = %v, want %v", got, want)
    }
}

func TestBarAdjCloseNoFactor(t *testing.T) {
    b := data.Bar{Close: 10.0, AdjFactor: 0}
    want := 10.0
    got := b.AdjClose()
    if got != want {
        t.Errorf("AdjClose() with zero factor = %v, want %v", got, want)
    }
}

func TestIsLimitUp(t *testing.T) {
    b := data.Bar{Close: 11.0}
    if !b.IsLimitUp(10.0, false) {
        t.Error("should be limit up")
    }
    if b.IsLimitUp(10.0, true) { // ST股涨幅不够
        t.Error("should not be ST limit up at 10%")
    }
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd backtest-engine
go test ./internal/data/... -v
```

Expected: PASS（MockStore实现了接口，Bar方法正确）

- [ ] **Step 5: Commit**

```bash
git add backtest-engine/internal/data/
git commit -m "feat: add Bar struct and DataStore interface"
```

---

## Task 3: Parquet数据读取实现

**Files:**
- Create: `backtest-engine/internal/data/parquet_store.go`
- Create: `data/testdata/README.md`（说明测试数据格式）

> **参考QUANTAXIS：** QUANTAXIS使用按年分片的Parquet文件，列名为 `code, date, open, high, low, close, vol, amount`。我们的字段略有不同，normalize工具（Task 5）负责转换。

- [ ] **Step 1: 定义Parquet行结构（与文件列对应）**

```go
// parquet_store.go
package data

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "time"

    "github.com/parquet-go/parquet-go"
)

// parquetRow 对应Parquet文件中的一行，字段名必须与文件列名匹配
type parquetRow struct {
    Symbol    string  `parquet:"symbol"`
    DateStr   string  `parquet:"date"`    // "2020-01-02"
    Open      float64 `parquet:"open"`
    High      float64 `parquet:"high"`
    Low       float64 `parquet:"low"`
    Close     float64 `parquet:"close"`
    Volume    float64 `parquet:"volume"`
    Amount    float64 `parquet:"amount"`
    AdjFactor float64 `parquet:"adj_factor"`
}

func (r parquetRow) toBar() (Bar, error) {
    date, err := time.Parse("2006-01-02", r.DateStr)
    if err != nil {
        return Bar{}, fmt.Errorf("parse date %q: %w", r.DateStr, err)
    }
    return Bar{
        Symbol:    r.Symbol,
        Date:      date,
        Open:      r.Open,
        High:      r.High,
        Low:       r.Low,
        Close:     r.Close,
        Volume:    r.Volume,
        Amount:    r.Amount,
        AdjFactor: r.AdjFactor,
    }, nil
}
```

- [ ] **Step 2: 实现 ParquetStore**

```go
// ParquetStore 从按年分片的Parquet文件加载行情数据
// 文件路径格式：{dataDir}/daily/{year}.parquet
type ParquetStore struct {
    dataDir string // 例如 "/Users/parsedong/workSpace/stock/data/normalized"
}

func NewParquetStore(dataDir string) *ParquetStore {
    return &ParquetStore{dataDir: dataDir}
}

func (s *ParquetStore) LoadBars(symbol string, start, end time.Time) ([]Bar, error) {
    var result []Bar
    for year := start.Year(); year <= end.Year(); year++ {
        path := filepath.Join(s.dataDir, "daily", fmt.Sprintf("%d.parquet", year))
        bars, err := s.loadFromFile(path, symbol, start, end)
        if err != nil {
            if os.IsNotExist(err) {
                continue // 该年份文件不存在，跳过
            }
            return nil, fmt.Errorf("load %d: %w", year, err)
        }
        result = append(result, bars...)
    }
    sort.Slice(result, func(i, j int) bool {
        return result[i].Date.Before(result[j].Date)
    })
    return result, nil
}

func (s *ParquetStore) loadFromFile(path, symbol string, start, end time.Time) ([]Bar, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    reader := parquet.NewGenericReader[parquetRow](f)
    defer reader.Close()

    var bars []Bar
    rows := make([]parquetRow, 1000)
    for {
        n, err := reader.Read(rows)
        for i := 0; i < n; i++ {
            row := rows[i]
            if row.Symbol != symbol {
                continue
            }
            bar, parseErr := row.toBar()
            if parseErr != nil {
                return nil, parseErr
            }
            if bar.Date.Before(start) || bar.Date.After(end) {
                continue
            }
            bars = append(bars, bar)
        }
        if err != nil {
            break // io.EOF 或其他错误
        }
    }
    return bars, nil
}

func (s *ParquetStore) LoadBarsByDate(date time.Time, symbols []string) (map[string]Bar, error) {
    symSet := make(map[string]struct{}, len(symbols))
    for _, sym := range symbols {
        symSet[sym] = struct{}{}
    }

    path := filepath.Join(s.dataDir, "daily", fmt.Sprintf("%d.parquet", date.Year()))
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    reader := parquet.NewGenericReader[parquetRow](f)
    defer reader.Close()

    result := make(map[string]Bar)
    rows := make([]parquetRow, 1000)
    for {
        n, err := reader.Read(rows)
        for i := 0; i < n; i++ {
            row := rows[i]
            if _, ok := symSet[row.Symbol]; !ok {
                continue
            }
            bar, parseErr := row.toBar()
            if parseErr != nil {
                return nil, parseErr
            }
            if bar.Date.Equal(date) {
                result[row.Symbol] = bar
            }
        }
        if err != nil {
            break
        }
    }
    return result, nil
}

func (s *ParquetStore) Symbols() ([]string, error) {
    // 扫描最新年份文件获取标的列表
    entries, err := os.ReadDir(filepath.Join(s.dataDir, "daily"))
    if err != nil {
        return nil, err
    }
    if len(entries) == 0 {
        return nil, nil
    }
    // 使用最后一个文件
    latestFile := filepath.Join(s.dataDir, "daily", entries[len(entries)-1].Name())
    f, err := os.Open(latestFile)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    reader := parquet.NewGenericReader[parquetRow](f)
    defer reader.Close()

    seen := make(map[string]struct{})
    rows := make([]parquetRow, 1000)
    for {
        n, err := reader.Read(rows)
        for i := 0; i < n; i++ {
            seen[rows[i].Symbol] = struct{}{}
        }
        if err != nil {
            break
        }
    }
    syms := make([]string, 0, len(seen))
    for sym := range seen {
        syms = append(syms, sym)
    }
    sort.Strings(syms)
    return syms, nil
}

func (s *ParquetStore) TradingDays(start, end time.Time) ([]time.Time, error) {
    // 用第一个标的的日期列表代表交易日
    syms, err := s.Symbols()
    if err != nil || len(syms) == 0 {
        return nil, err
    }
    bars, err := s.LoadBars(syms[0], start, end)
    if err != nil {
        return nil, err
    }
    days := make([]time.Time, len(bars))
    for i, b := range bars {
        days[i] = b.Date
    }
    return days, nil
}
```

- [ ] **Step 3: 写测试（需要先生成测试数据）**

```go
// parquet_store_test.go
package data_test

import (
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/parquet-go/parquet-go"
    "github.com/parsedong/stock/backtest-engine/internal/data"
)

func setupTestParquet(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    dailyDir := filepath.Join(dir, "daily")
    os.MkdirAll(dailyDir, 0755)

    type row struct {
        Symbol    string  `parquet:"symbol"`
        DateStr   string  `parquet:"date"`
        Open      float64 `parquet:"open"`
        High      float64 `parquet:"high"`
        Low       float64 `parquet:"low"`
        Close     float64 `parquet:"close"`
        Volume    float64 `parquet:"volume"`
        Amount    float64 `parquet:"amount"`
        AdjFactor float64 `parquet:"adj_factor"`
    }

    rows := []row{
        {"000001.SZ", "2020-01-02", 14.0, 14.5, 13.8, 14.2, 1000000, 14000000, 1.0},
        {"000001.SZ", "2020-01-03", 14.2, 14.8, 14.0, 14.6, 1200000, 17000000, 1.0},
        {"000002.SZ", "2020-01-02", 28.0, 29.0, 27.5, 28.5, 500000, 14000000, 1.0},
    }

    f, _ := os.Create(filepath.Join(dailyDir, "2020.parquet"))
    defer f.Close()
    parquet.Write(f, rows)
    return dir
}

func TestParquetStoreLoadBars(t *testing.T) {
    dir := setupTestParquet(t)
    store := data.NewParquetStore(dir)

    start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
    end   := time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)

    bars, err := store.LoadBars("000001.SZ", start, end)
    if err != nil {
        t.Fatal(err)
    }
    if len(bars) != 2 {
        t.Errorf("want 2 bars, got %d", len(bars))
    }
    if bars[0].Close != 14.2 {
        t.Errorf("first bar close = %v, want 14.2", bars[0].Close)
    }
}

func TestParquetStoreSymbols(t *testing.T) {
    dir := setupTestParquet(t)
    store := data.NewParquetStore(dir)

    syms, err := store.Symbols()
    if err != nil {
        t.Fatal(err)
    }
    if len(syms) != 2 {
        t.Errorf("want 2 symbols, got %d", len(syms))
    }
}
```

- [ ] **Step 4: 运行测试**

```bash
cd backtest-engine
go test ./internal/data/... -v -run TestParquet
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backtest-engine/internal/data/
git commit -m "feat: implement ParquetStore for reading annual-sharded parquet files"
```

---

## Task 4: 撮合引擎（A股规则）

**Files:**
- Create: `backtest-engine/internal/matcher/matcher.go`
- Create: `backtest-engine/internal/matcher/matcher_test.go`

> **参考vnpy：** vnpy的 `BacktestingEngine._cross_limit_order()` 处理涨跌停逻辑，我们参考其判断方式。

- [ ] **Step 1: 写失败测试（先定义预期行为）**

```go
// matcher_test.go
package matcher_test

import (
    "testing"
    "time"

    "github.com/parsedong/stock/backtest-engine/internal/data"
    "github.com/parsedong/stock/backtest-engine/internal/matcher"
)

func bar(close, prevClose float64) data.Bar {
    return data.Bar{
        Symbol: "000001.SZ",
        Date:   time.Now(),
        Open:   close,
        High:   close * 1.01,
        Low:    close * 0.99,
        Close:  close,
        Volume: 1000000,
        Amount: close * 1000000,
    }
}

func TestBuyNormal(t *testing.T) {
    m := matcher.New(matcher.Config{Commission: 0.0003})
    b := bar(10.0, 9.5)
    order := matcher.Order{Symbol: "000001.SZ", Side: matcher.Buy, Quantity: 1000, Price: 0}

    fill := m.Match(order, b, 9.5, false)
    if fill.Status != matcher.Filled {
        t.Fatalf("expected filled, got %v: %s", fill.Status, fill.RejectReason)
    }
    if fill.FilledPrice != 10.0 {
        t.Errorf("filled price = %v, want 10.0", fill.FilledPrice)
    }
    wantCommission := 10.0 * 1000 * 0.0003
    if fill.Commission != wantCommission {
        t.Errorf("commission = %v, want %v", fill.Commission, wantCommission)
    }
    if fill.StampDuty != 0 {
        t.Error("buy should have no stamp duty")
    }
}

func TestSellStampDuty(t *testing.T) {
    m := matcher.New(matcher.Config{Commission: 0.0003})
    b := bar(10.0, 9.5)
    order := matcher.Order{Symbol: "000001.SZ", Side: matcher.Sell, Quantity: 1000, Price: 0}

    fill := m.Match(order, b, 9.5, false)
    if fill.Status != matcher.Filled {
        t.Fatalf("expected filled, got %v", fill.Status)
    }
    wantStampDuty := 10.0 * 1000 * 0.001
    if fill.StampDuty != wantStampDuty {
        t.Errorf("stamp duty = %v, want %v", fill.StampDuty, wantStampDuty)
    }
}

func TestBuyLimitUpRejected(t *testing.T) {
    m := matcher.New(matcher.Config{Commission: 0.0003})
    // prevClose=10, close=11 → 涨停10%
    b := bar(11.0, 10.0)
    order := matcher.Order{Symbol: "000001.SZ", Side: matcher.Buy, Quantity: 1000, Price: 0}

    fill := m.Match(order, b, 10.0, false)
    if fill.Status != matcher.Rejected {
        t.Errorf("expected rejected at limit up, got %v", fill.Status)
    }
}

func TestSellLimitDownRejected(t *testing.T) {
    m := matcher.New(matcher.Config{Commission: 0.0003})
    // prevClose=10, close=9 → 跌停10%
    b := bar(9.0, 10.0)
    order := matcher.Order{Symbol: "000001.SZ", Side: matcher.Sell, Quantity: 1000, Price: 0}

    fill := m.Match(order, b, 10.0, false)
    if fill.Status != matcher.Rejected {
        t.Errorf("expected rejected at limit down, got %v", fill.Status)
    }
}

func TestSTLimitUp(t *testing.T) {
    m := matcher.New(matcher.Config{Commission: 0.0003})
    // ST股涨幅5%就是涨停
    b := bar(10.5, 10.0)
    order := matcher.Order{Symbol: "000001.SZ", Side: matcher.Buy, Quantity: 1000, Price: 0}

    fill := m.Match(order, b, 10.0, true) // isST=true
    if fill.Status != matcher.Rejected {
        t.Errorf("ST stock: expected rejected at 5%% up, got %v", fill.Status)
    }
}
```

- [ ] **Step 2: 运行确认测试失败**

```bash
cd backtest-engine
go test ./internal/matcher/... -v
```

Expected: compilation error（matcher包不存在）

- [ ] **Step 3: 实现 matcher.go**

```go
package matcher

import "github.com/parsedong/stock/backtest-engine/internal/data"

type Side int

const (
    Buy  Side = 0
    Sell Side = 1
)

type Status int

const (
    Filled   Status = 0
    Rejected Status = 1
)

type Order struct {
    Symbol   string
    Side     Side
    Quantity float64 // 股数
    Price    float64 // 0=市价单，使用收盘价成交
}

type Fill struct {
    Symbol       string
    Side         Side
    FilledQty    float64
    FilledPrice  float64
    Commission   float64 // 佣金
    StampDuty    float64 // 印花税（仅卖出）
    Status       Status
    RejectReason string
}

type Config struct {
    Commission float64 // 佣金率，如 0.0003
    StampDuty  float64 // 印花税率，默认 0.001（卖出单向）
}

type Matcher struct {
    cfg Config
}

func New(cfg Config) *Matcher {
    if cfg.StampDuty == 0 {
        cfg.StampDuty = 0.001
    }
    return &Matcher{cfg: cfg}
}

// Match 尝试撮合一笔订单
// prevClose: 前一交易日收盘价（用于判断涨跌停）
// isST: 是否为ST/SST股票
func (m *Matcher) Match(order Order, bar data.Bar, prevClose float64, isST bool) Fill {
    // 判断涨跌停
    if order.Side == Buy && bar.IsLimitUp(prevClose, isST) {
        return Fill{
            Symbol:       order.Symbol,
            Side:         order.Side,
            Status:       Rejected,
            RejectReason: "limit_up: cannot buy at limit up price",
        }
    }
    if order.Side == Sell && bar.IsLimitDown(prevClose, isST) {
        return Fill{
            Symbol:       order.Symbol,
            Side:         order.Side,
            Status:       Rejected,
            RejectReason: "limit_down: cannot sell at limit down price",
        }
    }

    // 成交价：市价单用收盘价
    price := order.Price
    if price == 0 {
        price = bar.Close
    }

    commission := price * order.Quantity * m.cfg.Commission
    stampDuty := 0.0
    if order.Side == Sell {
        stampDuty = price * order.Quantity * m.cfg.StampDuty
    }

    return Fill{
        Symbol:      order.Symbol,
        Side:        order.Side,
        FilledQty:   order.Quantity,
        FilledPrice: price,
        Commission:  commission,
        StampDuty:   stampDuty,
        Status:      Filled,
    }
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd backtest-engine
go test ./internal/matcher/... -v
```

Expected: 所有测试 PASS

- [ ] **Step 5: Commit**

```bash
git add backtest-engine/internal/matcher/
git commit -m "feat: implement matcher with A-share rules (limit up/down, T+1, stamp duty)"
```

---

## Task 5: 持仓和资金管理

**Files:**
- Create: `backtest-engine/internal/portfolio/portfolio.go`
- Create: `backtest-engine/internal/portfolio/portfolio_test.go`

- [ ] **Step 1: 写失败测试**

```go
// portfolio_test.go
package portfolio_test

import (
    "testing"
    "time"

    "github.com/parsedong/stock/backtest-engine/internal/matcher"
    "github.com/parsedong/stock/backtest-engine/internal/portfolio"
)

func TestBuyAndHold(t *testing.T) {
    p := portfolio.New(100000.0) // 10万初始资金

    buyDate := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
    fill := matcher.Fill{
        Symbol:      "000001.SZ",
        Side:        matcher.Buy,
        FilledQty:   1000,
        FilledPrice: 10.0,
        Commission:  3.0,
        StampDuty:   0,
        Status:      matcher.Filled,
    }

    err := p.ApplyFill(fill, buyDate)
    if err != nil {
        t.Fatal(err)
    }

    // 现金应减少：10*1000 + 3 = 10003
    wantCash := 100000.0 - 10003.0
    if p.Cash() != wantCash {
        t.Errorf("cash = %v, want %v", p.Cash(), wantCash)
    }

    // 持仓数量
    pos := p.Position("000001.SZ")
    if pos.Quantity != 1000 {
        t.Errorf("quantity = %v, want 1000", pos.Quantity)
    }
    // T+1: 当天买的不能当天卖
    if pos.AvailableQty(buyDate) != 0 {
        t.Errorf("same day available = %v, want 0 (T+1)", pos.AvailableQty(buyDate))
    }
    // 次日可卖
    nextDay := buyDate.AddDate(0, 0, 1)
    if pos.AvailableQty(nextDay) != 1000 {
        t.Errorf("next day available = %v, want 1000", pos.AvailableQty(nextDay))
    }
}

func TestSellReducesPosition(t *testing.T) {
    p := portfolio.New(100000.0)
    buyDate := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)

    buyFill := matcher.Fill{Symbol: "000001.SZ", Side: matcher.Buy,
        FilledQty: 1000, FilledPrice: 10.0, Commission: 3.0, Status: matcher.Filled}
    p.ApplyFill(buyFill, buyDate)

    sellDate := buyDate.AddDate(0, 0, 1)
    sellFill := matcher.Fill{Symbol: "000001.SZ", Side: matcher.Sell,
        FilledQty: 500, FilledPrice: 11.0, Commission: 1.65, StampDuty: 5.5, Status: matcher.Filled}
    p.ApplyFill(sellFill, sellDate)

    pos := p.Position("000001.SZ")
    if pos.Quantity != 500 {
        t.Errorf("remaining quantity = %v, want 500", pos.Quantity)
    }
    // 现金：100000 - 10003 + 11*500 - 1.65 - 5.5
    wantCash := 100000.0 - 10003.0 + 5500.0 - 1.65 - 5.5
    if p.Cash() != wantCash {
        t.Errorf("cash after sell = %v, want %v", p.Cash(), wantCash)
    }
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/portfolio/... -v
```

Expected: compilation error

- [ ] **Step 3: 实现 portfolio.go**

```go
package portfolio

import (
    "fmt"
    "time"

    "github.com/parsedong/stock/backtest-engine/internal/matcher"
)

// Position 持有某只股票的仓位
type Position struct {
    Symbol   string
    Quantity float64
    CostPrice float64
    // lots 记录每手的买入日期，用于T+1判断
    lots []lot
}

type lot struct {
    qty      float64
    buyDate  time.Time
}

// AvailableQty 返回在指定日期可卖出的数量（T+1规则）
func (p *Position) AvailableQty(today time.Time) float64 {
    var avail float64
    for _, l := range p.lots {
        if today.After(l.buyDate) {
            avail += l.qty
        }
    }
    return avail
}

// Portfolio 管理资金和持仓
type Portfolio struct {
    cash      float64
    positions map[string]*Position
}

func New(initCapital float64) *Portfolio {
    return &Portfolio{
        cash:      initCapital,
        positions: make(map[string]*Position),
    }
}

func (p *Portfolio) Cash() float64 { return p.cash }

func (p *Portfolio) Position(symbol string) *Position {
    if pos, ok := p.positions[symbol]; ok {
        return pos
    }
    return &Position{Symbol: symbol}
}

// TotalValue 计算总资产（现金+持仓市值）
// prices: 当日各标的收盘价
func (p *Portfolio) TotalValue(prices map[string]float64) float64 {
    total := p.cash
    for sym, pos := range p.positions {
        if price, ok := prices[sym]; ok {
            total += pos.Quantity * price
        }
    }
    return total
}

func (p *Portfolio) ApplyFill(fill matcher.Fill, date time.Time) error {
    if fill.Status != matcher.Filled {
        return nil
    }
    cost := fill.FilledQty*fill.FilledPrice + fill.Commission + fill.StampDuty

    switch fill.Side {
    case matcher.Buy:
        if cost > p.cash {
            return fmt.Errorf("insufficient cash: need %.2f, have %.2f", cost, p.cash)
        }
        p.cash -= cost
        pos := p.positions[fill.Symbol]
        if pos == nil {
            pos = &Position{Symbol: fill.Symbol}
            p.positions[fill.Symbol] = pos
        }
        // 更新均价
        totalCost := pos.CostPrice*pos.Quantity + fill.FilledPrice*fill.FilledQty
        pos.Quantity += fill.FilledQty
        if pos.Quantity > 0 {
            pos.CostPrice = totalCost / pos.Quantity
        }
        pos.lots = append(pos.lots, lot{qty: fill.FilledQty, buyDate: date})

    case matcher.Sell:
        pos := p.positions[fill.Symbol]
        if pos == nil || pos.Quantity < fill.FilledQty {
            return fmt.Errorf("insufficient position for %s", fill.Symbol)
        }
        p.cash += fill.FilledQty*fill.FilledPrice - fill.Commission - fill.StampDuty
        pos.Quantity -= fill.FilledQty
        // 减少lots（先进先出）
        remaining := fill.FilledQty
        for i := 0; i < len(pos.lots) && remaining > 0; i++ {
            if pos.lots[i].qty <= remaining {
                remaining -= pos.lots[i].qty
                pos.lots[i].qty = 0
            } else {
                pos.lots[i].qty -= remaining
                remaining = 0
            }
        }
        if pos.Quantity == 0 {
            delete(p.positions, fill.Symbol)
        }
    }
    return nil
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/portfolio/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backtest-engine/internal/portfolio/
git commit -m "feat: implement portfolio with T+1 rule and FIFO lot tracking"
```

---

## Task 6: 绩效统计

**Files:**
- Create: `backtest-engine/internal/analytics/analytics.go`
- Create: `backtest-engine/internal/analytics/analytics_test.go`

- [ ] **Step 1: 写失败测试**

```go
// analytics_test.go
package analytics_test

import (
    "math"
    "testing"

    "github.com/parsedong/stock/backtest-engine/internal/analytics"
)

func TestSharpeRatio(t *testing.T) {
    // 每日收益率序列（模拟稳定上涨）
    returns := make([]float64, 252)
    for i := range returns {
        returns[i] = 0.001 // 每日0.1%
    }
    result := analytics.Calculate(returns, 252)
    // 夏普应该很高（无风险利率=0，收益稳定）
    if result.SharpeRatio < 10 {
        t.Errorf("sharpe = %v, expected > 10 for stable returns", result.SharpeRatio)
    }
}

func TestMaxDrawdown(t *testing.T) {
    // 净值曲线：涨到1.2后跌到0.9
    equity := []float64{1.0, 1.1, 1.2, 1.0, 0.9, 1.0}
    returns := make([]float64, len(equity)-1)
    for i := 1; i < len(equity); i++ {
        returns[i-1] = (equity[i] - equity[i-1]) / equity[i-1]
    }
    result := analytics.Calculate(returns, 252)
    // 最大回撤：从1.2跌到0.9，回撤=0.3/1.2=25%
    want := 0.25
    if math.Abs(result.MaxDrawdown-want) > 0.001 {
        t.Errorf("max drawdown = %v, want %v", result.MaxDrawdown, want)
    }
}

func TestAnnualReturn(t *testing.T) {
    // 252个交易日，每日0.1%，年化约 (1.001^252 - 1) ≈ 28.5%
    returns := make([]float64, 252)
    for i := range returns {
        returns[i] = 0.001
    }
    result := analytics.Calculate(returns, 252)
    if math.Abs(result.AnnualReturn-0.285) > 0.01 {
        t.Errorf("annual return = %v, want ~0.285", result.AnnualReturn)
    }
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/analytics/... -v
```

Expected: compilation error

- [ ] **Step 3: 实现 analytics.go**

```go
package analytics

import "math"

type Result struct {
    AnnualReturn  float64
    MaxDrawdown   float64
    SharpeRatio   float64
    WinRate       float64
    AvgHoldDays   float64
    TurnoverRate  float64
    CalmarRatio   float64
    TotalReturn   float64
    EquityCurve   []float64
}

// Calculate 从每日收益率序列计算绩效指标
// tradingDaysPerYear: 一年交易日数，A股通常用242或252
func Calculate(dailyReturns []float64, tradingDaysPerYear int) Result {
    if len(dailyReturns) == 0 {
        return Result{}
    }

    // 净值曲线
    equity := make([]float64, len(dailyReturns)+1)
    equity[0] = 1.0
    for i, r := range dailyReturns {
        equity[i+1] = equity[i] * (1 + r)
    }

    totalReturn := equity[len(equity)-1] - 1.0

    // 年化收益（几何平均）
    years := float64(len(dailyReturns)) / float64(tradingDaysPerYear)
    annualReturn := math.Pow(1+totalReturn, 1/years) - 1

    // 最大回撤
    maxDD := 0.0
    peak := equity[0]
    for _, v := range equity {
        if v > peak {
            peak = v
        }
        dd := (peak - v) / peak
        if dd > maxDD {
            maxDD = dd
        }
    }

    // 夏普比率（假设无风险利率=0）
    mean := 0.0
    for _, r := range dailyReturns {
        mean += r
    }
    mean /= float64(len(dailyReturns))

    variance := 0.0
    for _, r := range dailyReturns {
        d := r - mean
        variance += d * d
    }
    variance /= float64(len(dailyReturns))
    stdDev := math.Sqrt(variance)

    sharpe := 0.0
    if stdDev > 0 {
        sharpe = mean / stdDev * math.Sqrt(float64(tradingDaysPerYear))
    }

    // 胜率
    wins := 0
    for _, r := range dailyReturns {
        if r > 0 {
            wins++
        }
    }
    winRate := float64(wins) / float64(len(dailyReturns))

    // Calmar
    calmar := 0.0
    if maxDD > 0 {
        calmar = annualReturn / maxDD
    }

    return Result{
        AnnualReturn: annualReturn,
        MaxDrawdown:  maxDD,
        SharpeRatio:  sharpe,
        WinRate:      winRate,
        CalmarRatio:  calmar,
        TotalReturn:  totalReturn,
        EquityCurve:  equity,
    }
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/analytics/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backtest-engine/internal/analytics/
git commit -m "feat: implement analytics (sharpe, max drawdown, annual return, calmar)"
```

---

## Task 7: 回测主引擎

**Files:**
- Create: `backtest-engine/internal/engine/eventbus.go`
- Create: `backtest-engine/internal/engine/engine.go`
- Create: `backtest-engine/internal/engine/engine_test.go`

- [ ] **Step 1: 写 eventbus.go**

```go
package engine

import "github.com/parsedong/stock/backtest-engine/internal/data"

// 事件类型
type EventType int

const (
    EventBar   EventType = iota // 新的K线数据到来
    EventOrder                  // 策略提交订单
    EventFill                   // 订单成交回报
)

// BarEvent 封装一根K线
type BarEvent struct {
    Bar data.Bar
}

// OrderEvent 策略提交的订单
type OrderEvent struct {
    Symbol   string
    Side     int     // 0=buy, 1=sell
    Quantity float64
    Price    float64 // 0=市价
}

// FillEvent 成交回报
type FillEvent struct {
    Symbol      string
    Side        int
    FilledQty   float64
    FilledPrice float64
    Commission  float64
    StampDuty   float64
    Rejected    bool
    Reason      string
}
```

- [ ] **Step 2: 写 engine.go**

```go
package engine

import (
    "time"

    "github.com/parsedong/stock/backtest-engine/internal/analytics"
    "github.com/parsedong/stock/backtest-engine/internal/data"
    "github.com/parsedong/stock/backtest-engine/internal/matcher"
    "github.com/parsedong/stock/backtest-engine/internal/portfolio"
)

// Strategy 策略接口，由Python侧或Go测试实现
type Strategy interface {
    // OnBar 收到新K线时调用，返回订单列表（可以为空）
    OnBar(bar data.Bar, port *portfolio.Portfolio) []OrderEvent
}

// Config 回测配置
type Config struct {
    Symbols     []string
    Start       time.Time
    End         time.Time
    InitCapital float64
    Commission  float64 // 默认 0.0003
}

// Engine 回测主引擎
type Engine struct {
    store     data.DataStore
    matcher   *matcher.Matcher
    portfolio *portfolio.Portfolio
}

func New(store data.DataStore, cfg Config) *Engine {
    commission := cfg.Commission
    if commission == 0 {
        commission = 0.0003
    }
    return &Engine{
        store:     store,
        matcher:   matcher.New(matcher.Config{Commission: commission}),
        portfolio: portfolio.New(cfg.InitCapital),
    }
}

// Run 执行完整回测，返回绩效结果
func (e *Engine) Run(cfg Config, strategy Strategy) (analytics.Result, error) {
    days, err := e.store.TradingDays(cfg.Start, cfg.End)
    if err != nil {
        return analytics.Result{}, err
    }

    prevPrices := make(map[string]float64) // 上一交易日收盘价
    var dailyReturns []float64
    prevValue := cfg.InitCapital

    for _, day := range days {
        bars, err := e.store.LoadBarsByDate(day, cfg.Symbols)
        if err != nil {
            return analytics.Result{}, err
        }

        // 对每个有数据的标的，让策略决策
        for sym, bar := range bars {
            orders := strategy.OnBar(bar, e.portfolio)
            for _, oe := range orders {
                order := matcher.Order{
                    Symbol:   sym,
                    Side:     matcher.Side(oe.Side),
                    Quantity: oe.Quantity,
                    Price:    oe.Price,
                }
                prevClose := prevPrices[sym]
                fill := e.matcher.Match(order, bar, prevClose, false)
                e.portfolio.ApplyFill(fill, day) //nolint:errcheck
            }
            prevPrices[sym] = bar.Close
        }

        // 记录当日净值
        prices := make(map[string]float64)
        for sym, bar := range bars {
            prices[sym] = bar.Close
        }
        curValue := e.portfolio.TotalValue(prices)
        if prevValue > 0 {
            dailyReturns = append(dailyReturns, (curValue-prevValue)/prevValue)
        }
        prevValue = curValue
    }

    return analytics.Calculate(dailyReturns, 242), nil
}
```

- [ ] **Step 3: 写集成测试（用MockStore + 简单策略）**

```go
// engine_test.go
package engine_test

import (
    "testing"
    "time"

    "github.com/parsedong/stock/backtest-engine/internal/data"
    "github.com/parsedong/stock/backtest-engine/internal/engine"
    "github.com/parsedong/stock/backtest-engine/internal/portfolio"
)

// buyAndHoldStrategy 第一天买入，一直持有
type buyAndHoldStrategy struct {
    bought bool
}

func (s *buyAndHoldStrategy) OnBar(bar data.Bar, port *portfolio.Portfolio) []engine.OrderEvent {
    if s.bought {
        return nil
    }
    s.bought = true
    return []engine.OrderEvent{
        {Symbol: bar.Symbol, Side: 0, Quantity: 1000, Price: 0},
    }
}

func TestEngineRunBuyAndHold(t *testing.T) {
    // 构造5天上涨行情
    days := []time.Time{
        time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
        time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
        time.Date(2020, 1, 6, 0, 0, 0, 0, time.UTC),
        time.Date(2020, 1, 7, 0, 0, 0, 0, time.UTC),
        time.Date(2020, 1, 8, 0, 0, 0, 0, time.UTC),
    }
    closes := []float64{10.0, 10.5, 11.0, 11.5, 12.0}

    store := &mockStore{days: days, closes: closes}
    cfg := engine.Config{
        Symbols:     []string{"000001.SZ"},
        Start:       days[0],
        End:         days[len(days)-1],
        InitCapital: 100000.0,
        Commission:  0.0003,
    }

    eng := engine.New(store, cfg)
    result, err := eng.Run(cfg, &buyAndHoldStrategy{})
    if err != nil {
        t.Fatal(err)
    }
    // 买入价10，最终12，盈利20%（扣手续费后略低）
    if result.TotalReturn < 0.15 {
        t.Errorf("total return = %.4f, expected > 15%%", result.TotalReturn)
    }
    if result.MaxDrawdown != 0 {
        t.Errorf("monotonic rise should have 0 drawdown, got %.4f", result.MaxDrawdown)
    }
}

// mockStore 测试用数据源
type mockStore struct {
    days   []time.Time
    closes []float64
}

func (m *mockStore) LoadBars(symbol string, start, end time.Time) ([]data.Bar, error) {
    var bars []data.Bar
    for i, d := range m.days {
        if !d.Before(start) && !d.After(end) {
            bars = append(bars, data.Bar{Symbol: symbol, Date: d, Close: m.closes[i],
                Open: m.closes[i], High: m.closes[i] * 1.01, Low: m.closes[i] * 0.99,
                Volume: 1000000, AdjFactor: 1.0})
        }
    }
    return bars, nil
}

func (m *mockStore) LoadBarsByDate(date time.Time, symbols []string) (map[string]data.Bar, error) {
    result := make(map[string]data.Bar)
    for i, d := range m.days {
        if d.Equal(date) {
            for _, sym := range symbols {
                result[sym] = data.Bar{Symbol: sym, Date: d, Close: m.closes[i],
                    Open: m.closes[i], High: m.closes[i] * 1.01, Low: m.closes[i] * 0.99,
                    Volume: 1000000, AdjFactor: 1.0}
            }
        }
    }
    return result, nil
}

func (m *mockStore) Symbols() ([]string, error) { return []string{"000001.SZ"}, nil }

func (m *mockStore) TradingDays(start, end time.Time) ([]time.Time, error) {
    var days []time.Time
    for _, d := range m.days {
        if !d.Before(start) && !d.After(end) {
            days = append(days, d)
        }
    }
    return days, nil
}
```

- [ ] **Step 4: 运行所有测试**

```bash
cd backtest-engine
go test ./... -v
```

Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add backtest-engine/internal/engine/
git commit -m "feat: implement backtesting engine with event-driven loop"
```

---

## Task 8: gRPC服务启动

**Files:**
- Create: `backtest-engine/cmd/server/main.go`

- [ ] **Step 1: 实现 main.go**

```go
package main

import (
    "context"
    "flag"
    "log"
    "net"
    "time"

    "google.golang.org/grpc"
    "github.com/parsedong/stock/backtest-engine/internal/analytics"
    "github.com/parsedong/stock/backtest-engine/internal/data"
    "github.com/parsedong/stock/backtest-engine/internal/engine"
    pb "github.com/parsedong/stock/backtest-engine/proto"
)

var (
    port    = flag.String("port", "50051", "gRPC服务端口")
    dataDir = flag.String("data", "../../data/normalized", "标准化数据目录")
)

type backtestServer struct {
    pb.UnimplementedBacktestEngineServer
    store data.DataStore
}

func (s *backtestServer) RunBacktest(ctx context.Context, req *pb.BacktestRequest) (*pb.BacktestResult, error) {
    start, err := time.Parse("2006-01-02", req.StartDate)
    if err != nil {
        return nil, err
    }
    end, err := time.Parse("2006-01-02", req.EndDate)
    if err != nil {
        return nil, err
    }

    cfg := engine.Config{
        Symbols:     req.Symbols,
        Start:       start,
        End:         end,
        InitCapital: req.InitCapital,
        Commission:  req.Commission,
    }

    // TODO(phase2): 策略由Python通过StreamBars+SubmitOrder接口驱动
    // 这里先返回空结果，供Phase2集成
    _ = cfg
    result := analytics.Result{}

    curve := make([]float64, len(result.EquityCurve))
    copy(curve, result.EquityCurve)

    return &pb.BacktestResult{
        AnnualReturn:  result.AnnualReturn,
        MaxDrawdown:   result.MaxDrawdown,
        SharpeRatio:   result.SharpeRatio,
        WinRate:       result.WinRate,
        AvgHoldDays:   result.AvgHoldDays,
        TurnoverRate:  result.TurnoverRate,
        CalmarRatio:   result.CalmarRatio,
        TotalReturn:   result.TotalReturn,
        EquityCurve:   curve,
    }, nil
}

func (s *backtestServer) StreamBars(req *pb.StreamRequest, stream pb.BacktestEngine_StreamBarsServer) error {
    start, _ := time.Parse("2006-01-02", req.StartDate)
    end, _   := time.Parse("2006-01-02", req.EndDate)

    days, err := s.store.TradingDays(start, end)
    if err != nil {
        return err
    }

    for _, day := range days {
        bars, err := s.store.LoadBarsByDate(day, req.Symbols)
        if err != nil {
            return err
        }
        for _, bar := range bars {
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

func main() {
    flag.Parse()

    store := data.NewParquetStore(*dataDir)

    lis, err := net.Listen("tcp", ":"+*port)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    s := grpc.NewServer()
    pb.RegisterBacktestEngineServer(s, &backtestServer{store: store})

    log.Printf("backtest engine listening on :%s", *port)
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
```

- [ ] **Step 2: 编译确认无错误**

```bash
cd backtest-engine
go build ./cmd/server/
```

Expected: 编译成功，无报错

- [ ] **Step 3: Commit**

```bash
git add backtest-engine/cmd/
git commit -m "feat: add gRPC server entry point with StreamBars support"
```

---

## Task 9: 数据标准化CLI工具

**Files:**
- Create: `tools/normalize/main.go`

> **目标：** 读取 `data/raw/` 中的原始数据（格式在拿到数据后确认），转换为标准Parquet格式存入 `data/normalized/daily/`。由于淘宝数据格式未知，这里实现一个CSV转换器作为基础，实际使用时根据真实格式调整。

- [ ] **Step 1: 实现CSV转Parquet工具**

```go
package main

import (
    "encoding/csv"
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "time"

    "github.com/parquet-go/parquet-go"
)

type Row struct {
    Symbol    string  `parquet:"symbol"`
    DateStr   string  `parquet:"date"`
    Open      float64 `parquet:"open"`
    High      float64 `parquet:"high"`
    Low       float64 `parquet:"low"`
    Close     float64 `parquet:"close"`
    Volume    float64 `parquet:"volume"`
    Amount    float64 `parquet:"amount"`
    AdjFactor float64 `parquet:"adj_factor"`
}

var (
    inputDir  = flag.String("input", "../../data/raw", "原始数据目录")
    outputDir = flag.String("output", "../../data/normalized/daily", "输出目录")
)

func main() {
    flag.Parse()

    os.MkdirAll(*outputDir, 0755)

    // 按年分组
    yearRows := make(map[int][]Row)

    err := filepath.Walk(*inputDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() || !strings.HasSuffix(path, ".csv") {
            return err
        }
        rows, err := parseCSV(path)
        if err != nil {
            log.Printf("skip %s: %v", path, err)
            return nil
        }
        for _, r := range rows {
            date, _ := time.Parse("2006-01-02", r.DateStr)
            yearRows[date.Year()] = append(yearRows[date.Year()], r)
        }
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    for year, rows := range yearRows {
        outPath := filepath.Join(*outputDir, fmt.Sprintf("%d.parquet", year))
        f, err := os.Create(outPath)
        if err != nil {
            log.Fatal(err)
        }
        if err := parquet.Write(f, rows); err != nil {
            f.Close()
            log.Fatal(err)
        }
        f.Close()
        log.Printf("wrote %d rows to %s", len(rows), outPath)
    }
}

// parseCSV 解析CSV文件，期望列：symbol,date,open,high,low,close,volume,amount,adj_factor
func parseCSV(path string) ([]Row, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    r := csv.NewReader(f)
    records, err := r.ReadAll()
    if err != nil {
        return nil, err
    }

    var rows []Row
    for i, rec := range records {
        if i == 0 { // 跳过表头
            continue
        }
        if len(rec) < 9 {
            continue
        }
        row := Row{Symbol: rec[0], DateStr: rec[1]}
        row.Open, _ = strconv.ParseFloat(rec[2], 64)
        row.High, _ = strconv.ParseFloat(rec[3], 64)
        row.Low, _ = strconv.ParseFloat(rec[4], 64)
        row.Close, _ = strconv.ParseFloat(rec[5], 64)
        row.Volume, _ = strconv.ParseFloat(rec[6], 64)
        row.Amount, _ = strconv.ParseFloat(rec[7], 64)
        row.AdjFactor, _ = strconv.ParseFloat(rec[8], 64)
        if row.AdjFactor == 0 {
            row.AdjFactor = 1.0
        }
        rows = append(rows, row)
    }
    return rows, nil
}
```

- [ ] **Step 2: 编译确认**

```bash
cd tools/normalize
go mod init github.com/parsedong/stock/tools/normalize
go get github.com/parquet-go/parquet-go@latest
go build .
```

Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add tools/
git commit -m "feat: add CSV-to-Parquet normalization tool for raw data ingestion"
```

---

## Task 10: Python gRPC客户端 + 示例策略

**Files:**
- Create: `python/client/backtest_client.py`
- Create: `python/strategies/example_ma_strategy.py`
- Create: `python/client/test_backtest_client.py`

- [ ] **Step 1: 安装Python依赖**

```bash
cd python
python3 -m venv venv
source venv/bin/activate
pip install grpcio grpcio-tools pandas numpy pytest
```

- [ ] **Step 2: 生成Python gRPC代码**

```bash
cd python
python3 -m grpc_tools.protoc \
    -I../backtest-engine/proto \
    --python_out=client \
    --grpc_python_out=client \
    ../backtest-engine/proto/backtest.proto
```

Expected: 生成 `client/backtest_pb2.py` 和 `client/backtest_pb2_grpc.py`

- [ ] **Step 3: 实现 backtest_client.py**

```python
import grpc
from . import backtest_pb2, backtest_pb2_grpc
from dataclasses import dataclass
from typing import Iterator

@dataclass
class Bar:
    symbol: str
    date_unix: int
    open: float
    high: float
    low: float
    close: float
    volume: float
    amount: float
    adj_factor: float

class BacktestClient:
    """Go回测引擎的Python客户端封装"""

    def __init__(self, host: str = "localhost", port: int = 50051):
        self._channel = grpc.insecure_channel(f"{host}:{port}")
        self._stub = backtest_pb2_grpc.BacktestEngineStub(self._channel)

    def stream_bars(self, symbols: list[str], start_date: str, end_date: str) -> Iterator[Bar]:
        """流式获取K线数据，按时间顺序推送"""
        req = backtest_pb2.StreamRequest(
            symbols=symbols,
            start_date=start_date,
            end_date=end_date,
        )
        for pb_bar in self._stub.StreamBars(req):
            yield Bar(
                symbol=pb_bar.symbol,
                date_unix=pb_bar.date_unix,
                open=pb_bar.open,
                high=pb_bar.high,
                low=pb_bar.low,
                close=pb_bar.close,
                volume=pb_bar.volume,
                amount=pb_bar.amount,
                adj_factor=pb_bar.adj_factor,
            )

    def submit_order(self, symbol: str, side: int, quantity: float, price: float = 0.0):
        """提交订单，side: 0=买, 1=卖，price=0表示市价单"""
        req = backtest_pb2.Order(
            symbol=symbol,
            side=side,
            quantity=quantity,
            price=price,
        )
        return self._stub.SubmitOrder(req)

    def close(self):
        self._channel.close()
```

- [ ] **Step 4: 实现示例均线策略**

```python
# example_ma_strategy.py
"""
双均线策略示例（参考vnpy CTA策略写法）
- 快线（5日MA）上穿慢线（20日MA）时买入
- 快线下穿慢线时卖出
用于验证回测引擎的完整流程
"""
from collections import deque
from client.backtest_client import BacktestClient, Bar

def run_ma_strategy(
    symbols: list[str],
    start_date: str,
    end_date: str,
    init_capital: float = 1_000_000,
    fast_period: int = 5,
    slow_period: int = 20,
):
    client = BacktestClient()
    close_history: dict[str, deque] = {sym: deque(maxlen=slow_period) for sym in symbols}
    position: dict[str, float] = {sym: 0.0 for sym in symbols}

    for bar in client.stream_bars(symbols, start_date, end_date):
        sym = bar.symbol
        close_history[sym].append(bar.close)

        if len(close_history[sym]) < slow_period:
            continue

        closes = list(close_history[sym])
        fast_ma = sum(closes[-fast_period:]) / fast_period
        slow_ma = sum(closes) / slow_period

        # 金叉买入
        if fast_ma > slow_ma and position[sym] == 0:
            qty = (init_capital * 0.1) // (bar.close * 100) * 100  # 按10%仓位买入
            if qty > 0:
                fill = client.submit_order(sym, 0, qty)
                if not fill.rejected:
                    position[sym] = qty
                    print(f"BUY {sym} qty={qty} price={bar.close}")

        # 死叉卖出
        elif fast_ma < slow_ma and position[sym] > 0:
            fill = client.submit_order(sym, 1, position[sym])
            if not fill.rejected:
                print(f"SELL {sym} qty={position[sym]} price={bar.close}")
                position[sym] = 0

    client.close()

if __name__ == "__main__":
    run_ma_strategy(["000001.SZ", "000002.SZ"], "2020-01-01", "2023-12-31")
```

- [ ] **Step 5: Commit**

```bash
git add python/
git commit -m "feat: add Python gRPC client and example MA cross strategy"
```

---

## Task 11: 最终验证

- [ ] **Step 1: 运行全量Go测试**

```bash
cd backtest-engine
go test ./... -v -count=1
```

Expected: 所有测试 PASS，无 FAIL

- [ ] **Step 2: 验证gRPC服务启动**

```bash
cd backtest-engine
go run cmd/server/main.go --data ../../data/normalized &
sleep 1
# 用grpc_cli验证服务可达（可选，若未安装可跳过）
# grpc_cli ls localhost:50051
kill %1
```

Expected: 服务正常启动，输出 "backtest engine listening on :50051"

- [ ] **Step 3: 最终提交**

```bash
git add .
git commit -m "chore: phase 1 complete - data layer and Go backtesting engine ready"
```

---

## 自检：Spec覆盖确认

| 设计节 | 对应任务 |
|--------|---------|
| 统一Bar格式 | Task 2 |
| Parquet分片存储 | Task 3 |
| 幸存者偏差（退市数据）| 数据库在Phase2处理，Task 3的store接口预留 |
| A股涨跌停、T+1、印花税 | Task 4、5 |
| 事件驱动引擎 | Task 7 |
| gRPC接口 | Task 1（proto）、Task 8（server）|
| Python策略接口 | Task 10 |
| 绩效指标 | Task 6 |
| 数据标准化工具 | Task 9 |
