from datetime import date
from unittest.mock import MagicMock
import pytest

from backtest.walk_forward import WalkForwardValidator, WalkForwardConfig, WFWindow
from backtest.analytics import Metrics


def _make_metrics(annual_return=0.15, max_drawdown=0.10, sharpe=1.2, win_rate=0.55):
    return Metrics(
        total_return=annual_return,
        annual_return=annual_return,
        max_drawdown=max_drawdown,
        sharpe_ratio=sharpe,
        win_rate=win_rate,
        calmar_ratio=annual_return / max_drawdown if max_drawdown else 0,
        equity_curve=[1.0],
    )


def test_expanding_window_split():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        mode="expanding",
    )
    wf = WalkForwardValidator(config)
    windows = wf.windows()

    assert len(windows) == 3
    # Expanding: all windows start at config.start_date
    assert all(w.train_start == date(2015, 1, 1) for w in windows)
    # Each test window starts where previous test ended
    for i in range(1, len(windows)):
        assert windows[i].test_start == windows[i - 1].test_end


def test_rolling_window_split():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        mode="rolling",
    )
    wf = WalkForwardValidator(config)
    windows = wf.windows()

    assert len(windows) == 3
    # Rolling: each window's train_start advances by test_years
    for i in range(1, len(windows)):
        assert windows[i].train_start > windows[i - 1].train_start


def test_passes_filter_with_good_metrics():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        min_annual_return=0.10,
        min_sharpe=0.8,
        max_drawdown=0.30,
        min_win_rate=0.45,
    )
    wf = WalkForwardValidator(config)
    good = _make_metrics(annual_return=0.20, max_drawdown=0.10, sharpe=1.5, win_rate=0.55)
    assert wf.passes_filter(good) is True


def test_fails_filter_with_bad_drawdown():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2020, 1, 1),
        train_years=2,
        test_years=1,
        max_drawdown=0.30,
    )
    wf = WalkForwardValidator(config)
    bad = _make_metrics(max_drawdown=0.50)
    assert wf.passes_filter(bad) is False


def test_run_aggregates_test_metrics():
    config = WalkForwardConfig(
        start_date=date(2015, 1, 1),
        end_date=date(2018, 1, 1),
        train_years=1,
        test_years=1,
        mode="expanding",
    )
    wf = WalkForwardValidator(config)

    mock_runner = MagicMock()
    mock_runner.run.return_value = _make_metrics()
    mock_runner_factory = MagicMock(return_value=mock_runner)

    from backtest.strategy import Strategy

    class DummyStrategy(Strategy):
        def on_bar(self, bar, portfolio):
            return []

    result = wf.run(DummyStrategy, mock_runner_factory)
    assert result.num_windows == 2
    assert len(result.test_metrics) == 2
    assert result.avg_annual_return > 0
