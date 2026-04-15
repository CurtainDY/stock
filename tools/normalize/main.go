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
		log.Printf("wrote %d rows → %s", len(rows), outPath)
	}
}

// parseCSV expects columns: symbol,date,open,high,low,close,volume,amount,adj_factor
func parseCSV(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}

	var rows []Row
	for i, rec := range records {
		if i == 0 {
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
