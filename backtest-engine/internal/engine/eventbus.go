package engine

import "github.com/parsedong/stock/backtest-engine/internal/data"

type BarEvent struct {
	Bar data.Bar
}

type OrderEvent struct {
	Symbol   string
	Side     int
	Quantity float64
	Price    float64
}

type FillEvent struct {
	Symbol      string
	Side        int
	FilledQty   float64
	FilledPrice float64
	Commission  float64
	StampDuty   float64
	Rejected    bool
	Reason      string
}
