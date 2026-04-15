package analytics_test

import (
	"math"
	"testing"

	"github.com/parsedong/stock/backtest-engine/internal/analytics"
)

func TestSharpeHighForStableReturns(t *testing.T) {
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = 0.001
	}
	r := analytics.Calculate(returns, 252)
	if r.SharpeRatio < 10 {
		t.Errorf("sharpe = %v, expected > 10 for stable returns", r.SharpeRatio)
	}
}

func TestMaxDrawdown(t *testing.T) {
	equity := []float64{1.0, 1.1, 1.2, 1.0, 0.9, 1.0}
	returns := make([]float64, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		returns[i-1] = (equity[i] - equity[i-1]) / equity[i-1]
	}
	r := analytics.Calculate(returns, 252)
	want := (1.2 - 0.9) / 1.2
	if math.Abs(r.MaxDrawdown-want) > 0.001 {
		t.Errorf("max drawdown = %v, want %v", r.MaxDrawdown, want)
	}
}

func TestAnnualReturn(t *testing.T) {
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = 0.001
	}
	r := analytics.Calculate(returns, 252)
	expected := math.Pow(1.001, 252) - 1
	if math.Abs(r.AnnualReturn-expected) > 0.001 {
		t.Errorf("annual return = %v, want ~%v", r.AnnualReturn, expected)
	}
}
