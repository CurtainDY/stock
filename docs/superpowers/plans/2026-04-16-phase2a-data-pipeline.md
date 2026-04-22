# Phase 2a: 数据管道补全 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补全数据管道：导入复权因子并写入 Parquet 文件，引入真实 A 股交易日历替换近似估算。

**Architecture:** 两个独立工具。`tools/adjfactor` 读取复权因子 zip，将因子合并写入已有 Parquet 文件。`tools/calendar` 从 tushare 拉取交易日历并缓存到 `data/calendar.csv`，供 `tools/importer` 用于精确的完整性校验。

**Tech Stack:** Go 1.22+, parquet-go, lib/pq, tushare HTTP API（需 token）

---

## 参考资料

- 复权因子数据路径：`data/raw/复权因子/复权因子(同花顺).zip`（每日更新版取最新一份）
- 复权因子 CSV 格式：`日期, 前复权因子, 后复权因子`（同花顺格式，前复权因子为负数，含义见 Task 1）
- tushare 交易日历 API：`trade_cal`，返回字段 `cal_date, is_open`
- 现有 Parquet 路径：`data/normalized/{freq}/{symbol}.parquet`

---

## 文件结构

```
tools/
├── adjfactor/
│   ├── go.mod
│   └── main.go          # 复权因子导入工具：zip → 读 Parquet → 更新 AdjFactor → 写回
├── calendar/
│   ├── go.mod
│   └── main.go          # 交易日历工具：tushare → data/calendar.csv
└── importer/
    └── main.go          # 修改：替换 estimateTradingDays，改用 calendar.csv
data/
└── calendar.csv         # 交易日历缓存（程序生成，gitignore）
```

---

## Task 1: 理解复权因子格式并实现读取

**背景：** 同花顺的「前复权因子」是负数（如 -0.0818），不是乘法因子。
其含义是加法调整：`adj_price = raw_price + factor`（即 factor 为历史价格的负向偏移）。
换算为乘法因子：`adj_multiplier = (raw_price + factor) / raw_price`，但由于我们存储的是原始价格，
实际使用时直接存储加法因子（`AdjFactor` 字段存 factor 值），`Bar.AdjClose()` 改为 `Close + AdjFactor`。

> **注意：** 规格中 `AdjClose = Close × AdjFactor` 需更新为加法形式。

**Files:**
- Create: `tools/adjfactor/main.go`
- Modify: `backtest-engine/internal/data/bar.go`（更新 AdjClose 计算方式）
- Modify: `backtest-engine/internal/data/bar_test.go`（更新测试）

- [ ] **Step 1: 修改 bar.go，支持加法复权**

打开 `backtest-engine/internal/data/bar.go`，将 `AdjClose` 方法改为：

```go
// AdjClose 返回前复权价格。
// AdjFactor 存储的是同花顺加法偏移量（通常为负数）。
// 若 AdjFactor == 0 表示无复权数据，直接返回 Close。
func (b Bar) AdjClose() float64 {
    if b.AdjFactor == 0 {
        return b.Close
    }
    return b.Close + b.AdjFactor
}
```

- [ ] **Step 2: 更新 bar_test.go**

在 `backtest-engine/internal/data/store_test.go` 中找到 `TestBarAdjClose`，更新为：

```go
func TestBarAdjClose(t *testing.T) {
    // 同花顺加法复权：adj = close + factor
    b := data.Bar{Close: 10.0, AdjFactor: -1.0}
    want := 9.0
    got := b.AdjClose()
    if got != want {
        t.Errorf("AdjClose() = %v, want %v", got, want)
    }
}

func TestBarAdjCloseNoFactor(t *testing.T) {
    // AdjFactor=0 表示无数据，直接返回 Close
    b := data.Bar{Close: 10.0, AdjFactor: 0}
    want := 10.0
    got := b.AdjClose()
    if got != want {
        t.Errorf("AdjClose() zero factor = %v, want %v", got, want)
    }
}
```

- [ ] **Step 3: 运行测试确认通过**

