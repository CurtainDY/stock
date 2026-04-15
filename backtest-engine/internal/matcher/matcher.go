package matcher

import "github.com/parsedong/stock/backtest-engine/internal/data"

type Side int

const (
	Buy  Side = 0
	Sell Side = 1
)

type Status int

const (
	Filled   Status = 0
	Rejected Status = 1
)

type Order struct {
	Symbol   string
	Side     Side
	Quantity float64
	Price    float64
}

type Fill struct {
	Symbol       string
	Side         Side
	FilledQty    float64
	FilledPrice  float64
	Commission   float64
	StampDuty    float64
	Status       Status
	RejectReason string
}

type Config struct {
	Commission float64
	StampDuty  float64
}

type Matcher struct {
	cfg Config
}

func New(cfg Config) *Matcher {
	if cfg.StampDuty == 0 {
		cfg.StampDuty = 0.001
	}
	return &Matcher{cfg: cfg}
}

func (m *Matcher) Match(order Order, bar data.Bar, prevClose float64, isST bool) Fill {
	if order.Side == Buy && bar.IsLimitUp(prevClose, isST) {
		return Fill{Symbol: order.Symbol, Side: order.Side, Status: Rejected,
			RejectReason: "limit_up: cannot buy at limit up price"}
	}
	if order.Side == Sell && bar.IsLimitDown(prevClose, isST) {
		return Fill{Symbol: order.Symbol, Side: order.Side, Status: Rejected,
			RejectReason: "limit_down: cannot sell at limit down price"}
	}

	price := order.Price
	if price == 0 {
		price = bar.Close
	}

	commission := price * order.Quantity * m.cfg.Commission
	stampDuty := 0.0
	if order.Side == Sell {
		stampDuty = price * order.Quantity * m.cfg.StampDuty
	}

	return Fill{
		Symbol: order.Symbol, Side: order.Side,
		FilledQty: order.Quantity, FilledPrice: price,
		Commission: commission, StampDuty: stampDuty, Status: Filled,
	}
}
