# A股量化回测系统 — 进展记录与问题规格

**日期：** 2026-04-16  
**状态：** Phase 1 基本完成，Phase 2 待启动

---

## 一、项目背景与目标

构建一个私有A股量化回测系统，目标是通过严谨的历史回测验证策略，最终服务于实盘交易决策。

**核心思路：**
1. 批量回测多种策略，按绩效指标过滤
2. 通过 Walk-Forward 验证防止过拟合
3. LLM 多 Agent 评审团从不同角度审查通过筛选的策略
4. 人工最终确认，选定策略实盘

---

## 二、技术架构决策

### 2.1 已确定的技术栈

| 层级 | 技术 | 决策理由 |
|------|------|---------|
| 回测引擎 | Go | 性能好，用户主力语言 |
| 策略逻辑 | Python | 生态丰富（pandas/numpy/ta-lib），便于快速迭代 |
| 跨语言通信 | gRPC + protobuf | 强类型接口，Go/Python 双端支持好 |
| 行情数据存储 | Parquet（按频率/股票分片） | 参考 QUANTAXIS，读取性能优秀，无需额外服务 |
| 元数据/结果存储 | PostgreSQL（Docker） | 支持复杂查询，适合策略记录、完整性报告 |
| 前端 | React + TailwindCSS + ECharts | 待 Phase 3 实现 |
| LLM 接入 | 可配置（OpenAI/Claude/Ollama） | 待 Phase 4 实现 |

### 2.2 参考的开源项目