```bash
cd backtest-engine
go test ./internal/data/... -v -run TestBar
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add backtest-engine/internal/data/bar.go backtest-engine/internal/data/store_test.go
git commit -m "fix: update AdjClose to use additive factor (tonghuashun format)"
```

---

## Task 2: 复权因子导入工具

**Files:**
- Create: `tools/adjfactor/go.mod`
- Create: `tools/adjfactor/main.go`

**逻辑：**
1. 打开复权因子 zip（取最新日期的一份）
2. 解析每只股票的 CSV：`日期 → 前复权因子`
3. 读取对应股票的 Parquet 文件
4. 按日期匹配，更新 Bar.AdjFactor
5. 写回 Parquet 文件

- [ ] **Step 1: 初始化 Go 模块**

```bash
mkdir -p tools/adjfactor
cd tools/adjfactor
go mod init github.com/parsedong/stock/tools/adjfactor
go get github.com/parquet-go/parquet-go@latest golang.org/x/text@latest
```

- [ ] **Step 2: 实现 main.go**

```go
package main

import (
	"archive/zip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/parquet-go/parquet-go"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type parquetRow struct {
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
	adjDir    = flag.String("adj", "data/raw/复权因子", "复权因子目录（含zip文件）")
	dataDir   = flag.String("data", "data/normalized", "Parquet数据根目录")
	freqFlag  = flag.String("freq", "1m", "频率: 1m/5m/15m/30m/60m")
	symFilter = flag.String("symbol", "", "只处理指定股票，逗号分隔，空=全部")
)

func main() {
	flag.Parse()

	// 找最新的同花顺复权因子zip
	zipPath, err := findLatestZip(*adjDir, "同花顺")
	if err != nil {
		log.Fatalf("find adj zip: %v", err)
	}
	log.Printf("using adj factor zip: %s", zipPath)

	// 加载复权因子：symbol -> date -> factor
	factors, err := loadFactors(zipPath)
	if err != nil {
		log.Fatalf("load factors: %v", err)
	}
	log.Printf("loaded factors for %d symbols", len(factors))

	// 确定要处理的 symbol 列表
	symbols := symbolList(factors, *symFilter)
	log.Printf("processing %d symbols", len(symbols))

	ok, skip, errCount := 0, 0, 0
	for _, sym := range symbols {
		parquetPath := filepath.Join(*dataDir, *freqFlag, sym+".parquet")
		if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
			skip++
			continue
		}
		if err := applyFactors(parquetPath, factors[sym]); err != nil {
			log.Printf("[ERROR] %s: %v", sym, err)
			errCount++
		} else {
			ok++
		}
		if (ok+skip+errCount)%500 == 0 {
			log.Printf("progress: ok=%d skip=%d err=%d", ok, skip, errCount)
		}
	}
	log.Printf("\n=== 完成 === ok=%d skip=%d err=%d", ok, skip, errCount)
}

// findLatestZip 在目录下找文件名含 keyword 的最新 zip
func findLatestZip(dir, keyword string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, e := range entries {
		if strings.Contains(e.Name(), keyword) && strings.HasSuffix(e.Name(), ".zip") {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no zip containing %q in %s", keyword, dir)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil // 按文件名排序，最后一个最新
}

// loadFactors 解析 zip 中所有 CSV，返回 symbol -> date(YYYY-MM-DD) -> 前复权因子
func loadFactors(zipPath string) (map[string]map[string]float64, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	result := make(map[string]map[string]float64)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".csv") {
			continue
		}
		sym := strings.ToLower(strings.TrimSuffix(filepath.Base(f.Name), ".csv"))
		if sym == "" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		dateFactors, err := parseFactorCSV(rc)
		rc.Close()
		if err != nil || len(dateFactors) == 0 {
			continue
		}
		result[sym] = dateFactors
	}
	return result, nil
}

// parseFactorCSV 解析复权因子CSV：日期,前复权因子,后复权因子
func parseFactorCSV(r io.Reader) (map[string]float64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	// 去BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	var src string
	if utf8.Valid(data) {
		src = string(data)
	} else {
		decoded, _, _ := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), data)
		src = string(decoded)
	}

	records, err := csv.NewReader(strings.NewReader(src)).ReadAll()
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64, len(records))
	for i, rec := range records {
		if i == 0 || len(rec) < 2 {
			continue
		}
		dateStr := strings.TrimSpace(rec[0])
		// 验证日期格式
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			continue
		}
		factor, err := strconv.ParseFloat(strings.TrimSpace(rec[1]), 64)
		if err != nil {
			continue
		}
		result[dateStr] = factor
	}
	return result, nil
}

// applyFactors 读取 Parquet，更新 AdjFactor，写回
func applyFactors(path string, dateFactors map[string]float64) error {
	// 读取所有行
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	reader := parquet.NewGenericReader[parquetRow](f)
	var rows []parquetRow
	buf := make([]parquetRow, 1000)
	for {
		n, err := reader.Read(buf)
		rows = append(rows, buf[:n]...)
		if err != nil {
			break
		}
	}
	reader.Close()
	f.Close()

	if len(rows) == 0 {
		return nil
	}

	// 更新 AdjFactor
	updated := 0
	for i := range rows {
		day := rows[i].DateStr[:10] // "2020-01-02"
		if factor, ok := dateFactors[day]; ok {
			rows[i].AdjFactor = factor
			updated++
		}
	}

	if updated == 0 {
		return nil // 没有匹配的因子，跳过写回
	}

	// 写回
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return parquet.Write(out, rows)
}

func symbolList(factors map[string]map[string]float64, filter string) []string {
	filterSet := make(map[string]bool)
	if filter != "" {
		for _, s := range strings.Split(filter, ",") {
			filterSet[strings.TrimSpace(s)] = true
		}
	}
	var syms []string
	for sym := range factors {
		if len(filterSet) == 0 || filterSet[sym] {
			syms = append(syms, sym)
		}
	}
	sort.Strings(syms)
	return syms
}
```

