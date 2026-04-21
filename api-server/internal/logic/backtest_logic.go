package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

type BacktestLogic struct {
	ctx context.Context
	svc *svc.ServiceContext
}

func NewBacktestLogic(ctx context.Context, svc *svc.ServiceContext) *BacktestLogic {
	return &BacktestLogic{ctx: ctx, svc: svc}
}

// Run creates a backtest record, executes Python subprocess synchronously, writes result to DB
func (l *BacktestLogic) Run(req *types.RunBacktestReq) (*types.BacktestRunResp, error) {
	strategyName := req.StrategyName
	if req.StrategyID != nil {
		s, err := NewStrategyLogic(l.ctx, l.svc).Get(*req.StrategyID)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, fmt.Errorf("strategy %d not found", *req.StrategyID)
		}
		strategyName = s.ClassName
		if req.Params == nil {
			req.Params = s.Params
		}
	}

	paramsJSON, _ := json.Marshal(req.Params)
	initCapital := req.InitCapital
	if initCapital == 0 {
		initCapital = 1_000_000
	}

	// Insert record with status=running
	var runID int64
	var createdAt time.Time
	err := l.svc.DB.QueryRowContext(l.ctx, `
		INSERT INTO backtest_runs
		(strategy_id, strategy_name, symbols, start_date, end_date, init_capital, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'running')
		RETURNING id, created_at`,
		req.StrategyID, strategyName,
		pq.Array(req.Symbols),
		req.StartDate, req.EndDate, initCapital,
	).Scan(&runID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert backtest_run: %w", err)
	}

	// Execute Python subprocess
	result := l.executePython(strategyName, string(paramsJSON), req.Symbols,
		req.StartDate, req.EndDate, initCapital)

	// Update DB with result
	if result["status"] == "done" {
		annualReturn := floatVal(result, "annual_return")
		maxDrawdown := floatVal(result, "max_drawdown")
		sharpe := floatVal(result, "sharpe_ratio")
		winRate := floatVal(result, "win_rate")
		calmar := floatVal(result, "calmar_ratio")
		totalReturn := floatVal(result, "total_return")

		equityCurveRaw, _ := json.Marshal(result["equity_curve"])
		l.svc.DB.ExecContext(l.ctx, `
			UPDATE backtest_runs SET
				status='done', annual_return=$1, max_drawdown=$2, sharpe_ratio=$3,
				win_rate=$4, calmar_ratio=$5, total_return=$6, equity_curve=$7,
				finished_at=NOW()
			WHERE id=$8`,
			annualReturn, maxDrawdown, sharpe, winRate, calmar, totalReturn,
			equityCurveRaw, runID)
	} else {
		errMsg, _ := result["error"].(string)
		l.svc.DB.ExecContext(l.ctx, `
			UPDATE backtest_runs SET status='failed', error_msg=$1, finished_at=NOW()
			WHERE id=$2`, errMsg, runID)
	}

	return l.Get(runID)
}

func (l *BacktestLogic) executePython(
	strategyName, paramsJSON string,
	symbols []string, startDate, endDate string,
	initCapital float64,
) map[string]interface{} {
	c := l.svc.Config.Python
	pythonExe := c.Executable
	if pythonExe == "" {
		pythonExe = "python3"
	}
	workDir := c.WorkDir
	if workDir == "" {
		workDir = "../python"
	}
	if !filepath.IsAbs(workDir) {
		if abs, err := filepath.Abs(workDir); err == nil {
			workDir = abs
		}
	}

	args := []string{
		"run_backtest.py",
		"--strategy", strategyName,
		"--params", paramsJSON,
		"--symbols", strings.Join(symbols, ","),
		"--start", startDate,
		"--end", endDate,
		"--capital", fmt.Sprintf("%.0f", initCapital),
		"--grpc-host", l.svc.Config.BacktestEngine.Host,
		"--grpc-port", fmt.Sprintf("%d", l.svc.Config.BacktestEngine.Port),
	}
	cmd := exec.CommandContext(l.ctx, pythonExe, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PYTHONPATH="+workDir)

	out, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{"status": "failed", "error": err.Error()}
	}

	var result map[string]interface{}
	if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
		return map[string]interface{}{"status": "failed", "error": string(out)}
	}
	return result
}

func (l *BacktestLogic) Get(id int64) (*types.BacktestRunResp, error) {
	row := l.svc.DB.QueryRowContext(l.ctx, `
		SELECT id, strategy_name, symbols, start_date, end_date, init_capital,
		       status, annual_return, max_drawdown, sharpe_ratio, win_rate,
		       calmar_ratio, total_return, equity_curve, error_msg, created_at, finished_at
		FROM backtest_runs WHERE id=$1`, id)
	return scanBacktestRun(row)
}

func (l *BacktestLogic) List() ([]*types.BacktestRunResp, error) {
	rows, err := l.svc.DB.QueryContext(l.ctx, `
		SELECT id, strategy_name, symbols, start_date, end_date, init_capital,
		       status, annual_return, max_drawdown, sharpe_ratio, win_rate,
		       calmar_ratio, total_return, equity_curve, error_msg, created_at, finished_at
		FROM backtest_runs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*types.BacktestRunResp
	for rows.Next() {
		r, err := scanBacktestRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBacktestRun(s scanner) (*types.BacktestRunResp, error) {
	var r types.BacktestRunResp
	var symbols pq.StringArray
	var startDate, endDate time.Time
	var annualReturn, maxDrawdown, sharpe, winRate, calmar, totalReturn *float64
	var equityCurveRaw []byte
	var finishedAt *time.Time

	err := s.Scan(
		&r.ID, &r.StrategyName, &symbols, &startDate, &endDate, &r.InitCapital,
		&r.Status, &annualReturn, &maxDrawdown, &sharpe, &winRate,
		&calmar, &totalReturn, &equityCurveRaw, &r.ErrorMsg,
		&r.CreatedAt, &finishedAt,
	)
	if err != nil {
		return nil, err
	}
	r.Symbols = []string(symbols)
	r.StartDate = startDate.Format("2006-01-02")
	r.EndDate = endDate.Format("2006-01-02")
	r.AnnualReturn = annualReturn
	r.MaxDrawdown = maxDrawdown
	r.SharpeRatio = sharpe
	r.WinRate = winRate
	r.CalmarRatio = calmar
	r.TotalReturn = totalReturn
	if len(equityCurveRaw) > 0 {
		json.Unmarshal(equityCurveRaw, &r.EquityCurve) //nolint:errcheck
	}
	if finishedAt != nil {
		s := finishedAt.Format(time.RFC3339)
		r.FinishedAt = &s
	}
	return &r, nil
}

func floatVal(m map[string]interface{}, key string) *float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return &f
		}
	}
	return nil
}
