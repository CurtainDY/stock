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

	zipPath, err := findLatestZip(*adjDir, "同花顺")
	if err != nil {
		log.Fatalf("findLatestZip: %v", err)
	}
	log.Printf("使用复权因子文件: %s", zipPath)

	factors, err := loadFactors(zipPath)
	if err != nil {
		log.Fatalf("loadFactors: %v", err)
	}
	log.Printf("加载了 %d 只股票的复权因子", len(factors))

	symbols := symbolList(factors, *symFilter)
	log.Printf("待处理股票数: %d", len(symbols))

	freqDir := filepath.Join(*dataDir, *freqFlag)
	for _, sym := range symbols {
		dateFactors := factors[sym]
		parquetPath := filepath.Join(freqDir, sym+".parquet")
		if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
			continue
		}
		if err := applyFactors(parquetPath, dateFactors); err != nil {
			log.Printf("applyFactors %s: %v", sym, err)
		}
	}
	log.Println("完成")
}

// findLatestZip returns the last (alphabetically sorted) zip file in dir whose name contains keyword.
func findLatestZip(dir, keyword string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read dir %s: %w", dir, err)
	}
	var matches []string
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), keyword) && strings.HasSuffix(e.Name(), ".zip") {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no zip containing %q in %s", keyword, dir)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// loadFactors opens a zip and parses each CSV file inside.
// Returns map[symbol]map[date]factor.
func loadFactors(zipPath string) (map[string]map[string]float64, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	defer r.Close()

	result := make(map[string]map[string]float64)
	for _, f := range r.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			continue
		}
		sym := strings.ToLower(strings.TrimSuffix(filepath.Base(f.Name), ".csv"))
		rc, err := f.Open()
		if err != nil {
			log.Printf("open %s in zip: %v", f.Name, err)
			continue
		}
		dateFactors, err := parseFactorCSV(rc)
		rc.Close()
		if err != nil {
			log.Printf("parseFactorCSV %s: %v", f.Name, err)
			continue
		}
		result[sym] = dateFactors
	}
	return result, nil
}

// parseFactorCSV parses a Tonghuashun adj-factor CSV (GBK or UTF-8, optional BOM).
// CSV columns: 日期, 前复权因子, 后复权因子
// Returns map[date "YYYY-MM-DD"]factor (column index 1, 前复权因子).
func parseFactorCSV(r io.Reader) (map[string]float64, error) {
	// Read all bytes to detect encoding.
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Strip UTF-8 BOM if present.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	// Detect whether the content is valid UTF-8; if not, decode as GBK.
	var reader io.Reader
	if utf8.Valid(data) {
		reader = strings.NewReader(string(data))
	} else {
		decoder := simplifiedchinese.GBK.NewDecoder()
		decoded, _, err := transform.Bytes(decoder, data)
		if err != nil {
			return nil, fmt.Errorf("GBK decode: %w", err)
		}
		reader = strings.NewReader(string(decoded))
	}

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1 // allow variable fields

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv read: %w", err)
	}

	result := make(map[string]float64)
	for i, rec := range records {
		if i == 0 {
			// Skip header.
			continue
		}
		if len(rec) < 2 {
			continue
		}
		dateStr := strings.TrimSpace(rec[0])
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			continue
		}
		factorStr := strings.TrimSpace(rec[1])
		factor, err := strconv.ParseFloat(factorStr, 64)
		if err != nil {
			continue
		}
		result[dateStr] = factor
	}
	return result, nil
}

// applyFactors reads a Parquet file, updates AdjFactor for matching dates, and writes it back.
func applyFactors(path string, dateFactors map[string]float64) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	reader := parquet.NewGenericReader[parquetRow](f)
	var rows []parquetRow
	buf := make([]parquetRow, 1000)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			rows = append(rows, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	reader.Close()
	f.Close()

	updated := 0
	for i := range rows {
		dateKey := rows[i].DateStr
		if len(dateKey) >= 10 {
			dateKey = dateKey[:10]
		}
		if factor, ok := dateFactors[dateKey]; ok {
			rows[i].AdjFactor = factor
			updated++
		}
	}

	if updated == 0 {
		return nil
	}

	// Write back to the same path (overwrite).
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer out.Close()

	if err := parquet.Write[parquetRow](out, rows); err != nil {
		return fmt.Errorf("parquet write %s: %w", path, err)
	}
	log.Printf("updated %d rows in %s", updated, filepath.Base(path))
	return nil
}

// symbolList returns a sorted list of symbols from the factors map,
// filtered by the comma-separated filter string (empty = all).
func symbolList(factors map[string]map[string]float64, filter string) []string {
	filterSet := make(map[string]bool)
	if filter != "" {
		for _, s := range strings.Split(filter, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			if s != "" {
				filterSet[s] = true
			}
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