- [ ] **Step 3: 编译确认**

```bash
cd tools/adjfactor
go mod tidy
go build .
```

Expected: 编译成功，无报错

- [ ] **Step 4: 用示例数据验证（需要先有 Parquet 文件）**

```bash
# 先用 importer 导入示例数据
cd tools/importer
./importer \
  -zip "/Users/parsedong/workSpace/stock/data/raw/分钟K线-股票241/2000-2025/1分钟.zip" \
  -output "/Users/parsedong/workSpace/stock/data/normalized" \
  -freq 1m \
  -symbol "sz000001,sh600000" \
  -check-only=false

# 再运行复权因子导入
cd ../adjfactor
./adjfactor \
  -adj "/Users/parsedong/workSpace/stock/data/raw/复权因子" \
  -data "/Users/parsedong/workSpace/stock/data/normalized" \
  -freq 1m \
  -symbol "sz000001,sh600000"
```

Expected: `ok=2 skip=0 err=0`

- [ ] **Step 5: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add tools/adjfactor/
git commit -m "feat: add adj factor importer (tonghuashun additive format)"
```

---

## Task 3: 交易日历工具

**Files:**
- Create: `tools/calendar/go.mod`
- Create: `tools/calendar/main.go`

**逻辑：** 调用 tushare `trade_cal` API 拉取 SSE（上交所）交易日历，保存为 `data/calendar.csv`（两列：`date,is_open`）。支持 `-offline` 模式直接使用内置的简单规则（不调用 API）。

- [ ] **Step 1: 初始化模块**

```bash
mkdir -p tools/calendar
cd tools/calendar
go mod init github.com/parsedong/stock/tools/calendar
```

- [ ] **Step 2: 实现 main.go**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	token   = flag.String("token", os.Getenv("TUSHARE_TOKEN"), "tushare API token（或设置 TUSHARE_TOKEN 环境变量）")
	output  = flag.String("output", "data/calendar.csv", "输出路径")
	start   = flag.String("start", "19900101", "开始日期 YYYYMMDD")
	end     = flag.String("end", "", "结束日期 YYYYMMDD，默认今天")
	offline = flag.Bool("offline", false, "离线模式：用工作日近似，不调用 tushare")
)

func main() {
	flag.Parse()

	if *end == "" {
		*end = time.Now().Format("20060102")
	}

	os.MkdirAll(filepath.Dir(*output), 0755)

	if *offline {
		if err := generateOfflineCalendar(*start, *end, *output); err != nil {
			log.Fatalf("generate offline calendar: %v", err)
		}
		log.Printf("offline calendar written to %s", *output)
		return
	}

	if *token == "" {
		log.Fatal("tushare token required. Set -token flag or TUSHARE_TOKEN env var.\nUse -offline for offline mode.")
	}

	days, err := fetchTushareCalendar(*token, *start, *end)
	if err != nil {
		log.Fatalf("fetch tushare: %v", err)
	}

	if err := writeCSV(*output, days); err != nil {
		log.Fatalf("write csv: %v", err)
	}
	log.Printf("written %d trading days to %s", len(days), *output)
}

type calendarDay struct {
	Date   string // YYYY-MM-DD
	IsOpen bool
}

func fetchTushareCalendar(token, start, end string) ([]calendarDay, error) {
	body := fmt.Sprintf(`{
		"api_name": "trade_cal",
		"token": %q,
		"params": {"exchange": "SSE", "start_date": %q, "end_date": %q},
		"fields": "cal_date,is_open"
	}`, token, start, end)

	resp, err := http.Post(
		"https://api.tushare.pro",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Items [][]interface{} `json:"items"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w\nbody: %s", err, data)
	}
	if result.Msg != "" && result.Msg != "ok" {
		return nil, fmt.Errorf("tushare error: %s", result.Msg)
	}

	var days []calendarDay
	for _, item := range result.Data.Items {
		if len(item) < 2 {
			continue
		}
		dateRaw, _ := item[0].(string) // "20200102"
		if len(dateRaw) != 8 {
			continue
		}
		date := dateRaw[:4] + "-" + dateRaw[4:6] + "-" + dateRaw[6:]
		isOpenRaw, _ := item[1].(json.Number)
		isOpen := isOpenRaw.String() == "1"
		days = append(days, calendarDay{Date: date, IsOpen: isOpen})
	}
	return days, nil
}

