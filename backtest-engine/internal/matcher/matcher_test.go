package matcher_test

import (
	"testing"
	"time"

	"github.com/parsedong/stock/backtest-engine/internal/data"
	"github.com/parsedong/stock/backtest-engine/internal/matcher"
)

func makeBar(close float64) data.Bar {
	return data.Bar{Symbol: "000001.SZ", Date: time.Now(),
		Open: close, High: close * 1.01, Low: close * 0.99,
		Close: close, Volume: 1e6, Amount: close * 1e6}
}

func TestBuyNormal(t *testing.T) {
	m := matcher.New(matcher.Config{Commission: 0.0003})
	fill := m.Match(matcher.Order{Symbol: "000001.SZ", Side: matcher.Buy, Quantity: 1000}, makeBar(10.0), 9.5, false)
	if fill.Status != matcher.Filled {
		t.Fatalf("expected filled, got rejected: %s", fill.RejectReason)
	}
	if fill.FilledPrice != 10.0 {
		t.Errorf("price = %v, want 10.0", fill.FilledPrice)
	}
	if wantComm := 10.0 * 1000 * 0.0003; fill.Commission < wantComm-0.0001 || fill.Commission > wantComm+0.0001 {
		t.Errorf("commission = %v, want ~%v", fill.Commission, wantComm)
	}
	if fill.StampDuty != 0 {
		t.Error("buy should have no stamp duty")
	}
}

func TestSellStampDuty(t *testing.T) {
	m := matcher.New(matcher.Config{Commission: 0.0003})
	fill := m.Match(matcher.Order{Symbol: "000001.SZ", Side: matcher.Sell, Quantity: 1000}, makeBar(10.0), 9.5, false)
	if fill.Status != matcher.Filled {
		t.Fatalf("expected filled")
	}
	if fill.StampDuty != 10.0*1000*0.001 {
		t.Errorf("stamp duty = %v, want %v", fill.StampDuty, 10.0*1000*0.001)
	}
}

func TestBuyLimitUpRejected(t *testing.T) {
	m := matcher.New(matcher.Config{Commission: 0.0003})
	fill := m.Match(matcher.Order{Symbol: "000001.SZ", Side: matcher.Buy, Quantity: 1000}, makeBar(11.0), 10.0, false)
	if fill.Status != matcher.Rejected {
		t.Errorf("expected rejected at limit up, got filled")
	}
}

func TestSellLimitDownRejected(t *testing.T) {
	m := matcher.New(matcher.Config{Commission: 0.0003})
	fill := m.Match(matcher.Order{Symbol: "000001.SZ", Side: matcher.Sell, Quantity: 1000}, makeBar(9.0), 10.0, false)
	if fill.Status != matcher.Rejected {
		t.Errorf("expected rejected at limit down, got filled")
	}
}

func TestSTLimitUp(t *testing.T) {
	m := matcher.New(matcher.Config{Commission: 0.0003})
	fill := m.Match(matcher.Order{Symbol: "000001.SZ", Side: matcher.Buy, Quantity: 1000}, makeBar(10.5), 10.0, true)
	if fill.Status != matcher.Rejected {
		t.Errorf("ST: expected rejected at 5%% up, got filled")
	}
}
