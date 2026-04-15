package data

import "time"

type Bar struct {
	Symbol    string
	Date      time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Amount    float64
	AdjFactor float64
}

func (b Bar) AdjClose() float64 {
	if b.AdjFactor == 0 {
		return b.Close
	}
	return b.Close * b.AdjFactor
}

func (b Bar) IsLimitUp(prevClose float64, isST bool) bool {
	if prevClose == 0 {
		return false
	}
	limit := 0.099
	if isST {
		limit = 0.049
	}
	return (b.Close-prevClose)/prevClose >= limit
}

func (b Bar) IsLimitDown(prevClose float64, isST bool) bool {
	if prevClose == 0 {
		return false
	}
	limit := 0.099
	if isST {
		limit = 0.049
	}
	return (prevClose-b.Close)/prevClose >= limit
}