// generateOfflineCalendar 用工作日近似（不含节假日）
func generateOfflineCalendar(startStr, endStr, output string) error {
	start, err := time.Parse("20060102", startStr)
	if err != nil {
		return err
	}
	end, err := time.Parse("20060102", endStr)
	if err != nil {
		return err
	}

	var days []calendarDay
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		isOpen := d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
		days = append(days, calendarDay{Date: d.Format("2006-01-02"), IsOpen: isOpen})
	}
	return writeCSV(output, days)
}

func writeCSV(path string, days []calendarDay) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("date,is_open\n")
	for _, d := range days {
		open := "0"
		if d.IsOpen {
			open = "1"
		}
		fmt.Fprintf(f, "%s,%s\n", d.Date, open)
	}
	return nil
}
```

- [ ] **Step 3: 编译确认**

```bash
cd tools/calendar
go mod tidy
go build .
```

Expected: 编译成功

- [ ] **Step 4: 用离线模式生成测试日历**

```bash
./calendar -offline -output /Users/parsedong/workSpace/stock/data/calendar.csv
head -5 /Users/parsedong/workSpace/stock/data/calendar.csv
```

Expected:
```
date,is_open
1990-01-01,0
1990-01-02,1
1990-01-03,1
1990-01-04,1
```

- [ ] **Step 5: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add tools/calendar/
git commit -m "feat: add trading calendar tool (tushare + offline mode)"
```

---

## Task 4: 更新 importer 使用真实交易日历

**Files:**
- Modify: `tools/importer/main.go`

**修改点：** 替换 `estimateTradingDays` 函数，改为从 `data/calendar.csv` 加载交易日历；若文件不存在则 fallback 到原有近似算法并打印警告。

- [ ] **Step 1: 在 importer/main.go 中添加日历加载函数**

