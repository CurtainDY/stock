package analytics

import "math"

type Result struct {
	AnnualReturn float64
	MaxDrawdown  float64
	SharpeRatio  float64
	WinRate      float64
	CalmarRatio  float64
	TotalReturn  float64
	EquityCurve  []float64
}

func Calculate(dailyReturns []float64, tradingDaysPerYear int) Result {
	if len(dailyReturns) == 0 {
		return Result{}
	}

	equity := make([]float64, len(dailyReturns)+1)
	equity[0] = 1.0
	for i, r := range dailyReturns {
		equity[i+1] = equity[i] * (1 + r)
	}

	totalReturn := equity[len(equity)-1] - 1.0
	years := float64(len(dailyReturns)) / float64(tradingDaysPerYear)
	annualReturn := math.Pow(1+totalReturn, 1/years) - 1

	maxDD := 0.0
	peak := equity[0]
	for _, v := range equity {
		if v > peak {
			peak = v
		}
		if dd := (peak - v) / peak; dd > maxDD {
			maxDD = dd
		}
	}

	mean := 0.0
	for _, r := range dailyReturns {
		mean += r
	}
	mean /= float64(len(dailyReturns))

	variance := 0.0
	for _, r := range dailyReturns {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(dailyReturns))

	sharpe := 0.0
	if stdDev := math.Sqrt(variance); stdDev > 0 {
		sharpe = mean / stdDev * math.Sqrt(float64(tradingDaysPerYear))
	}

	wins := 0
	for _, r := range dailyReturns {
		if r > 0 {
			wins++
		}
	}

	calmar := 0.0
	if maxDD > 0 {
		calmar = annualReturn / maxDD
	}

	return Result{
		AnnualReturn: annualReturn,
		MaxDrawdown:  maxDD,
		SharpeRatio:  sharpe,
		WinRate:      float64(wins) / float64(len(dailyReturns)),
		CalmarRatio:  calmar,
		TotalReturn:  totalReturn,
		EquityCurve:  equity,
	}
}
