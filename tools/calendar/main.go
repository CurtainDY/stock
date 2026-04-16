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
	log.Printf("written %d days to %s", len(days), *output)
}

type calendarDay struct {
	Date   string
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
		isOpenRaw, _ := item[1].(float64)
		isOpen := isOpenRaw == 1
		days = append(days, calendarDay{Date: date, IsOpen: isOpen})
	}
	return days, nil
}

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
