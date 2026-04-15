# A股量化回测系统 — 技术设计文档

**日期：** 2026-04-14  
**目标：** 构建一个支持多策略批量回测、Walk-Forward验证、LLM多Agent评审的A股量化投资系统，最终服务于实盘交易决策。

---

## 一、系统整体架构

系统分为五层，执行顺序从下到上：

```
第四层：可视化界面 + 策略广场后台
         ↑
第三层：Agent 评审层（LLM多角色评审）
         ↑
第二层：策略评估层（Walk-Forward + 过拟合检测）
         ↑
第一层：Go 回测引擎（事件驱动，高性能）
         ↑
第零层：数据层（本地文件 + 数据库 + tushare同步）
```

**执行流程：**
1. 数据层提供标准化历史行情
2. Go回测引擎按时间顺序驱动策略，输出绩效指标
3. 策略评估层做Walk-Forward验证，过滤过拟合策略
4. 通过过滤的策略进入策略池，触发Agent评审
5. Agent评审输出报告，人工确认后选定策略实盘

---

## 二、数据层

### 2.1 统一数据格式

无论数据来源（淘宝历史数据、tushare），统一转换为内部标准Bar格式：

```go
type Bar struct {
    Symbol    string    // 股票代码，如 "000001.SZ"
    Date      time.Time // 日期
    Open      float64
    High      float64
    Low       float64
    Close     float64
    Volume    float64   // 成交量（股）
    Amount    float64   // 成交额（元）
    AdjFactor float64   // 复权因子
}
```

### 2.2 目录结构

```
data/
├── raw/                 # 淘宝原始数据，原样保存，不修改
├── normalized/          # 标准化后的Parquet文件
│   ├── daily/           # 日频数据，按年分片
│   │   ├── 2020.parquet
│   │   └── 2021.parquet
│   └── minute/          # 分钟频（如有）
├── factors/             # 计算好的因子缓存
└── db/                  # 数据库（具体方案待定）
    └── meta             # 股票元数据（名称、上市日期、退市日期）
```

> **注：** 数据库具体选型（PostgreSQL/DuckDB等）待后续讨论确定，不使用SQLite。

### 2.3 幸存者偏差处理

元数据库中**必须保存退市股票记录**。回测时按照回测日期动态确定股票池，而不是用当前在市股票池，确保历史回测的真实性。

### 2.4 tushare 数据同步

```
每日收盘后定时任务：
→ tushare拉取当日行情
→ 更新复权因子
→ 写入normalized/daily/
→ 更新元数据（处理新上市/退市）
```

---

## 三、Go 回测引擎

### 3.1 项目结构

```
backtest-engine/
├── cmd/
│   └── server/          # gRPC服务入口
├── internal/
│   ├── data/
│   │   ├── store.go     # 统一数据接口（面向接口，可插拔数据源）
│   │   ├── parquet.go   # Parquet文件读取
│   │   └── db.go        # 数据库读写
│   ├── engine/
│   │   ├── engine.go    # 主循环，按时间顺序驱动事件
│   │   └── eventbus.go  # 事件总线
│   ├── matcher/
│   │   └── matcher.go   # 撮合引擎
│   ├── portfolio/
│   │   └── portfolio.go # 持仓、资金、手续费管理
│   ├── risk/
│   │   └── risk.go      # 风控规则
│   └── analytics/
│       └── analytics.go # 绩效指标计算
└── proto/
    └── backtest.proto   # gRPC接口定义
```

### 3.2 事件驱动模型

```
Bar事件 → [Python策略] → Order事件 → [撮合引擎] → Fill事件 → [持仓更新] → 下一个Bar
```

严格按时间顺序推送Bar，策略只能看到当前时间点之前的数据，物理上杜绝前视偏差。

### 3.3 A股特有规则

| 规则 | 处理方式 |
|------|---------|
| 涨跌停 | 收盘价触及涨跌停的Order标记为未成交 |
| T+1 | 当日买入的持仓次日才可卖出 |
| 手续费 | 买入0.03%，卖出0.03% + 印花税0.1% |
| 停牌 | 停牌期间Order自动拒绝 |
| 退市 | 按元数据判断，强制清仓 |

### 3.4 gRPC 接口

```protobuf
service BacktestEngine {
    // 批量回测，同步返回结果
    rpc RunBacktest(BacktestRequest) returns (BacktestResult);
    // 流式推送Bar，策略实时响应（用于调试和实时回测）
    rpc StreamBars(StreamRequest) returns (stream Bar);
    // 提交订单
    rpc SubmitOrder(Order) returns (OrderResult);
}
```

Python策略通过gRPC与引擎通信，策略逻辑完全在Python侧，引擎专注撮合和状态管理。

---

## 四、策略评估层

### 4.1 Walk-Forward 验证

防止过拟合的核心机制：