- **[vnpy/vnpy](https://github.com/vnpy/vnpy)**（⭐39k）：事件驱动回测引擎设计、A股撮合规则
- **[QUANTAXIS](https://github.com/QUANTAXIS/QUANTAXIS)**（⭐10k）：Parquet 分片存储方案、数据处理
- **[TradingAgents](https://github.com/TauricResearch/TradingAgents)**（⭐45k）：多 Agent 评审架构参考
- **arXiv:2409.06289**：LLM 自动生成 Alpha 因子，在 SSE50 实现 53% 累计收益

---

## 三、当前实现进展

### Phase 1：数据层 + Go 回测引擎（✅ 基本完成）

#### 已实现文件

```
backtest-engine/
├── cmd/server/main.go              # gRPC 服务入口
├── internal/
│   ├── data/
│   │   ├── bar.go                  # Bar 数据结构（含涨跌停判断）
│   │   ├── store.go                # DataStore 接口
│   │   └── parquet_store.go        # Parquet 文件读取实现
│   ├── engine/
│   │   ├── engine.go               # 回测主循环（事件驱动）
│   │   └── eventbus.go             # 事件类型定义
│   ├── matcher/
│   │   └── matcher.go              # 撮合引擎（A股规则）
│   ├── portfolio/
│   │   └── portfolio.go            # 持仓/资金管理（T+1）
│   └── analytics/
│       └── analytics.go            # 绩效指标计算
└── proto/backtest.proto            # gRPC 接口定义

tools/
├── importer/main.go                # 数据导入工具（zip→Parquet+PG）
└── normalize/main.go               # CSV→Parquet 转换工具（旧版，已被 importer 替代）

migrations/001_init.sql             # PostgreSQL schema
docker-compose.yml                  # PostgreSQL 16 容器配置
```

#### 已实现的 A股特有规则

| 规则 | 实现方式 |
|------|---------|
| 涨跌停（普通股10%，ST股5%） | `Bar.IsLimitUp/IsLimitDown` + Matcher 拒单 |
| T+1（当日买次日可卖） | Portfolio 持仓 lot 记录买入日期 |
| 印花税（卖出0.1%） | Matcher 成交时计算 |
| 佣金（双向0.03%） | Matcher 成交时计算 |
| 停牌处理 | DataStore 无数据时跳过（接口预留） |

#### 测试覆盖

所有模块均有单元测试，`go test ./...` 全部通过：
- `matcher_test.go`：普通买卖、涨跌停拒单、ST涨跌停、印花税
- `portfolio_test.go`：买入、T+1验证、卖出减仓、现金计算
- `analytics_test.go`：夏普比率、最大回撤、年化收益
- `engine_test.go`：买入持有集成测试（mockStore）

---

## 四、数据情况

### 4.1 原始数据格式（淘宝购买）

```
data/raw/
├── 分钟K线-股票241/
│   ├── 2000-2025/
│   │   ├── 1分钟.zip     # ~5487只股票，约400GB未压缩
│   │   ├── 5分钟.zip
│   │   ├── 15分钟.zip
│   │   ├── 30分钟.zip
│   │   └── 60分钟.zip
│   └── 示例/             # 少量样本CSV
├── 分钟K线-ETF/
├── 分钟K线-指数/
└── 复权因子/             # zip包含每只股票的复权因子CSV
```

**CSV 列格式：**
```
日期, 开盘, 最高, 最低, 收盘, 成交量(股), 成交额(元),
涨跌(元), 涨跌幅(%), 换手率(%), 流通股本(股), 总股本(股)
```

**文件命名规则：**`{exchange}{code}.csv`，如 `sz000001.csv`、`sh600000.csv`、`bj920000.csv`

**编码：** UTF-8（含BOM）或 GBK，导入工具自动检测转换

### 4.2 数据完整性检查机制

导入工具（`tools/importer`）内置完整性校验：

```
校验维度：
├── 实际 Bar 数 vs 预期 Bar 数（按交易日历估算）
├── 缺失交易日数（按日期分组统计）
├── 成交量为 0 的 Bar 数
└── 首末日期范围
```

**状态分级：**

| 状态 | 条件 | 含义 |
|------|------|------|
| `ok` | 完整度 ≥ 80% | 正常（含合理停牌缺口） |
| `incomplete` | 完整度 50%-80% | 可能部分年份未下载 |
| `incomplete` | 完整度 < 50% | 严重缺失，疑似下载不完整 |
| `empty` | 0条数据 | 文件为空 |
| `error` | 解析失败 | 文件损坏或格式异常 |

**使用方式：**
```bash
# 仅检查，不导入（不需要 DB）
./importer -zip "data/raw/.../1分钟.zip" -freq 1m -check-only

# 检查 + 导入 Parquet + 写入 PG 记录
./importer -zip "data/raw/.../1分钟.zip" -freq 1m

# 只检查特定股票
./importer -zip "..." -freq 1m -check-only -symbol "sz000001,sh600000"
```

---

## 五、遇到的问题与解决方案

### 问题1：浮点精度导致测试失败

**现象：** `commission = 2.9999999999999996, want 3`  
**原因：** Go 浮点乘法精度损失（`10.0 * 1000 * 0.0003`）  
**解决：** 测试中改用容差比较（`±0.0001`），不用精确等号  
**规格要求：** 所有涉及金额的比较必须使用容差，生产代码如需精确计算考虑引入 `decimal` 库

---

### 问题2：回测引擎测试返回错误收益率

**现象：** 买入持有测试期望 >15% 收益，实际只有 2%  
**原因：** 测试只买了 1000 股（10元/股 = 1万元），占 10万本金的 10%，价格涨 20% 对总收益只有 2%  
**解决：** 改为买 9000 股（占本金 90%），收益 >15% ✓  
**规格要求：** 回测测试用例需明确资金使用率，确保测试场景有意义

---

### 问题3：最大回撤断言错误

**现象：** 单调上涨行情下，`MaxDrawdown != 0`  
**原因：** 第一天买入时扣除手续费，导致净值从 100000 略降到 99997，产生 0.003% 微小回撤  
**解决：** 断言改为 `MaxDrawdown < 1%`  
**规格要求：** 最大回撤计算包含手续费成本，不能假设初始净值等于投入资金

---

### 问题4：交易日估算不准确

**现象：** `estimateTradingDays` 使用工作日 × 0.97 近似，sh600000 显示完整度 106.4%（超过100%）  
**原因：** A股实际节假日扣除率约为工作日的 0.94-0.95，用 0.97 导致低估预期Bar数，使实际值看起来超过100%  
**当前处理：** 完整度 > 80% 统一视为 ok，超过 100% 不报错  
**待优化：** Phase 2 引入准确的 A股交易日历（tushare 提供），替换估算逻辑  
**规格要求：** 最终版本必须使用真实交易日历，不能用近似估算

---

### 问题5：Docker 未安装

**现象：** `docker: command not found`  
**影响：** PostgreSQL 无法启动，导入工具的 DB 写入功能无法测试  
**临时方案：** 导入工具增加 `-check-only` 模式，跳过 DB 连接，只校验数据和写 Parquet  
**待解决：** 用户需安装 Docker Desktop，然后 `docker compose up -d` 启动 PG  
**规格要求：** 所有外部依赖（PostgreSQL、LLM服务）必须可以通过 `-check-only` 或 mock 模式绕过，确保离线可用

---

### 问题6：复权因子未集成

**现象：** 当前所有 Bar 的 `AdjFactor = 1.0`，使用的是未复权价格  
**影响：** 长期回测中，股价因分红/送股出现跳空，会被误判为极端行情  
**复权因子数据：** 已在 `data/raw/复权因子/` 目录，zip 内每只股票一个 CSV  
**待实现：** Phase 2 导入复权因子，更新 Parquet 文件中的 `AdjFactor` 字段  
**规格要求：** 回测引擎必须支持前复权价格（`Bar.AdjClose()`），所有价格信号基于复权价

---

### 问题7：gRPC SubmitOrder 未完整实现

**现象：** `cmd/server/main.go` 中 `SubmitOrder` 返回固定 REJECTED  
**原因：** Phase 1 仅实现引擎内部回测（Go 策略接口），gRPC 驱动的 Python 策略回测在 Phase 2  
**规格要求：** Phase 2 必须实现完整的 gRPC 会话管理：  
- 每个回测会话独立的 Portfolio 实例  
- StreamBars 推送 → Python 决策 → SubmitOrder 撮合 → 结果返回  
- 会话超时和清理机制

---

### 问题8：幸存者偏差未完全处理

**现象：** `stocks` 表已有 `delisted_date` 字段，但尚未填充退市数据  
**影响：** 如果回测股票池只用当前在市股票，会高估历史收益（退市股票往往业绩差）  
**待实现：** 导入 tushare 的退市股票列表，填充 `stocks.delisted_date`  
**规格要求：** 所有策略回测必须按回测日期动态构建股票池，不能使用当前在市列表

---

## 六、待完成工作（分阶段）

### Phase 2：Python 策略接口 + 策略评估层

- [ ] 实现 gRPC 会话管理（每次回测独立 context）
- [ ] Python gRPC 客户端（`BacktestClient`）
- [ ] 示例策略：双均线（MA Cross）
- [ ] Walk-Forward 验证框架
- [ ] 过滤规则引擎（可配置阈值）
- [ ] 导入复权因子，更新 Parquet
- [ ] 引入真实 A 股交易日历（替换 estimateTradingDays）

### Phase 3：可视化界面 + 策略广场

- [ ] Go HTTP API 层（复用回测引擎）
- [ ] React + TailwindCSS + ECharts 前端
- [ ] 策略广场后台（录入、版本控制、生命周期管理）
- [ ] 回测结果可视化（净值曲线、月度热力图、Walk-Forward 图）
- [ ] 过滤规则可视化配置界面

### Phase 4：Agent 评审层

- [ ] 四个 Agent 角色（收益/风险/环境/魔鬼代言人）
- [ ] LLM 可配置接入（OpenAI/Claude/Ollama）
- [ ] 评审报告持久化与展示

### Phase 5：实盘对接

- [ ] tushare 实时行情同步
- [ ] 券商 API 对接（下单执行）
- [ ] 策略实盘监控

---

## 七、已知技术债务

| 项目 | 严重程度 | 说明 |
|------|---------|------|
| 复权因子未集成 | 高 | 长期回测数据不准确 |
| 交易日历用近似估算 | 中 | 影响完整性校验准确度 |
| gRPC 无会话管理 | 中 | Phase 2 必须解决 |
| 退市股票数据缺失 | 中 | 存在幸存者偏差风险 |
| 工具模块独立 go.mod | 低 | 应改为 Go workspace（go.work）统一管理 |
| tools/normalize 已废弃 | 低 | 被 tools/importer 替代，可删除 |
