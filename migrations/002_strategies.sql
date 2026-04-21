-- 策略定义表
CREATE TABLE IF NOT EXISTS strategies (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    class_name  VARCHAR(100) NOT NULL,   -- Python 类名，如 MACrossStrategy
    params      JSONB DEFAULT '{}',      -- 策略参数，如 {"fast":5,"slow":20}
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 回测运行记录表
CREATE TABLE IF NOT EXISTS backtest_runs (
    id              SERIAL PRIMARY KEY,
    strategy_id     INT REFERENCES strategies(id) ON DELETE SET NULL,
    strategy_name   VARCHAR(100) NOT NULL,
    symbols         TEXT[]  NOT NULL,
    start_date      DATE    NOT NULL,
    end_date        DATE    NOT NULL,
    init_capital    DOUBLE PRECISION DEFAULT 1000000,
    status          VARCHAR(16) DEFAULT 'pending',  -- pending/running/done/failed
    annual_return   DOUBLE PRECISION,
    max_drawdown    DOUBLE PRECISION,
    sharpe_ratio    DOUBLE PRECISION,
    win_rate        DOUBLE PRECISION,
    calmar_ratio    DOUBLE PRECISION,
    total_return    DOUBLE PRECISION,
    equity_curve    DOUBLE PRECISION[],
    error_msg       TEXT DEFAULT '',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);
CREATE INDEX ON backtest_runs(strategy_id);
CREATE INDEX ON backtest_runs(status);
CREATE INDEX ON backtest_runs(created_at DESC);
