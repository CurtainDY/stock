package main

import (
	"archive/zip"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	_ "github.com/lib/pq"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ---- 数据结构 ----

type Bar struct {
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

type CheckResult struct {
	Symbol         string
	Frequency      string
	ExpectedBars   int
	ActualBars     int
	MissingDays    int
	ZeroVolumeBars int
	FirstDate      time.Time
	LastDate       time.Time
	Status         string
	Detail         string
}

// ---- 命令行参数 ----

var (
	zipPath      = flag.String("zip", "", "源zip文件路径")
	outputDir    = flag.String("output", "data/normalized", "Parquet输出根目录")
	dsn          = flag.String("dsn", "postgres://stock:stock123@localhost:5432/stock?sslmode=disable", "PostgreSQL连接串")
	workers      = flag.Int("workers", 4, "并发worker数")
	freqLabel    = flag.String("freq", "1m", "频率标签: 1m/5m/15m/30m/60m")
	checkOnly    = flag.Bool("check-only", false, "只检查不导入，不需要DB连接")
	symbolFilter = flag.String("symbol", "", "只处理指定股票，逗号分隔，空=全部")
)

var symbolRe = regexp.MustCompile(`^(sz|sh|bj)\d+$`)

func main() {
	flag.Parse()
	if *zipPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// 尝试加载交易日历，失败时用近似算法
	var cal *tradingCalendar
	calPath := filepath.Join(filepath.Dir(*outputDir), "calendar.csv")
	if c, err := loadCalendar(calPath); err == nil {
		cal = c
		log.Printf("trading calendar loaded from %s", calPath)
	} else {
		log.Printf("WARNING: calendar not found at %s, using approximate weekday calculation", calPath)
	}

	// DB连接（check-only模式跳过）
	var db *sql.DB
	var batchID int
	if !*checkOnly {
		var err error
		db, err = sql.Open("postgres", *dsn)
		if err != nil {
			log.Fatalf("connect pg: %v", err)
		}
		defer db.Close()
		if err := db.Ping(); err != nil {
			log.Fatalf("ping pg: %v", err)
		}
		log.Println("PostgreSQL connected")
		db.QueryRow(
			`INSERT INTO import_batches(source_file, frequency) VALUES($1,$2) RETURNING id`,
			*zipPath, *freqLabel,
		).Scan(&batchID)
		log.Printf("batch #%d started", batchID)
	}

	// 过滤集合
	filterSet := make(map[string]bool)
	if *symbolFilter != "" {
		for _, s := range strings.Split(*symbolFilter, ",") {
			filterSet[strings.TrimSpace(s)] = true
		}
	}

	// 打开zip，收集目标CSV
	zr, err := zip.OpenReader(*zipPath)
	if err != nil {
		log.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	var csvFiles []*zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".csv") {
			continue
		}
		sym := extractSymbol(f.Name)
		if sym == "" {
			continue
		}
		if len(filterSet) > 0 && !filterSet[sym] {
			continue
		}
		csvFiles = append(csvFiles, f)
	}
	log.Printf("found %d CSV files to process", len(csvFiles))

	if db != nil {
		db.Exec(`UPDATE import_batches SET total_files=$1 WHERE id=$2`, len(csvFiles), batchID)
	}

	// 并发处理
	jobs := make(chan *zip.File, len(csvFiles))
	for _, f := range csvFiles {
		jobs <- f
	}
	close(jobs)

	var (
		okCount    int64
		skipCount  int64
		errCount   int64
		mu         sync.Mutex
		allChecks  []CheckResult
	)

	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				sym := extractSymbol(f.Name)
				check, err := processFile(f, sym, *freqLabel, *outputDir, *checkOnly, cal)
				if err != nil {
					log.Printf("[ERROR] %s: %v", sym, err)
					atomic.AddInt64(&errCount, 1)
					check = CheckResult{Symbol: sym, Frequency: *freqLabel, Status: "error", Detail: err.Error()}
				} else if check.Status == "ok" {
					atomic.AddInt64(&okCount, 1)
				} else {
					atomic.AddInt64(&skipCount, 1)
				}

				mu.Lock()
				allChecks = append(allChecks, check)
				total := int64(len(allChecks))
				mu.Unlock()

				if total%100 == 0 {
					log.Printf("progress %d/%d ok=%d skip=%d err=%d",
						total, len(csvFiles),
						atomic.LoadInt64(&okCount),
						atomic.LoadInt64(&skipCount),
						atomic.LoadInt64(&errCount))
				}
			}
		}()
	}
	wg.Wait()

	// 写check结果到DB
	if db != nil {
		for _, c := range allChecks {
			db.Exec(`
				INSERT INTO import_checks
				(batch_id,symbol,frequency,expected_bars,actual_bars,missing_days,
				 zero_volume_bars,first_date,last_date,status,detail)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				batchID, c.Symbol, c.Frequency,
				c.ExpectedBars, c.ActualBars, c.MissingDays, c.ZeroVolumeBars,
				nullTime(c.FirstDate), nullTime(c.LastDate), c.Status, c.Detail,
			)
		}
		db.Exec(`UPDATE import_batches SET finished_at=NOW(),status='done',imported=$1,skipped=$2,errors=$3 WHERE id=$4`,
			okCount, skipCount, errCount, batchID)
	}

	// 汇总输出
	log.Printf("\n=== 完成 ===")
	log.Printf("总计: %d  成功: %d  问题: %d  错误: %d",
		len(csvFiles), okCount, skipCount, errCount)

	var problems []CheckResult
	for _, c := range allChecks {
		if c.Status != "ok" {
			problems = append(problems, c)
		}
	}
	if len(problems) > 0 {
		log.Printf("\n=== 数据问题 (%d只) ===", len(problems))
		for _, p := range problems {
			log.Printf("  %-15s [%8s] bars=%d/%d missing_days=%d  %s",
				p.Symbol, p.Status, p.ActualBars, p.ExpectedBars, p.MissingDays, p.Detail)
		}
	} else {
		log.Printf("数据完整性：全部通过 ✓")
	}
}

// processFile 解析CSV → 校验 → 写Parquet
func processFile(f *zip.File, symbol, freq, outputDir string, checkOnly bool, cal *tradingCalendar) (CheckResult, error) {
	rc, err := f.Open()
	if err != nil {
		return CheckResult{}, err
	}
	defer rc.Close()

	bars, err := parseCSV(rc, symbol)
	if err != nil {
		return CheckResult{}, fmt.Errorf("parse: %w", err)
	}

	check := checkBars(bars, symbol, freq, cal)

	if !checkOnly && len(bars) > 0 {
		if err := writeParquet(bars, symbol, freq, outputDir); err != nil {
			return check, fmt.Errorf("write parquet: %w", err)
		}
	}
	return check, nil
}

// parseCSV 支持UTF-8（含BOM）和GBK编码
func parseCSV(r io.Reader, symbol string) ([]Bar, error) {
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
		decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), data)
		if err != nil {
			return nil, fmt.Errorf("gbk decode: %w", err)
		}
		src = string(decoded)
	}

	records, err := csv.NewReader(strings.NewReader(src)).ReadAll()
	if err != nil {
		return nil, err
	}

	var bars []Bar
	for i, rec := range records {
		if i == 0 {
			continue
		}
		if len(rec) < 7 {
			continue
		}
		b := Bar{Symbol: symbol, DateStr: strings.TrimSpace(rec[0]), AdjFactor: 1.0}
		b.Open, _ = strconv.ParseFloat(strings.TrimSpace(rec[1]), 64)
		b.High, _ = strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		b.Low, _ = strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		b.Close, _ = strconv.ParseFloat(strings.TrimSpace(rec[4]), 64)
		b.Volume, _ = strconv.ParseFloat(strings.TrimSpace(rec[5]), 64)
		b.Amount, _ = strconv.ParseFloat(strings.TrimSpace(rec[6]), 64)
		bars = append(bars, b)
	}
	return bars, nil
}

// checkBars 完整性校验
func checkBars(bars []Bar, symbol, freq string, cal *tradingCalendar) CheckResult {
	c := CheckResult{Symbol: symbol, Frequency: freq}
	if len(bars) == 0 {
		c.Status = "empty"
		c.Detail = "no data"
		return c
	}

	first, err1 := parseDate(bars[0].DateStr)
	last, err2 := parseDate(bars[len(bars)-1].DateStr)
	if err1 != nil || err2 != nil {
		c.Status = "error"
		c.Detail = "cannot parse dates"
		return c
	}
	c.FirstDate = first
	c.LastDate = last
	c.ActualBars = len(bars)
	tradingDays := 0
	if cal != nil {
		tradingDays = cal.countTradingDays(first, last)
	} else {
		tradingDays = estimateTradingDays(first, last)
	}
	c.ExpectedBars = tradingDays * barsPerDay(freq)

	// 按日统计
	daySet := make(map[string]bool)
	for _, b := range bars {
		if b.Volume == 0 {
			c.ZeroVolumeBars++
		}
		daySet[b.DateStr[:10]] = true
	}
	expectedDays := tradingDays // reuse from above
	c.MissingDays = expectedDays - len(daySet)
	if c.MissingDays < 0 {
		c.MissingDays = 0
	}

	completeness := float64(c.ActualBars) / float64(c.ExpectedBars) * 100
	switch {
	case completeness < 50:
		// 严重缺失：数据文件可能损坏或未下载完整
		c.Status = "incomplete"
		c.Detail = fmt.Sprintf("only %.1f%% complete, likely download issue", completeness)
	case completeness < 80:
		// 较多缺失：可能部分年份未下载
		c.Status = "incomplete"
		c.Detail = fmt.Sprintf("%.1f%% complete, missing %d trading days", completeness, c.MissingDays)
	default:
		// 80%以上视为正常（停牌、节假日误差等）
		c.Status = "ok"
		if c.MissingDays > 50 {
			c.Detail = fmt.Sprintf("%.1f%% complete (%d days gap, likely suspensions)", completeness, c.MissingDays)
		}
	}
	return c
}

// writeParquet 输出路径：{outputDir}/{freq}/{symbol}.parquet
func writeParquet(bars []Bar, symbol, freq, outputDir string) error {
	dir := filepath.Join(outputDir, freq)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, symbol+".parquet"))
	if err != nil {
		return err
	}
	defer f.Close()
	return parquet.Write(f, bars)
}

func extractSymbol(name string) string {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(name), ".csv"))
	if symbolRe.MatchString(base) {
		return base
	}
	return ""
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date %q", s)
}

// tradingCalendar 从 CSV 文件加载交易日历
// 文件格式：date,is_open（两列，有表头）
type tradingCalendar struct {
	openDays map[string]bool // key: "2006-01-02"
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

func estimateTradingDays(start, end time.Time) int {
	n := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n++
		}
	}
	return int(float64(n) * 0.97)
}

func barsPerDay(freq string) int {
	switch freq {
	case "1m":
		return 240
	case "5m":
		return 48
	case "15m":
		return 16
	case "30m":
		return 8
	case "60m":
		return 4
	}
	return 240
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
