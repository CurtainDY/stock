package portfolio_test

import (
	"testing"
	"time"

	"github.com/parsedong/stock/backtest-engine/internal/matcher"
	"github.com/parsedong/stock/backtest-engine/internal/portfolio"
)

var buyDate = time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)

func TestBuyAndHold(t *testing.T) {
	p := portfolio.New(100000.0)
	fill := matcher.Fill{Symbol: "000001.SZ", Side: matcher.Buy, FilledQty: 1000, FilledPrice: 10.0, Commission: 3.0, Status: matcher.Filled}
	if err := p.ApplyFill(fill, buyDate); err != nil {
		t.Fatal(err)
	}
	if p.Cash() != 100000.0-10003.0 {
		t.Errorf("cash = %v, want %v", p.Cash(), 100000.0-10003.0)
	}
	pos := p.Position("000001.SZ")
	if pos.Quantity != 1000 {
		t.Errorf("qty = %v, want 1000", pos.Quantity)
	}
	if pos.AvailableQty(buyDate) != 0 {
		t.Error("T+1: same day should not be available")
	}
	if pos.AvailableQty(buyDate.AddDate(0, 0, 1)) != 1000 {
		t.Error("T+1: next day should be fully available")
	}
}

func TestSellReducesPosition(t *testing.T) {
	p := portfolio.New(100000.0)
	p.ApplyFill(matcher.Fill{Symbol: "000001.SZ", Side: matcher.Buy, FilledQty: 1000, FilledPrice: 10.0, Commission: 3.0, Status: matcher.Filled}, buyDate)
	sellDate := buyDate.AddDate(0, 0, 1)
	p.ApplyFill(matcher.Fill{Symbol: "000001.SZ", Side: matcher.Sell, FilledQty: 500, FilledPrice: 11.0, Commission: 1.65, StampDuty: 5.5, Status: matcher.Filled}, sellDate)

	if p.Position("000001.SZ").Quantity != 500 {
		t.Errorf("qty = %v, want 500", p.Position("000001.SZ").Quantity)
	}
	wantCash := 100000.0 - 10003.0 + 5500.0 - 1.65 - 5.5
	if p.Cash() != wantCash {
		t.Errorf("cash = %v, want %v", p.Cash(), wantCash)
	}
}
