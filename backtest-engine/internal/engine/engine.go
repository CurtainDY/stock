package engine

import (
	"time"

	"github.com/parsedong/stock/backtest-engine/internal/analytics"
	"github.com/parsedong/stock/backtest-engine/internal/data"
	"github.com/parsedong/stock/backtest-engine/internal/matcher"
	"github.com/parsedong/stock/backtest-engine/internal/portfolio"
)

type Strategy interface {
	OnBar(bar data.Bar, port *portfolio.Portfolio) []OrderEvent
}

type Config struct {
	Symbols     []string
	Start       time.Time
	End         time.Time
	InitCapital float64
	Commission  float64
}

type Engine struct {
	store     data.DataStore
	matcher   *matcher.Matcher
	portfolio *portfolio.Portfolio
}

func New(store data.DataStore, cfg Config) *Engine {
	commission := cfg.Commission
	if commission == 0 {
		commission = 0.0003
	}
	return &Engine{
		store:     store,
		matcher:   matcher.New(matcher.Config{Commission: commission}),
		portfolio: portfolio.New(cfg.InitCapital),
	}
}

func (e *Engine) Run(cfg Config, strategy Strategy) (analytics.Result, error) {
	days, err := e.store.TradingDays(cfg.Start, cfg.End)
	if err != nil {
		return analytics.Result{}, err
	}

	prevPrices := make(map[string]float64)
	var dailyReturns []float64
	prevValue := cfg.InitCapital

	for _, day := range days {
		bars, err := e.store.LoadBarsByDate(day, cfg.Symbols)
		if err != nil {
			return analytics.Result{}, err
		}

		for sym, bar := range bars {
			orders := strategy.OnBar(bar, e.portfolio)
			for _, oe := range orders {
				order := matcher.Order{
					Symbol:   sym,
					Side:     matcher.Side(oe.Side),
					Quantity: oe.Quantity,
					Price:    oe.Price,
				}
				fill := e.matcher.Match(order, bar, prevPrices[sym], false)
				e.portfolio.ApplyFill(fill, day) //nolint:errcheck
			}
			prevPrices[sym] = bar.Close
		}

		prices := make(map[string]float64)
		for sym, bar := range bars {
			prices[sym] = bar.Close
		}
		curValue := e.portfolio.TotalValue(prices)
		if prevValue > 0 {
			dailyReturns = append(dailyReturns, (curValue-prevValue)/prevValue)
		}
		prevValue = curValue
	}

	return analytics.Calculate(dailyReturns, 242), nil
}