```
完整历史（例如2010-2024）
├── 训练窗口（2年）→ 参数优化
├── 验证窗口（6个月）→ 样本外测试
├── 滑动向前，重复...
└── 拼接所有验证期结果 = 真实样本外表现
```

窗口大小（训练期、验证期）可在可视化界面配置。

### 4.2 标准化绩效指标

```go
type StrategyResult struct {
    StrategyID      string
    AnnualReturn    float64  // 年化收益率
    MaxDrawdown     float64  // 最大回撤
    SharpeRatio     float64  // 夏普比率
    WinRate         float64  // 胜率
    AvgHoldDays     float64  // 平均持仓天数
    TurnoverRate    float64  // 年换手率
    CalmarRatio     float64  // 年化收益 / 最大回撤
    OutOfSamplePerf float64  // 样本外表现（Walk-Forward）
}
```

### 4.3 自动过滤规则（可视化配置）

以下阈值均可在界面上调整：

| 指标 | 默认淘汰条件 |
|------|------------|
| 最大回撤 | > 30% |
| 夏普比率 | < 0.5 |
| 样本外/样本内表现比 | < 60%（过拟合信号）|
| 年换手率 | > 1000% |

通过过滤的策略自动进入**策略池**，等待Agent评审。

---

## 五、Agent 评审层

### 5.1 四个Agent角色

| Agent | 职责 | 主要关注点 |
|-------|------|-----------|
| 收益Agent | 评估盈利能力 | 年化收益、夏普趋势、胜率稳定性 |
| 风险Agent | 评估风险控制 | 最大回撤、极端亏损、波动率 |
| 环境Agent | 判断市场适配性 | 策略在牛/熊/震荡市的分段表现 |
| 魔鬼代言人 | 专门找问题 | 逻辑漏洞、数据问题、过拟合迹象 |

### 5.2 评审流程

```
策略池
  │
  ▼
[收益Agent + 风险Agent + 环境Agent] ← 并行评审
  │
  ▼
[魔鬼代言人] ← 读取三份报告，专门反驳和挑毛病
  │
  ▼
[汇总报告] → 每个策略得到综合评分 + 结构化评审意见
  │
  ▼
人工确认 → 选定策略 → 实盘
```

**重要原则：** Agent评审不自动下单，最终投资决策由人工确认。

### 5.3 LLM 接入

- 支持可配置切换：OpenAI / Claude / 本地模型（Ollama）
- 每个Agent有独立的system prompt，定义角色、评审标准和输出格式
- 评审结果持久化，可追溯历史评审记录

---

## 六、可视化界面

**技术栈：** Go（HTTP API） + React + TailwindCSS + ECharts

### 主要页面

| 页面 | 功能 |
|------|------|
| 数据管理 | 导入原始数据、转换状态、tushare同步状态 |
| 策略工厂 | 创建/编辑策略、批量回测配置、过滤规则配置 |
| 回测结果 | 净值曲线、月度热力图、Walk-Forward分段图、策略对比 |
| Agent评审 | 评审队列、实时进度、评审报告、PDF导出 |
| 策略池 | 通过评审的策略列表、历史决策记录 |

---

## 七、策略广场（私有后台）

### 7.1 策略生命周期

```
草稿 → 待回测 → 回测中 → 待评审 → 评审中 → 策略池 → 实盘中
                                                    ↓
                                                 已归档
```

### 7.2 核心功能

- **策略录入：** 名称、描述、类型（技术面/基本面/事件驱动）、标签、Python代码编辑器（语法高亮）
- **参数配置：** 可调参数列表，支持范围扫描批量回测
- **版本控制：** 每次代码修改自动保存版本，支持版本对比和回滚
- **横向对比：** 所有策略绩效对比看板，按当前市场环境筛选历史最优策略

---

## 八、技术栈汇总

| 层级 | 技术 |
|------|------|
| 回测引擎 | Go |
| 策略逻辑 | Python（通过gRPC调用引擎）|
| 跨语言通信 | gRPC + Protocol Buffers |
| 数据格式 | Parquet（行情）、待定数据库（元数据）|
| 数据获取 | tushare Pro |
| LLM接入 | OpenAI / Claude / Ollama（可配置）|
| 前端 | React + TailwindCSS + ECharts |
| 后端API | Go HTTP |

---

## 九、开发阶段规划

| 阶段 | 内容 | 产出 |
|------|------|------|
| 第一阶段 | 数据层 + Go回测引擎 | 能跑单个策略的回测 |
| 第二阶段 | Python策略接口 + 策略评估层 | 批量回测 + Walk-Forward验证 |
| 第三阶段 | 可视化界面 + 策略广场 | 完整可操作的管理后台 |
| 第四阶段 | Agent评审层 | LLM多Agent评审报告 |
| 第五阶段 | 实盘对接 | tushare实时数据 + 券商API |
