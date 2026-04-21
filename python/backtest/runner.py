from dataclasses import dataclass
from datetime import date, datetime, timezone
from typing import Any

from backtest.analytics import Metrics, compute_metrics
from backtest.portfolio import PortfolioTracker
from backtest.proto import backtest_pb2
from backtest.strategy import Strategy
from backtest.types import Bar, Fill, FillStatus, Order, OrderSide


@dataclass
class BacktestConfig:
    symbols: list[str]
    start_date: date
    end_date: date
    init_capital: float = 1_000_000.0
    commission: float = 0.0003


class BacktestRunner:
    """
    驱动单次回测：
    1. 通过 StreamBars 从 Go 服务器接收 Bar
    2. 调用策略的 on_bar
    3. 通过 SubmitOrder 提交订单
    4. 本地追踪持仓和净值
    5. 返回 Metrics
    """

    def __init__(self, stub: Any, config: BacktestConfig, session_id: str):
        self._stub = stub
        self._config = config
        self._session_id = session_id

    def run(self, strategy: Strategy) -> Metrics:
        portfolio = PortfolioTracker(self._config.init_capital)
        equity_curve: list[float] = [1.0]

        strategy.on_start()

        stream_req = backtest_pb2.StreamRequest(
            symbols=self._config.symbols,
            start_date=self._config.start_date.isoformat(),
            end_date=self._config.end_date.isoformat(),
        )
        metadata = [("x-session-id", self._session_id)]

        current_day: date | None = None
        day_prices: dict[str, float] = {}

        for pb_bar in self._stub.StreamBars(stream_req, metadata=metadata):
            bar = _pb_to_bar(pb_bar)

            # 新的一天：记录净值
            if current_day is not None and bar.date != current_day:
                nav = portfolio.total_value(day_prices)
                equity_curve.append(nav / self._config.init_capital)
                day_prices = {}

            current_day = bar.date
            day_prices[bar.symbol] = bar.close

            snapshot = portfolio.snapshot(day_prices)
            orders = strategy.on_bar(bar, snapshot)

            for order in orders:
                fill = self._submit_order(order, bar.date)
                portfolio.apply_fill(fill, bar.date)

        # 记录最后一天净值
        if current_day is not None and day_prices:
            nav = portfolio.total_value(day_prices)
            equity_curve.append(nav / self._config.init_capital)

        strategy.on_end()

        return compute_metrics(equity_curve)

    def _submit_order(self, order: Order, trade_date: date) -> Fill:
        dt = datetime(trade_date.year, trade_date.month, trade_date.day, tzinfo=timezone.utc)
        metadata = [
            ("x-session-id", self._session_id),
            ("x-date-unix", str(int(dt.timestamp()))),
        ]
        pb_order = backtest_pb2.Order(
            symbol=order.symbol,
            side=backtest_pb2.OrderSide.Value(order.side.name),
            quantity=order.quantity,
            price=order.price,
        )
        pb_fill = self._stub.SubmitOrder(pb_order, metadata=metadata)
        return _pb_to_fill(pb_fill)


def _pb_to_bar(pb: Any) -> Bar:
    dt = datetime.fromtimestamp(pb.date_unix, tz=timezone.utc).date()
    return Bar(
        symbol=pb.symbol,
        date=dt,
        open=pb.open,
        high=pb.high,
        low=pb.low,
        close=pb.close,
        volume=pb.volume,
        amount=pb.amount,
        adj_factor=pb.adj_factor,
    )


def _pb_to_fill(pb: Any) -> Fill:
    status = FillStatus.FILLED if pb.status == backtest_pb2.FILLED else FillStatus.REJECTED
    return Fill(
        symbol=pb.symbol,
        side=OrderSide(pb.side),
        filled_qty=pb.filled_qty,
        filled_price=pb.filled_price,
        commission=pb.commission,
        stamp_duty=pb.stamp_duty,
        status=status,
        reject_reason=pb.reject_reason,
    )
