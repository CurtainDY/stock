import math
from backtest.analytics import compute_metrics


def test_flat_equity_returns_zero():
    equity = [1.0] * 10
    m = compute_metrics(equity)
    assert m.total_return == 0.0
    assert m.max_drawdown == 0.0


def test_positive_return():
    equity = [1.0 + i * 0.1 for i in range(11)]  # 1.0 → 2.0
    m = compute_metrics(equity)
    assert abs(m.total_return - 1.0) < 0.01


def test_max_drawdown():
    equity = [1.0, 1.5, 2.0, 1.5, 1.0]
    m = compute_metrics(equity)
    assert abs(m.max_drawdown - 0.5) < 0.01


def test_sharpe_positive_trend():
    equity = [1.0 + i * 0.001 for i in range(242)]
    m = compute_metrics(equity)
    assert m.sharpe_ratio > 0


def test_win_rate():
    equity = [1.0, 1.1, 1.2, 1.3, 1.4, 1.3, 1.2, 1.1, 1.0, 0.9]
    m = compute_metrics(equity)
    assert abs(m.win_rate - 4 / 9) < 0.01


def test_calmar_ratio():
    equity = [1.0, 1.5, 2.0, 1.5, 1.0]
    m = compute_metrics(equity)
    if m.max_drawdown > 0:
        assert abs(m.calmar_ratio - m.annual_return / m.max_drawdown) < 0.001
