import pytest
from datetime import date
from backtest.types import Fill, FillStatus, OrderSide
from backtest.portfolio import PortfolioTracker


def make_fill(symbol, side, qty, price, commission=0.0, stamp_duty=0.0):
    return Fill(
        symbol=symbol,
        side=side,
        filled_qty=qty,
        filled_price=price,
        commission=commission,
        stamp_duty=stamp_duty,
        status=FillStatus.FILLED,
    )


def test_buy_increases_position():
    p = PortfolioTracker(init_cash=100_000)
    fill = make_fill("sz000001", OrderSide.BUY, 1000, 10.0, commission=3.0)
    p.apply_fill(fill, date(2020, 1, 2))
    assert p.position("sz000001").quantity == 1000
    assert abs(p.cash - (100_000 - 10_000 - 3.0)) < 0.01


def test_sell_decreases_position():
    p = PortfolioTracker(init_cash=100_000)
    p.apply_fill(make_fill("sz000001", OrderSide.BUY, 1000, 10.0), date(2020, 1, 2))
    p.apply_fill(make_fill("sz000001", OrderSide.SELL, 500, 11.0, stamp_duty=5.5), date(2020, 1, 3))
    assert p.position("sz000001").quantity == 500
    assert abs(p.cash - (100_000 - 10_000 + 5_500 - 5.5)) < 0.01


def test_total_value():
    p = PortfolioTracker(init_cash=100_000)
    p.apply_fill(make_fill("sz000001", OrderSide.BUY, 1000, 10.0), date(2020, 1, 2))
    val = p.total_value({"sz000001": 12.0})
    assert abs(val - (100_000 - 10_000 + 1000 * 12.0)) < 0.01


def test_snapshot():
    p = PortfolioTracker(init_cash=100_000)
    p.apply_fill(make_fill("sz000001", OrderSide.BUY, 1000, 10.0), date(2020, 1, 2))
    snap = p.snapshot({"sz000001": 11.0})
    assert snap.cash == p.cash
    assert len(snap.positions) == 1
    assert snap.positions["sz000001"].quantity == 1000
    assert abs(snap.total_value - (p.cash + 11_000)) < 0.01
