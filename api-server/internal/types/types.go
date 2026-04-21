package types

// ---- 策略 ----

type CreateStrategyReq struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ClassName   string                 `json:"class_name"`
	Params      map[string]interface{} `json:"params"`
}

type UpdateStrategyReq struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ClassName   string                 `json:"class_name"`
	Params      map[string]interface{} `json:"params"`
}

type StrategyResp struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ClassName   string                 `json:"class_name"`
	Params      map[string]interface{} `json:"params"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// ---- 回测 ----

type RunBacktestReq struct {
	StrategyID   *int64                 `json:"strategy_id"`
	StrategyName string                 `json:"strategy_name"`
	Params       map[string]interface{} `json:"params"`
	Symbols      []string               `json:"symbols"`
	StartDate    string                 `json:"start_date"`
	EndDate      string                 `json:"end_date"`
	InitCapital  float64                `json:"init_capital"`
}

type BacktestRunResp struct {
	ID           int64     `json:"id"`
	StrategyName string    `json:"strategy_name"`
	Symbols      []string  `json:"symbols"`
	StartDate    string    `json:"start_date"`
	EndDate      string    `json:"end_date"`
	InitCapital  float64   `json:"init_capital"`
	Status       string    `json:"status"`
	AnnualReturn *float64  `json:"annual_return,omitempty"`
	MaxDrawdown  *float64  `json:"max_drawdown,omitempty"`
	SharpeRatio  *float64  `json:"sharpe_ratio,omitempty"`
	WinRate      *float64  `json:"win_rate,omitempty"`
	CalmarRatio  *float64  `json:"calmar_ratio,omitempty"`
	TotalReturn  *float64  `json:"total_return,omitempty"`
	EquityCurve  []float64 `json:"equity_curve,omitempty"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
	CreatedAt    string    `json:"created_at"`
	FinishedAt   *string   `json:"finished_at,omitempty"`
}

// ---- Symbol ----

type SymbolsResp struct {
	Symbols []string `json:"symbols"`
	Total   int      `json:"total"`
}