在 `tools/importer/main.go` 的工具函数区（`estimateTradingDays` 之前）添加：

```go
// tradingCalendar 从 CSV 文件加载交易日历
// 文件格式：date,is_open（两列，有表头）
type tradingCalendar struct {
	openDays map[string]bool // key: "2020-01-02"
}

func loadCalendar(path string) (*tradingCalendar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}

	cal := &tradingCalendar{openDays: make(map[string]bool, len(records))}
	for i, rec := range records {
		if i == 0 || len(rec) < 2 {
			continue
		}
		cal.openDays[strings.TrimSpace(rec[0])] = strings.TrimSpace(rec[1]) == "1"
	}
	return cal, nil
}

func (c *tradingCalendar) countTradingDays(start, end time.Time) int {
	n := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if c.openDays[d.Format("2006-01-02")] {
			n++
		}
	}
	return n
}
```

- [ ] **Step 2: 添加全局 calendar 变量和初始化**

在 `main()` 函数顶部，`flag.Parse()` 之后添加：

```go
// 尝试加载交易日历，失败时用近似算法
var cal *tradingCalendar
calPath := filepath.Join(filepath.Dir(*outputDir), "calendar.csv")
if c, err := loadCalendar(calPath); err == nil {
    cal = c
    log.Printf("trading calendar loaded from %s", calPath)
} else {
    log.Printf("WARNING: calendar not found at %s, using approximate weekday calculation", calPath)
}
```

- [ ] **Step 3: 修改 checkBars 接受 calendar 参数**

将 `checkBars` 签名改为：

```go
func checkBars(bars []Bar, symbol, freq string, cal *tradingCalendar) CheckResult {
```

在函数内部将 `estimateTradingDays(first, last)` 改为：

```go
tradingDays := 0
if cal != nil {
    tradingDays = cal.countTradingDays(first, last)
} else {
    tradingDays = estimateTradingDays(first, last)
}
```

- [ ] **Step 4: 更新 processFile 传入 calendar**

```go
func processFile(f *zip.File, symbol, freq, outputDir string, checkOnly bool, cal *tradingCalendar) (CheckResult, error) {
    // ... 现有代码 ...
    check := checkBars(bars, symbol, freq, cal)
    // ...
}
```

在 `main()` 中调用 `processFile` 时传入 `cal`：

```go
check, err := processFile(f, sym, *freqLabel, *outputDir, *checkOnly, cal)
```

- [ ] **Step 5: 编译测试**

```bash
cd tools/importer
go build .
./importer \
  -zip "/Users/parsedong/workSpace/stock/data/raw/分钟K线-股票241/2000-2025/1分钟.zip" \
  -freq 1m \
  -symbol "sz000001,sh600000" \
  -check-only 2>&1
```

Expected: 输出中包含 `trading calendar loaded from` 或 `WARNING: calendar not found`，完整性数字更准确

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add tools/importer/main.go
git commit -m "feat: use real trading calendar in importer completeness check"
```

---

## Task 5: 更新 .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: 添加生成文件到 gitignore**

```bash
cat >> /Users/parsedong/workSpace/stock/.gitignore << 'EOF'
data/pg/
data/calendar.csv
data/normalized/
tools/adjfactor/adjfactor
tools/calendar/calendar
tools/importer/importer
EOF
```

- [ ] **Step 2: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add .gitignore
git commit -m "chore: gitignore generated data and binaries"
```

---

## 自检：Spec 覆盖确认

| Spec 要求 | 对应任务 |
|----------|---------|
| AdjFactor 在导入阶段填充 | Task 2 |
| AdjClose = Close + AdjFactor（加法复权） | Task 1 |
| 使用真实交易日历验证完整性 | Task 3 + Task 4 |
| check-only 模式不需要 DB | 已在 Phase 1 实现，Task 4 保持兼容 |
| data/raw 只读 | adjfactor 工具不修改 raw 目录 ✓ |
| 禁止硬编码连接信息 | tushare token 通过环境变量注入 ✓ |
