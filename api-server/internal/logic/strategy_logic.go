package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parsedong/stock/api-server/internal/svc"
	"github.com/parsedong/stock/api-server/internal/types"
)

type StrategyLogic struct {
	ctx context.Context
	svc *svc.ServiceContext
}

func NewStrategyLogic(ctx context.Context, svc *svc.ServiceContext) *StrategyLogic {
	return &StrategyLogic{ctx: ctx, svc: svc}
}

func (l *StrategyLogic) Create(req *types.CreateStrategyReq) (*types.StrategyResp, error) {
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	var id int64
	var createdAt, updatedAt time.Time
	err = l.svc.DB.QueryRowContext(l.ctx, `
		INSERT INTO strategies (name, description, class_name, params)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		req.Name, req.Description, req.ClassName, string(paramsJSON),
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert strategy: %w", err)
	}
	return &types.StrategyResp{
		ID: id, Name: req.Name, Description: req.Description,
		ClassName: req.ClassName, Params: req.Params,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}, nil
}

func (l *StrategyLogic) List() ([]*types.StrategyResp, error) {
	rows, err := l.svc.DB.QueryContext(l.ctx, `
		SELECT id, name, description, class_name, params, created_at, updated_at
		FROM strategies ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query strategies: %w", err)
	}
	defer rows.Close()

	var result []*types.StrategyResp
	for rows.Next() {
		var s types.StrategyResp
		var paramsRaw string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.ClassName,
			&paramsRaw, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(paramsRaw), &s.Params) //nolint:errcheck
		s.CreatedAt = createdAt.Format(time.RFC3339)
		s.UpdatedAt = updatedAt.Format(time.RFC3339)
		result = append(result, &s)
	}
	return result, rows.Err()
}

func (l *StrategyLogic) Get(id int64) (*types.StrategyResp, error) {
	var s types.StrategyResp
	var paramsRaw string
	var createdAt, updatedAt time.Time
	err := l.svc.DB.QueryRowContext(l.ctx, `
		SELECT id, name, description, class_name, params, created_at, updated_at
		FROM strategies WHERE id=$1`, id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.ClassName,
		&paramsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get strategy: %w", err)
	}
	json.Unmarshal([]byte(paramsRaw), &s.Params) //nolint:errcheck
	s.CreatedAt = createdAt.Format(time.RFC3339)
	s.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &s, nil
}

func (l *StrategyLogic) Update(id int64, req *types.UpdateStrategyReq) (*types.StrategyResp, error) {
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	var updatedAt time.Time
	err = l.svc.DB.QueryRowContext(l.ctx, `
		UPDATE strategies SET name=$1, description=$2, class_name=$3, params=$4, updated_at=NOW()
		WHERE id=$5 RETURNING updated_at`,
		req.Name, req.Description, req.ClassName, string(paramsJSON), id,
	).Scan(&updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update strategy: %w", err)
	}
	return l.Get(id)
}

func (l *StrategyLogic) Delete(id int64) error {
	_, err := l.svc.DB.ExecContext(l.ctx, `DELETE FROM strategies WHERE id=$1`, id)
	return err
}
