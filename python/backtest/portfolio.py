from datetime import date
from backtest.types import Fill, FillStatus, OrderSide, Position, PortfolioSnapshot


class PortfolioTracker:
    """本地持仓追踪器，根据 Fill 更新现金和仓位。"""

    def __init__(self, init_cash: float):
        self._cash = init_cash
        self._positions: dict[str, _PositionDetail] = {}

    @property
    def cash(self) -> float:
        return self._cash

    def position(self, symbol: str) -> Position:
        if symbol not in self._positions:
            return Position(symbol=symbol, quantity=0.0, cost_price=0.0)
        p = self._positions[symbol]
        return Position(symbol=symbol, quantity=p.quantity, cost_price=p.cost_price)

    def apply_fill(self, fill: Fill, trade_date: date) -> None:
        if fill.status != FillStatus.FILLED:
            return

        if fill.side == OrderSide.BUY:
            cost = fill.filled_qty * fill.filled_price + fill.commission + fill.stamp_duty
            self._cash -= cost
            if fill.symbol not in self._positions:
                self._positions[fill.symbol] = _PositionDetail()
            self._positions[fill.symbol].add_lot(fill.filled_qty, fill.filled_price)

        elif fill.side == OrderSide.SELL:
            proceeds = fill.filled_qty * fill.filled_price - fill.commission - fill.stamp_duty
            self._cash += proceeds
            if fill.symbol in self._positions:
                self._positions[fill.symbol].reduce(fill.filled_qty)
                if self._positions[fill.symbol].quantity <= 0:
                    del self._positions[fill.symbol]

    def total_value(self, prices: dict[str, float]) -> float:
        total = self._cash
        for sym, pos in self._positions.items():
            total += pos.quantity * prices.get(sym, 0.0)
        return total

    def snapshot(self, prices: dict[str, float]) -> PortfolioSnapshot:
        positions = {
            sym: Position(symbol=sym, quantity=p.quantity, cost_price=p.cost_price)
            for sym, p in self._positions.items()
        }
        return PortfolioSnapshot(
            cash=self._cash,
            positions=positions,
            total_value=self.total_value(prices),
        )


class _PositionDetail:
    def __init__(self):
        self.quantity = 0.0
        self.cost_price = 0.0

    def add_lot(self, qty: float, price: float) -> None:
        total_cost = self.cost_price * self.quantity + price * qty
        self.quantity += qty
        self.cost_price = total_cost / self.quantity if self.quantity > 0 else 0.0

    def reduce(self, qty: float) -> None:
        self.quantity -= qty
