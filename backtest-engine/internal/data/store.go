package data

import "time"

type DataStore interface {
	LoadBars(symbol string, start, end time.Time) ([]Bar, error)
	LoadBarsByDate(date time.Time, symbols []string) (map[string]Bar, error)
	Symbols() ([]string, error)
	TradingDays(start, end time.Time) ([]time.Time, error)
}
