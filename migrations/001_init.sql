-- 股票元数据
CREATE TABLE IF NOT EXISTS stocks (
    symbol      VARCHAR(12) PRIMARY KEY,  -- 如 sz000001, sh600000, bj920000
    name        VARCHAR(64),
    exchange    VARCHAR(4),               -- sz / sh / bj
    listed_date DATE,
    delisted_date DATE,                   -- NULL 表示仍在市
    is_st       BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 数据导入批次记录
CREATE TABLE IF NOT EXISTS import_batches (
    id          SERIAL PRIMARY KEY,
    source_file VARCHAR(512) NOT NULL,    -- 源zip文件路径
    frequency   VARCHAR(8) NOT NULL,      -- 1m / 5m / 15m / 30m / 60m
    started_at  TIMESTAMPTZ DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    status      VARCHAR(16) DEFAULT 'running', -- running / done / failed
    total_files INT DEFAULT 0,
    imported    INT DEFAULT 0,
    skipped     INT DEFAULT 0,
    errors      INT DEFAULT 0
);

-- 每只股票每次导入的完整性报告
CREATE TABLE IF NOT EXISTS import_checks (
    id              SERIAL PRIMARY KEY,
    batch_id        INT REFERENCES import_batches(id),
    symbol          VARCHAR(12) NOT NULL,
    frequency       VARCHAR(8)  NOT NULL,
    expected_bars   INT,          -- 按交易日历估算的预期Bar数
    actual_bars     INT,          -- 实际导入Bar数
    missing_days    INT,          -- 缺失的交易日数
    zero_volume_bars INT,         -- 成交量为0的Bar数（可能有问题）
    first_date      TIMESTAMPTZ,
    last_date       TIMESTAMPTZ,
    status          VARCHAR(16),  -- ok / incomplete / empty / error
    detail          TEXT,         -- 问题描述
    checked_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON import_checks(symbol);
CREATE INDEX ON import_checks(status);
CREATE INDEX ON import_checks(batch_id);

-- 汇总视图：快速查看哪些股票数据不完整
CREATE VIEW data_quality_summary AS
SELECT
    frequency,
    status,
    COUNT(*) as count,
    AVG(actual_bars::float / NULLIF(expected_bars, 0) * 100) AS avg_completeness_pct
FROM import_checks
GROUP BY frequency, status
ORDER BY frequency, status;
