# A股量化回测系统 — 系统规格说明

**版本：** 1.0  
**日期：** 2026-04-16

---

## 一、技术栈约束

### 必须使用（不可替换）

| 组件 | 技术 | 约束原因 |
|------|------|---------|
| 回测引擎 | **Go 1.22+** | 性能要求，主力开发语言 |
| 策略逻辑 | **Python 3.11+** | 生态（pandas/numpy/ta-lib），快速迭代 |
| 引擎↔策略通信 | **gRPC + Protocol Buffers 3** | 强类型跨语言接口，不得用 HTTP/JSON |
| 行情存储 | **Apache Parquet** | 列式存储，读取性能，不得用 CSV 或 SQLite |
| 关系型数据库 | **PostgreSQL 16+** | 不得用 MySQL / SQLite |
| 数据库部署 | **Docker Compose** | 标准化环境，不允许裸机安装依赖 |
| HTTP API 层 | **go-zero** | Phase 3 前端接口，不得用 Gin / Echo 等其他框架 |
| 前端 | **React + TailwindCSS + ECharts** | 不得用 Vue / Ant Design 等其他方案 |

### 可替换组件

| 组件 | 默认 | 可替换条件 |
|------|------|----------|
| LLM 服务 | Claude API | 可配置切换 OpenAI / Ollama，通过配置文件指定 |
| Parquet 库（Go） | parquet-go | 同等性能的替代库 |

---

## 二、代码组织约束

### 目录结构（不可调整顶层布局）

```
stock/
├── backtest-engine/        # Go 回测引擎，原生 gRPC（独立 go module）
├── api-server/             # go-zero HTTP API 层，Phase 3（独立 go module）
├── python/                 # Python 策略层
├── tools/                  # 独立 CLI 工具（各自独立 go module）
├── migrations/             # PostgreSQL 迁移 SQL，按序号命名
├── docs/                   # 设计文档和规格
├── data/
│   ├── raw/                # 原始数据，只读，不得修改
│   ├── normalized/         # 标准化 Parquet 文件，程序生成
│   └── pg/                 # PostgreSQL 数据卷，gitignore
└── docker-compose.yml
```

### 层级职责分离（强制）

- `backtest-engine/` — 只做计算，不做 HTTP，对外只暴露 gRPC
- `api-server/` — 只做接口转发和业务编排，调用 backtest-engine 的 gRPC client，不直接操作数据
- `python/` — 只写策略逻辑，通过 gRPC client 与引擎交互，不直接读文件或数据库

### Go 模块规范

- `backtest-engine/` 为核心模块，路径 `github.com/parsedong/stock/backtest-engine`
- `tools/` 下每个工具独立 `go.mod`，不依赖 `backtest-engine` 内部包
- 禁止在 `internal/` 外引用 `internal/` 包

### Python 规范

- 所有策略必须实现统一接口（见第五节）
- 禁止策略代码直接读取数据文件，必须通过 gRPC client 获取数据

---

## 三、数据规格

### 3.1 Bar 标准格式

所有行情数据统一转换为以下格式后存储，不保留原始字段：

```go
type Bar struct {
    Symbol    string    // 股票代码，格式：{exchange}{code}，如 sz000001 / sh600000 / bj920000
    Date      time.Time // 时间戳，分钟K线精确到分钟，日K线精确到日
    Open      float64   // 未复权开盘价
    High      float64   // 未复权最高价
    Low       float64   // 未复权最低价
    Close     float64   // 未复权收盘价
    Volume    float64   // 成交量（股）
    Amount    float64   // 成交额（元）
    AdjFactor float64   // 前复权因子，默认 1.0，必须在导入时填充
}
```

**约束：**
- `AdjFactor` 必须在数据导入阶段填充，不允许在回测引擎中实时计算
- 所有价格信号计算必须使用 `Bar.AdjClose()`（= Close × AdjFactor），不得直接用 Close
- Symbol 统一小写，不使用 `.SZ` / `.SH` 后缀格式

### 3.2 Parquet 文件布局

```
data/normalized/
├── 1m/         # 1分钟K线，每只股票一个文件
│   ├── sz000001.parquet
│   └── sh600000.parquet
├── 5m/
├── 15m/
├── 30m/
└── 60m/
```

- 文件名 = Symbol（小写）
- 单文件包含该股票全部历史数据，按日期升序排列
- 不按年分片（与日频方案不同，分钟频按股票分片更适合策略使用）

### 3.3 原始数据格式（只读参考）

淘宝数据 CSV 列顺序：
```
日期, 开盘, 最高, 最低, 收盘, 成交量(股), 成交额(元),
涨跌(元), 涨跌幅(%), 换手率(%), 流通股本(股), 总股本(股)
```

- 编码：UTF-8（含BOM）或 GBK，导入工具必须自动检测
- `data/raw/` 内容只读，导入工具不得修改原始文件

---

## 四、A股规则规格（回测引擎必须遵守）

以下规则在 `internal/matcher/` 中实现，任何策略均受此约束：

