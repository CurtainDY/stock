from datetime import date, datetime, timezone
from unittest.mock import MagicMock
import pytest

from backtest.types import Bar, Order, OrderSide, Fill, FillStatus
from backtest.strategy import Strategy
from backtest.runner import BacktestRunner, BacktestConfig
from backtest.analytics import Metrics


class BuyAndHoldStrategy(Strategy):
    """每次收到第一根 Bar 时全仓买入，之后持有。"""

    def __init__(self):
        self.bought = False

    def on_bar(self, bar: Bar, portfolio) -> list[Order]:
        if self.bought:
            return []
        self.bought = True
        qty = int(portfolio.cash / bar.close / 100) * 100
        return [Order(symbol=bar.symbol, side=OrderSide.BUY, quantity=qty)]


def _make_pb_bar(symbol, date_obj, close=10.0):
    dt = datetime(date_obj.year, date_obj.month, date_obj.day, tzinfo=timezone.utc)
    bar = MagicMock()
    bar.symbol = symbol
    bar.date_unix = int(dt.timestamp())
    bar.open = close * 0.99
    bar.high = close * 1.01
    bar.low = close * 0.98
    bar.close = close
    bar.volume = 100_000
    bar.amount = 100_000 * close
    bar.adj_factor = 0.0
    return bar


def _make_pb_fill(symbol, qty, price, status_value):
    fill = MagicMock()
    fill.symbol = symbol
    fill.side = 0  # BUY
    fill.filled_qty = qty
    fill.filled_price = price
    fill.commission = price * qty * 0.0003
    fill.stamp_duty = 0.0
    fill.status = status_value  # use integer: 1=FILLED, 2=REJECTED
    fill.reject_reason = ""
    return fill


def test_runner_builds_equity_curve():
    """BacktestRunner 应该返回包含净值曲线的 Metrics。"""
    # 5 days, price rises from 10.2 to 11.0
    bars = [_make_pb_bar("sz000001", date(2020, 1, d), 10.0 + d * 0.2) for d in range(1, 6)]

    stub_mock = MagicMock()
    stub_mock.StreamBars.return_value = iter(bars)
    # First order fills, subsequent orders not sent (BuyAndHold only buys once)
    stub_mock.SubmitOrder.return_value = _make_pb_fill("sz000001", 9000, 10.2, 1)  # status=FILLED

    config = BacktestConfig(
        symbols=["sz000001"],
        start_date=date(2020, 1, 1),
        end_date=date(2020, 1, 5),
        init_capital=100_000,
    )
    runner = BacktestRunner(stub=stub_mock, config=config, session_id="test-session")
    metrics = runner.run(BuyAndHoldStrategy())

    assert isinstance(metrics, Metrics)
    assert len(metrics.equity_curve) > 0
    assert metrics.equity_curve[0] == pytest.approx(1.0)


def test_runner_rejected_order_does_not_crash():
    """被拒绝的订单不应使 runner 崩溃。"""
    bars = [_make_pb_bar("sz000001", date(2020, 1, d), 10.0) for d in range(1, 4)]

    reject_fill = _make_pb_fill("sz000001", 0, 0, 2)  # status=REJECTED
    reject_fill.reject_reason = "limit_up"

    stub_mock = MagicMock()
    stub_mock.StreamBars.return_value = iter(bars)
    stub_mock.SubmitOrder.return_value = reject_fill

    config = BacktestConfig(
        symbols=["sz000001"],
        start_date=date(2020, 1, 1),
        end_date=date(2020, 1, 3),
        init_capital=100_000,
    )
    runner = BacktestRunner(stub=stub_mock, config=config, session_id="test-session")
    metrics = runner.run(BuyAndHoldStrategy())
    assert metrics is not None
