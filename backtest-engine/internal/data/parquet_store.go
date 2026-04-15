package data

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/parquet-go/parquet-go"
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

func (r parquetRow) toBar() (Bar, error) {
	date, err := time.Parse("2006-01-02", r.DateStr)
	if err != nil {
		return Bar{}, fmt.Errorf("parse date %q: %w", r.DateStr, err)
	}
	return Bar{
		Symbol: r.Symbol, Date: date,
		Open: r.Open, High: r.High, Low: r.Low, Close: r.Close,
		Volume: r.Volume, Amount: r.Amount, AdjFactor: r.AdjFactor,
	}, nil
}

type ParquetStore struct {
	dataDir string
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
				continue
			}
			return nil, fmt.Errorf("load %d: %w", year, err)
		}
		result = append(result, bars...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date.Before(result[j].Date) })
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
			if rows[i].Symbol != symbol {
				continue
			}
			bar, parseErr := rows[i].toBar()
			if parseErr != nil {
				return nil, parseErr
			}
			if !bar.Date.Before(start) && !bar.Date.After(end) {
				bars = append(bars, bar)
			}
		}
		if err != nil {
			break
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
			if _, ok := symSet[rows[i].Symbol]; !ok {
				continue
			}
			bar, parseErr := rows[i].toBar()
			if parseErr != nil {
				return nil, parseErr
			}
			if bar.Date.Equal(date) {
				result[rows[i].Symbol] = bar
			}
		}
		if err != nil {
			break
		}
	}
	return result, nil
}

func (s *ParquetStore) Symbols() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.dataDir, "daily"))
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
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