| 规则 | 规格 |
|------|------|
| 涨停 | 当日涨幅 ≥ 9.9%（普通股）/ 4.9%（ST/SST）时，买单强制拒绝 |
| 跌停 | 当日跌幅 ≥ 9.9%（普通股）/ 4.9%（ST/SST）时，卖单强制拒绝 |
| T+1 | 当日买入的持仓，次交易日才计入可卖数量 |
| 印花税 | 卖出方向收取 0.1%，买入方向不收 |
| 佣金 | 买卖双向各收 0.03%（默认值，可通过 Config 覆盖） |
| 市价单 | `Price = 0` 时以当日收盘价成交 |
| 停牌 | 当日无 Bar 数据时，该标的所有挂单自动跳过（不拒绝，下一交易日继续） |
| 退市 | 按 `stocks.delisted_date` 判断，退市日强制平仓，后续不再接受订单 |

**不实现的规则（超出当前范围）：**
- 集合竞价（开盘价偏差）
- 大单冲击成本（滑点模型）
- 融资融券

---

## 五、接口规格

### 5.1 gRPC 服务接口（不可变更）

```protobuf
service BacktestEngine {
    rpc RunBacktest(BacktestRequest) returns (BacktestResult);   // 同步回测
    rpc StreamBars(StreamRequest)   returns (stream Bar);        // 推送 Bar 给 Python 策略
    rpc SubmitOrder(Order)          returns (Fill);              // Python 策略提交订单
}
```

- 端口默认 `50051`，可通过 `-port` 参数覆盖
- 每个回测会话独立上下文，`StreamBars` 和 `SubmitOrder` 必须在同一会话内配对使用
- `StreamBars` 按交易日升序推送，不允许乱序

### 5.2 Python 策略接口

所有策略必须实现以下接口，不允许直接操作数据库或文件：

```python
class Strategy:
    def on_bar(self, bar: Bar, portfolio: PortfolioSnapshot) -> list[Order]:
        """
        接收新 Bar，返回订单列表（空列表表示不操作）
        bar: 当前 Bar（只包含当前及之前的信息，禁止前视）
        portfolio: 当前持仓快照（只读）
        """
        ...
```

### 5.3 绩效指标输出规格

每次回测必须输出以下标准指标，缺一不可：

| 指标 | 字段名 | 说明 |
|------|--------|------|
| 年化收益率 | `annual_return` | 几何平均，以小数表示（0.15 = 15%） |
| 最大回撤 | `max_drawdown` | 正数，以小数表示（0.20 = 20%） |
| 夏普比率 | `sharpe_ratio` | 无风险利率 = 0，基于日收益率 × √242 |
| 胜率 | `win_rate` | 日收益为正的天数占比 |
| Calmar 比率 | `calmar_ratio` | annual_return / max_drawdown |
| 总收益率 | `total_return` | 小数表示 |
| 净值曲线 | `equity_curve` | 每日净值序列，初始值 = 1.0 |

---

## 六、数据库规格

### 必须存入 PostgreSQL 的数据

| 表 | 内容 |
|----|------|
| `stocks` | 股票元数据：代码、名称、上市/退市日期、是否ST |
| `import_batches` | 每次导入记录：来源文件、频率、状态、统计 |
| `import_checks` | 每只股票的完整性校验结果 |
| 策略记录（Phase 3） | 策略代码版本、参数、回测结果 |
| Agent 评审记录（Phase 4） | 评审报告、评分、最终决策 |

### 禁止存入 PostgreSQL 的数据

- 原始行情 Bar 数据（存 Parquet）
- 因子计算中间结果（存 Parquet 缓存）

---

## 七、完整性校验规格

数据导入后必须执行完整性校验，结果写入 `import_checks` 表。

**校验维度：**

| 维度 | 合格标准 | 不合格处理 |
|------|---------|----------|
| 数据完整度 | 实际 Bar 数 ≥ 预期的 80% | 标记 `incomplete`，不阻断导入 |
| 日期连续性 | 使用真实交易日历（tushare）验证 | 标记缺失日期列表 |
| 价格合理性 | Open/High/Low/Close 均 > 0 | 标记异常行，继续导入其余数据 |
| 复权因子 | `AdjFactor > 0` | AdjFactor ≤ 0 时强制设为 1.0 并告警 |

**必须支持的运行模式：**

```bash
--check-only   # 只校验，不写入 Parquet 和 DB，无需 DB 连接
--dry-run      # 校验 + 写 Parquet，不写 DB
（默认）       # 完整导入：写 Parquet + 写 DB
```

---

## 八、性能约束

| 场景 | 要求 |
|------|------|
| 单只股票5年日频回测 | < 100ms |
| 全A股（5000只）5年日频回测 | < 60s |
| 分钟频单只股票回测 | < 1s |
| Parquet 单文件读取（1只股票全量1分钟数据） | < 3s |

---

## 九、禁止事项

- 禁止策略代码读取 `data/` 目录下的任何文件
- 禁止回测引擎在撮合时访问未来数据（严格按 Bar 推送顺序）
- 禁止将退市前数据从回测中排除（必须保留，防止幸存者偏差）
- 禁止硬编码数据库连接信息，必须通过环境变量或配置文件注入
- 禁止在测试中使用真实数据文件，必须使用 mock 或 testdata
