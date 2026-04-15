package engine_test

import (
	"testing"
	"time"

	"github.com/parsedong/stock/backtest-engine/internal/data"
	"github.com/parsedong/stock/backtest-engine/internal/engine"
	"github.com/parsedong/stock/backtest-engine/internal/portfolio"
)

type buyAndHold struct{ bought bool }

func (s *buyAndHold) OnBar(bar data.Bar, _ *portfolio.Portfolio) []engine.OrderEvent {
	if s.bought {
		return nil
	}
	s.bought = true
	// Buy ~90% of capital: 100000 * 0.9 / 10.0 = 9000 shares
	return []engine.OrderEvent{{Symbol: bar.Symbol, Side: 0, Quantity: 9000}}
}

type mockStore struct {
	days   []time.Time
	closes []float64
}

func (m *mockStore) LoadBars(symbol string, start, end time.Time) ([]data.Bar, error) {
	var bars []data.Bar
	for i, d := range m.days {
		if !d.Before(start) && !d.After(end) {
			bars = append(bars, data.Bar{Symbol: symbol, Date: d, Close: m.closes[i],
				Open: m.closes[i], High: m.closes[i], Low: m.closes[i], Volume: 1e6, AdjFactor: 1.0})
		}
	}
	return bars, nil
}

func (m *mockStore) LoadBarsByDate(date time.Time, symbols []string) (map[string]data.Bar, error) {
	result := make(map[string]data.Bar)
	for i, d := range m.days {
		if d.Equal(date) {
			for _, sym := range symbols {
				result[sym] = data.Bar{Symbol: sym, Date: d, Close: m.closes[i],
					Open: m.closes[i], High: m.closes[i], Low: m.closes[i], Volume: 1e6, AdjFactor: 1.0}
			}
		}
	}
	return result, nil
}

func (m *mockStore) Symbols() ([]string, error) { return []string{"000001.SZ"}, nil }

func (m *mockStore) TradingDays(start, end time.Time) ([]time.Time, error) {
	var days []time.Time
	for _, d := range m.days {
		if !d.Before(start) && !d.After(end) {
			days = append(days, d)
		}
	}
	return days, nil
}

func TestEngineRunBuyAndHold(t *testing.T) {
	days := []time.Time{
		time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2020, 1, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2020, 1, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2020, 1, 8, 0, 0, 0, 0, time.UTC),
	}
	closes := []float64{10.0, 10.5, 11.0, 11.5, 12.0}
	store := &mockStore{days: days, closes: closes}
	cfg := engine.Config{
		Symbols: []string{"000001.SZ"}, Start: days[0], End: days[len(days)-1],
		InitCapital: 100000.0, Commission: 0.0003,
	}
	result, err := engine.New(store, cfg).Run(cfg, &buyAndHold{})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalReturn < 0.15 {
		t.Errorf("total return = %.4f, expected > 15%%", result.TotalReturn)
	}
	// Small drawdown expected due to commission on day 1
	if result.MaxDrawdown > 0.01 {
		t.Errorf("monotonic rise: drawdown = %.4f, expected < 1%%", result.MaxDrawdown)
	}
}
