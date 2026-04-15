package portfolio

import (
	"fmt"
	"time"

	"github.com/parsedong/stock/backtest-engine/internal/matcher"
)

type lot struct {
	qty     float64
	buyDate time.Time
}

type Position struct {
	Symbol    string
	Quantity  float64
	CostPrice float64
	lots      []lot
}

func (p *Position) AvailableQty(today time.Time) float64 {
	var avail float64
	for _, l := range p.lots {
		if today.After(l.buyDate) {
			avail += l.qty
		}
	}
	return avail
}

type Portfolio struct {
	cash      float64
	positions map[string]*Position
}

func New(initCapital float64) *Portfolio {
	return &Portfolio{cash: initCapital, positions: make(map[string]*Position)}
}

func (p *Portfolio) Cash() float64 { return p.cash }

func (p *Portfolio) Position(symbol string) *Position {
	if pos, ok := p.positions[symbol]; ok {
		return pos
	}
	return &Position{Symbol: symbol}
}

func (p *Portfolio) TotalValue(prices map[string]float64) float64 {
	total := p.cash
	for sym, pos := range p.positions {
		if price, ok := prices[sym]; ok {
			total += pos.Quantity * price
		}
	}
	return total
}

func (p *Portfolio) ApplyFill(fill matcher.Fill, date time.Time) error {
	if fill.Status != matcher.Filled {
		return nil
	}
	cost := fill.FilledQty*fill.FilledPrice + fill.Commission + fill.StampDuty

	switch fill.Side {
	case matcher.Buy:
		if cost > p.cash {
			return fmt.Errorf("insufficient cash: need %.2f, have %.2f", cost, p.cash)
		}
		p.cash -= cost
		pos := p.positions[fill.Symbol]
		if pos == nil {
			pos = &Position{Symbol: fill.Symbol}
			p.positions[fill.Symbol] = pos
		}
		totalCost := pos.CostPrice*pos.Quantity + fill.FilledPrice*fill.FilledQty
		pos.Quantity += fill.FilledQty
		if pos.Quantity > 0 {
			pos.CostPrice = totalCost / pos.Quantity
		}
		pos.lots = append(pos.lots, lot{qty: fill.FilledQty, buyDate: date})

	case matcher.Sell:
		pos := p.positions[fill.Symbol]
		if pos == nil || pos.Quantity < fill.FilledQty {
			return fmt.Errorf("insufficient position for %s", fill.Symbol)
		}
		p.cash += fill.FilledQty*fill.FilledPrice - fill.Commission - fill.StampDuty
		pos.Quantity -= fill.FilledQty
		remaining := fill.FilledQty
		for i := range pos.lots {
			if remaining <= 0 {
				break
			}
			if pos.lots[i].qty <= remaining {
				remaining -= pos.lots[i].qty
				pos.lots[i].qty = 0
			} else {
				pos.lots[i].qty -= remaining
				remaining = 0
			}
		}
		if pos.Quantity == 0 {
			delete(p.positions, fill.Symbol)
		}
	}
	return nil
}
